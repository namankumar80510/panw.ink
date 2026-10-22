// Package auth is Pawn.Ink's browser-session bridge over Collar app auth
// (IMPLEMENTATION.md §1.1 item 7, §5.1) and the application account features
// Collar does not provide (registration, email verification, reset).
//
// Collar remains the only JWT/refresh-token authority. The bridge keeps each
// browser's Collar tokens encrypted in browser_sessions and gives the browser
// only an opaque HttpOnly cookie. Per request it validates the cookie, the
// session's expiry/revocation and the live user state, refreshes the Collar
// access token under a per-session row lock, and injects it as a bearer
// token before Collar authenticates.
package auth

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/namankumar80510/panw.ink/internal/config"
	"github.com/namankumar80510/panw.ink/internal/cryptox"
	"github.com/namankumar80510/panw.ink/internal/db"
	"github.com/namankumar80510/panw.ink/internal/httpx"
)

// Roles.
const (
	RoleReader    = "reader"
	RoleAuthor    = "author"
	RoleModerator = "moderator"
	RoleOwner     = "owner"
)

// refreshSkew refreshes access tokens this long before they expire.
const refreshSkew = 30 * time.Second

// lastSeenEvery throttles last_seen_at writes.
const lastSeenEvery = 5 * time.Minute

// Viewer is the authenticated app user of a request.
type Viewer struct {
	UserID        uint64
	Email         string
	Handle        string
	DisplayName   string
	Role          string
	EmailVerified bool
	Timezone      string
	SessionID     uint64
	CSRFToken     string
}

// IsStaff reports owner or moderator.
func (v *Viewer) IsStaff() bool { return v != nil && (v.Role == RoleOwner || v.Role == RoleModerator) }

// CanUseStudio reports author or owner.
func (v *Viewer) CanUseStudio() bool {
	return v != nil && (v.Role == RoleOwner || v.Role == RoleAuthor)
}

type viewerKey struct{}

// WithViewer attaches v to ctx.
func WithViewer(ctx context.Context, v *Viewer) context.Context {
	return context.WithValue(ctx, viewerKey{}, v)
}

// ViewerFrom returns the request's viewer, or nil for anonymous requests.
func ViewerFrom(ctx context.Context) *Viewer {
	v, _ := ctx.Value(viewerKey{}).(*Viewer)
	return v
}

// Errors of session resolution.
var (
	errSessionGone = errors.New("auth: session revoked or expired")
	// ErrInvalidCredentials is the single, non-enumerating login failure.
	ErrInvalidCredentials = httpx.NewError(http.StatusUnauthorized, "INVALID_CREDENTIALS", "Email or password is incorrect.")
)

// Bridge owns browser sessions.
type Bridge struct {
	cfg    *config.Config
	db     *sql.DB
	t      db.Tables
	sealer *cryptox.Sealer
	collar dispatcher
	log    *slog.Logger
	now    func() time.Time

	locks sync.Map // session id -> *sync.Mutex (in-process; the row lock serializes across processes)
}

// NewBridge returns a Bridge. SetCollar must be called before serving.
func NewBridge(cfg *config.Config, database *sql.DB, t db.Tables, log *slog.Logger) (*Bridge, error) {
	s, err := cryptox.NewSealer(cfg.TokenEncryptionKey)
	if err != nil {
		return nil, err
	}
	return &Bridge{cfg: cfg, db: database, t: t, sealer: s, log: log, now: func() time.Time { return time.Now().UTC() }}, nil
}

// SetCollar sets Collar's complete handler for internal dispatch.
func (b *Bridge) SetCollar(h http.Handler) { b.collar.handler = h }

// SetClock replaces the clock (tests).
func (b *Bridge) SetClock(now func() time.Time) { b.now = now }

// CookieName is the session cookie name: __Host- prefixed over HTTPS.
func (b *Bridge) CookieName() string {
	if b.cfg.SecureCookies {
		return "__Host-pawn_session"
	}
	return "pawn_session"
}

func (b *Bridge) tokenHash(raw string) []byte {
	return cryptox.KeyedHash(b.cfg.SessionHMACKey, "session", raw)
}

// csrfFor derives the CSRF token of a session cookie value. It is never
// stored in clear; the SPA reads it from /auth/me and login responses.
func (b *Bridge) csrfFor(raw string) string {
	return base64.RawURLEncoding.EncodeToString(cryptox.KeyedHash(b.cfg.CSRFHMACKey, "csrf", raw))
}

// CheckCSRF reports whether header matches the CSRF token of the cookie.
func (b *Bridge) CheckCSRF(cookieValue, header string) bool {
	if cookieValue == "" || header == "" {
		return false
	}
	return cryptox.Equal([]byte(b.csrfFor(cookieValue)), []byte(header))
}

func adFor(kind string, userID uint64) []byte {
	return []byte("pawn:" + kind + ":" + strconv.FormatUint(userID, 10))
}

// SetCookie writes the session cookie.
func (b *Bridge) SetCookie(w http.ResponseWriter, raw string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name: b.CookieName(), Value: raw, Path: "/", HttpOnly: true, Secure: b.cfg.SecureCookies,
		SameSite: http.SameSiteLaxMode, Expires: expires, MaxAge: int(time.Until(expires).Seconds()),
	})
}

// ClearCookie expires the session cookie.
func (b *Bridge) ClearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: b.CookieName(), Value: "", Path: "/", HttpOnly: true, Secure: b.cfg.SecureCookies,
		SameSite: http.SameSiteLaxMode, MaxAge: -1, Expires: time.Unix(0, 0),
	})
}

// CookieValue returns the session cookie value of r.
func (b *Bridge) CookieValue(r *http.Request) string {
	c, err := r.Cookie(b.CookieName())
	if err != nil {
		return ""
	}
	return c.Value
}

// Resolved is the outcome of resolving a request's cookie.
type Resolved struct {
	Viewer      *Viewer
	AccessToken string
	// ClearCookie is set when the browser presented a cookie that is no
	// longer valid; the request continues anonymously.
	ClearCookie bool
}

type sessionRow struct {
	id, userID      uint64
	accessCT        []byte
	accessExpires   db.Time
	expires         db.Time
	lastSeen        db.Time
	email, display  string
	handle          sql.NullString
	role, state, tz string
	verifiedAt      db.Time
}

// Resolve validates the cookie of r and returns its viewer and a fresh
// Collar access token. Invalid/stale cookies resolve to anonymous with
// ClearCookie set, so public reading keeps working.
func (b *Bridge) Resolve(ctx context.Context, r *http.Request, clientIP string) Resolved {
	raw := b.CookieValue(r)
	if raw == "" {
		return Resolved{}
	}
	if len(raw) > 128 {
		return Resolved{ClearCookie: true}
	}
	var s sessionRow
	err := b.db.QueryRowContext(ctx, "SELECT s.id, s.user_id, s.access_token_ciphertext, s.access_expires_at, s.expires_at, s.last_seen_at,"+
		" u.email, u.display_name, u.handle, u.role, u.state, u.timezone, u.email_verified_at"+
		" FROM "+b.t.T("browser_sessions")+" s JOIN "+b.t.T("users")+" u ON u.id = s.user_id"+
		" WHERE s.token_hash = ? AND s.revoked_at IS NULL", b.tokenHash(raw)).
		Scan(&s.id, &s.userID, &s.accessCT, &s.accessExpires, &s.expires, &s.lastSeen,
			&s.email, &s.display, &s.handle, &s.role, &s.state, &s.tz, &s.verifiedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Resolved{ClearCookie: true}
	}
	if err != nil {
		b.log.Error("resolve session", "err", err)
		return Resolved{} // transient: anonymous for this request, keep the cookie
	}
	now := b.now()
	if !now.Before(s.expires.Time) {
		return Resolved{ClearCookie: true}
	}
	if s.state != "active" {
		b.revokeQuiet(ctx, s.id, s.userID, clientIP, "")
		return Resolved{ClearCookie: true}
	}

	var access string
	if now.Add(refreshSkew).Before(s.accessExpires.Time) {
		pt, err := b.sealer.Open(s.accessCT, adFor("access", s.userID))
		if err != nil {
			b.log.Warn("session access token undecryptable; revoking", "session_id", s.id)
			b.revokeQuiet(ctx, s.id, s.userID, clientIP, "")
			return Resolved{ClearCookie: true}
		}
		access = string(pt)
	} else {
		access, err = b.refresh(ctx, s.id, s.userID, clientIP, r.UserAgent())
		if errors.Is(err, errSessionGone) {
			return Resolved{ClearCookie: true}
		}
		if err != nil {
			b.log.Error("refresh session", "err", err, "session_id", s.id)
			return Resolved{}
		}
	}

	if now.Sub(s.lastSeen.Time) > lastSeenEvery {
		_, _ = b.db.ExecContext(ctx, "UPDATE "+b.t.T("browser_sessions")+" SET last_seen_at = ? WHERE id = ?", now, s.id)
	}
	return Resolved{
		Viewer: &Viewer{
			UserID: s.userID, Email: s.email, Handle: s.handle.String, DisplayName: s.display, Role: s.role,
			EmailVerified: s.verifiedAt.Valid, Timezone: s.tz, SessionID: s.id, CSRFToken: b.csrfFor(raw),
		},
		AccessToken: access,
	}
}

func (b *Bridge) lockFor(sessionID uint64) *sync.Mutex {
	m, _ := b.locks.LoadOrStore(sessionID, &sync.Mutex{})
	return m.(*sync.Mutex)
}

// refresh rotates the session's Collar tokens. Concurrent tabs serialize on
// an in-process mutex and on the session row (SELECT … FOR UPDATE), and a
// waiter that finds a fresh token reuses it instead of replaying the old
// refresh token, so Collar's reuse detection never fires (COL-04).
func (b *Bridge) refresh(ctx context.Context, sessionID, userID uint64, clientIP, ua string) (string, error) {
	mu := b.lockFor(sessionID)
	mu.Lock()
	defer mu.Unlock()

	tx, err := b.db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("auth: begin refresh: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var (
		accessCT, refreshCT []byte
		accessExp, expires  db.Time
		revoked             db.Time
	)
	err = tx.QueryRowContext(ctx, "SELECT access_token_ciphertext, access_expires_at, refresh_token_ciphertext, expires_at, revoked_at FROM "+
		b.t.T("browser_sessions")+" WHERE id = ? FOR UPDATE", sessionID).Scan(&accessCT, &accessExp, &refreshCT, &expires, &revoked)
	if errors.Is(err, sql.ErrNoRows) {
		return "", errSessionGone
	}
	if err != nil {
		return "", fmt.Errorf("auth: lock session: %w", err)
	}
	now := b.now()
	if revoked.Valid || !now.Before(expires.Time) {
		return "", errSessionGone
	}
	if now.Add(refreshSkew).Before(accessExp.Time) {
		pt, err := b.sealer.Open(accessCT, adFor("access", userID))
		if err != nil {
			return "", errSessionGone
		}
		return string(pt), tx.Commit()
	}
	refreshTok, err := b.sealer.Open(refreshCT, adFor("refresh", userID))
	if err != nil {
		return "", b.revokeInTx(ctx, tx, sessionID)
	}
	res, err := b.collar.do(ctx, http.MethodPost, collarRefreshPath, "", map[string]string{"refreshToken": string(refreshTok)}, clientIP, ua)
	if err != nil {
		return "", err
	}
	switch {
	case res.Status == http.StatusOK:
	case res.Status >= 400 && res.Status < 500 && res.Status != http.StatusTooManyRequests:
		// Expired, reused, or the user became inactive in Collar's view.
		return "", b.revokeInTx(ctx, tx, sessionID)
	default:
		return "", fmt.Errorf("auth: collar refresh status %d", res.Status)
	}
	var lr collarLogin
	if err := res.decode(&lr); err != nil {
		return "", err
	}
	newAccess, err := b.sealer.Seal([]byte(lr.Token), adFor("access", userID))
	if err != nil {
		return "", err
	}
	newRefresh, err := b.sealer.Seal([]byte(lr.RefreshToken), adFor("refresh", userID))
	if err != nil {
		return "", err
	}
	_, err = tx.ExecContext(ctx, "UPDATE "+b.t.T("browser_sessions")+
		" SET access_token_ciphertext = ?, access_expires_at = ?, refresh_token_ciphertext = ?, refresh_version = refresh_version + 1 WHERE id = ?",
		newAccess, now.Add(time.Duration(lr.ExpiresIn)*time.Second), newRefresh, sessionID)
	if err != nil {
		return "", fmt.Errorf("auth: store refreshed tokens: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("auth: commit refresh: %w", err)
	}
	return lr.Token, nil
}

func (b *Bridge) revokeInTx(ctx context.Context, tx *sql.Tx, sessionID uint64) error {
	if _, err := tx.ExecContext(ctx, "UPDATE "+b.t.T("browser_sessions")+" SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL", b.now(), sessionID); err != nil {
		return fmt.Errorf("auth: revoke: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("auth: revoke commit: %w", err)
	}
	return errSessionGone
}

// LoginResult is a successful login.
type LoginResult struct {
	Viewer    *Viewer
	RawCookie string
	Expires   time.Time
}

// Login authenticates through Collar's app login (identity + password) and
// creates a bridge session. Every failure that could reveal whether the
// account exists maps to ErrInvalidCredentials.
func (b *Bridge) Login(ctx context.Context, email, password, clientIP, ua string) (*LoginResult, error) {
	res, err := b.collar.do(ctx, http.MethodPost, collarLoginPath, "", map[string]string{"identity": email, "password": password}, clientIP, ua)
	if err != nil {
		return nil, err
	}
	switch {
	case res.Status == http.StatusOK:
	case res.Status == http.StatusTooManyRequests:
		return nil, httpx.ErrRateLimited
	case res.Status >= 400 && res.Status < 500:
		return nil, ErrInvalidCredentials
	default:
		return nil, fmt.Errorf("auth: collar login status %d", res.Status)
	}
	var lr collarLogin
	if err := res.decode(&lr); err != nil {
		return nil, err
	}
	if lr.Identity.Kind != "record" || lr.Identity.App != "pawn" || lr.Identity.Collection != "content.users" {
		return nil, fmt.Errorf("auth: unexpected login identity kind=%q app=%q", lr.Identity.Kind, lr.Identity.App)
	}
	userID, err := strconv.ParseUint(fmt.Sprint(lr.Identity.ID), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("auth: login identity id: %w", err)
	}
	me, err := b.collar.do(ctx, http.MethodGet, collarMePath, lr.Token, nil, clientIP, ua)
	if err != nil {
		return nil, err
	}
	var meBody collarMe
	if me.Status != http.StatusOK {
		return nil, fmt.Errorf("auth: collar me status %d", me.Status)
	}
	if err := me.decode(&meBody); err != nil {
		return nil, err
	}
	return b.createSession(ctx, userID, meBody.SessionID, lr, ua)
}

func (b *Bridge) createSession(ctx context.Context, userID uint64, collarSessionID string, lr collarLogin, ua string) (*LoginResult, error) {
	raw, err := cryptox.RandomToken(32) // 256 bits
	if err != nil {
		return nil, err
	}
	accessCT, err := b.sealer.Seal([]byte(lr.Token), adFor("access", userID))
	if err != nil {
		return nil, err
	}
	refreshCT, err := b.sealer.Seal([]byte(lr.RefreshToken), adFor("refresh", userID))
	if err != nil {
		return nil, err
	}
	now := b.now()
	expires := now.Add(b.cfg.SessionTTL)
	csrf := b.csrfFor(raw)
	res, err := b.db.ExecContext(ctx, "INSERT INTO "+b.t.T("browser_sessions")+
		" (user_id, token_hash, csrf_token_hash, collar_session_id, access_token_ciphertext, access_expires_at, refresh_token_ciphertext, expires_at, last_seen_at, user_agent_summary, created_at)"+
		" VALUES (?,?,?,?,?,?,?,?,?,?,?)",
		userID, b.tokenHash(raw), cryptox.SHA256([]byte(csrf)), collarSessionID, accessCT,
		now.Add(time.Duration(lr.ExpiresIn)*time.Second), refreshCT, expires, now, summarizeUA(ua), now)
	if err != nil {
		return nil, fmt.Errorf("auth: create session: %w", err)
	}
	sid, _ := res.LastInsertId()
	v, err := b.loadViewer(ctx, userID)
	if err != nil {
		return nil, err
	}
	v.SessionID, v.CSRFToken = uint64(sid), csrf
	return &LoginResult{Viewer: v, RawCookie: raw, Expires: expires}, nil
}

func (b *Bridge) loadViewer(ctx context.Context, userID uint64) (*Viewer, error) {
	v := &Viewer{UserID: userID}
	var handle sql.NullString
	var verified db.Time
	err := b.db.QueryRowContext(ctx, "SELECT email, display_name, handle, role, timezone, email_verified_at FROM "+b.t.T("users")+" WHERE id = ?", userID).
		Scan(&v.Email, &v.DisplayName, &handle, &v.Role, &v.Timezone, &verified)
	if err != nil {
		return nil, fmt.Errorf("auth: load viewer: %w", err)
	}
	v.Handle, v.EmailVerified = handle.String, verified.Valid
	return v, nil
}

// Rotate replaces the cookie token of a session (after a password change)
// and returns the new raw cookie value.
func (b *Bridge) Rotate(ctx context.Context, sessionID uint64) (string, time.Time, error) {
	raw, err := cryptox.RandomToken(32)
	if err != nil {
		return "", time.Time{}, err
	}
	var expires db.Time
	if err := b.db.QueryRowContext(ctx, "SELECT expires_at FROM "+b.t.T("browser_sessions")+" WHERE id = ?", sessionID).Scan(&expires); err != nil {
		return "", time.Time{}, fmt.Errorf("auth: rotate: %w", err)
	}
	_, err = b.db.ExecContext(ctx, "UPDATE "+b.t.T("browser_sessions")+" SET token_hash = ?, csrf_token_hash = ? WHERE id = ? AND revoked_at IS NULL",
		b.tokenHash(raw), cryptox.SHA256([]byte(b.csrfFor(raw))), sessionID)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("auth: rotate: %w", err)
	}
	return raw, expires.Time, nil
}

// CSRFFor exposes the CSRF token of a raw cookie value (login/rotate responses).
func (b *Bridge) CSRFFor(raw string) string { return b.csrfFor(raw) }

// Revoke ends one bridge session immediately and best-effort ends its Collar
// session too (§5.1: do not rely solely on JWT expiry).
func (b *Bridge) Revoke(ctx context.Context, sessionID, userID uint64, clientIP, ua string) {
	b.revokeQuiet(ctx, sessionID, userID, clientIP, ua)
}

// RevokeAllForUser ends every live bridge session of a user except keep (0 =
// none), reconciling the Collar sessions behind them.
func (b *Bridge) RevokeAllForUser(ctx context.Context, userID, keep uint64, clientIP string) error {
	rows, err := b.db.QueryContext(ctx, "SELECT id FROM "+b.t.T("browser_sessions")+" WHERE user_id = ? AND revoked_at IS NULL AND id <> ?", userID, keep)
	if err != nil {
		return fmt.Errorf("auth: list sessions: %w", err)
	}
	var ids []uint64
	for rows.Next() {
		var id uint64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return fmt.Errorf("auth: list sessions: %w", err)
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("auth: list sessions: %w", err)
	}
	for _, id := range ids {
		b.revokeQuiet(ctx, id, userID, clientIP, "")
	}
	return nil
}

// revokeQuiet marks the session revoked first (immediate effect), then
// tries to end the Collar session with its tokens. Collar failures are
// logged, never surfaced: the browser can no longer reach those tokens.
func (b *Bridge) revokeQuiet(ctx context.Context, sessionID, userID uint64, clientIP, ua string) {
	var accessCT, refreshCT []byte
	var accessExp db.Time
	err := db.InTx(ctx, b.db, func(tx *sql.Tx) error {
		var revoked db.Time
		err := tx.QueryRowContext(ctx, "SELECT access_token_ciphertext, refresh_token_ciphertext, access_expires_at, revoked_at FROM "+
			b.t.T("browser_sessions")+" WHERE id = ? FOR UPDATE", sessionID).Scan(&accessCT, &refreshCT, &accessExp, &revoked)
		if err != nil {
			return err
		}
		if revoked.Valid {
			accessCT, refreshCT = nil, nil
			return nil
		}
		_, err = tx.ExecContext(ctx, "UPDATE "+b.t.T("browser_sessions")+" SET revoked_at = ? WHERE id = ?", b.now(), sessionID)
		return err
	})
	if err != nil {
		b.log.Error("revoke session", "err", err, "session_id", sessionID)
		return
	}
	if accessCT == nil {
		return
	}
	access := ""
	if b.now().Add(refreshSkew).Before(accessExp.Time) {
		if pt, err := b.sealer.Open(accessCT, adFor("access", userID)); err == nil {
			access = string(pt)
		}
	}
	if access == "" {
		// Access expired: rotate once to obtain a token that can log out.
		rt, err := b.sealer.Open(refreshCT, adFor("refresh", userID))
		if err != nil {
			return
		}
		res, err := b.collar.do(ctx, http.MethodPost, collarRefreshPath, "", map[string]string{"refreshToken": string(rt)}, clientIP, ua)
		if err != nil || res.Status != http.StatusOK {
			return // already invalid in Collar
		}
		var lr collarLogin
		if res.decode(&lr) != nil {
			return
		}
		access = lr.Token
	}
	if res, err := b.collar.do(ctx, http.MethodPost, collarLogoutPath, access, nil, clientIP, ua); err != nil || res.Status >= 500 {
		b.log.Warn("collar logout failed", "session_id", sessionID, "status", res.Status)
	}
}

// ChangePassword changes the password through Collar's change-password
// endpoint (which revokes the user's other Collar sessions), then revokes
// the user's other bridge sessions. Field errors use the custom API keys.
func (b *Bridge) ChangePassword(ctx context.Context, accessToken, oldPassword, newPassword, clientIP, ua string) error {
	res, err := b.collar.do(ctx, http.MethodPost, collarChangePasswordPath, accessToken,
		map[string]string{"oldPassword": oldPassword, "password": newPassword, "passwordConfirm": newPassword}, clientIP, ua)
	if err != nil {
		return err
	}
	switch {
	case res.Status == http.StatusNoContent || res.Status == http.StatusOK:
		return nil
	case res.Status == http.StatusUnauthorized:
		return httpx.ErrUnauthenticated
	case res.Status == http.StatusBadRequest || res.Status == http.StatusUnprocessableEntity:
		_, details := res.errorCode()
		fields := map[string]string{}
		for k, msgs := range details {
			name := map[string]string{"oldPassword": "old_password", "password": "new_password"}[k]
			if name == "" || len(msgs) == 0 {
				continue
			}
			fields[name] = msgs[0]
		}
		if len(fields) == 0 {
			fields["new_password"] = "Could not change the password."
		}
		return httpx.Validation(fields)
	}
	return fmt.Errorf("auth: collar change-password status %d", res.Status)
}

// SessionInfo is one session in the account's session list.
type SessionInfo struct {
	ID         string    `json:"id"`
	CreatedAt  time.Time `json:"created_at"`
	LastSeenAt time.Time `json:"last_seen_at"`
	ExpiresAt  time.Time `json:"expires_at"`
	Device     string    `json:"device"`
	Current    bool      `json:"current"`
}

// ListSessions lists a user's live sessions, newest first.
func (b *Bridge) ListSessions(ctx context.Context, userID, current uint64) ([]SessionInfo, error) {
	rows, err := b.db.QueryContext(ctx, "SELECT id, created_at, last_seen_at, expires_at, COALESCE(user_agent_summary, '') FROM "+b.t.T("browser_sessions")+
		" WHERE user_id = ? AND revoked_at IS NULL AND expires_at > ? ORDER BY created_at DESC LIMIT 50", userID, b.now())
	if err != nil {
		return nil, fmt.Errorf("auth: sessions: %w", err)
	}
	defer rows.Close()
	out := []SessionInfo{}
	for rows.Next() {
		var s SessionInfo
		var id uint64
		var created, seen, exp db.Time
		if err := rows.Scan(&id, &created, &seen, &exp, &s.Device); err != nil {
			return nil, fmt.Errorf("auth: sessions: %w", err)
		}
		s.CreatedAt, s.LastSeenAt, s.ExpiresAt = created.Time, seen.Time, exp.Time
		s.ID, s.Current = strconv.FormatUint(id, 10), id == current
		out = append(out, s)
	}
	return out, rows.Err()
}

// OwnsSession reports whether a live session belongs to a user.
func (b *Bridge) OwnsSession(ctx context.Context, userID, sessionID uint64) (bool, error) {
	var n int
	err := b.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+b.t.T("browser_sessions")+" WHERE id = ? AND user_id = ? AND revoked_at IS NULL", sessionID, userID).Scan(&n)
	return n > 0, err
}

// summarizeUA keeps a coarse, non-identifying device label.
func summarizeUA(ua string) string {
	l := strings.ToLower(ua)
	browser := "Browser"
	switch {
	case strings.Contains(l, "firefox/"):
		browser = "Firefox"
	case strings.Contains(l, "edg/"):
		browser = "Edge"
	case strings.Contains(l, "chrome/"):
		browser = "Chrome"
	case strings.Contains(l, "safari/"):
		browser = "Safari"
	}
	osName := "unknown OS"
	switch {
	case strings.Contains(l, "android"):
		osName = "Android"
	case strings.Contains(l, "iphone"), strings.Contains(l, "ipad"):
		osName = "iOS"
	case strings.Contains(l, "windows"):
		osName = "Windows"
	case strings.Contains(l, "mac os"):
		osName = "macOS"
	case strings.Contains(l, "linux"):
		osName = "Linux"
	}
	return browser + " on " + osName
}

type accessKey struct{}

// WithAccessToken attaches the session's Collar access token to ctx, for
// internal dispatch on behalf of the caller (e.g. change-password).
func WithAccessToken(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, accessKey{}, token)
}

// AccessTokenFrom returns the caller's Collar access token, or "".
func AccessTokenFrom(ctx context.Context) string {
	s, _ := ctx.Value(accessKey{}).(string)
	return s
}
