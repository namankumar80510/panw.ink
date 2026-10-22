package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-sql-driver/mysql"

	"github.com/namankumar80510/panw.ink/internal/cryptox"
	"github.com/namankumar80510/panw.ink/internal/db"
	"github.com/namankumar80510/panw.ink/internal/httpx"
	pmail "github.com/namankumar80510/panw.ink/internal/mail"
)

// Token lifetimes.
const (
	verifyTokenTTL = 48 * time.Hour
	resetTokenTTL  = time.Hour
)

var handleRE = regexp.MustCompile(`^[a-z0-9_]{3,40}$`)

// reservedHandles cannot be claimed by readers.
var reservedHandles = map[string]bool{
	"admin": true, "administrator": true, "owner": true, "moderator": true, "staff": true, "support": true,
	"pawn": true, "pawnink": true, "pawn_ink": true, "system": true, "root": true, "studio": true, "api": true,
}

// NormalizeEmail lowercases and validates an address; "" when invalid.
func NormalizeEmail(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if len(s) < 3 || len(s) > 254 || strings.ContainsAny(s, " \t\r\n<>") {
		return ""
	}
	addr, err := mail.ParseAddress(s)
	if err != nil || addr.Address != s || !strings.Contains(s[strings.LastIndexByte(s, '@')+1:], ".") {
		return ""
	}
	return s
}

// Accounts implements the application account features: registration,
// email verification and password reset. Collar supplies none of these.
type Accounts struct {
	Bridge *Bridge
	Jobs   pmail.Enqueuer
}

// RegisterInput is POST /auth/register.
type RegisterInput struct {
	Email       string  `json:"email"`
	Password    string  `json:"password"`
	Handle      *string `json:"handle"`
	DisplayName string  `json:"display_name"`
}

// Register creates an unverified reader and queues verification mail. For
// an existing address it queues an "account exists" notice instead and
// returns the same result, so responses never reveal registered emails.
func (a *Accounts) Register(ctx context.Context, in RegisterInput, requestID string) error {
	b := a.Bridge
	fields := map[string]string{}
	email := NormalizeEmail(in.Email)
	if email == "" {
		fields["email"] = "Enter a valid email address."
	}
	if msg := CheckPassword(in.Password, email); msg != "" {
		fields["password"] = msg
	}
	display := strings.TrimSpace(in.DisplayName)
	if n := utf8.RuneCountInString(display); n < 1 || n > 80 {
		fields["display_name"] = "Use 1 to 80 characters."
	}
	var handle any
	if in.Handle != nil && strings.TrimSpace(*in.Handle) != "" {
		h := strings.ToLower(strings.TrimSpace(*in.Handle))
		switch {
		case !handleRE.MatchString(h):
			fields["handle"] = "Use 3 to 40 lowercase letters, digits or underscores."
		case reservedHandles[h]:
			fields["handle"] = "This handle is reserved."
		default:
			handle = h
		}
	}
	if len(fields) > 0 {
		return httpx.Validation(fields)
	}
	hash, err := HashPassword(in.Password)
	if err != nil {
		return err
	}
	return db.InTx(ctx, b.db, func(tx *sql.Tx) error {
		var existing uint64
		err := tx.QueryRowContext(ctx, "SELECT id FROM "+b.t.T("users")+" WHERE email = ? FOR UPDATE", email).Scan(&existing)
		switch {
		case err == nil:
			uid := existing
			return pmail.Enqueue(ctx, tx, b.t, a.Jobs, &uid, email, pmail.TemplateExistingAccount,
				map[string]string{"link": b.cfg.BaseOrigin + "/login"})
		case !errors.Is(err, sql.ErrNoRows):
			return fmt.Errorf("auth: register lookup: %w", err)
		}
		res, err := tx.ExecContext(ctx, "INSERT INTO "+b.t.T("users")+
			" (email, handle, display_name, password_hash, role, state) VALUES (?,?,?,?, 'reader', 'active')",
			email, handle, display, hash)
		var me *mysql.MySQLError
		if errors.As(err, &me) && me.Number == 1062 {
			if strings.Contains(me.Message, "handle") {
				return httpx.Validation(map[string]string{"handle": "This handle is taken."})
			}
			return nil // concurrent registration of the same email: same generic outcome
		}
		if err != nil {
			return fmt.Errorf("auth: register insert: %w", err)
		}
		id64, _ := res.LastInsertId()
		uid := uint64(id64)
		if err := db.Audit(ctx, tx, b.t, &uid, "user.register", "user", &uid, nil, nil, requestID); err != nil {
			return err
		}
		return a.queueToken(ctx, tx, uid, email, "verify_email", verifyTokenTTL, pmail.TemplateVerifyEmail, "/verify-email")
	})
}

func (a *Accounts) queueToken(ctx context.Context, tx *sql.Tx, userID uint64, email, kind string, ttl time.Duration, template, route string) error {
	b := a.Bridge
	raw, err := cryptox.RandomToken(32)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO "+b.t.T("auth_tokens")+" (user_id, type, token_hash, expires_at) VALUES (?,?,?,?)",
		userID, kind, cryptox.KeyedHash(b.cfg.SessionHMACKey, "auth:"+kind, raw), b.now().Add(ttl))
	if err != nil {
		return fmt.Errorf("auth: store %s token: %w", kind, err)
	}
	link := b.cfg.BaseOrigin + route + "?token=" + raw
	return pmail.Enqueue(ctx, tx, b.t, a.Jobs, &userID, email, template, map[string]string{"link": link})
}

// consumeToken atomically consumes a one-time token; it returns the user id
// or an error that does not reveal why the token is unusable.
func (a *Accounts) consumeToken(ctx context.Context, tx *sql.Tx, kind, raw string) (uint64, error) {
	b := a.Bridge
	bad := httpx.NewError(http.StatusBadRequest, "TOKEN_INVALID", "This link is invalid or has expired. Request a new one.")
	if raw == "" || len(raw) > 128 {
		return 0, bad
	}
	var id, userID uint64
	err := tx.QueryRowContext(ctx, "SELECT id, user_id FROM "+b.t.T("auth_tokens")+
		" WHERE token_hash = ? AND type = ? AND consumed_at IS NULL AND expires_at > ? FOR UPDATE",
		cryptox.KeyedHash(b.cfg.SessionHMACKey, "auth:"+kind, raw), kind, b.now()).Scan(&id, &userID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, bad
	}
	if err != nil {
		return 0, fmt.Errorf("auth: token lookup: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "UPDATE "+b.t.T("auth_tokens")+" SET consumed_at = ? WHERE id = ?", b.now(), id); err != nil {
		return 0, fmt.Errorf("auth: consume token: %w", err)
	}
	return userID, nil
}

// VerifyEmail consumes a verification token.
func (a *Accounts) VerifyEmail(ctx context.Context, token, requestID string) error {
	b := a.Bridge
	return db.InTx(ctx, b.db, func(tx *sql.Tx) error {
		uid, err := a.consumeToken(ctx, tx, "verify_email", token)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "UPDATE "+b.t.T("users")+" SET email_verified_at = COALESCE(email_verified_at, ?) WHERE id = ? AND state = 'active'", b.now(), uid); err != nil {
			return fmt.Errorf("auth: verify: %w", err)
		}
		return db.Audit(ctx, tx, b.t, &uid, "user.verify_email", "user", &uid, nil, nil, requestID)
	})
}

// ResendVerification queues a new verification link for an unverified,
// active account. It is silent about whether the address exists.
func (a *Accounts) ResendVerification(ctx context.Context, email string) error {
	b := a.Bridge
	email = NormalizeEmail(email)
	if email == "" {
		return nil
	}
	return db.InTx(ctx, b.db, func(tx *sql.Tx) error {
		var uid uint64
		err := tx.QueryRowContext(ctx, "SELECT id FROM "+b.t.T("users")+" WHERE email = ? AND state = 'active' AND email_verified_at IS NULL FOR UPDATE", email).Scan(&uid)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("auth: resend lookup: %w", err)
		}
		return a.queueToken(ctx, tx, uid, email, "verify_email", verifyTokenTTL, pmail.TemplateVerifyEmail, "/verify-email")
	})
}

// ForgotPassword queues a reset link for an active account, silently.
func (a *Accounts) ForgotPassword(ctx context.Context, email string) error {
	b := a.Bridge
	email = NormalizeEmail(email)
	if email == "" {
		return nil
	}
	return db.InTx(ctx, b.db, func(tx *sql.Tx) error {
		var uid uint64
		err := tx.QueryRowContext(ctx, "SELECT id FROM "+b.t.T("users")+" WHERE email = ? AND state = 'active' FOR UPDATE", email).Scan(&uid)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("auth: forgot lookup: %w", err)
		}
		return a.queueToken(ctx, tx, uid, email, "reset_password", resetTokenTTL, pmail.TemplateResetPassword, "/reset-password")
	})
}

// ResetPassword consumes a reset token, sets the new password and revokes
// every session of the user (bridge and Collar).
func (a *Accounts) ResetPassword(ctx context.Context, token, newPassword, clientIP, requestID string) error {
	b := a.Bridge
	var uid uint64
	var email string
	err := db.InTx(ctx, b.db, func(tx *sql.Tx) error {
		var err error
		uid, err = a.consumeToken(ctx, tx, "reset_password", token)
		if err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, "SELECT email FROM "+b.t.T("users")+" WHERE id = ? AND state = 'active' FOR UPDATE", uid).Scan(&email); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return httpx.NewError(http.StatusBadRequest, "TOKEN_INVALID", "This link is invalid or has expired. Request a new one.")
			}
			return fmt.Errorf("auth: reset lookup: %w", err)
		}
		if msg := CheckPassword(newPassword, email); msg != "" {
			return httpx.Validation(map[string]string{"new_password": msg})
		}
		hash, err := HashPassword(newPassword)
		if err != nil {
			return err
		}
		// A reset proves control of the mailbox, so it also verifies it.
		if _, err := tx.ExecContext(ctx, "UPDATE "+b.t.T("users")+" SET password_hash = ?, email_verified_at = COALESCE(email_verified_at, ?) WHERE id = ?", hash, b.now(), uid); err != nil {
			return fmt.Errorf("auth: reset update: %w", err)
		}
		// Invalidate any other outstanding reset links.
		if _, err := tx.ExecContext(ctx, "UPDATE "+b.t.T("auth_tokens")+" SET consumed_at = ? WHERE user_id = ? AND type = 'reset_password' AND consumed_at IS NULL", b.now(), uid); err != nil {
			return fmt.Errorf("auth: reset tokens: %w", err)
		}
		return db.Audit(ctx, tx, b.t, &uid, "user.password_reset", "user", &uid, nil, nil, requestID)
	})
	if err != nil {
		return err
	}
	return b.RevokeAllForUser(ctx, uid, 0, clientIP)
}

// ProfileInput is PATCH /users/me. Unknown fields (role, email, state, …)
// are rejected by strict decoding before reaching here (AUTH-04).
type ProfileInput struct {
	DisplayName *string         `json:"display_name"`
	Timezone    *string         `json:"timezone"`
	Settings    *map[string]any `json:"settings"`
}

// UpdateProfile applies allowed profile fields.
func (a *Accounts) UpdateProfile(ctx context.Context, userID uint64, in ProfileInput, settingsJSON []byte) error {
	b := a.Bridge
	sets, args, fields := []string{}, []any{}, map[string]string{}
	if in.DisplayName != nil {
		d := strings.TrimSpace(*in.DisplayName)
		if n := utf8.RuneCountInString(d); n < 1 || n > 80 {
			fields["display_name"] = "Use 1 to 80 characters."
		}
		sets, args = append(sets, "display_name = ?"), append(args, d)
	}
	if in.Timezone != nil {
		if _, err := time.LoadLocation(*in.Timezone); err != nil || len(*in.Timezone) > 60 || *in.Timezone == "" || *in.Timezone == "Local" {
			fields["timezone"] = "Choose a valid IANA time zone such as Europe/London."
		}
		sets, args = append(sets, "timezone = ?"), append(args, *in.Timezone)
	}
	if in.Settings != nil {
		if len(settingsJSON) > 8<<10 {
			fields["settings"] = "Settings are too large."
		}
		sets, args = append(sets, "reader_settings_json = ?"), append(args, string(settingsJSON))
	}
	if len(fields) > 0 {
		return httpx.Validation(fields)
	}
	if len(sets) == 0 {
		return nil
	}
	_, err := b.db.ExecContext(ctx, "UPDATE "+b.t.T("users")+" SET "+strings.Join(sets, ", ")+" WHERE id = ?", append(args, userID)...)
	if err != nil {
		return fmt.Errorf("auth: profile: %w", err)
	}
	return nil
}

// Settings returns the stored reader settings JSON of a user (or nil).
func (a *Accounts) Settings(ctx context.Context, userID uint64) ([]byte, error) {
	var raw sql.NullString
	err := a.Bridge.db.QueryRowContext(ctx, "SELECT reader_settings_json FROM "+a.Bridge.t.T("users")+" WHERE id = ?", userID).Scan(&raw)
	if err != nil || !raw.Valid {
		return nil, err
	}
	return []byte(raw.String), nil
}
