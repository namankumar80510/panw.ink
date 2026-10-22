package httpx

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"
)

func TestLimiter(t *testing.T) {
	l := NewLimiter(2, time.Minute)
	now := time.Unix(0, 0)
	l.now = func() time.Time { return now }
	if !l.Allow("a") || !l.Allow("a") || l.Allow("a") {
		t.Fatal("third event in window must be refused")
	}
	if !l.Allow("b") {
		t.Fatal("keys are independent")
	}
	now = now.Add(time.Minute)
	if !l.Allow("a") {
		t.Fatal("new window must allow")
	}
}

func TestClientIP(t *testing.T) {
	trusted := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}
	cases := []struct{ remote, xff, want string }{
		{"203.0.113.9:5555", "1.2.3.4", "203.0.113.9"},    // untrusted peer: XFF ignored
		{"10.0.0.2:5555", "1.2.3.4", "1.2.3.4"},           // trusted proxy
		{"10.0.0.2:5555", "6.6.6.6, 1.2.3.4", "1.2.3.4"},  // right-most untrusted wins
		{"10.0.0.2:5555", "1.2.3.4, 10.0.0.9", "1.2.3.4"}, // skip trusted hops
		{"10.0.0.2:5555", "garbage", "10.0.0.2"},          // malformed: fall back to peer
	}
	for _, c := range cases {
		if got := ClientIP(c.remote, c.xff, trusted); got != c.want {
			t.Errorf("ClientIP(%q,%q)=%q want %q", c.remote, c.xff, got, c.want)
		}
	}
	if IPPrefix("203.0.113.9") != "203.0.113.0/24" {
		t.Fatal(IPPrefix("203.0.113.9"))
	}
}

func TestDecodeJSONStrict(t *testing.T) {
	var dst struct {
		Name string `json:"name"`
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPatch, "/", strings.NewReader(`{"name":"a","role":"owner"}`))
	err := DecodeJSON(w, r, &dst, 1024)
	ae, ok := err.(*Error)
	if !ok || ae.Status != http.StatusUnprocessableEntity || !strings.Contains(ae.Error(), "VALIDATION") {
		t.Fatalf("unknown field: %v", err)
	}
	r = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(strings.Repeat(" ", 2000)+"{}"))
	if err := DecodeJSON(w, r, &dst, 1024); err == nil {
		t.Fatal("oversized body must fail")
	}
}
