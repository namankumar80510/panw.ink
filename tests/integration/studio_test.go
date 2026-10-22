//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/namankumar80510/panw.ink/internal/catalog"
)

const st = "/api/v1/pawn/x/studio"

// author registers, verifies and promotes an account to author before its
// first login (so Collar's identity cache sees the role immediately).
func author(t *testing.T, prefix string) (*client, uint64) {
	t.Helper()
	email := uniqueEmail(prefix)
	tok := register(t, email)
	expect(t, newClient(t).do("POST", "/api/v1/pawn/x/auth/verify-email", map[string]any{"token": tok}), http.StatusOK)
	if _, err := sqlDB.Exec("UPDATE "+tables.T("users")+" SET role = 'author' WHERE email = ?", email); err != nil {
		t.Fatal(err)
	}
	return login(t, email, testPassword), userID(t, email)
}

func str(m map[string]any, k string) string {
	s, _ := m[k].(string)
	return s
}

type novel struct {
	c                *client
	pen, work, slug  string
	entries, entrySl []string
}

// newNovel creates a pen name, a listed novel and n chapters with text.
func newNovel(t *testing.T, prefix string, n int) *novel {
	t.Helper()
	c, _ := author(t, prefix)
	nv := &novel{c: c, slug: prefix + "-" + randHex(3)}
	pen := c.do("POST", st+"/pen-names", map[string]any{"slug": "pn-" + nv.slug, "name": "Pen " + prefix})
	expect(t, pen, http.StatusCreated)
	nv.pen = str(data(t, pen), "id")
	w := c.do("POST", st+"/works", map[string]any{"pen_name_id": nv.pen, "kind": "novel", "title": "Novel " + prefix, "slug": nv.slug, "genre": "fantasy", "synopsis": "A test novel."})
	expect(t, w, http.StatusCreated)
	nv.work = str(data(t, w), "id")
	for i := 1; i <= n; i++ {
		e := c.do("POST", st+"/works/"+nv.work+"/entries", map[string]any{"title": fmt.Sprintf("Chapter %d", i)})
		expect(t, e, http.StatusCreated)
		id := str(data(t, e), "id")
		nv.entries = append(nv.entries, id)
		nv.entrySl = append(nv.entrySl, str(data(t, e), "slug"))
		saveDraft(t, c, id, 1, fmt.Sprintf("Text of chapter %d. It has words.", i))
	}
	r := c.do("PATCH", "/api/v1/pawn/content/works/"+nv.work, map[string]any{"visibility": "listed"})
	expect(t, r, http.StatusOK)
	return nv
}

func saveDraft(t *testing.T, c *client, entryID string, expected int, md string) response {
	t.Helper()
	r := c.do("PUT", st+"/entries/"+entryID+"/draft", map[string]any{"markdown": md, "expected_revision": expected})
	if r.Status != http.StatusOK && r.Status != http.StatusConflict {
		t.Fatalf("save draft: %d %s", r.Status, r.Body)
	}
	return r
}

func ts(d time.Duration) string { return time.Now().Add(d).UTC().Format(time.RFC3339) }

// Phase 3 exit: a tomorrow-public chapter and a supporter-early chapter are
// unreadable before their times and readable after, with no worker running;
// draft edits stay private until explicitly published.
func TestStudioScheduleLifecycle(t *testing.T) {
	nv := newNovel(t, "sched", 2)
	c := nv.c
	r := c.do("POST", st+"/entries/"+nv.entries[0]+"/schedule", map[string]any{"public_at": ts(24 * time.Hour)})
	expect(t, r, http.StatusOK)
	if a := str(data(t, r), "access"); a != "upcoming" {
		t.Fatalf("access after schedule: %s", a)
	}
	r = c.do("POST", st+"/entries/"+nv.entries[1]+"/schedule", map[string]any{"public_at": ts(72 * time.Hour), "early_at": ts(-time.Minute), "min_access_rank": 1})
	expect(t, r, http.StatusOK)

	anon := newClient(t)
	ch := func(i int) string { return x + "/works/" + nv.slug + "/entries/" + nv.entrySl[i] }
	if r := anon.do("GET", ch(0), nil); r.Status != http.StatusForbidden || r.errorCode(t) != "ENTRY_UPCOMING" {
		t.Fatalf("ch1 before release: %d %s", r.Status, r.Body)
	}
	if r := anon.do("GET", ch(1), nil); r.Status != http.StatusForbidden || r.errorCode(t) != "EARLY_ACCESS_REQUIRED" {
		t.Fatalf("ch2 early: %d %s", r.Status, r.Body)
	}
	// The author's own new draft text is private until published.
	saveDraft(t, c, nv.entries[0], 2, "SECRET-DRAFT-EDIT and more words.")

	shiftClock(t, 25*time.Hour)
	r = anon.do("GET", ch(0), nil)
	expect(t, r, http.StatusOK)
	if body := str(data(t, r), "body_html"); !strings.Contains(body, "Text of chapter 1") || strings.Contains(body, "SECRET-DRAFT") {
		t.Fatalf("published text wrong: %s", body)
	}
	shiftClock(t, 73*time.Hour)
	expect(t, anon.do("GET", ch(1), nil), http.StatusOK)

	// Publishing the correction swaps the text without changing dates. First
	// run the rollup refresh the worker performs at each public_at.
	if err := catalog.RefreshRollups(context.Background(), sqlDB, tables, atou(nv.work), time.Now().Add(73*time.Hour)); err != nil {
		t.Fatal(err)
	}
	before := data(t, anon.do("GET", x+"/works/"+nv.slug, nil))["last_public_update_at"]
	if before == nil {
		t.Fatal("rollups not refreshed")
	}
	expect(t, c.do("POST", st+"/entries/"+nv.entries[0]+"/publish-revision", nil), http.StatusOK)
	r = anon.do("GET", ch(0), nil)
	if body := str(data(t, r), "body_html"); !strings.Contains(body, "SECRET-DRAFT-EDIT") {
		t.Fatalf("correction not published: %s", body)
	}
	if after := data(t, anon.do("GET", x+"/works/"+nv.slug, nil))["last_public_update_at"]; after != before {
		t.Fatalf("PUB-08: correction bumped last update %v -> %v", before, after)
	}
	// Once public, the date is locked; re-scheduling is refused.
	r = c.do("POST", st+"/entries/"+nv.entries[0]+"/schedule", map[string]any{"public_at": ts(100 * time.Hour)})
	if r.Status != http.StatusConflict || r.errorCode(t) != "ALREADY_PUBLIC" {
		t.Fatalf("reschedule public: %d %s", r.Status, r.Body)
	}
	var audits int
	_ = sqlDB.QueryRow("SELECT COUNT(*) FROM "+tables.T("audit_logs")+" WHERE object_type = 'entry' AND action IN ('entry.schedule','entry.publish_revision') AND object_id IN (?, ?)", atou(nv.entries[0]), atou(nv.entries[1])).Scan(&audits)
	if audits != 3 {
		t.Fatalf("audit rows: %d", audits)
	}
}

// PUB-05, PUB-06 and empty drafts.
func TestScheduleValidation(t *testing.T) {
	nv := newNovel(t, "valid", 2)
	c := nv.c
	cases := []struct {
		name string
		body map[string]any
		code string
	}{
		{"missing public date", map[string]any{}, "VALIDATION_FAILED"},
		{"early lead over 30 days", map[string]any{"public_at": ts(40 * 24 * time.Hour), "early_at": ts(9 * 24 * time.Hour), "min_access_rank": 1}, "VALIDATION_FAILED"},
		{"early after public", map[string]any{"public_at": ts(24 * time.Hour), "early_at": ts(48 * time.Hour), "min_access_rank": 1}, "VALIDATION_FAILED"},
		{"early without tier", map[string]any{"public_at": ts(48 * time.Hour), "early_at": ts(24 * time.Hour)}, "VALIDATION_FAILED"},
		{"in the past", map[string]any{"public_at": ts(-24 * time.Hour)}, "VALIDATION_FAILED"},
	}
	for _, cs := range cases {
		r := c.do("POST", st+"/entries/"+nv.entries[0]+"/schedule", cs.body)
		if r.Status != http.StatusUnprocessableEntity || r.errorCode(t) != cs.code {
			t.Errorf("%s: %d %s", cs.name, r.Status, r.Body)
		}
	}
	saveDraft(t, c, nv.entries[1], 2, "")
	r := c.do("POST", st+"/entries/"+nv.entries[1]+"/schedule", map[string]any{"public_at": ts(time.Hour)})
	if r.Status != http.StatusUnprocessableEntity || !strings.Contains(string(r.Body), "empty") {
		t.Fatalf("empty draft scheduled: %d %s", r.Status, r.Body)
	}
	// Early access already running: the public date may only move earlier.
	expect(t, c.do("POST", st+"/entries/"+nv.entries[0]+"/schedule", map[string]any{"public_at": ts(5 * 24 * time.Hour), "early_at": ts(-time.Minute), "min_access_rank": 2}), http.StatusOK)
	r = c.do("POST", st+"/entries/"+nv.entries[0]+"/schedule", map[string]any{"public_at": ts(6 * 24 * time.Hour), "early_at": ts(-time.Minute), "min_access_rank": 2})
	if r.Status != http.StatusConflict || r.errorCode(t) != "PUBLIC_DATE_LOCKED" {
		t.Fatalf("public date moved later during early access: %d %s", r.Status, r.Body)
	}
	expect(t, c.do("POST", st+"/entries/"+nv.entries[0]+"/publish-now", nil), http.StatusOK)
	expect(t, newClient(t).do("GET", x+"/works/"+nv.slug+"/entries/"+nv.entrySl[0], nil), http.StatusOK)
}

// PUB-07: stale autosaves conflict; both versions stay recoverable.
func TestDraftConflict(t *testing.T) {
	nv := newNovel(t, "conflict", 1)
	c, id := nv.c, nv.entries[0]
	r := saveDraft(t, c, id, 2, "Tab A writes this.")
	if v := data(t, r)["version"]; v != float64(3) {
		t.Fatalf("version after save: %v", v)
	}
	r = saveDraft(t, c, id, 2, "Tab B had an old copy.")
	if r.Status != http.StatusConflict || r.errorCode(t) != "REVISION_CONFLICT" {
		t.Fatalf("stale save: %d %s", r.Status, r.Body)
	}
	details := r.json(t)["error"].(map[string]any)["details"].(map[string]any)
	if details["server_version"] != float64(3) {
		t.Fatalf("conflict details: %v", details)
	}
	expect(t, saveDraft(t, c, id, 3, "Tab B had an old copy."), http.StatusOK)
	r = saveDraft(t, c, id, 4, "Tab B had an old copy.") // identical text: no new revision
	if data(t, r)["version"] != float64(4) {
		t.Fatalf("identical save created a revision: %s", r.Body)
	}
	revs := c.do("GET", "/api/v1/pawn/content/entry_revisions?filter="+urlq("entry_id = "+id)+"&sort=version&fields=version,markdown", nil)
	expect(t, revs, http.StatusOK)
	var texts []string
	for _, it := range revs.json(t)["items"].([]any) {
		texts = append(texts, it.(map[string]any)["markdown"].(string))
	}
	if len(texts) != 4 || texts[2] != "Tab A writes this." || texts[3] != "Tab B had an old copy." {
		t.Fatalf("history: %q", texts)
	}
	// Restore creates a new revision from an old one; history is kept.
	var v2 string
	_ = sqlDB.QueryRow("SELECT id FROM "+tables.T("entry_revisions")+" WHERE entry_id = ? AND version = 3", atou(id)).Scan(&v2)
	r = c.do("POST", st+"/entries/"+id+"/restore", map[string]any{"revision_id": v2})
	expect(t, r, http.StatusOK)
	if d := data(t, r); d["version"] != float64(5) || d["markdown"] != "Tab A writes this." {
		t.Fatalf("restore: %v", d)
	}
	r = c.do("POST", st+"/entries/"+id+"/preview", map[string]any{"markdown": "<script>x()</script>\n\n*hi*"})
	expect(t, r, http.StatusOK)
	if h := str(data(t, r), "html"); strings.Contains(h, "<script") || !strings.Contains(h, "<em>hi</em>") {
		t.Fatalf("preview html: %s", h)
	}
}

// PUB-09: completion warns about unreleased drafts and never publishes them.
func TestCompleteWarnsAboutDrafts(t *testing.T) {
	nv := newNovel(t, "complete", 2)
	c := nv.c
	r := c.do("POST", st+"/works/"+nv.work+"/complete", nil)
	if r.Status != http.StatusConflict || r.errorCode(t) != "NOTHING_RELEASED" {
		t.Fatalf("complete with nothing released: %d %s", r.Status, r.Body)
	}
	expect(t, c.do("POST", st+"/entries/"+nv.entries[0]+"/publish-now", nil), http.StatusOK)
	r = c.do("POST", st+"/works/"+nv.work+"/complete", nil)
	if r.Status != http.StatusConflict || r.errorCode(t) != "UNRELEASED_ENTRIES" {
		t.Fatalf("complete with drafts: %d %s", r.Status, r.Body)
	}
	expect(t, c.do("POST", st+"/works/"+nv.work+"/complete", map[string]any{"force": true}), http.StatusNoContent)
	toc := data(t, newClient(t).do("GET", x+"/works/"+nv.slug+"/toc", nil))["entries"].([]any)
	if len(toc) != 1 {
		t.Fatalf("completion exposed drafts: %v", toc)
	}
	if d := data(t, newClient(t).do("GET", x+"/works/"+nv.slug, nil)); d["publication_status"] != "completed" {
		t.Fatalf("status: %v", d["publication_status"])
	}
}

// CAT-01/02 through the studio API.
func TestShortStoryThroughStudio(t *testing.T) {
	c, _ := author(t, "story")
	pen := data(t, c.do("POST", st+"/pen-names", map[string]any{"slug": "pn-story-" + randHex(3), "name": "Story Pen"}))
	r := c.do("POST", st+"/works", map[string]any{"pen_name_id": str(pen, "id"), "kind": "short_story", "title": "A Small Story", "genre": "mystery"})
	expect(t, r, http.StatusCreated)
	w := data(t, r)
	entry := str(w, "entry_id")
	if entry == "" {
		t.Fatal("short story created without its entry")
	}
	r = c.do("POST", st+"/works/"+str(w, "id")+"/entries", map[string]any{"title": "Part two"})
	if r.Status != http.StatusConflict || r.errorCode(t) != "SHORT_STORY_SINGLE_ENTRY" {
		t.Fatalf("second entry: %d %s", r.Status, r.Body)
	}
	r = c.do("POST", st+"/works/"+str(w, "id")+"/initialize-story", nil)
	if str(data(t, r), "entry_id") != entry {
		t.Fatalf("initialize-story not idempotent: %s", r.Body)
	}
	expect(t, c.do("POST", st+"/works/"+str(w, "id")+"/volumes", map[string]any{"title": "V"}), http.StatusConflict)
}

// CAT-04: reordering keeps ids/slugs; concurrent reorders serialize.
func TestReorderChapters(t *testing.T) {
	nv := newNovel(t, "reorder", 4)
	c := nv.c
	expect(t, c.do("POST", st+"/entries/"+nv.entries[3]+"/reorder", map[string]any{"position": 1}), http.StatusNoContent)
	items := c.do("GET", "/api/v1/pawn/content/entries?filter="+urlq("work_id = "+nv.work)+"&sort=ordinal&fields=id,slug,ordinal", nil).json(t)["items"].([]any)
	var order []string
	for _, it := range items {
		m := it.(map[string]any)
		order = append(order, str(m, "id")+":"+str(m, "slug"))
	}
	want := []string{nv.entries[3] + ":" + nv.entrySl[3], nv.entries[0] + ":" + nv.entrySl[0], nv.entries[1] + ":" + nv.entrySl[1], nv.entries[2] + ":" + nv.entrySl[2]}
	if strings.Join(order, ",") != strings.Join(want, ",") {
		t.Fatalf("order %v want %v", order, want)
	}
	var wg sync.WaitGroup
	for i := range 6 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.do("POST", st+"/entries/"+nv.entries[i%4]+"/reorder", map[string]any{"position": 1 + (i*3)%4})
		}()
	}
	wg.Wait()
	var n, distinct, maxOrd int
	_ = sqlDB.QueryRow("SELECT COUNT(*), COUNT(DISTINCT ordinal), MAX(ordinal) FROM "+tables.T("entries")+" WHERE work_id = ?", atou(nv.work)).Scan(&n, &distinct, &maxOrd)
	if n != 4 || distinct != 4 || maxOrd != 4 {
		t.Fatalf("ordinals after concurrent reorders: n=%d distinct=%d max=%d", n, distinct, maxOrd)
	}
	expect(t, c.do("POST", st+"/entries/"+nv.entries[0]+"/reorder", map[string]any{"position": 9}), http.StatusUnprocessableEntity)
}

// COL-01, COL-02, SEC-02: authors are isolated; protected fields are
// read-only; moderation-hidden works are frozen.
func TestAuthorIsolationAndProtectedFields(t *testing.T) {
	a := newNovel(t, "alice", 1)
	saveDraft(t, a.c, a.entries[0], 2, "Alice's private draft words.")
	b, _ := author(t, "bob")
	for _, p := range []string{"works", "entries", "entry_revisions", "studio_entries", "pen_names", "volumes"} {
		r := b.do("GET", "/api/v1/pawn/content/"+p+"?perPage=100", nil)
		expect(t, r, http.StatusOK)
		if len(r.json(t)["items"].([]any)) != 0 {
			t.Errorf("bob lists %s of alice: %s", p, r.Body)
		}
	}
	for _, p := range []string{"/api/v1/pawn/content/works/" + a.work, "/api/v1/pawn/content/entries/" + a.entries[0] + "?expand=work_id"} {
		expect(t, b.do("GET", p, nil), http.StatusNotFound)
	}
	for _, cmd := range []struct {
		method, path string
		body         map[string]any
	}{
		{"GET", st + "/entries/" + a.entries[0] + "/draft", nil},
		{"PUT", st + "/entries/" + a.entries[0] + "/draft", map[string]any{"markdown": "x", "expected_revision": 2}},
		{"POST", st + "/entries/" + a.entries[0] + "/schedule", map[string]any{"public_at": ts(time.Hour)}},
		{"POST", st + "/entries/" + a.entries[0] + "/preview", map[string]any{}},
		{"POST", st + "/entries/" + a.entries[0] + "/unpublish", map[string]any{"reason": "mine now"}},
		{"POST", st + "/works/" + a.work + "/entries", map[string]any{"title": "x"}},
		{"POST", st + "/works/" + a.work + "/complete", map[string]any{"force": true}},
		{"PUT", st + "/works/" + a.work + "/tags", map[string]any{"tags": []string{}}},
	} {
		r := b.do(cmd.method, cmd.path, cmd.body)
		if r.Status != http.StatusNotFound || strings.Contains(string(r.Body), "private draft") {
			t.Errorf("bob %s %s: %d %s", cmd.method, cmd.path, r.Status, r.Body)
		}
	}
	// Readers cannot use the studio at all.
	_, reader := verifiedUser(t, "studioreader")
	expect(t, reader.do("POST", st+"/works", map[string]any{"kind": "novel"}), http.StatusForbidden)

	// COL-02: protected fields through native PATCH.
	for _, p := range []struct {
		path string
		body map[string]any
	}{
		{"works/" + a.work, map[string]any{"slug": "stolen"}},
		{"works/" + a.work, map[string]any{"owner_user_id": "1"}},
		{"works/" + a.work, map[string]any{"is_featured": true}},
		{"works/" + a.work, map[string]any{"first_public_at": "2020-01-01T00:00:00Z"}},
		{"entries/" + a.entries[0], map[string]any{"public_at": "2020-01-01T00:00:00Z"}},
		{"entries/" + a.entries[0], map[string]any{"workflow": "published"}},
		{"entries/" + a.entries[0], map[string]any{"published_revision_id": "1"}},
		{"entries/" + a.entries[0], map[string]any{"ordinal": 9}},
	} {
		r := a.c.do("PATCH", "/api/v1/pawn/content/"+p.path, p.body)
		if r.Status != http.StatusBadRequest {
			t.Errorf("PATCH %s %v: %d %s", p.path, p.body, r.Status, r.Body)
		}
	}
	for _, p := range []string{"works", "entries", "entry_revisions", "pen_names", "collections"} {
		r := a.c.do("POST", "/api/v1/pawn/content/"+p, map[string]any{"title": "x"})
		if r.Status < 400 {
			t.Errorf("native create %s allowed: %d", p, r.Status)
		}
	}
	for _, body := range []map[string]any{{"visibility": "hidden"}, {"publication_status": "completed"}} {
		if r := a.c.do("PATCH", "/api/v1/pawn/content/works/"+a.work, body); r.Status < 400 {
			t.Errorf("PATCH works %v allowed: %d", body, r.Status)
		}
	}
	expect(t, a.c.do("PATCH", "/api/v1/pawn/content/works/"+a.work, map[string]any{"title": "Renamed Novel"}), http.StatusOK)
	expect(t, a.c.do("PATCH", "/api/v1/pawn/content/entries/"+a.entries[0], map[string]any{"title": "Renamed Chapter"}), http.StatusOK)

	// Moderation-hidden works are frozen for their author.
	if _, err := sqlDB.Exec("UPDATE "+tables.T("works")+" SET visibility = 'hidden' WHERE id = ?", atou(a.work)); err != nil {
		t.Fatal(err)
	}
	r := a.c.do("POST", st+"/entries/"+a.entries[0]+"/schedule", map[string]any{"public_at": ts(time.Hour)})
	if r.Status != http.StatusForbidden || r.errorCode(t) != "WORK_HIDDEN" {
		t.Fatalf("schedule on hidden work: %d %s", r.Status, r.Body)
	}
	if r := a.c.do("PATCH", "/api/v1/pawn/content/works/"+a.work, map[string]any{"visibility": "listed"}); r.Status < 400 {
		t.Fatalf("author un-hid a moderated work: %d", r.Status)
	}
}

func TestBulkSchedule(t *testing.T) {
	nv := newNovel(t, "bulk", 3)
	c := nv.c
	body := map[string]any{"anchor": ts(48 * time.Hour), "interval_days": 7, "early_lead_days": 3, "min_access_rank": 1}
	r := c.do("POST", st+"/works/"+nv.work+"/bulk-schedule", body)
	expect(t, r, http.StatusOK)
	items := data(t, r)["items"].([]any)
	if len(items) != 3 || data(t, r)["applied"] != false {
		t.Fatalf("preview: %s", r.Body)
	}
	var n int
	_ = sqlDB.QueryRow("SELECT COUNT(*) FROM "+tables.T("entries")+" WHERE work_id = ? AND workflow <> 'draft'", atou(nv.work)).Scan(&n)
	if n != 0 {
		t.Fatal("preview changed data")
	}
	saveDraft(t, c, nv.entries[2], 2, "") // an empty chapter blocks the whole batch
	body["apply"] = true
	r = c.do("POST", st+"/works/"+nv.work+"/bulk-schedule", body)
	if r.Status != http.StatusUnprocessableEntity || r.errorCode(t) != "BULK_SCHEDULE_INVALID" {
		t.Fatalf("bulk with empty chapter: %d %s", r.Status, r.Body)
	}
	_ = sqlDB.QueryRow("SELECT COUNT(*) FROM "+tables.T("entries")+" WHERE work_id = ? AND workflow <> 'draft'", atou(nv.work)).Scan(&n)
	if n != 0 {
		t.Fatal("failed batch partially applied")
	}
	saveDraft(t, c, nv.entries[2], 3, "Now it has text.")
	expect(t, c.do("POST", st+"/works/"+nv.work+"/bulk-schedule", body), http.StatusOK)
	toc := data(t, newClient(t).do("GET", x+"/works/"+nv.slug+"/toc", nil))["entries"].([]any)
	// Anchor +2 days with a 3-day lead: chapter 1's early window is already open.
	if len(toc) != 3 || toc[0].(map[string]any)["access"] != "early" || toc[1].(map[string]any)["access"] != "upcoming" {
		t.Fatalf("toc after bulk: %v", toc)
	}
}

func TestCollectionsAcceptStoriesOnly(t *testing.T) {
	nv := newNovel(t, "anth", 1)
	c := nv.c
	story := data(t, c.do("POST", st+"/works", map[string]any{"pen_name_id": nv.pen, "kind": "short_story", "title": "Anthology Story", "genre": "horror"}))
	col := c.do("POST", st+"/collections", map[string]any{"pen_name_id": nv.pen, "title": "My Anthology " + randHex(2)})
	expect(t, col, http.StatusCreated)
	cid := str(data(t, col), "id")
	r := c.do("PUT", st+"/collections/"+cid+"/items", map[string]any{"work_ids": []string{nv.work}})
	expect(t, r, http.StatusUnprocessableEntity)
	expect(t, c.do("PUT", st+"/collections/"+cid+"/items", map[string]any{"work_ids": []string{str(story, "id")}}), http.StatusNoContent)
	other, _ := author(t, "anthother")
	expect(t, other.do("PUT", st+"/collections/"+cid+"/items", map[string]any{"work_ids": []string{}}), http.StatusNotFound)
}

// SEC-04 and the asset lifecycle through Collar's presign/confirm.
func TestAssetUploadValidation(t *testing.T) {
	nv := newNovel(t, "cover", 1)
	c := nv.c

	key := upload(t, c, "cover.png", "image/png", []byte("<script>alert(1)</script> not really a png"))
	r := c.do("POST", st+"/assets", map[string]any{"key": key, "alt_text": "fake"})
	if r.Status != http.StatusUnprocessableEntity || r.errorCode(t) != "IMAGE_REJECTED" {
		t.Fatalf("disguised script: %d %s", r.Status, r.Body)
	}

	key = upload(t, c, "cover.png", "image/png", testPNG(600, 900))
	r = c.do("POST", st+"/assets", map[string]any{"key": key, "alt_text": "A cover"})
	expect(t, r, http.StatusOK)
	asset := data(t, r)
	if asset["state"] != "ready" || asset["card_url"] == nil {
		t.Fatalf("asset: %v", asset)
	}
	// Someone else's key cannot be registered.
	other, _ := author(t, "thief")
	if r := other.do("POST", st+"/assets", map[string]any{"key": key}); r.Status != http.StatusUnprocessableEntity {
		t.Fatalf("foreign key registered: %d", r.Status)
	}
	// Readers cannot presign uploads at all.
	_, reader := verifiedUser(t, "uploader")
	if r := reader.do("POST", "/api/v1/pawn/storage/uploads/presign", map[string]any{"filename": "x.png", "contentType": "image/png", "size": 10}); r.Status < 400 {
		t.Fatalf("reader presigned: %d", r.Status)
	}

	cardURL := str(asset, "card_url")
	anon := newClient(t)
	expect(t, anon.do("GET", cardURL, nil), http.StatusNotFound) // not referenced publicly yet
	expect(t, c.do("GET", cardURL, nil), http.StatusFound)       // the owner can see it
	expect(t, c.do("PATCH", "/api/v1/pawn/content/works/"+nv.work, map[string]any{"cover_asset_id": atou(str(asset, "id"))}), http.StatusOK)
	r = anon.do("GET", cardURL, nil)
	expect(t, r, http.StatusFound)
	res, err := http.Get(r.Header.Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	img, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || !bytes.HasPrefix(img, []byte{0xFF, 0xD8}) {
		t.Fatalf("variant download: %d", res.StatusCode)
	}
	// Another author's asset cannot become my cover.
	if r := other.do("PATCH", "/api/v1/pawn/content/works/"+nv.work, map[string]any{"cover_asset_id": atou(str(asset, "id"))}); r.Status < 400 {
		t.Fatalf("cross-owner cover: %d", r.Status)
	}
}

func upload(t *testing.T, c *client, name, ctype string, body []byte) string {
	t.Helper()
	r := c.do("POST", "/api/v1/pawn/storage/uploads/presign", map[string]any{"filename": name, "contentType": ctype, "size": len(body)})
	expect(t, r, http.StatusOK)
	var p struct {
		Key, URL, UploadToken string
		Headers               map[string]string
	}
	_ = json.Unmarshal(r.Body, &p)
	req, _ := http.NewRequest(http.MethodPut, p.URL, bytes.NewReader(body))
	for k, v := range p.Headers {
		req.Header.Set(k, v)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode >= 300 {
		t.Fatalf("PUT to bucket: %d", res.StatusCode)
	}
	expect(t, c.do("POST", "/api/v1/pawn/storage/uploads/confirm", map[string]any{"uploadToken": p.UploadToken}), http.StatusOK)
	return p.Key
}

func testPNG(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{uint8(x), uint8(y), 120, 255})
		}
	}
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}
