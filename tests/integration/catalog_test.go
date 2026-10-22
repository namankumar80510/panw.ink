//go:build integration

package integration

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

const x = "/api/v1/pawn/x"

func data(t *testing.T, r response) map[string]any {
	t.Helper()
	d, ok := r.json(t)["data"].(map[string]any)
	if !ok {
		t.Fatalf("no data object: %s", r.Body)
	}
	return d
}

func list(t *testing.T, r response) []any {
	t.Helper()
	d, ok := r.json(t)["data"].([]any)
	if !ok {
		t.Fatalf("no data array: %s", r.Body)
	}
	return d
}

func slugs(items []any) []string {
	out := []string{}
	for _, it := range items {
		out = append(out, it.(map[string]any)["slug"].(string))
	}
	return out
}

func shiftClock(t *testing.T, d time.Duration) {
	t.Helper()
	clockOffset.Store(int64(d))
	t.Cleanup(func() { clockOffset.Store(0) })
}

// CAT-01 / CAT-07: a short story is one complete text, readable anonymously.
func TestShortStoryIsOneText(t *testing.T) {
	c := newClient(t)
	r := c.do("GET", x+"/stories/the-locked-study/read", nil)
	expect(t, r, http.StatusOK)
	d := data(t, r)
	if d["work_kind"] != "short_story" || d["ordinal"] != float64(1) || d["toc_count"] != float64(1) || d["previous"] != nil || d["next"] != nil {
		t.Fatalf("short story shape: %v", d)
	}
	if body := d["body_html"].(string); len(body) < 500 {
		t.Fatalf("body too short: %d", len(body))
	}
	if cc := r.Header.Get("Cache-Control"); cc != "public, max-age=60" {
		t.Fatalf("public body cache-control %q", cc)
	}
	// The same entry through the generic path works, but a novel never
	// resolves through the story alias.
	expect(t, c.do("GET", x+"/works/the-locked-study/entries/full-story", nil), http.StatusOK)
	expect(t, c.do("GET", x+"/stories/holunkai/read", nil), http.StatusNotFound)
}

// CAT-02: the database itself refuses a second short-story entry, a
// chapter in the wrong format, novel-only volumes and novels in anthologies.
func TestDatabaseFormatInvariants(t *testing.T) {
	var storyID, novelID, owner uint64
	_ = sqlDB.QueryRow("SELECT id, owner_user_id FROM "+tables.T("works")+" WHERE slug = 'the-locked-study'").Scan(&storyID, &owner)
	_ = sqlDB.QueryRow("SELECT id FROM " + tables.T("works") + " WHERE slug = 'holunkai'").Scan(&novelID)
	if storyID == 0 || novelID == 0 || owner == 0 {
		t.Fatal("fixtures missing")
	}
	// Control: a valid novel chapter insert succeeds, so the failures below
	// come from the invariants, not from a broken statement.
	if _, err := sqlDB.Exec("INSERT INTO "+tables.T("entries")+" (work_id, work_kind, owner_user_id, ordinal, slug, title) VALUES (?, 'novel', ?, 99, 'control-chapter', 'Control')", novelID, owner); err != nil {
		t.Fatalf("control insert: %v", err)
	}
	t.Cleanup(func() {
		_, _ = sqlDB.Exec("DELETE FROM "+tables.T("entries")+" WHERE work_id = ? AND slug = 'control-chapter'", novelID)
	})
	bad := map[string]struct {
		q    string
		args []any
		want string // the constraint (or key) that must reject it
	}{
		"second story entry":      {"INSERT INTO " + tables.T("entries") + " (work_id, work_kind, owner_user_id, ordinal, slug, title) VALUES (?, 'short_story', ?, 2, 'part-two', 'Part two')", []any{storyID, owner}, "ck_entries_story_single"},
		"duplicate story entry":   {"INSERT INTO " + tables.T("entries") + " (work_id, work_kind, owner_user_id, ordinal, slug, title) VALUES (?, 'short_story', ?, 1, 'again', 'Again')", []any{storyID, owner}, "uq_entries_ordinal"},
		"story entry as novel":    {"INSERT INTO " + tables.T("entries") + " (work_id, work_kind, owner_user_id, ordinal, slug, title) VALUES (?, 'novel', ?, 2, 'sneaky', 'Sneaky')", []any{storyID, owner}, "fk_entries_work_kind"},
		"volume on a story":       {"INSERT INTO " + tables.T("volumes") + " (work_id, work_kind, owner_user_id, number, title) VALUES (?, 'short_story', ?, 1, 'V')", []any{storyID, owner}, "ck_volumes_novel"},
		"novel in anthology":      {"INSERT INTO " + tables.T("collection_items") + " (collection_id, work_id, work_kind, ordinal) SELECT id, ?, 'novel', 99 FROM " + tables.T("collections") + " LIMIT 1", []any{novelID}, "ck_collection_items_story"},
		"scheduled without date":  {"UPDATE " + tables.T("entries") + " SET public_at = NULL WHERE work_id = ? AND workflow = 'published'", []any{novelID}, "ck_entries_release"},
		"early lead over 30 days": {"UPDATE " + tables.T("entries") + " SET early_at = DATE_SUB(public_at, INTERVAL 31 DAY) WHERE work_id = ? AND slug = 'the-first-specimen'", []any{novelID}, "ck_entries_early"},
		"early after public":      {"UPDATE " + tables.T("entries") + " SET early_at = DATE_ADD(public_at, INTERVAL 1 HOUR) WHERE work_id = ? AND slug = 'the-first-specimen'", []any{novelID}, "ck_entries_early"},
		"foreign owner":           {"UPDATE " + tables.T("entries") + " SET owner_user_id = owner_user_id + 1000 WHERE work_id = ? AND slug = 'control-chapter'", []any{novelID}, "fk_entries_work_owner"},
	}
	for name, b := range bad {
		res, err := sqlDB.Exec(b.q, b.args...)
		if err == nil {
			n, _ := res.RowsAffected()
			t.Errorf("%s: accepted by the database (%d rows)", name, n)
		} else if !strings.Contains(err.Error(), b.want) {
			t.Errorf("%s: want %s, got %v", name, b.want, err)
		}
	}
}

// CAT-03: stable, ordered contents without drafts or archived entries.
func TestNovelTOC(t *testing.T) {
	r := newClient(t).do("GET", x+"/works/holunkai/toc", nil)
	expect(t, r, http.StatusOK)
	entries := data(t, r)["entries"].([]any)
	got := []string{}
	for _, e := range entries {
		m := e.(map[string]any)
		got = append(got, m["slug"].(string)+":"+m["access"].(string))
	}
	want := "the-last-breath:public,the-wrong-savior:public,the-first-specimen:early"
	if strings.Join(got, ",") != want {
		t.Fatalf("toc = %v", got)
	}
	if strings.Contains(string(r.Body), "body_html") || strings.Contains(string(r.Body), "An Unfinished Thing") || strings.Contains(string(r.Body), "A Cut Scene") {
		t.Fatalf("toc leaks bodies or unreleased entries: %s", r.Body)
	}
}

// CAT-05: hidden, draft, archived and unpublished content is a plain 404
// with no body bytes or metadata.
func TestHiddenContentIs404(t *testing.T) {
	c := newClient(t)
	_, reader := verifiedUser(t, "hidden")
	for _, p := range []string{
		"/works/holunkai/entries/an-unfinished-thing", "/works/holunkai/entries/a-cut-scene",
		"/works/the-quiet-manuscript", "/works/the-quiet-manuscript/toc", "/stories/the-quiet-manuscript/read",
		"/works/a-withdrawn-tale", "/stories/a-withdrawn-tale/read", "/works/the-quiet-manuscript/entries/full-story",
	} {
		for _, cl := range []*client{c, reader} {
			r := cl.do("GET", x+p, nil)
			if r.Status != http.StatusNotFound || r.errorCode(t) != "NOT_FOUND" {
				t.Errorf("%s: %d %s", p, r.Status, r.Body)
			}
			for _, leak := range []string{"Unfinished", "Cut Scene", "Quiet Manuscript", "Withdrawn", "body_html", "public_at"} {
				if strings.Contains(string(r.Body), leak) {
					t.Errorf("%s leaks %q", p, leak)
				}
			}
		}
	}
	r := c.do("GET", x+"/works?limit=50", nil)
	for _, s := range slugs(list(t, r)) {
		if s == "the-quiet-manuscript" || s == "a-withdrawn-tale" || s == "beyond-the-door" {
			t.Errorf("listing includes %s", s)
		}
	}
	// Content tables are not exposed through Collar's generic API at all.
	for _, tbl := range []string{"entry_revisions", "entries", "works"} {
		if r := reader.do("GET", "/api/v1/pawn/content/"+tbl, nil); r.Status != http.StatusNotFound {
			t.Errorf("native %s: %d", tbl, r.Status)
		}
	}
}

// CAT-06: HTML and javascript: links in Markdown never reach readers.
func TestMarkdownSanitized(t *testing.T) {
	r := newClient(t).do("GET", x+"/works/holunkai/entries/the-wrong-savior", nil)
	expect(t, r, http.StatusOK)
	body := strings.ToLower(data(t, r)["body_html"].(string))
	for _, bad := range []string{"<script", "javascript:", "alert("} {
		if strings.Contains(body, bad) {
			t.Fatalf("body contains %q", bad)
		}
	}
}

// PUB-01/02 (no entitlement yet) and PUB-04: early bodies are locked for
// everyone without entitlement; at public_at they open with no job running.
func TestEarlyAccessAndPublicRelease(t *testing.T) {
	_, reader := verifiedUser(t, "early")
	for _, c := range []*client{newClient(t), reader} {
		r := c.do("GET", x+"/works/holunkai/entries/the-first-specimen", nil)
		if r.Status != http.StatusForbidden || r.errorCode(t) != "EARLY_ACCESS_REQUIRED" {
			t.Fatalf("early entry: %d %s", r.Status, r.Body)
		}
		if strings.Contains(string(r.Body), "body_html") || strings.Contains(string(r.Body), "<p>") {
			t.Fatalf("locked response leaks body: %s", r.Body)
		}
		if cc := r.Header.Get("Cache-Control"); cc != "private, no-store" {
			t.Fatalf("locked cache-control %q", cc)
		}
		details := r.json(t)["error"].(map[string]any)["details"].(map[string]any)
		if details["public_at"] == nil || details["entry"].(map[string]any)["title"] != "The First Specimen" {
			t.Fatalf("lock details: %v", details)
		}
	}
	r := newClient(t).do("GET", x+"/stories/beyond-the-door/read", nil)
	if r.Status != http.StatusForbidden || r.errorCode(t) != "EARLY_ACCESS_REQUIRED" {
		t.Fatalf("early story: %d %s", r.Status, r.Body)
	}

	shiftClock(t, 7*24*time.Hour)
	for _, p := range []string{"/works/holunkai/entries/the-first-specimen", "/stories/beyond-the-door/read"} {
		r := newClient(t).do("GET", x+p, nil)
		expect(t, r, http.StatusOK)
		if data(t, r)["access"].(map[string]any)["state"] != "public" {
			t.Fatalf("%s not public after release time: %s", p, r.Body)
		}
	}
	// The anthology now counts the newly public story, from timestamps alone.
	cols := list(t, newClient(t).do("GET", x+"/collections", nil))
	if len(cols) != 1 || cols[0].(map[string]any)["story_count"] != float64(2) {
		t.Fatalf("collections after release: %v", cols)
	}
}

func TestUpcomingEntryLocked(t *testing.T) {
	shiftClock(t, -3*time.Hour) // before the early window of chapter 3 opens
	r := newClient(t).do("GET", x+"/works/holunkai/entries/the-first-specimen", nil)
	if r.Status != http.StatusForbidden || r.errorCode(t) != "ENTRY_UPCOMING" {
		t.Fatalf("upcoming: %d %s", r.Status, r.Body)
	}
}

func TestCatalogueFiltersAndCursor(t *testing.T) {
	c := newClient(t)
	r := c.do("GET", x+"/works", nil)
	expect(t, r, http.StatusOK)
	if got := strings.Join(slugs(list(t, r)), ","); got != "holunkai,the-locked-study" {
		t.Fatalf("listing = %s", got)
	}
	if got := slugs(list(t, c.do("GET", x+"/works?kind=short_story", nil))); len(got) != 1 || got[0] != "the-locked-study" {
		t.Fatalf("stories = %v", got)
	}
	if got := slugs(list(t, c.do("GET", x+"/works?tags=locked-room,unreliable-narrator", nil))); len(got) != 1 || got[0] != "the-locked-study" {
		t.Fatalf("tag intersection = %v", got)
	}
	if got := slugs(list(t, c.do("GET", x+"/works?tags=locked-room,gothic", nil))); len(got) != 0 {
		t.Fatalf("disjoint tags = %v", got)
	}
	if got := slugs(list(t, c.do("GET", x+"/works?max_minutes=5", nil))); strings.Join(got, ",") != "the-locked-study" {
		t.Fatalf("max_minutes = %v", got)
	}
	expect(t, c.do("GET", x+"/works?sort=popular", nil), http.StatusUnprocessableEntity)

	first := c.do("GET", x+"/works?limit=1&sort=title", nil)
	p1 := slugs(list(t, first))
	next, _ := first.json(t)["meta"].(map[string]any)["next_cursor"].(string)
	if len(p1) != 1 || next == "" {
		t.Fatalf("page 1: %v cursor=%q", p1, next)
	}
	second := c.do("GET", x+"/works?limit=1&sort=title&cursor="+url.QueryEscape(next), nil)
	p2 := slugs(list(t, second))
	if len(p2) != 1 || p2[0] == p1[0] {
		t.Fatalf("page 2: %v after %v", p2, p1)
	}
	r = c.do("GET", x+"/works?limit=1&sort=latest&cursor="+url.QueryEscape(next), nil)
	if r.Status != http.StatusBadRequest || r.errorCode(t) != "INVALID_CURSOR" {
		t.Fatalf("cursor reused with other sort: %d", r.Status)
	}
	r = c.do("GET", x+"/works?limit=1&sort=title&cursor="+url.QueryEscape(next[:len(next)-3]+"xyz"), nil)
	expect(t, r, http.StatusBadRequest)

	tags := slugs(list(t, c.do("GET", x+"/tags", nil)))
	for _, s := range tags {
		if s == "proposed-tag" {
			t.Fatal("unapproved tag listed")
		}
	}
}

func TestWorkDetailAndRedirect(t *testing.T) {
	c := newClient(t)
	d := data(t, c.do("GET", x+"/works/holunkai", nil))
	if d["start"].(map[string]any)["slug"] != "the-last-breath" || d["next_release"].(map[string]any)["slug"] != "the-first-specimen" {
		t.Fatalf("start/next: %v %v", d["start"], d["next_release"])
	}
	if d["rating"].(map[string]any)["state"] != "insufficient_data" {
		t.Fatalf("rating must not be invented: %v", d["rating"])
	}
	if tags := d["tags"].([]any); len(tags) != 3 {
		t.Fatalf("approved tags only: %v", tags)
	}
	r := c.do("GET", x+"/works/holunkai-old", nil)
	if r.Status != http.StatusNotFound || r.errorCode(t) != "MOVED" {
		t.Fatalf("redirect: %d %s", r.Status, r.Body)
	}
	if loc := r.json(t)["error"].(map[string]any)["details"].(map[string]any)["location"]; loc != "/fiction/holunkai" {
		t.Fatalf("location %v", loc)
	}
	a := data(t, c.do("GET", x+"/authors/a-n-other", nil))
	if got := slugs(a["works"].([]any)); strings.Join(got, ",") != "the-locked-study" {
		t.Fatalf("author works %v", got)
	}
	expect(t, c.do("GET", x+"/authors/nobody-here", nil), http.StatusNotFound)
	h := data(t, c.do("GET", x+"/home", nil))
	if len(h["short_reads"].([]any)) != 1 || len(h["featured"].([]any)) != 1 {
		t.Fatalf("home rails: %v", h)
	}
}

func TestSearchIsSafe(t *testing.T) {
	c := newClient(t)
	r := c.do("GET", x+"/search?q=locked", nil)
	expect(t, r, http.StatusOK)
	if got := slugs(data(t, r)["works"].([]any)); len(got) != 1 || got[0] != "the-locked-study" {
		t.Fatalf("search: %v", got)
	}
	for _, q := range []string{`+(-"'*%_\`, `' OR 1=1 --`, "a", strings.Repeat("é", 100), "Vale"} {
		r := c.do("GET", x+"/search?q="+url.QueryEscape(q), nil)
		expect(t, r, http.StatusOK)
	}
	if got := data(t, c.do("GET", x+"/search?q=Vale&type=authors", nil))["authors"].([]any); len(got) != 1 {
		t.Fatalf("author search: %v", got)
	}
	expect(t, c.do("GET", x+"/search?q="+strings.Repeat("a", 101), nil), http.StatusUnprocessableEntity)
	expect(t, c.do("GET", x+"/search?q=quiet", nil), http.StatusOK)
	if strings.Contains(string(c.do("GET", x+"/search?q=quiet", nil).Body), "the-quiet-manuscript") {
		t.Fatal("search finds a draft work")
	}
}
