// Command maintenance holds operator-only account operations that must never
// be reachable over public HTTP (IMPLEMENTATION.md §5.1, §6, §17.3):
//
//	maintenance owner-bootstrap --email owner@example.com [--display-name N] [--password-stdin]
//	maintenance set-role --email user@example.com --role author|moderator|reader
//	maintenance reset-password --email user@example.com [--password-stdin]
//
// The application owner is an ordinary Collar app user with role=owner, never
// a Collar superuser. Restricted operator superusers are created separately
// with `collar superuser create` (cmd/collar).
package main

import (
	"bufio"
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"golang.org/x/term"

	"github.com/namankumar80510/panw.ink/internal/auth"
	"github.com/namankumar80510/panw.ink/internal/config"
	"github.com/namankumar80510/panw.ink/internal/db"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "maintenance:", err)
		os.Exit(1)
	}
}

func usage() error {
	return errors.New("usage: maintenance owner-bootstrap|set-role|reset-password --email <address> [flags]")
}

func run(args []string) error {
	if len(args) == 0 {
		return usage()
	}
	cmd, args := args[0], args[1:]
	fset := flag.NewFlagSet(cmd, flag.ContinueOnError)
	email := fset.String("email", "", "account email")
	display := fset.String("display-name", "Owner", "display name for a newly created owner")
	role := fset.String("role", "", "new role: reader, author or moderator")
	stdin := fset.Bool("password-stdin", false, "read the password from standard input instead of prompting")
	if err := fset.Parse(args); err != nil {
		return err
	}

	if err := config.LoadDotEnv(".env"); err != nil {
		return err
	}
	config.ExportCollarDefaults()
	addr := auth.NormalizeEmail(*email)
	if addr == "" && cmd == "owner-bootstrap" {
		addr = auth.NormalizeEmail(os.Getenv("OWNER_BOOTSTRAP_EMAIL"))
	}
	if addr == "" {
		return errors.New("--email must be a valid address")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	database, err := openRuntime(os.Getenv("COLLAR_DSN"))
	if err != nil {
		return err
	}
	defer database.Close()
	t := db.NewTables(os.Getenv("APP_DATABASE"))

	switch cmd {
	case "owner-bootstrap":
		return ownerBootstrap(ctx, database, t, addr, strings.TrimSpace(*display), *stdin)
	case "set-role":
		return setRole(ctx, database, t, addr, *role)
	case "reset-password":
		return resetPassword(ctx, database, t, addr, *stdin)
	}
	return usage()
}

func openRuntime(dsn string) (*sql.DB, error) {
	if dsn == "" {
		return nil, errors.New("COLLAR_DSN is required")
	}
	c, err := mysql.ParseDSN(dsn)
	if err != nil {
		return nil, fmt.Errorf("COLLAR_DSN: %w", err)
	}
	c.ParseTime, c.Loc = true, time.UTC
	if c.Params == nil {
		c.Params = map[string]string{}
	}
	c.Params["time_zone"] = "'+00:00'"
	return sql.Open("mysql", c.FormatDSN())
}

func readPassword(fromStdin bool, email string) (string, error) {
	var pw string
	if fromStdin || !term.IsTerminal(int(os.Stdin.Fd())) {
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && line == "" {
			return "", fmt.Errorf("read password: %w", err)
		}
		pw = strings.TrimRight(line, "\r\n")
	} else {
		fmt.Fprint(os.Stderr, "New password: ")
		a, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return "", err
		}
		fmt.Fprint(os.Stderr, "Repeat password: ")
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return "", err
		}
		if string(a) != string(b) {
			return "", errors.New("passwords do not match")
		}
		pw = string(a)
	}
	if msg := auth.CheckPassword(pw, email); msg != "" {
		return "", errors.New(msg)
	}
	return pw, nil
}

// ownerBootstrap creates the first owner, or promotes an existing account
// when no owner exists yet. It refuses once an owner exists.
func ownerBootstrap(ctx context.Context, database *sql.DB, t db.Tables, email, display string, stdin bool) error {
	var owners int
	if err := database.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+t.T("users")+" WHERE role = 'owner' AND state <> 'deleted'").Scan(&owners); err != nil {
		return err
	}
	if owners > 0 {
		return errors.New("an owner already exists; owner bootstrap is one-time (use set-role for other staff)")
	}
	var id uint64
	err := database.QueryRowContext(ctx, "SELECT id FROM "+t.T("users")+" WHERE email = ?", email).Scan(&id)
	switch {
	case err == nil:
		return db.InTx(ctx, database, func(tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, "UPDATE "+t.T("users")+" SET role = 'owner', email_verified_at = COALESCE(email_verified_at, UTC_TIMESTAMP(6)) WHERE id = ? AND state = 'active'", id); err != nil {
				return err
			}
			fmt.Printf("promoted existing account %s (id %d) to owner\n", email, id)
			return db.Audit(ctx, tx, t, nil, "owner.bootstrap.promote", "user", &id, nil, []byte(`{"role":"owner"}`), "cli")
		})
	case !errors.Is(err, sql.ErrNoRows):
		return err
	}
	if display == "" {
		display = "Owner"
	}
	pw, err := readPassword(stdin, email)
	if err != nil {
		return err
	}
	hash, err := auth.HashPassword(pw)
	if err != nil {
		return err
	}
	return db.InTx(ctx, database, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, "INSERT INTO "+t.T("users")+
			" (email, display_name, password_hash, role, state, email_verified_at) VALUES (?,?,?,'owner','active',UTC_TIMESTAMP(6))", email, display, hash)
		if err != nil {
			return err
		}
		n, _ := res.LastInsertId()
		id = uint64(n)
		fmt.Printf("created owner account %s (id %d)\n", email, id)
		return db.Audit(ctx, tx, t, nil, "owner.bootstrap.create", "user", &id, nil, []byte(`{"role":"owner"}`), "cli")
	})
}

func setRole(ctx context.Context, database *sql.DB, t db.Tables, email, role string) error {
	switch role {
	case auth.RoleReader, auth.RoleAuthor, auth.RoleModerator:
	default:
		return errors.New("--role must be reader, author or moderator (owner is bootstrap-only)")
	}
	return db.InTx(ctx, database, func(tx *sql.Tx) error {
		var id uint64
		var before string
		err := tx.QueryRowContext(ctx, "SELECT id, role FROM "+t.T("users")+" WHERE email = ? AND state = 'active' FOR UPDATE", email).Scan(&id, &before)
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("no active account with that email")
		}
		if err != nil {
			return err
		}
		if before == auth.RoleOwner {
			return errors.New("refusing to demote the owner from the CLI")
		}
		if _, err := tx.ExecContext(ctx, "UPDATE "+t.T("users")+" SET role = ? WHERE id = ?", role, id); err != nil {
			return err
		}
		// Role changes take effect on the next request: the bridge reads the
		// live role, and Collar's identity cache expires within seconds.
		fmt.Printf("%s: role %s -> %s\n", email, before, role)
		return db.Audit(ctx, tx, t, nil, "user.role.set", "user", &id,
			[]byte(`{"role":"`+before+`"}`), []byte(`{"role":"`+role+`"}`), "cli")
	})
}

func resetPassword(ctx context.Context, database *sql.DB, t db.Tables, email string, stdin bool) error {
	var id uint64
	if err := database.QueryRowContext(ctx, "SELECT id FROM "+t.T("users")+" WHERE email = ? AND state = 'active'", email).Scan(&id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("no active account with that email")
		}
		return err
	}
	pw, err := readPassword(stdin, email)
	if err != nil {
		return err
	}
	hash, err := auth.HashPassword(pw)
	if err != nil {
		return err
	}
	return db.InTx(ctx, database, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "UPDATE "+t.T("users")+" SET password_hash = ? WHERE id = ?", hash, id); err != nil {
			return err
		}
		// Revoke every browser session at once. The Collar sessions behind
		// them become unreachable (their tokens live only in those rows)
		// and expire on their own.
		res, err := tx.ExecContext(ctx, "UPDATE "+t.T("browser_sessions")+" SET revoked_at = UTC_TIMESTAMP(6) WHERE user_id = ? AND revoked_at IS NULL", id)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		fmt.Printf("password reset for %s; %d sessions revoked\n", email, n)
		return db.Audit(ctx, tx, t, nil, "user.password_reset.cli", "user", &id, nil, nil, "cli")
	})
}
