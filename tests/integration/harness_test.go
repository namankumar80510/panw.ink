//go:build integration

// Package integration runs the real API (embedded Collar + session bridge +
// custom routes) against disposable MariaDB databases on the development
// Compose stack. Every run creates randomly named pi_test_* databases and
// drops them afterwards; it never touches pawn_ink or pawn_ink_collar.
//
//	make dev-db && make test-integration
package integration

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/pressly/goose/v3"
	"github.com/xmfunew/collar"

	"github.com/namankumar80510/panw.ink/internal/auth"
	"github.com/namankumar80510/panw.ink/internal/boot"
	"github.com/namankumar80510/panw.ink/internal/config"
	"github.com/namankumar80510/panw.ink/internal/db"
	"github.com/namankumar80510/panw.ink/internal/httpx"
	"github.com/namankumar80510/panw.ink/internal/seed"
	"github.com/namankumar80510/panw.ink/migrations"
)

const origin = "http://pawn.test"

var (
	// clockOffset shifts the API's release clock (tests of time-based access).
	clockOffset atomic.Int64
	api         *boot.API
	srv         *httptest.Server
	sqlDB       *sql.DB
	tables      db.Tables
)

func repoRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..")
}

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
	dsn := os.Getenv("PAWN_TEST_DSN")
	if dsn == "" {
		dsn = "pawn_test:pawn-test-dev@tcp(127.0.0.1:13306)/"
	}
	suffix := randHex(4)
	appDB, sysDB := "pi_test_"+suffix, "pi_test_"+suffix+"_c"

	mc, err := mysql.ParseDSN(dsn)
	if err != nil {
		fmt.Fprintln(os.Stderr, "PAWN_TEST_DSN:", err)
		return 1
	}
	mc.ParseTime, mc.Loc = true, time.UTC
	mc.Params = map[string]string{"time_zone": "'+00:00'"}
	root, err := sql.Open("mysql", mc.FormatDSN())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer root.Close()
	ctx := context.Background()
	if err := root.PingContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "integration tests need the dev MariaDB (make dev-db):", err)
		return 1
	}
	for _, name := range []string{appDB, sysDB} {
		if _, err := root.ExecContext(ctx, "CREATE DATABASE `"+name+"` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}
	defer func() {
		for _, name := range []string{appDB, sysDB} {
			_, _ = root.ExecContext(context.Background(), "DROP DATABASE IF EXISTS `"+name+"`") // only the pi_test_* databases created above
		}
	}()

	// Goose migrations on the application database.
	appCfg := *mc
	appCfg.DBName = appDB
	appConn, err := sql.Open("mysql", appCfg.FormatDSN())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer appConn.Close()
	p, err := goose.NewProvider(goose.DialectMySQL, appConn, migrations.FS)
	if err == nil {
		_, err = p.Up(ctx)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "goose:", err)
		return 1
	}

	env := map[string]string{
		"APP_ENV": "test", "APP_BASE_URL": origin, "HTTP_ADDR": "127.0.0.1:0", "COLLAR_ADDR": "",
		"COLLAR_DSN": dsn, "COLLAR_ADMIN_DSN": dsn, "APP_DATABASE": appDB, "COLLAR_SYSTEM_DATABASE": sysDB,
		"COLLAR_CONFIG": filepath.Join(repoRoot(), "collar.yaml"), "COLLAR_UI_CONFIG": filepath.Join(repoRoot(), "collar.ui.yaml"),
		"COLLAR_JWT_SECRET": randHex(32), "COLLAR_ACCESS_TTL": "15m", "COLLAR_REFRESH_TTL": "720h",
		"COLLAR_LOG_LEVEL": "error", "COLLAR_AUTO_MIGRATE": "false", "COLLAR_CORS_ORIGINS": origin,
		"SESSION_HMAC_KEY": randHex(32), "CSRF_HMAC_KEY": randHex(32), "TOKEN_ENCRYPTION_KEY": randHex(32),
		"RANKING_HMAC_KEY": randHex(32), "ANALYTICS_HMAC_KEY": randHex(32),
		"REDIS_URL": envOr("PAWN_TEST_REDIS_URL", "redis://127.0.0.1:16379/15"), "LOG_LEVEL": "error",
	}
	for k, v := range env {
		_ = os.Setenv(k, v)
	}
	if code := collar.Main([]string{"migrate"}, collar.Options{ConfigPath: env["COLLAR_CONFIG"]}); code != 0 {
		fmt.Fprintln(os.Stderr, "collar migrate failed:", code)
		return 1
	}

	cfg, err := config.Parse(os.LookupEnv)
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		return 1
	}
	limits := auth.DefaultLimits()
	limits.Register = httpx.NewLimiter(10000, time.Hour)
	limits.Reset = httpx.NewLimiter(10000, time.Hour)
	limits.Resend = httpx.NewLimiter(10000, time.Hour)
	limits.Verify = httpx.NewLimiter(10000, time.Hour)
	api, err = boot.NewAPI(ctx, cfg, boot.Options{
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		AuthLimit: &limits,
		Clock:     func() time.Time { return time.Now().Add(time.Duration(clockOffset.Load())) },
		WebFS: fstest.MapFS{
			"index.html":      {Data: []byte("<!doctype html><title>reader</title>")},
			"dashboard.html":  {Data: []byte("<!doctype html><title>dashboard</title>")},
			"web.0123abcd.js": {Data: []byte("console.log(1)")},
		},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "boot:", err)
		return 1
	}
	defer api.Close()
	sqlDB, tables = api.Collar.DB(), api.Tables
	if err := seed.Run(ctx, appConn, tables, time.Now(), ""); err != nil {
		fmt.Fprintln(os.Stderr, "seed:", err)
		return 1
	}
	srv = httptest.NewServer(api.Handler)
	defer srv.Close()
	return m.Run()
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

// client is a browser-like HTTP client: cookie jar, same-origin Origin
// header and the CSRF token captured from login/me responses.
type client struct {
	t    *testing.T
	http *http.Client
	mu   sync.Mutex
	csrf string
}

func newClient(t *testing.T) *client {
	jar, _ := cookiejar.New(nil)
	return &client{t: t, http: &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

type response struct {
	Status int
	Header http.Header
	Body   []byte
}

func (r response) json(t *testing.T) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(r.Body, &m); err != nil {
		t.Fatalf("decode %d %s: %v", r.Status, r.Body, err)
	}
	return m
}

func (r response) errorCode(t *testing.T) string {
	t.Helper()
	e, _ := r.json(t)["error"].(map[string]any)
	code, _ := e["code"].(string)
	return code
}

type reqOpt func(*http.Request)

func withHeader(k, v string) reqOpt { return func(r *http.Request) { r.Header.Set(k, v) } }
func noCSRF() reqOpt                { return func(r *http.Request) { r.Header.Del("X-CSRF-Token") } }

func (c *client) do(method, path string, body any, opts ...reqOpt) response {
	c.t.Helper()
	var rdr io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		rdr = strings.NewReader(string(raw))
	}
	req, _ := http.NewRequest(method, srv.URL+path, rdr)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Origin", origin)
	c.mu.Lock()
	if c.csrf != "" {
		req.Header.Set("X-CSRF-Token", c.csrf)
	}
	c.mu.Unlock()
	for _, o := range opts {
		o(req)
	}
	res, err := c.http.Do(req)
	if err != nil {
		c.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	r := response{Status: res.StatusCode, Header: res.Header, Body: b}
	if res.StatusCode < 300 && len(b) > 0 && strings.HasPrefix(res.Header.Get("Content-Type"), "application/json") {
		var env struct {
			Data struct {
				CSRF string `json:"csrf_token"`
			} `json:"data"`
		}
		if json.Unmarshal(b, &env) == nil && env.Data.CSRF != "" {
			c.mu.Lock()
			c.csrf = env.Data.CSRF
			c.mu.Unlock()
		}
	}
	return r
}

func (c *client) cookie() string {
	u, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	for _, ck := range c.http.Jar.Cookies(u.URL) {
		if ck.Name == "pawn_session" {
			return ck.Value
		}
	}
	return ""
}

func expect(t *testing.T, r response, status int) {
	t.Helper()
	if r.Status != status {
		t.Fatalf("status %d, want %d: %s", r.Status, status, r.Body)
	}
}

func uniqueEmail(prefix string) string {
	return prefix + "-" + randHex(4) + "@example.invalid"
}

const testPassword = "a sufficiently long passphrase"

// register creates an account and returns the verification token from the
// mail outbox (the worker is not running in tests).
func register(t *testing.T, email string) string {
	t.Helper()
	c := newClient(t)
	expect(t, c.do("POST", "/api/v1/pawn/x/auth/register", map[string]any{"email": email, "password": testPassword, "display_name": "Test Reader"}), http.StatusAccepted)
	return latestToken(t, email, "verify_email")
}

func latestToken(t *testing.T, email, template string) string {
	t.Helper()
	var raw string
	err := sqlDB.QueryRow("SELECT template_data_json FROM "+tables.T("email_outbox")+" WHERE recipient = ? AND template = ? ORDER BY id DESC LIMIT 1", email, template).Scan(&raw)
	if err != nil {
		t.Fatalf("no %s mail for %s: %v", template, email, err)
	}
	var data map[string]string
	_ = json.Unmarshal([]byte(raw), &data)
	_, tok, ok := strings.Cut(data["link"], "token=")
	if !ok {
		t.Fatalf("no token in link %q", data["link"])
	}
	return tok
}

func login(t *testing.T, email, password string) *client {
	t.Helper()
	c := newClient(t)
	expect(t, c.do("POST", "/api/v1/pawn/x/auth/login", map[string]any{"email": email, "password": password}), http.StatusOK)
	if c.cookie() == "" || c.csrf == "" {
		t.Fatal("login must set the session cookie and return a CSRF token")
	}
	return c
}

func verifiedUser(t *testing.T, prefix string) (string, *client) {
	t.Helper()
	email := uniqueEmail(prefix)
	tok := register(t, email)
	expect(t, newClient(t).do("POST", "/api/v1/pawn/x/auth/verify-email", map[string]any{"token": tok}), http.StatusOK)
	return email, login(t, email, testPassword)
}

func userID(t *testing.T, email string) uint64 {
	t.Helper()
	var id uint64
	if err := sqlDB.QueryRow("SELECT id FROM "+tables.T("users")+" WHERE email = ?", email).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}
