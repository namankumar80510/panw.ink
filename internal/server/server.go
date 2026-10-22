// Package server is Pawn.Ink's HTTP front: one handler that owns /, /studio,
// /admin and the health routes, and wraps Collar's complete handler for
// every /api/* path behind the browser-session ingress (IMPLEMENTATION.md
// §1.1 items 4–7, §4, §13.2).
package server

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"strings"

	"github.com/namankumar80510/panw.ink/internal/auth"
	"github.com/namankumar80510/panw.ink/internal/config"
	"github.com/namankumar80510/panw.ink/internal/httpx"
)

// Health reports readiness: ok=false makes /readyz return 503. Degraded
// dependencies (Redis, worker lag) are reported in checks without failing
// readiness, because they must not stop public reading.
type Health func(ctx context.Context) (ok bool, checks map[string]string)

// Deps are the server's collaborators.
type Deps struct {
	Config *config.Config
	Collar http.Handler
	Bridge *auth.Bridge
	SPA    http.Handler
	Custom *Routes
	Health Health
	Log    *slog.Logger
}

// Server is the root handler.
type Server struct {
	d   Deps
	csp string
}

// New returns the root handler.
func New(d Deps) *Server {
	return &Server{d: d, csp: buildCSP(d.Config)}
}

// ClientIP is the resolved client address of a request that passed through
// the Server (its RemoteAddr was rewritten on ingress).
func ClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	cfg := s.d.Config
	ip := httpx.ClientIP(r.RemoteAddr, r.Header.Get("X-Forwarded-For"), cfg.TrustedProxies)
	r.RemoteAddr = net.JoinHostPort(ip, "0")
	r.Header.Del("X-Forwarded-For") // Collar never trusts it; nobody downstream should either

	s.securityHeaders(w)
	if cfg.IsProduction() && !strings.EqualFold(hostOnly(r.Host), cfg.BaseURL.Hostname()) {
		http.Error(w, "Unknown host.", http.StatusMisdirectedRequest)
		return
	}

	switch p := r.URL.Path; {
	case p == "/healthz":
		s.healthz(w, r)
	case p == "/readyz":
		s.readyz(w, r)
	case p == "/api" || strings.HasPrefix(p, "/api/"):
		s.api(w, r)
	default:
		s.d.SPA.ServeHTTP(w, r)
	}
}

func hostOnly(hostport string) string {
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		return h
	}
	return hostport
}

func (s *Server) securityHeaders(w http.ResponseWriter) {
	h := w.Header()
	if s.d.Config.CSPReportOnly {
		h.Set("Content-Security-Policy-Report-Only", s.csp)
	} else {
		h.Set("Content-Security-Policy", s.csp)
	}
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
	h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=(), interest-cohort=()")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Cross-Origin-Opener-Policy", "same-origin")
	if s.d.Config.SecureCookies {
		h.Set("Strict-Transport-Security", "max-age=31536000")
	}
}

// buildCSP allows only same-origin scripts, styles and fonts (Parcel bundles
// Bootstrap and Bootstrap Icons locally; no CDN), plus configured asset
// origins for cover images.
func buildCSP(cfg *config.Config) string {
	img, connect := "'self' data:", "'self'"
	for _, o := range cfg.AssetOrigins {
		img += " " + o
		connect += " " + o // browser uploads PUT straight to presigned object-store URLs
	}
	return strings.Join([]string{
		"default-src 'self'",
		"script-src 'self'",
		"style-src 'self'",
		"font-src 'self'",
		"img-src " + img,
		"connect-src " + connect,
		"object-src 'none'",
		"base-uri 'self'",
		"form-action 'self'",
		"frame-ancestors 'none'",
		"frame-src 'none'",
	}, "; ")
}

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	httpx.WriteData(w, r, http.StatusOK, map[string]string{"status": "ok"}, nil)
}

func (s *Server) readyz(w http.ResponseWriter, r *http.Request) {
	ok, checks := s.d.Health(r.Context())
	status, state := http.StatusOK, "ready"
	if !ok {
		status, state = http.StatusServiceUnavailable, "not_ready"
	}
	httpx.WriteData(w, r, status, map[string]any{"status": state, "checks": checks}, nil)
}

// blockedNative reports native Collar routes browsers must never reach:
// every token-issuing or token-managing auth route. The bridge reaches them
// by internal dispatch only (§7.3).
func blockedNative(p string) bool {
	if strings.HasPrefix(p, "/api/v1/auth/") || p == "/api/v1/auth" {
		return true
	}
	// /api/v1/{app}/auth/... (login, register, set-password of any app)
	if rest, ok := strings.CutPrefix(p, "/api/v1/"); ok {
		if _, tail, ok := strings.Cut(rest, "/"); ok && (tail == "auth" || strings.HasPrefix(tail, "auth/")) {
			return true
		}
	}
	return false
}

// csrfExempt are anonymous-capable custom routes. They still require a
// same-origin Origin header; they do not require the session CSRF token
// because the browser may hold a stale cookie whose token it lost.
var csrfExempt = map[string]bool{
	auth.Base + "/auth/login":               true,
	auth.Base + "/auth/register":            true,
	auth.Base + "/auth/verify-email":        true,
	auth.Base + "/auth/resend-verification": true,
	auth.Base + "/auth/password/forgot":     true,
	auth.Base + "/auth/password/reset":      true,
	auth.Base + "/auth/logout":              true,
}

func isSafeMethod(m string) bool {
	return m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions
}

func (s *Server) api(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	// Browsers never present bearer tokens: the opaque cookie is the only
	// credential accepted at ingress.
	r.Header.Del("Authorization")
	if q := r.URL.Query(); q.Has("token") {
		q.Del("token")
		r.URL.RawQuery = q.Encode()
	}
	if blockedNative(r.URL.Path) {
		httpx.WriteError(w, r, httpx.ErrNotFound)
		return
	}
	if isCustomPath(r.URL.Path) && s.d.Custom != nil {
		if ok, allow := s.d.Custom.match(r); !ok {
			if len(allow) > 0 {
				w.Header().Set("Allow", strings.Join(allow, ", "))
				httpx.WriteError(w, r, httpx.NewError(http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed."))
				return
			}
			httpx.WriteError(w, r, httpx.ErrNotFound)
			return
		}
	}

	unsafe := !isSafeMethod(r.Method)
	if unsafe && !s.sameOrigin(r) {
		httpx.WriteError(w, r, httpx.ErrCSRF)
		return
	}

	res := s.d.Bridge.Resolve(r.Context(), r, ClientIP(r))
	if res.ClearCookie {
		s.d.Bridge.ClearCookie(w)
	}
	ctx := r.Context()
	if v := res.Viewer; v != nil {
		if unsafe && !csrfExempt[r.URL.Path] && !s.d.Bridge.CheckCSRF(s.d.Bridge.CookieValue(r), r.Header.Get("X-CSRF-Token")) {
			httpx.WriteError(w, r, httpx.ErrCSRF)
			return
		}
		r.Header.Set("Authorization", "Bearer "+res.AccessToken)
		ctx = auth.WithAccessToken(auth.WithViewer(ctx, v), res.AccessToken)
	}
	s.d.Collar.ServeHTTP(w, r.WithContext(ctx))
}

// sameOrigin checks Origin (or, when absent, Referer) against the base
// origin. Development also accepts the loopback alias of the base origin.
func (s *Server) sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		ref := r.Header.Get("Referer")
		if ref == "" {
			return false
		}
		origin = originOf(ref)
	}
	if origin == s.d.Config.BaseOrigin {
		return true
	}
	if s.d.Config.IsDevelopment() || s.d.Config.Env == config.EnvTest {
		u := s.d.Config.BaseURL
		alt := strings.NewReplacer("://localhost", "://127.0.0.1", "://127.0.0.1", "://localhost").Replace(u.Scheme + "://" + u.Host)
		return origin == alt
	}
	return false
}

func originOf(ref string) string {
	scheme, rest, ok := strings.Cut(ref, "://")
	if !ok {
		return ""
	}
	host, _, _ := strings.Cut(rest, "/")
	return scheme + "://" + host
}
