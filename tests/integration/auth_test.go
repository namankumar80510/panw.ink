//go:build integration

package integration

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/xmfunew/collar"

	"github.com/namankumar80510/panw.ink/internal/jobs"
	"github.com/namankumar80510/panw.ink/internal/mail"
)

// AUTH-01 (account half): register, sign in unverified, verify, token single use.
func TestRegisterVerifyLogin(t *testing.T) {
	email := uniqueEmail("reader")
	tok := register(t, email)

	c := newClient(t)
	expect(t, c.do("GET", "/api/v1/pawn/x/auth/me", nil), http.StatusUnauthorized)

	c = login(t, email, testPassword) // proves Collar verifies Pawn.Ink's argon2id hashes
	me := c.do("GET", "/api/v1/pawn/x/auth/me", nil)
	expect(t, me, http.StatusOK)
	user := me.json(t)["data"].(map[string]any)["user"].(map[string]any)
	if user["role"] != "reader" || user["email_verified"] != false {
		t.Fatalf("new account: %v", user)
	}
	if perms := user["permissions"].(map[string]any); perms["can_comment"] != false || perms["studio"] != false {
		t.Fatalf("unverified reader permissions: %v", perms)
	}
	if bytes.Contains(me.Body, []byte("password")) || bytes.Contains(me.Body, []byte("token\":\"ey")) {
		t.Fatalf("me leaks secrets: %s", me.Body)
	}

	expect(t, newClient(t).do("POST", "/api/v1/pawn/x/auth/verify-email", map[string]any{"token": tok}), http.StatusOK)
	r := newClient(t).do("POST", "/api/v1/pawn/x/auth/verify-email", map[string]any{"token": tok})
	if r.Status != http.StatusBadRequest || r.errorCode(t) != "TOKEN_INVALID" {
		t.Fatalf("token reuse: %d %s", r.Status, r.Body)
	}
	user = c.do("GET", "/api/v1/pawn/x/auth/me", nil).json(t)["data"].(map[string]any)["user"].(map[string]any)
	if user["email_verified"] != true || user["permissions"].(map[string]any)["can_comment"] != true {
		t.Fatalf("verified account: %v", user)
	}
}

// Registering an existing address answers exactly like a new one.
func TestRegisterDoesNotEnumerate(t *testing.T) {
	email := uniqueEmail("dup")
	register(t, email)
	r := newClient(t).do("POST", "/api/v1/pawn/x/auth/register", map[string]any{"email": email, "password": testPassword, "display_name": "Someone"})
	expect(t, r, http.StatusAccepted)
	var n int
	_ = sqlDB.QueryRow("SELECT COUNT(*) FROM "+tables.T("email_outbox")+" WHERE recipient = ? AND template = 'existing_account'", email).Scan(&n)
	if n != 1 {
		t.Fatalf("existing-account notice queued %d times", n)
	}
	var users int
	_ = sqlDB.QueryRow("SELECT COUNT(*) FROM "+tables.T("users")+" WHERE email = ?", email).Scan(&users)
	if users != 1 {
		t.Fatalf("%d users for one email", users)
	}
}

func TestRegisterValidation(t *testing.T) {
	r := newClient(t).do("POST", "/api/v1/pawn/x/auth/register", map[string]any{"email": "nope", "password": "short", "display_name": "", "handle": "A!"})
	expect(t, r, http.StatusUnprocessableEntity)
	fields := r.json(t)["error"].(map[string]any)["details"].(map[string]any)["fields"].(map[string]any)
	for _, f := range []string{"email", "password", "display_name", "handle"} {
		if fields[f] == nil {
			t.Errorf("missing field error %q in %v", f, fields)
		}
	}
	r = newClient(t).do("POST", "/api/v1/pawn/x/auth/register", map[string]any{"email": uniqueEmail("x"), "password": testPassword, "display_name": "X", "role": "owner"})
	expect(t, r, http.StatusUnprocessableEntity)
}

// AUTH-02: wrong passwords are throttled and never reveal account existence.
func TestLoginThrottleAndNoEnumeration(t *testing.T) {
	email := uniqueEmail("throttle")
	register(t, email)
	c := newClient(t)
	known := c.do("POST", "/api/v1/pawn/x/auth/login", map[string]any{"email": email, "password": "wrong password!!"})
	unknown := c.do("POST", "/api/v1/pawn/x/auth/login", map[string]any{"email": uniqueEmail("ghost"), "password": "wrong password!!"})
	if known.Status != unknown.Status || known.errorCode(t) != unknown.errorCode(t) || known.errorCode(t) != "INVALID_CREDENTIALS" {
		t.Fatalf("login failures differ: %d %s / %d %s", known.Status, known.Body, unknown.Status, unknown.Body)
	}
	limited := false
	for i := 0; i < 20; i++ {
		r := c.do("POST", "/api/v1/pawn/x/auth/login", map[string]any{"email": email, "password": "wrong password!!"})
		if r.Status == http.StatusTooManyRequests {
			limited = true
			break
		}
		expect(t, r, http.StatusUnauthorized)
	}
	if !limited {
		t.Fatal("20 wrong passwords were never throttled")
	}
}

// AUTH-03 and forgot-password anti-enumeration; reset revokes sessions.
func TestPasswordReset(t *testing.T) {
	email, session := verifiedUser(t, "reset")
	c := newClient(t)
	a := c.do("POST", "/api/v1/pawn/x/auth/password/forgot", map[string]any{"email": email})
	b := c.do("POST", "/api/v1/pawn/x/auth/password/forgot", map[string]any{"email": uniqueEmail("nobody")})
	expect(t, a, http.StatusAccepted)
	expect(t, b, http.StatusAccepted)
	strip := func(r response) string { s := string(r.Body); return s[:strings.Index(s, `"meta"`)] }
	if strip(a) != strip(b) {
		t.Fatalf("forgot responses differ:\n%s\n%s", a.Body, b.Body)
	}
	tok := latestToken(t, email, "reset_password")
	newPw := "another long passphrase"
	expect(t, c.do("POST", "/api/v1/pawn/x/auth/password/reset", map[string]any{"token": tok, "new_password": newPw}), http.StatusOK)
	r := c.do("POST", "/api/v1/pawn/x/auth/password/reset", map[string]any{"token": tok, "new_password": "yet another passphrase"})
	if r.Status != http.StatusBadRequest || r.errorCode(t) != "TOKEN_INVALID" {
		t.Fatalf("reset token reuse: %d %s", r.Status, r.Body)
	}
	expect(t, session.do("GET", "/api/v1/pawn/x/auth/me", nil), http.StatusUnauthorized)
	r = newClient(t).do("POST", "/api/v1/pawn/x/auth/login", map[string]any{"email": email, "password": testPassword})
	expect(t, r, http.StatusUnauthorized)
	login(t, email, newPw)
}

// AUTH-04: privilege fields cannot be set through the profile endpoint.
func TestProfileRejectsPrivilegeFields(t *testing.T) {
	email, c := verifiedUser(t, "escalate")
	for _, body := range []map[string]any{
		{"role": "owner"}, {"display_name": "X", "role": "owner"}, {"email": "x@example.invalid"},
		{"state": "active"}, {"email_verified_at": "2020-01-01T00:00:00Z"},
	} {
		r := c.do("PATCH", "/api/v1/pawn/x/users/me", body)
		expect(t, r, http.StatusUnprocessableEntity)
	}
	var role string
	_ = sqlDB.QueryRow("SELECT role FROM "+tables.T("users")+" WHERE email = ?", email).Scan(&role)
	if role != "reader" {
		t.Fatalf("role changed to %q", role)
	}
	r := c.do("PATCH", "/api/v1/pawn/x/users/me", map[string]any{"display_name": "Renamed", "timezone": "Asia/Kolkata", "settings": map[string]any{"theme": "sepia"}})
	expect(t, r, http.StatusOK)
	d := r.json(t)["data"].(map[string]any)
	if d["display_name"] != "Renamed" || d["timezone"] != "Asia/Kolkata" || d["settings"].(map[string]any)["theme"] != "sepia" {
		t.Fatalf("profile update: %v", d)
	}
	expect(t, c.do("PATCH", "/api/v1/pawn/x/users/me", map[string]any{"timezone": "Mars/Base"}), http.StatusUnprocessableEntity)
	// Native generic writes to users are locked too.
	r = c.do("PATCH", "/api/v1/pawn/content/users/"+itoa(userID(t, email)), map[string]any{"role": "owner"})
	if r.Status < 400 {
		t.Fatalf("native users PATCH allowed: %d %s", r.Status, r.Body)
	}
}

// SEC-03: cookie mutations need same-origin Origin and the CSRF token.
func TestCSRFAndOrigin(t *testing.T) {
	_, c := verifiedUser(t, "csrf")
	body := map[string]any{"display_name": "Changed"}
	r := c.do("PATCH", "/api/v1/pawn/x/users/me", body, noCSRF())
	if r.Status != http.StatusForbidden || r.errorCode(t) != "CSRF_REJECTED" {
		t.Fatalf("missing token: %d %s", r.Status, r.Body)
	}
	r = c.do("PATCH", "/api/v1/pawn/x/users/me", body, withHeader("X-CSRF-Token", "forged"))
	expect(t, r, http.StatusForbidden)
	r = c.do("PATCH", "/api/v1/pawn/x/users/me", body, withHeader("Origin", "https://evil.example"))
	expect(t, r, http.StatusForbidden)
	r = c.do("POST", "/api/v1/pawn/x/auth/login", map[string]any{"email": "a@b.co", "password": "x"}, withHeader("Origin", "https://evil.example"))
	expect(t, r, http.StatusForbidden)
	expect(t, c.do("PATCH", "/api/v1/pawn/x/users/me", body), http.StatusOK)
}

// Native token-issuing endpoints are unreachable from browsers, and browser
// bearer tokens are ignored.
func TestNativeAuthBlocked(t *testing.T) {
	email, c := verifiedUser(t, "native")
	for _, p := range []string{"/api/v1/pawn/auth/content/users/login", "/api/v1/pawn/auth/login", "/api/v1/auth/refresh", "/api/v1/auth/superusers/login", "/api/v1/auth/users/register", "/api/v1/pawn/auth/content/users/register"} {
		r := c.do("POST", p, map[string]any{"identity": email, "password": testPassword})
		if r.Status != http.StatusNotFound || bytes.Contains(r.Body, []byte("refreshToken")) {
			t.Errorf("%s reachable: %d %s", p, r.Status, r.Body)
		}
	}
	for _, p := range []string{"/api/v1/auth/me", "/api/v1/auth/sessions"} {
		expect(t, c.do("GET", p, nil), http.StatusNotFound)
	}
	anon := newClient(t)
	r := anon.do("GET", "/api/v1/pawn/x/auth/me", nil, withHeader("Authorization", "Bearer forged.jwt.value"))
	expect(t, r, http.StatusUnauthorized)
	if r.errorCode(t) != "UNAUTHENTICATED" {
		t.Fatalf("bearer header must be stripped, got %s", r.Body)
	}
}

// COL-04: concurrent requests on an expired access token rotate once.
func TestConcurrentRefresh(t *testing.T) {
	_, c := verifiedUser(t, "refresh")
	var sid uint64
	var before uint64
	cookieHashSession(t, c, &sid, &before)
	if _, err := sqlDB.Exec("UPDATE "+tables.T("browser_sessions")+" SET access_expires_at = ? WHERE id = ?", time.Now().UTC().Add(-time.Minute), sid); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	statuses := make([]int, 10)
	for i := range statuses {
		wg.Add(1)
		go func() {
			defer wg.Done()
			statuses[i] = c.do("GET", "/api/v1/pawn/x/auth/me", nil).Status
		}()
	}
	wg.Wait()
	for i, s := range statuses {
		if s != http.StatusOK {
			t.Fatalf("request %d: %d", i, s)
		}
	}
	var after uint64
	var revoked *time.Time
	_ = sqlDB.QueryRow("SELECT refresh_version, revoked_at FROM "+tables.T("browser_sessions")+" WHERE id = ?", sid).Scan(&after, &revoked)
	if after != before+1 || revoked != nil {
		t.Fatalf("refresh_version %d -> %d, revoked=%v", before, after, revoked)
	}
	// The rotated tokens keep working, including against native Collar routes.
	expect(t, c.do("GET", "/api/v1/pawn/x/auth/me", nil), http.StatusOK)
	r := c.do("GET", "/api/v1/pawn/content/audit_logs", nil)
	if r.Status == http.StatusUnauthorized {
		t.Fatalf("native route after refresh: %d %s", r.Status, r.Body)
	}
}

func cookieHashSession(t *testing.T, c *client, sid, version *uint64) {
	t.Helper()
	// The newest session of the user behind this client.
	me := c.do("GET", "/api/v1/pawn/x/users/me/sessions", nil)
	expect(t, me, http.StatusOK)
	for _, s := range me.json(t)["data"].([]any) {
		m := s.(map[string]any)
		if m["current"] == true {
			*sid = atou(m["id"].(string))
		}
	}
	if *sid == 0 {
		t.Fatal("current session not listed")
	}
	_ = sqlDB.QueryRow("SELECT refresh_version FROM "+tables.T("browser_sessions")+" WHERE id = ?", *sid).Scan(version)
}

func TestLogoutAndLogoutAll(t *testing.T) {
	email, a := verifiedUser(t, "logout")
	b := login(t, email, testPassword)
	stale := a.cookie()
	expect(t, a.do("POST", "/api/v1/pawn/x/auth/logout", nil), http.StatusNoContent)
	if a.cookie() != "" {
		t.Fatal("logout must clear the cookie")
	}
	// Replaying the old cookie value does not work.
	replay := newClient(t)
	req, _ := http.NewRequest("GET", srv.URL+"/api/v1/pawn/x/auth/me", nil)
	req.AddCookie(&http.Cookie{Name: "pawn_session", Value: stale})
	res, err := replay.http.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("revoked cookie accepted: %d", res.StatusCode)
	}
	expect(t, b.do("GET", "/api/v1/pawn/x/auth/me", nil), http.StatusOK)
	c := login(t, email, testPassword)
	expect(t, c.do("POST", "/api/v1/pawn/x/auth/logout-all", nil), http.StatusNoContent)
	expect(t, b.do("GET", "/api/v1/pawn/x/auth/me", nil), http.StatusUnauthorized)
	expect(t, c.do("GET", "/api/v1/pawn/x/auth/me", nil), http.StatusUnauthorized)
}

// Suspension takes effect on the very next request.
func TestSuspensionRevokesImmediately(t *testing.T) {
	email, c := verifiedUser(t, "suspend")
	if _, err := sqlDB.Exec("UPDATE "+tables.T("users")+" SET state = 'suspended' WHERE email = ?", email); err != nil {
		t.Fatal(err)
	}
	expect(t, c.do("GET", "/api/v1/pawn/x/auth/me", nil), http.StatusUnauthorized)
	expect(t, newClient(t).do("POST", "/api/v1/pawn/x/auth/login", map[string]any{"email": email, "password": testPassword}), http.StatusUnauthorized)
	// Public routes keep working for the (now anonymous) browser.
	expect(t, c.do("GET", "/api/v1/pawn/x/public-config", nil), http.StatusOK)
}

func TestChangePassword(t *testing.T) {
	email, a := verifiedUser(t, "change")
	b := login(t, email, testPassword)
	oldCookie := a.cookie()
	r := a.do("POST", "/api/v1/pawn/x/auth/password/change", map[string]any{"old_password": "wrong old password", "new_password": "a brand new passphrase"})
	expect(t, r, http.StatusUnprocessableEntity)
	r = a.do("POST", "/api/v1/pawn/x/auth/password/change", map[string]any{"old_password": testPassword, "new_password": "a brand new passphrase"})
	expect(t, r, http.StatusOK)
	if a.cookie() == oldCookie {
		t.Fatal("password change must rotate the session cookie")
	}
	expect(t, a.do("GET", "/api/v1/pawn/x/auth/me", nil), http.StatusOK)
	expect(t, b.do("GET", "/api/v1/pawn/x/auth/me", nil), http.StatusUnauthorized)
	login(t, email, "a brand new passphrase")
}

// COL-03 and role-gated native reads.
func TestRolesOnNativeCollections(t *testing.T) {
	email, c := verifiedUser(t, "staff")
	r := c.do("GET", "/api/v1/pawn/content/admin_user_summaries", nil)
	if r.Status == http.StatusOK && bytes.Contains(r.Body, []byte("@example.invalid")) {
		t.Fatalf("reader can list users: %s", r.Body)
	}
	if _, err := sqlDB.Exec("UPDATE "+tables.T("users")+" SET role = 'owner' WHERE email = ?", email); err != nil {
		t.Fatal(err)
	}
	time.Sleep(6 * time.Second) // Collar's identity cache (COLLAR_AUTH_CACHE_TTL, 5s)
	r = c.do("GET", "/api/v1/pawn/content/admin_user_summaries?perPage=5", nil)
	expect(t, r, http.StatusOK)
	if bytes.Contains(r.Body, []byte("password_hash")) || bytes.Contains(r.Body, []byte("$argon2id$")) {
		t.Fatalf("user summaries leak hashes: %s", r.Body)
	}
	expect(t, c.do("GET", "/api/v1/pawn/content/audit_logs", nil), http.StatusOK)
	r = c.do("GET", "/api/v1/admin/jobs", nil)
	if r.Status != http.StatusForbidden {
		t.Fatalf("app owner reached Collar superuser API: %d %s", r.Status, r.Body)
	}
	r = c.do("GET", "/api/v1/pawn/collections", nil)
	if r.Status != http.StatusForbidden {
		t.Fatalf("app owner reached collection introspection: %d", r.Status)
	}
}

// UI-01 and API boundary behaviour.
func TestRoutingBoundaries(t *testing.T) {
	c := newClient(t)
	for path, want := range map[string]string{"/": "reader", "/stories": "reader", "/fiction/holunkai": "reader", "/studio/works/1/edit": "dashboard", "/admin": "dashboard"} {
		r := c.do("GET", path, nil)
		if r.Status != 200 || !strings.Contains(string(r.Body), want) {
			t.Errorf("%s: %d %s", path, r.Status, r.Body)
		}
	}
	for _, p := range []string{"/api/v1/pawn/x/nope", "/api/v1/pawn/x/works/missing/extra/deep", "/api/unknown", "/missing.css"} {
		r := c.do("GET", p, nil)
		if r.Status != http.StatusNotFound || strings.Contains(string(r.Body), "<!doctype") {
			t.Errorf("%s: %d %s", p, r.Status, r.Body)
		}
		if strings.HasPrefix(p, "/api") && !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			t.Errorf("%s: content type %s", p, r.Header.Get("Content-Type"))
		}
	}
	r := c.do("DELETE", "/api/v1/pawn/x/public-config", nil)
	expect(t, r, http.StatusMethodNotAllowed)
	r = c.do("GET", "/readyz", nil)
	expect(t, r, http.StatusOK)
	if got := r.json(t)["data"].(map[string]any)["checks"].(map[string]any)["schema"]; got != "ok" {
		t.Fatalf("schema check: %v", got)
	}
	r = c.do("GET", "/api/v1/pawn/x/public-config", nil)
	if r.Header.Get("Content-Security-Policy") == "" || r.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("security headers missing: %v", r.Header)
	}
}

// The send_email handler delivers once and scrubs the one-time link.
func TestMailJobScrubsLink(t *testing.T) {
	email := uniqueEmail("mail")
	register(t, email)
	var id uint64
	_ = sqlDB.QueryRow("SELECT id FROM "+tables.T("email_outbox")+" WHERE recipient = ? ORDER BY id DESC LIMIT 1", email).Scan(&id)
	sender := &fakeSender{}
	h := &mail.Handler{DB: sqlDB, Tables: tables, Sender: sender, From: "no-reply@example.invalid", AppName: "Pawn.Ink"}
	job := &collar.Job{Payload: []byte(itoa(id)), Attempts: 1, MaxAttempts: 8}
	for range 2 {
		if err := h.HandleJob(context.Background(), job); err != nil {
			t.Fatal(err)
		}
	}
	if len(sender.sent) != 1 || !strings.Contains(sender.sent[0].Body, "/verify-email?token=") {
		t.Fatalf("sent %d messages", len(sender.sent))
	}
	var state, data string
	_ = sqlDB.QueryRow("SELECT state, template_data_json FROM "+tables.T("email_outbox")+" WHERE id = ?", id).Scan(&state, &data)
	if state != "sent" || data != "{}" {
		t.Fatalf("after send: state=%s data=%s", state, data)
	}
}

type fakeSender struct{ sent []mail.Message }

func (f *fakeSender) Send(_ context.Context, _ string, m mail.Message) error {
	f.sent = append(f.sent, m)
	return nil
}

// The scheduler enqueues each UTC bucket exactly once.
func TestSchedulerBucketOnce(t *testing.T) {
	name := "test_" + randHex(3)
	s := &jobs.Scheduler{DB: sqlDB, Tables: tables, Jobs: api.Collar.Jobs(), Tick: time.Hour, Log: slog.Default(),
		Schedules: []jobs.Schedule{jobs.Daily(name, "noop_test", name)}}
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	for range 3 {
		wg.Add(1)
		go func() { defer wg.Done(); s.Run(ctx) }()
	}
	time.Sleep(500 * time.Millisecond)
	cancel()
	wg.Wait()
	var n int
	_ = sqlDB.QueryRow("SELECT COUNT(*) FROM "+tables.T("scheduler_runs")+" WHERE name = ?", name).Scan(&n)
	if n != 1 {
		t.Fatalf("scheduler_runs rows: %d", n)
	}
}

func itoa(n uint64) string { return strconv.FormatUint(n, 10) }

func atou(s string) uint64 {
	n, _ := strconv.ParseUint(s, 10, 64)
	return n
}
