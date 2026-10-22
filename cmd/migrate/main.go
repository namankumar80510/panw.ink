// Command migrate applies Pawn.Ink's Goose migrations to the application
// database (APP_DATABASE) using the migration account (COLLAR_ADMIN_DSN).
// Collar's system database is migrated separately with `collar migrate`.
//
// It never runs down migrations. Pending migrations whose Up section contains
// destructive DDL are refused unless --allow-destructive and --backup-ref
// name a verified backup (IMPLEMENTATION.md §5.6, §13.4).
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/pressly/goose/v3"

	"github.com/namankumar80510/panw.ink/internal/config"
	"github.com/namankumar80510/panw.ink/migrations"
)

var destructive = regexp.MustCompile(`(?i)\b(DROP\s+(TABLE|DATABASE|SCHEMA|COLUMN|VIEW)|TRUNCATE\b|ALTER\s+TABLE\s+\S+\s+DROP\b|DELETE\s+FROM\b|RENAME\s+(TABLE|COLUMN)\b)`)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "migrate:", err)
		os.Exit(1)
	}
}

func run() error {
	fset := flag.NewFlagSet("migrate", flag.ContinueOnError)
	dryRun := fset.Bool("dry-run", false, "list pending migrations and destructive statements without applying")
	allow := fset.Bool("allow-destructive", false, "permit pending migrations with destructive DDL (requires --backup-ref)")
	backupRef := fset.String("backup-ref", "", "identifier of the verified backup taken before a destructive migration")
	if err := fset.Parse(os.Args[1:]); err != nil {
		return err
	}
	cmd := fset.Arg(0)
	if cmd == "" {
		cmd = "up"
	}
	if cmd != "up" && cmd != "status" {
		return fmt.Errorf("unknown command %q (want up or status)", cmd)
	}

	if err := config.LoadDotEnv(".env"); err != nil {
		return err
	}
	config.ExportCollarDefaults()
	dsn := os.Getenv("COLLAR_ADMIN_DSN")
	if dsn == "" {
		return fmt.Errorf("COLLAR_ADMIN_DSN (the migration account) is required")
	}
	dbName := os.Getenv("APP_DATABASE")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	database, err := open(ctx, dsn, dbName)
	if err != nil {
		return err
	}
	defer database.Close()

	p, err := goose.NewProvider(goose.DialectMySQL, database, migrations.FS)
	if err != nil {
		return err
	}
	status, err := p.Status(ctx)
	if err != nil {
		return err
	}
	var pending []*goose.MigrationStatus
	for _, s := range status {
		if s.State == goose.StatePending {
			pending = append(pending, s)
		}
	}
	fmt.Printf("database %s: %d migrations, %d pending\n", dbName, len(status), len(pending))
	var flagged []string
	for _, s := range pending {
		up, err := upSection(s.Source.Path)
		if err != nil {
			return err
		}
		mark := ""
		if m := destructive.FindAllString(up, -1); len(m) > 0 {
			mark = "  [DESTRUCTIVE: " + strings.Join(m, ", ") + "]"
			flagged = append(flagged, s.Source.Path)
		}
		fmt.Printf("  pending %05d %s%s\n", s.Source.Version, s.Source.Path, mark)
	}
	if cmd == "status" || *dryRun {
		return nil
	}
	if len(flagged) > 0 && (!*allow || strings.TrimSpace(*backupRef) == "") {
		return fmt.Errorf("refusing destructive migrations %v: take and verify a backup, then rerun with --allow-destructive --backup-ref=<id>", flagged)
	}
	if len(flagged) > 0 {
		fmt.Printf("destructive migrations approved against backup %q\n", *backupRef)
	}
	results, err := p.Up(ctx)
	for _, r := range results {
		fmt.Printf("  applied %05d %s (%s)\n", r.Source.Version, r.Source.Path, r.Duration.Round(time.Millisecond))
	}
	return err
}

// open connects with the migration DSN to dbName, creating it if absent.
func open(ctx context.Context, dsn, dbName string) (*sql.DB, error) {
	c, err := mysql.ParseDSN(dsn)
	if err != nil {
		return nil, fmt.Errorf("COLLAR_ADMIN_DSN: %w", err)
	}
	c.ParseTime, c.Loc = true, time.UTC
	if c.Params == nil {
		c.Params = map[string]string{}
	}
	c.Params["time_zone"] = "'+00:00'"
	c.DBName = ""
	bootstrap, err := sql.Open("mysql", c.FormatDSN())
	if err != nil {
		return nil, err
	}
	defer bootstrap.Close()
	if _, err := bootstrap.ExecContext(ctx, "CREATE DATABASE IF NOT EXISTS `"+dbName+"` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		return nil, fmt.Errorf("ensure database %s: %w", dbName, err)
	}
	c.DBName = dbName
	return sql.Open("mysql", c.FormatDSN())
}

func upSection(path string) (string, error) {
	b, err := fs.ReadFile(migrations.FS, path)
	if err != nil {
		return "", err
	}
	s := string(b)
	if i := strings.Index(s, "-- +goose Up"); i >= 0 {
		s = s[i:]
	}
	if i := strings.Index(s, "-- +goose Down"); i >= 0 {
		s = s[:i]
	}
	return s, nil
}
