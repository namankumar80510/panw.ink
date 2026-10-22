package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestSPARouting(t *testing.T) {
	spa := NewSPA(fstest.MapFS{
		"index.html":      {Data: []byte("READER")},
		"dashboard.html":  {Data: []byte("DASHBOARD")},
		"web.0123abcd.js": {Data: []byte("js")},
		"favicon.svg":     {Data: []byte("<svg/>")},
	})
	cases := []struct {
		path, wantBody string
		wantCode       int
		wantCache      string
	}{
		{"/", "READER", 200, "no-cache"},
		{"/stories", "READER", 200, "no-cache"},
		{"/fiction/holunkai/ch/the-last-breath", "READER", 200, "no-cache"},
		{"/studio", "DASHBOARD", 200, "no-cache"},
		{"/studio/works/1/edit", "DASHBOARD", 200, "no-cache"},
		{"/admin/users", "DASHBOARD", 200, "no-cache"},
		{"/studios", "READER", 200, "no-cache"},
		{"/web.0123abcd.js", "js", 200, "public, max-age=31536000, immutable"},
		{"/favicon.svg", "<svg/>", 200, "public, max-age=3600"},
		{"/missing.js", "Not found.", 404, "no-store"},
		{"/fiction/x/cover.png", "Not found.", 404, "no-store"},
		{"/../../etc/passwd", "READER", 200, "no-cache"},
	}
	for _, c := range cases {
		w := httptest.NewRecorder()
		spa.ServeHTTP(w, httptest.NewRequest(http.MethodGet, c.path, nil))
		if w.Code != c.wantCode || !strings.Contains(w.Body.String(), c.wantBody) || w.Header().Get("Cache-Control") != c.wantCache {
			t.Errorf("%s: %d %q cache=%q", c.path, w.Code, w.Body.String(), w.Header().Get("Cache-Control"))
		}
	}
	w := httptest.NewRecorder()
	spa.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/", nil))
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST: %d", w.Code)
	}
}

func TestBlockedNative(t *testing.T) {
	for _, p := range []string{"/api/v1/auth/refresh", "/api/v1/auth/me", "/api/v1/auth/superusers/login",
		"/api/v1/pawn/auth/content/users/login", "/api/v1/pawn/auth/login", "/api/v1/other/auth/x/y/register"} {
		if !blockedNative(p) {
			t.Errorf("%s should be blocked", p)
		}
	}
	for _, p := range []string{"/api/v1/pawn/x/auth/login", "/api/v1/pawn/content/works", "/api/v1/health"} {
		if blockedNative(p) {
			t.Errorf("%s should not be blocked", p)
		}
	}
}
