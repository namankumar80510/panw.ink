package studio

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/namankumar80510/panw.ink/internal/auth"
	"github.com/namankumar80510/panw.ink/internal/catalog"
	"github.com/namankumar80510/panw.ink/internal/db"
	"github.com/namankumar80510/panw.ink/internal/httpx"
)

// ---- pen names ---------------------------------------------------------------

type penNameInput struct {
	Slug       string  `json:"slug"`
	Name       string  `json:"name"`
	Bio        string  `json:"bio"`
	WebsiteURL *string `json:"website_url"`
}

func (s *Service) createPenName(w http.ResponseWriter, r *http.Request) {
	v := auth.ViewerFrom(r.Context())
	var in penNameInput
	if err := httpx.DecodeJSON(w, r, &in, httpx.MaxJSONBody); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	fields := map[string]string{}
	in.Slug = strings.TrimSpace(in.Slug)
	checkSlug(fields, "slug", in.Slug, 2, 100)
	in.Name = checkText(fields, "name", in.Name, 1, 140)
	in.Bio = checkText(fields, "bio", in.Bio, 0, 4000)
	if in.WebsiteURL != nil {
		u := strings.TrimSpace(*in.WebsiteURL)
		if u == "" {
			in.WebsiteURL = nil
		} else if len(u) > 500 || !(strings.HasPrefix(u, "https://") || strings.HasPrefix(u, "http://")) {
			fields["website_url"] = "Use an http(s) address."
		} else {
			in.WebsiteURL = &u
		}
	}
	if len(fields) > 0 {
		httpx.WriteError(w, r, httpx.Validation(fields))
		return
	}
	var id uint64
	err := db.InTx(r.Context(), s.DB, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(r.Context(), "INSERT INTO "+s.T.T("pen_names")+" (owner_user_id, slug, name, bio, website_url) VALUES (?,?,?,?,?)",
			v.UserID, in.Slug, in.Name, in.Bio, in.WebsiteURL)
		if isDuplicate(err, "") {
			return httpx.Validation(map[string]string{"slug": "This address is taken."})
		}
		if err != nil {
			return err
		}
		n, _ := res.LastInsertId()
		id = uint64(n)
		return s.audit(r.Context(), tx, r, w, "pen_name.create", "pen_name", id, nil, map[string]string{"slug": in.Slug})
	})
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteData(w, r, http.StatusCreated, map[string]string{"id": idStr(id), "slug": in.Slug}, nil)
}

// ---- works -------------------------------------------------------------------

type workInput struct {
	PenNameID       string   `json:"pen_name_id"`
	Kind            string   `json:"kind"`
	Title           string   `json:"title"`
	Subtitle        *string  `json:"subtitle"`
	Slug            string   `json:"slug"`
	Genre           string   `json:"genre"`
	Synopsis        string   `json:"synopsis"`
	LanguageCode    string   `json:"language_code"`
	AgeRating       string   `json:"age_rating"`
	ContentWarnings []string `json:"content_warnings"`
	Tags            []string `json:"tags"`
}

// createWork runs wizard steps 1–4 (§11.1): format, pen name and metadata,
// then the short story's single entry (a novel gets chapters separately).
// The work starts as a draft; nothing is public until an entry is scheduled
// and the work is listed.
func (s *Service) createWork(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	v := auth.ViewerFrom(ctx)
	var in workInput
	if err := httpx.DecodeJSON(w, r, &in, httpx.MaxJSONBody); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	fields := map[string]string{}
	if in.Kind != "novel" && in.Kind != "short_story" {
		fields["kind"] = "Choose novel or short_story."
	}
	in.Title = checkText(fields, "title", in.Title, 1, 240)
	if in.Subtitle != nil {
		sub := checkText(fields, "subtitle", *in.Subtitle, 0, 240)
		in.Subtitle = &sub
		if sub == "" {
			in.Subtitle = nil
		}
	}
	if in.Slug == "" {
		in.Slug = Slugify(in.Title, 160)
	}
	checkSlug(fields, "slug", in.Slug, 3, 160)
	in.Synopsis = checkText(fields, "synopsis", in.Synopsis, 0, 4000)
	if in.LanguageCode == "" {
		in.LanguageCode = "en"
	}
	if !langRE.MatchString(in.LanguageCode) {
		fields["language_code"] = "Use a language code such as en or pt-BR."
	}
	if in.AgeRating == "" {
		in.AgeRating = "teen"
	}
	if in.AgeRating != "general" && in.AgeRating != "teen" && in.AgeRating != "mature" {
		fields["age_rating"] = "Choose general, teen or mature."
	}
	warnings, msg := cleanWarnings(in.ContentWarnings)
	if msg != "" {
		fields["content_warnings"] = msg
	}
	penID, err := strconv.ParseUint(in.PenNameID, 10, 64)
	if err != nil {
		fields["pen_name_id"] = "Choose one of your pen names."
	}
	if len(fields) > 0 {
		httpx.WriteError(w, r, httpx.Validation(fields))
		return
	}
	status := "ongoing"
	if in.Kind == "short_story" {
		status = "completed" // a story is complete by definition once public (§11.1 step 10)
	}
	var workID, entryID uint64
	err = db.InTx(ctx, s.DB, func(tx *sql.Tx) error {
		var penOwner uint64
		var penState string
		err := tx.QueryRowContext(ctx, "SELECT owner_user_id, state FROM "+s.T.T("pen_names")+" WHERE id = ? FOR UPDATE", penID).Scan(&penOwner, &penState)
		if errors.Is(err, sql.ErrNoRows) || err == nil && (penOwner != v.UserID || penState != "active") {
			return httpx.Validation(map[string]string{"pen_name_id": "Choose one of your active pen names."})
		}
		if err != nil {
			return err
		}
		var genreID uint64
		err = tx.QueryRowContext(ctx, "SELECT id FROM "+s.T.T("genres")+" WHERE slug = ? AND enabled = 1", in.Genre).Scan(&genreID)
		if errors.Is(err, sql.ErrNoRows) {
			return httpx.Validation(map[string]string{"genre": "Choose a genre."})
		}
		if err != nil {
			return err
		}
		var wjson any
		if len(warnings) > 0 {
			wjson = string(jsonOf(warnings))
		}
		res, err := tx.ExecContext(ctx, "INSERT INTO "+s.T.T("works")+
			" (pen_name_id, owner_user_id, kind, slug, title, subtitle, synopsis, primary_genre_id, publication_status, visibility, age_rating, language_code, content_warnings_json)"+
			" VALUES (?,?,?,?,?,?,?,?,?,'draft',?,?,?)",
			penID, v.UserID, in.Kind, in.Slug, in.Title, in.Subtitle, in.Synopsis, genreID, status, in.AgeRating, in.LanguageCode, wjson)
		if isDuplicate(err, "") {
			return httpx.Validation(map[string]string{"slug": "This address is taken."})
		}
		if err != nil {
			return err
		}
		n, _ := res.LastInsertId()
		workID = uint64(n)
		if err := s.replaceTags(ctx, tx, workID, in.Tags); err != nil {
			return err
		}
		if in.Kind == "short_story" {
			if entryID, err = s.ensureStoryEntry(ctx, tx, workID, v.UserID, in.Title); err != nil {
				return err
			}
		}
		return s.audit(ctx, tx, r, w, "work.create", "work", workID, nil, map[string]any{"kind": in.Kind, "slug": in.Slug, "pen_name_id": idStr(penID)})
	})
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out := map[string]any{"id": idStr(workID), "slug": in.Slug, "kind": in.Kind, "entry_id": nil}
	if entryID != 0 {
		out["entry_id"] = idStr(entryID)
	}
	httpx.WriteData(w, r, http.StatusCreated, out, nil)
}

func cleanWarnings(in []string) ([]string, string) {
	out := []string{}
	seen := map[string]bool{}
	for _, raw := range in {
		x := strings.TrimSpace(raw)
		if x == "" || seen[strings.ToLower(x)] {
			continue
		}
		if len([]rune(x)) > 40 {
			return nil, "Keep each warning under 40 characters."
		}
		seen[strings.ToLower(x)] = true
		out = append(out, x)
	}
	if len(out) > 10 {
		return nil, "Use at most 10 content warnings."
	}
	return out, ""
}

// ensureStoryEntry creates a short story's single entry (ordinal 1, slug
// "full-story") with an empty first draft, idempotently.
func (s *Service) ensureStoryEntry(ctx context.Context, tx *sql.Tx, workID, owner uint64, title string) (uint64, error) {
	var id uint64
	err := tx.QueryRowContext(ctx, "SELECT id FROM "+s.T.T("entries")+" WHERE work_id = ? AND ordinal = 1", workID).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	return s.insertEntry(ctx, tx, workID, "short_story", owner, nil, 1, "full-story", title)
}

func (s *Service) insertEntry(ctx context.Context, tx *sql.Tx, workID uint64, kind string, owner uint64, volumeID *uint64, ordinal int, slug, title string) (uint64, error) {
	res, err := tx.ExecContext(ctx, "INSERT INTO "+s.T.T("entries")+" (work_id, work_kind, owner_user_id, volume_id, ordinal, slug, title) VALUES (?,?,?,?,?,?,?)",
		workID, kind, owner, volumeID, ordinal, slug, title)
	if err != nil {
		return 0, err
	}
	n, _ := res.LastInsertId()
	entryID := uint64(n)
	rev, err := catalog.CreateRevision(ctx, tx, s.T, entryID, owner, "", "Created")
	if err != nil {
		return 0, err
	}
	_, err = tx.ExecContext(ctx, "UPDATE "+s.T.T("entries")+" SET current_draft_revision_id = ? WHERE id = ?", rev.ID, entryID)
	return entryID, err
}

func (s *Service) initializeStory(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	v := auth.ViewerFrom(ctx)
	id, err := pathID(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	var entryID uint64
	err = db.InTx(ctx, s.DB, func(tx *sql.Tx) error {
		wr, err := s.lockWork(ctx, tx, id, v.UserID)
		if err != nil {
			return err
		}
		if wr.Kind != "short_story" {
			return conflict("NOT_A_SHORT_STORY", "Only short stories have a single text entry.")
		}
		entryID, err = s.ensureStoryEntry(ctx, tx, wr.ID, v.UserID, wr.Title)
		return err
	})
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteData(w, r, http.StatusOK, map[string]string{"entry_id": idStr(entryID)}, nil)
}

// ---- tags ---------------------------------------------------------------------

type tagsInput struct {
	Tags []string `json:"tags"`
}

func (s *Service) setTags(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	v := auth.ViewerFrom(ctx)
	id, err := pathID(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	var in tagsInput
	if err := httpx.DecodeJSON(w, r, &in, httpx.MaxJSONBody); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	err = db.InTx(ctx, s.DB, func(tx *sql.Tx) error {
		if _, err := s.lockWork(ctx, tx, id, v.UserID); err != nil {
			return err
		}
		return s.replaceTags(ctx, tx, id, in.Tags)
	})
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// replaceTags sets a work's tags to approved tags only (moderators maintain
// the canonical list, §5.2).
func (s *Service) replaceTags(ctx context.Context, tx *sql.Tx, workID uint64, slugs []string) error {
	uniq := map[string]bool{}
	for _, t := range slugs {
		if t = strings.TrimSpace(t); t != "" {
			uniq[t] = true
		}
	}
	if len(uniq) > 10 {
		return httpx.Validation(map[string]string{"tags": "Use at most 10 tags."})
	}
	ids := []uint64{}
	for slug := range uniq {
		var id uint64
		err := tx.QueryRowContext(ctx, "SELECT id FROM "+s.T.T("tags")+" WHERE slug = ? AND state = 'approved'", slug).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			return httpx.Validation(map[string]string{"tags": fmt.Sprintf("%q is not an approved tag.", slug)})
		}
		if err != nil {
			return err
		}
		ids = append(ids, id)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM "+s.T.T("work_tags")+" WHERE work_id = ?", workID); err != nil {
		return err
	}
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx, "INSERT INTO "+s.T.T("work_tags")+" (work_id, tag_id) VALUES (?,?)", workID, id); err != nil {
			return err
		}
	}
	return nil
}

// ---- completion / archive -----------------------------------------------------

type completeInput struct {
	Force bool `json:"force"`
}

// completeWork marks a novel finished (§11.1 step 10, PUB-09): it warns
// about unreleased chapters instead of deleting or publishing them, and only
// proceeds when the author confirms with force.
func (s *Service) completeWork(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	v := auth.ViewerFrom(ctx)
	id, err := pathID(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	var in completeInput
	if r.ContentLength != 0 {
		if err := httpx.DecodeJSON(w, r, &in, httpx.MaxJSONBody); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
	}
	err = db.InTx(ctx, s.DB, func(tx *sql.Tx) error {
		wr, err := s.lockWork(ctx, tx, id, v.UserID)
		if err != nil {
			return err
		}
		var released int
		if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+s.T.T("entries")+" WHERE work_id = ? AND workflow IN ('scheduled','published')", wr.ID).Scan(&released); err != nil {
			return err
		}
		if released == 0 {
			return conflict("NOTHING_RELEASED", "Schedule or publish at least one entry before marking the work complete.")
		}
		rows, err := tx.QueryContext(ctx, "SELECT id, ordinal, title FROM "+s.T.T("entries")+" WHERE work_id = ? AND workflow = 'draft' ORDER BY ordinal", wr.ID)
		if err != nil {
			return err
		}
		unreleased := []map[string]any{}
		for rows.Next() {
			var eid uint64
			var ord int
			var title string
			if err := rows.Scan(&eid, &ord, &title); err != nil {
				rows.Close()
				return err
			}
			unreleased = append(unreleased, map[string]any{"id": idStr(eid), "ordinal": ord, "title": title})
		}
		rows.Close()
		if len(unreleased) > 0 && !in.Force {
			return conflict("UNRELEASED_ENTRIES", "Some chapters are still drafts. They stay private; confirm to mark the work complete anyway.").
				WithDetails(map[string]any{"unreleased": unreleased})
		}
		if _, err := tx.ExecContext(ctx, "UPDATE "+s.T.T("works")+" SET publication_status = 'completed', completed_at = COALESCE(completed_at, ?) WHERE id = ?", s.now(), wr.ID); err != nil {
			return err
		}
		return s.audit(ctx, tx, r, w, "work.complete", "work", wr.ID, map[string]string{"status": wr.Status}, map[string]any{"status": "completed", "unreleased_drafts": len(unreleased)})
	})
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type reasonInput struct {
	Reason string `json:"reason"`
}

// archiveWork withdraws a work from the catalogue and reading (visibility
// draft) without deleting anything; progress and URLs stay valid for when
// it returns.
func (s *Service) archiveWork(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	v := auth.ViewerFrom(ctx)
	id, err := pathID(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	var in reasonInput
	if err := httpx.DecodeJSON(w, r, &in, httpx.MaxJSONBody); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	fields := map[string]string{}
	in.Reason = checkText(fields, "reason", in.Reason, 3, 500)
	if len(fields) > 0 {
		httpx.WriteError(w, r, httpx.Validation(fields))
		return
	}
	err = db.InTx(ctx, s.DB, func(tx *sql.Tx) error {
		wr, err := s.lockWork(ctx, tx, id, v.UserID)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "UPDATE "+s.T.T("works")+" SET visibility = 'draft' WHERE id = ?", wr.ID); err != nil {
			return err
		}
		return s.audit(ctx, tx, r, w, "work.archive", "work", wr.ID, map[string]string{"visibility": wr.Visibility}, map[string]string{"visibility": "draft", "reason": in.Reason})
	})
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- volumes and chapters -------------------------------------------------------

type volumeInput struct {
	Title       string  `json:"title"`
	Description *string `json:"description"`
}

func (s *Service) createVolume(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	v := auth.ViewerFrom(ctx)
	id, err := pathID(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	var in volumeInput
	if err := httpx.DecodeJSON(w, r, &in, httpx.MaxJSONBody); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	fields := map[string]string{}
	in.Title = checkText(fields, "title", in.Title, 1, 160)
	if len(fields) > 0 {
		httpx.WriteError(w, r, httpx.Validation(fields))
		return
	}
	var volID uint64
	var number int
	err = db.InTx(ctx, s.DB, func(tx *sql.Tx) error {
		wr, err := s.lockWork(ctx, tx, id, v.UserID)
		if err != nil {
			return err
		}
		if wr.Kind != "novel" {
			return conflict("NOT_A_NOVEL", "Only novels have volumes.")
		}
		if err := tx.QueryRowContext(ctx, "SELECT COALESCE(MAX(number), 0) + 1 FROM "+s.T.T("volumes")+" WHERE work_id = ?", wr.ID).Scan(&number); err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, "INSERT INTO "+s.T.T("volumes")+" (work_id, owner_user_id, number, title, description) VALUES (?,?,?,?,?)", wr.ID, v.UserID, number, in.Title, in.Description)
		if err != nil {
			return err
		}
		n, _ := res.LastInsertId()
		volID = uint64(n)
		return s.audit(ctx, tx, r, w, "volume.create", "volume", volID, nil, map[string]any{"work_id": idStr(wr.ID), "number": number})
	})
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteData(w, r, http.StatusCreated, map[string]any{"id": idStr(volID), "number": number}, nil)
}

type entryInput struct {
	Title    string  `json:"title"`
	Slug     string  `json:"slug"`
	VolumeID *string `json:"volume_id"`
}

// createEntry appends a chapter to a novel (CAT-02: short stories refuse).
func (s *Service) createEntry(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	v := auth.ViewerFrom(ctx)
	id, err := pathID(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	var in entryInput
	if err := httpx.DecodeJSON(w, r, &in, httpx.MaxJSONBody); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	fields := map[string]string{}
	in.Title = checkText(fields, "title", in.Title, 1, 240)
	explicitSlug := in.Slug != ""
	if !explicitSlug {
		in.Slug = Slugify(in.Title, 150)
		if in.Slug == "" {
			in.Slug = "chapter"
		}
	}
	checkSlug(fields, "slug", in.Slug, 1, 160)
	if len(fields) > 0 {
		httpx.WriteError(w, r, httpx.Validation(fields))
		return
	}
	var entryID uint64
	var ordinal int
	err = db.InTx(ctx, s.DB, func(tx *sql.Tx) error {
		wr, err := s.lockWork(ctx, tx, id, v.UserID)
		if err != nil {
			return err
		}
		if wr.Kind != "novel" {
			return conflict("SHORT_STORY_SINGLE_ENTRY", "A short story is one complete text; it cannot have chapters.")
		}
		var volume *uint64
		if in.VolumeID != nil && *in.VolumeID != "" {
			vid, err := strconv.ParseUint(*in.VolumeID, 10, 64)
			var n int
			if err == nil {
				err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+s.T.T("volumes")+" WHERE id = ? AND work_id = ?", vid, wr.ID).Scan(&n)
			}
			if err != nil || n == 0 {
				return httpx.Validation(map[string]string{"volume_id": "Choose a volume of this novel."})
			}
			volume = &vid
		}
		if err := tx.QueryRowContext(ctx, "SELECT COALESCE(MAX(ordinal), 0) + 1 FROM "+s.T.T("entries")+" WHERE work_id = ?", wr.ID).Scan(&ordinal); err != nil {
			return err
		}
		slug := in.Slug
		for i := 2; ; i++ {
			var n int
			if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+s.T.T("entries")+" WHERE work_id = ? AND slug = ?", wr.ID, slug).Scan(&n); err != nil {
				return err
			}
			if n == 0 {
				break
			}
			if explicitSlug {
				return httpx.Validation(map[string]string{"slug": "Another chapter uses this address."})
			}
			slug = fmt.Sprintf("%s-%d", in.Slug, i)
		}
		entryID, err = s.insertEntry(ctx, tx, wr.ID, "novel", v.UserID, volume, ordinal, slug, in.Title)
		if err != nil {
			return err
		}
		in.Slug = slug
		return s.audit(ctx, tx, r, w, "entry.create", "entry", entryID, nil, map[string]any{"work_id": idStr(wr.ID), "ordinal": ordinal})
	})
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteData(w, r, http.StatusCreated, map[string]any{"id": idStr(entryID), "ordinal": ordinal, "slug": in.Slug}, nil)
}

type positionInput struct {
	Position int `json:"position"`
}

// reorderEntry moves a chapter to a 1-based position. Entry ids and slugs
// never change, so links, bookmarks and progress stay valid (CAT-04). The
// work row lock serializes concurrent reorders; ordinals are rewritten
// through a temporary offset so UNIQUE(work_id, ordinal) never collides.
func (s *Service) reorderEntry(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	v := auth.ViewerFrom(ctx)
	id, err := pathID(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	var in positionInput
	if err := httpx.DecodeJSON(w, r, &in, httpx.MaxJSONBody); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	err = db.InTx(ctx, s.DB, func(tx *sql.Tx) error {
		e, err := s.lockEntry(ctx, tx, id, v.UserID)
		if err != nil {
			return err
		}
		if e.WorkKind != "novel" {
			return conflict("SHORT_STORY_SINGLE_ENTRY", "A short story has a single entry.")
		}
		ids, err := s.orderedIDs(ctx, tx, "SELECT id FROM "+s.T.T("entries")+" WHERE work_id = ? ORDER BY ordinal", e.WorkID)
		if err != nil {
			return err
		}
		if in.Position < 1 || in.Position > len(ids) {
			return httpx.Validation(map[string]string{"position": fmt.Sprintf("Use a position between 1 and %d.", len(ids))})
		}
		order := move(ids, e.ID, in.Position-1)
		if err := s.renumber(ctx, tx, "entries", "ordinal", order); err != nil {
			return err
		}
		if err := catalog.RefreshRollups(ctx, tx, s.T, e.WorkID, s.now()); err != nil {
			return err
		}
		return s.audit(ctx, tx, r, w, "entry.reorder", "entry", e.ID, map[string]int{"ordinal": e.Ordinal}, map[string]int{"ordinal": in.Position})
	})
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) reorderVolume(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	v := auth.ViewerFrom(ctx)
	id, err := pathID(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	var in positionInput
	if err := httpx.DecodeJSON(w, r, &in, httpx.MaxJSONBody); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	err = db.InTx(ctx, s.DB, func(tx *sql.Tx) error {
		var workID uint64
		var number int
		err := tx.QueryRowContext(ctx, "SELECT work_id, number FROM "+s.T.T("volumes")+" WHERE id = ? AND owner_user_id = ?", id, v.UserID).Scan(&workID, &number)
		if errors.Is(err, sql.ErrNoRows) {
			return httpx.ErrNotFound
		}
		if err != nil {
			return err
		}
		if _, err := s.lockWork(ctx, tx, workID, v.UserID); err != nil {
			return err
		}
		ids, err := s.orderedIDs(ctx, tx, "SELECT id FROM "+s.T.T("volumes")+" WHERE work_id = ? ORDER BY number", workID)
		if err != nil {
			return err
		}
		if in.Position < 1 || in.Position > len(ids) {
			return httpx.Validation(map[string]string{"position": fmt.Sprintf("Use a position between 1 and %d.", len(ids))})
		}
		if err := s.renumber(ctx, tx, "volumes", "number", move(ids, id, in.Position-1)); err != nil {
			return err
		}
		return s.audit(ctx, tx, r, w, "volume.reorder", "volume", id, map[string]int{"number": number}, map[string]int{"number": in.Position})
	})
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) orderedIDs(ctx context.Context, tx *sql.Tx, q string, arg any) ([]uint64, error) {
	rows, err := tx.QueryContext(ctx, q+" FOR UPDATE", arg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []uint64
	for rows.Next() {
		var id uint64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// move returns ids with target moved to index pos.
func move(ids []uint64, target uint64, pos int) []uint64 {
	out := make([]uint64, 0, len(ids))
	for _, id := range ids {
		if id != target {
			out = append(out, id)
		}
	}
	out = append(out[:pos], append([]uint64{target}, out[pos:]...)...)
	return out
}

// renumber assigns 1..n in order via a temporary offset (no UNIQUE clashes).
func (s *Service) renumber(ctx context.Context, tx *sql.Tx, table, col string, order []uint64) error {
	const offset = 1_000_000
	for i, id := range order {
		if _, err := tx.ExecContext(ctx, "UPDATE "+s.T.T(table)+" SET "+col+" = ? WHERE id = ?", offset+i+1, id); err != nil {
			return err
		}
	}
	for i, id := range order {
		if _, err := tx.ExecContext(ctx, "UPDATE "+s.T.T(table)+" SET "+col+" = ? WHERE id = ?", i+1, id); err != nil {
			return err
		}
	}
	return nil
}

// ---- collections ------------------------------------------------------------------

type collectionInput struct {
	PenNameID   string `json:"pen_name_id"`
	Slug        string `json:"slug"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

func (s *Service) createCollection(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	v := auth.ViewerFrom(ctx)
	var in collectionInput
	if err := httpx.DecodeJSON(w, r, &in, httpx.MaxJSONBody); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	fields := map[string]string{}
	in.Title = checkText(fields, "title", in.Title, 1, 240)
	if in.Slug == "" {
		in.Slug = Slugify(in.Title, 160)
	}
	checkSlug(fields, "slug", in.Slug, 3, 160)
	in.Description = checkText(fields, "description", in.Description, 0, 4000)
	penID, err := strconv.ParseUint(in.PenNameID, 10, 64)
	if err != nil {
		fields["pen_name_id"] = "Choose one of your pen names."
	}
	if len(fields) > 0 {
		httpx.WriteError(w, r, httpx.Validation(fields))
		return
	}
	var id uint64
	err = db.InTx(ctx, s.DB, func(tx *sql.Tx) error {
		var n int
		if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+s.T.T("pen_names")+" WHERE id = ? AND owner_user_id = ? AND state = 'active'", penID, v.UserID).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			return httpx.Validation(map[string]string{"pen_name_id": "Choose one of your active pen names."})
		}
		res, err := tx.ExecContext(ctx, "INSERT INTO "+s.T.T("collections")+" (pen_name_id, owner_user_id, slug, title, description) VALUES (?,?,?,?,?)",
			penID, v.UserID, in.Slug, in.Title, in.Description)
		if isDuplicate(err, "") {
			return httpx.Validation(map[string]string{"slug": "This address is taken."})
		}
		if err != nil {
			return err
		}
		nid, _ := res.LastInsertId()
		id = uint64(nid)
		return s.audit(ctx, tx, r, w, "collection.create", "collection", id, nil, map[string]string{"slug": in.Slug})
	})
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteData(w, r, http.StatusCreated, map[string]string{"id": idStr(id), "slug": in.Slug}, nil)
}

type itemsInput struct {
	WorkIDs []string `json:"work_ids"`
}

// setCollectionItems atomically replaces an anthology's ordered stories.
// Only the caller's own short stories are accepted (§3.1).
func (s *Service) setCollectionItems(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	v := auth.ViewerFrom(ctx)
	id, err := pathID(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	var in itemsInput
	if err := httpx.DecodeJSON(w, r, &in, httpx.MaxJSONBody); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if len(in.WorkIDs) > 200 {
		httpx.WriteError(w, r, httpx.Validation(map[string]string{"work_ids": "Use at most 200 stories."}))
		return
	}
	err = db.InTx(ctx, s.DB, func(tx *sql.Tx) error {
		var visibility string
		err := tx.QueryRowContext(ctx, "SELECT visibility FROM "+s.T.T("collections")+" WHERE id = ? AND owner_user_id = ? FOR UPDATE", id, v.UserID).Scan(&visibility)
		if errors.Is(err, sql.ErrNoRows) {
			return httpx.ErrNotFound
		}
		if err != nil {
			return err
		}
		if visibility == "hidden" {
			return httpx.NewError(http.StatusForbidden, "COLLECTION_HIDDEN", "This collection is hidden by moderation.")
		}
		seen := map[uint64]bool{}
		var works []uint64
		for _, raw := range in.WorkIDs {
			wid, err := strconv.ParseUint(raw, 10, 64)
			if err != nil || seen[wid] {
				return httpx.Validation(map[string]string{"work_ids": "Each story may appear once."})
			}
			var kind string
			err = tx.QueryRowContext(ctx, "SELECT kind FROM "+s.T.T("works")+" WHERE id = ? AND owner_user_id = ? AND deleted_at IS NULL", wid, v.UserID).Scan(&kind)
			if errors.Is(err, sql.ErrNoRows) {
				return httpx.Validation(map[string]string{"work_ids": "Add only your own short stories."})
			}
			if err != nil {
				return err
			}
			if kind != "short_story" {
				return httpx.Validation(map[string]string{"work_ids": "Collections contain short stories only; novels cannot be added."})
			}
			seen[wid] = true
			works = append(works, wid)
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+s.T.T("collection_items")+" WHERE collection_id = ?", id); err != nil {
			return err
		}
		for i, wid := range works {
			if _, err := tx.ExecContext(ctx, "INSERT INTO "+s.T.T("collection_items")+" (collection_id, work_id, ordinal) VALUES (?,?,?)", id, wid, i+1); err != nil {
				return err
			}
		}
		return s.audit(ctx, tx, r, w, "collection.items", "collection", id, nil, map[string]any{"work_ids": in.WorkIDs})
	})
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
