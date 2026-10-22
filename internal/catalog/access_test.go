package catalog

import (
	"testing"
	"time"
)

func TestStateAt(t *testing.T) {
	now := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	at := func(d time.Duration) *time.Time { x := now.Add(d); return &x }
	cases := []struct {
		name string
		r    Release
		want Access
	}{
		{"draft", Release{Workflow: "draft", PublicAt: at(-time.Hour), HasPublished: true}, AccessUnavailable},
		{"archived", Release{Workflow: "archived", PublicAt: at(-time.Hour), HasPublished: true}, AccessUnavailable},
		{"no revision", Release{Workflow: "published", PublicAt: at(-time.Hour)}, AccessUnavailable},
		{"no public date", Release{Workflow: "scheduled", HasPublished: true}, AccessUnavailable},
		{"public", Release{Workflow: "published", PublicAt: at(-time.Second), HasPublished: true}, AccessPublic},
		{"public exactly now", Release{Workflow: "scheduled", PublicAt: at(0), HasPublished: true}, AccessPublic},
		{"public after early window, scheduler never ran", Release{Workflow: "scheduled", EarlyAt: at(-48 * time.Hour), PublicAt: at(-time.Minute), HasPublished: true}, AccessPublic},
		{"early", Release{Workflow: "scheduled", EarlyAt: at(-time.Hour), PublicAt: at(time.Hour), HasPublished: true}, AccessEarly},
		{"upcoming before early", Release{Workflow: "scheduled", EarlyAt: at(time.Hour), PublicAt: at(2 * time.Hour), HasPublished: true}, AccessUpcoming},
		{"upcoming without early", Release{Workflow: "scheduled", PublicAt: at(time.Hour), HasPublished: true}, AccessUpcoming},
	}
	for _, c := range cases {
		if got := StateAt(c.r, now); got != c.want {
			t.Errorf("%s: %s, want %s", c.name, got, c.want)
		}
	}
}

func TestCursor(t *testing.T) {
	c := cursorCodec{key: []byte("k")}
	f := filterHash("novel", "fantasy")
	s := c.encode(cursorPayload{Sort: "latest", Filter: f, Key: "2026-01-01T00:00:00Z", ID: 9})
	p, err := c.decode(s, "latest", f)
	if err != nil || p.ID != 9 {
		t.Fatalf("round trip: %v %v", p, err)
	}
	if _, err := c.decode(s, "title", f); err == nil {
		t.Fatal("cursor reused with another sort")
	}
	if _, err := c.decode(s, "latest", filterHash("short_story")); err == nil {
		t.Fatal("cursor reused with other filters")
	}
	if _, err := (cursorCodec{key: []byte("other")}).decode(s, "latest", f); err == nil {
		t.Fatal("forged signature accepted")
	}
	if _, err := c.decode(s[:len(s)-2]+"AA", "latest", f); err == nil {
		t.Fatal("tampered cursor accepted")
	}
}
