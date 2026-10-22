package studio

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/xmfunew/collar"

	"github.com/namankumar80510/panw.ink/internal/auth"
	"github.com/namankumar80510/panw.ink/internal/catalog"
	"github.com/namankumar80510/panw.ink/internal/db"
	"github.com/namankumar80510/panw.ink/internal/httpx"
	"github.com/namankumar80510/panw.ink/internal/render"
)

// KindUpdateWorkRollups is catalog.KindUpdateWorkRollups.
const KindUpdateWorkRollups = catalog.KindUpdateWorkRollups

// Draft is the editor's view of an entry.
type Draft struct {
	EntryID     string     `json:"entry_id"`
	WorkID      string     `json:"work_id"`
	WorkKind    string     `json:"work_kind"`
	WorkSlug    string     `json:"work_slug"`
	WorkTitle   string     `json:"work_title"`
	Title       string     `json:"title"`
	Slug        string     `json:"slug"`
	Ordinal     int        `json:"ordinal"`
	Workflow    string     `json:"workflow"`
	Access      string     `json:"access"`
	EarlyAt     *time.Time `json:"early_at"`
	PublicAt    *time.Time `json:"public_at"`
	MinRank     *int       `json:"min_access_rank"`
	Markdown    string     `json:"markdown"`
	Version     int        `json:"version"`
	RevisionID  string     `json:"revision_id"`
	WordCount   int        `json:"word_count"`
	SavedAt     time.Time  `json:"saved_at"`
	Published   *RevRef    `json:"published"`
	DraftIsLive bool       `json:"draft_is_published"`
}

// RevRef identifies a revision.
type RevRef struct {
	ID      string `json:"id"`
	Version int    `json:"version"`
}

func (s *Service) loadDraft(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, entryID, owner uint64) (*Draft, error) {
	var d Draft
	var eid, wid, revID uint64
	var early, public, saved db.Time
	var minRank sql.NullInt64
	var pubID, pubVersion sql.NullInt64
	err := q.QueryRowContext(ctx, "SELECT e.id, w.id, w.kind, w.slug, w.title, e.title, e.slug, e.ordinal, e.workflow, e.early_at, e.public_at, a.minimum_access_rank,"+
		" r.id, r.version, r.markdown, r.word_count, r.created_at, p.id, p.version"+
		" FROM "+s.T.T("entries")+" e JOIN "+s.T.T("works")+" w ON w.id = e.work_id"+
		" JOIN "+s.T.T("entry_revisions")+" r ON r.id = e.current_draft_revision_id"+
		" LEFT JOIN "+s.T.T("entry_revisions")+" p ON p.id = e.published_revision_id"+
		" LEFT JOIN "+s.T.T("entry_access")+" a ON a.entry_id = e.id"+
		" WHERE e.id = ? AND e.owner_user_id = ? AND w.deleted_at IS NULL", entryID, owner).
		Scan(&eid, &wid, &d.WorkKind, &d.WorkSlug, &d.WorkTitle, &d.Title, &d.Slug, &d.Ordinal, &d.Workflow, &early, &public, &minRank,
			&revID, &d.Version, &d.Markdown, &d.WordCount, &saved, &pubID, &pubVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, httpx.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	d.EntryID, d.WorkID, d.RevisionID, d.SavedAt = idStr(eid), idStr(wid), idStr(revID), saved.Time
	if early.Valid {
		d.EarlyAt = &early.Time
	}
	if public.Valid {
		d.PublicAt = &public.Time
	}
	if minRank.Valid {
		n := int(minRank.Int64)
		d.MinRank = &n
	}
	if pubID.Valid {
		d.Published = &RevRef{ID: idStr(uint64(pubID.Int64)), Version: int(pubVersion.Int64)}
		d.DraftIsLive = uint64(pubID.Int64) == revID
	}
	st := catalog.StateAt(catalog.Release{Workflow: d.Workflow, EarlyAt: d.EarlyAt, PublicAt: d.PublicAt, HasPublished: pubID.Valid}, s.now())
	d.Access = string(st)
	if st == catalog.AccessUnavailable {
		d.Access = d.Workflow // draft | archived
	}
	return &d, nil
}

func (s *Service) getDraft(w http.ResponseWriter, r *http.Request) {
	v := auth.ViewerFrom(r.Context())
	id, err := pathID(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	d, err := s.loadDraft(r.Context(), s.DB, id, v.UserID)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteData(w, r, http.StatusOK, d, nil)
}

type draftInput struct {
	Markdown         string `json:"markdown"`
	ExpectedRevision int    `json:"expected_revision"`
	EditorVersion    string `json:"editor_version"`
	ChangeNote       string `json:"change_note"`
}

// saveDraft creates a new immutable revision and swaps the draft pointer only
// when the caller edited the current draft version; otherwise it returns 409
// REVISION_CONFLICT with the server's version, and nothing is overwritten
// (§12.2, PUB-07). Saving identical text is a no-op.
func (s *Service) saveDraft(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	v := auth.ViewerFrom(ctx)
	id, err := pathID(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	var in draftInput
	if err := httpx.DecodeJSON(w, r, &in, maxDraftBody); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if len(in.Markdown) > render.MaxMarkdownBytes {
		httpx.WriteError(w, r, httpx.Validation(map[string]string{"markdown": "The text exceeds 2 MB."}))
		return
	}
	fields := map[string]string{}
	in.ChangeNote = checkText(fields, "change_note", in.ChangeNote, 0, 250)
	if len(fields) > 0 {
		httpx.WriteError(w, r, httpx.Validation(fields))
		return
	}
	var out *Draft
	err = db.InTx(ctx, s.DB, func(tx *sql.Tx) error {
		e, err := s.lockEntry(ctx, tx, id, v.UserID)
		if err != nil {
			return err
		}
		var version int
		var checksum []byte
		var saved db.Time
		var words int
		if err := tx.QueryRowContext(ctx, "SELECT version, checksum, created_at, word_count FROM "+s.T.T("entry_revisions")+" WHERE id = ?", e.DraftRevID.Int64).Scan(&version, &checksum, &saved, &words); err != nil {
			return err
		}
		if in.ExpectedRevision != version {
			return conflict("REVISION_CONFLICT", "This text was changed elsewhere since you opened it. Your version was not saved.").
				WithDetails(map[string]any{"server_version": version, "saved_at": saved.Time, "word_count": words})
		}
		res, err := render.Markdown(in.Markdown)
		if err != nil {
			return err
		}
		if !bytes.Equal(res.Checksum, checksum) {
			rev, err := catalog.CreateRevision(ctx, tx, s.T, e.ID, v.UserID, in.Markdown, in.ChangeNote)
			if err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, "UPDATE "+s.T.T("entries")+" SET current_draft_revision_id = ?, last_edited_at = ? WHERE id = ?", rev.ID, s.now(), e.ID); err != nil {
				return err
			}
		}
		out, err = s.loadDraft(ctx, tx, e.ID, v.UserID)
		return err
	})
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out.Markdown = "" // the client already has it
	httpx.WriteData(w, r, http.StatusOK, out, nil)
}

type previewInput struct {
	Markdown *string `json:"markdown"`
}

// preview renders sanitized HTML for the author only; it creates no public
// URL and stores nothing (§11.1 step 6).
func (s *Service) preview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	v := auth.ViewerFrom(ctx)
	id, err := pathID(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	var in previewInput
	if err := httpx.DecodeJSON(w, r, &in, maxDraftBody); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	d, err := s.loadDraft(ctx, s.DB, id, v.UserID)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	src := d.Markdown
	if in.Markdown != nil {
		src = *in.Markdown
	}
	res, err := render.Markdown(src)
	if err != nil {
		httpx.WriteError(w, r, httpx.Validation(map[string]string{"markdown": "The text exceeds 2 MB."}))
		return
	}
	httpx.WriteData(w, r, http.StatusOK, map[string]any{"html": res.HTML, "word_count": res.WordCount, "read_seconds": res.ReadSeconds}, nil)
}

type restoreInput struct {
	RevisionID string `json:"revision_id"`
}

// restore makes a prior revision's text the new draft as a fresh revision;
// history is never rewritten.
func (s *Service) restore(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	v := auth.ViewerFrom(ctx)
	id, err := pathID(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	var in restoreInput
	if err := httpx.DecodeJSON(w, r, &in, httpx.MaxJSONBody); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	revID, err := strconv.ParseUint(in.RevisionID, 10, 64)
	if err != nil {
		httpx.WriteError(w, r, httpx.Validation(map[string]string{"revision_id": "Choose a revision."}))
		return
	}
	var out *Draft
	err = db.InTx(ctx, s.DB, func(tx *sql.Tx) error {
		e, err := s.lockEntry(ctx, tx, id, v.UserID)
		if err != nil {
			return err
		}
		var md string
		var version int
		err = tx.QueryRowContext(ctx, "SELECT markdown, version FROM "+s.T.T("entry_revisions")+" WHERE id = ? AND entry_id = ?", revID, e.ID).Scan(&md, &version)
		if errors.Is(err, sql.ErrNoRows) {
			return httpx.Validation(map[string]string{"revision_id": "That revision does not belong to this entry."})
		}
		if err != nil {
			return err
		}
		rev, err := catalog.CreateRevision(ctx, tx, s.T, e.ID, v.UserID, md, fmt.Sprintf("Restored from version %d", version))
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "UPDATE "+s.T.T("entries")+" SET current_draft_revision_id = ?, last_edited_at = ? WHERE id = ?", rev.ID, s.now(), e.ID); err != nil {
			return err
		}
		if err := s.audit(ctx, tx, r, w, "entry.restore", "entry", e.ID, nil, map[string]any{"from_version": version, "new_version": rev.Version}); err != nil {
			return err
		}
		out, err = s.loadDraft(ctx, tx, e.ID, v.UserID)
		return err
	})
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteData(w, r, http.StatusOK, out, nil)
}

// ---- release -----------------------------------------------------------------

type scheduleInput struct {
	PublicAt   *time.Time `json:"public_at"`
	EarlyAt    *time.Time `json:"early_at"`
	MinRank    *int       `json:"min_access_rank"`
	RevisionID *string    `json:"revision_id"`
}

type releasePlan struct {
	publicAt time.Time
	earlyAt  *time.Time
	minRank  int
	revID    uint64
}

// planRelease validates a schedule against the entry's current state
// (§3.2 items 2–4, §11.1 step 7). It never touches the database.
func (s *Service) planRelease(e *entryRow, in scheduleInput, revWords int, revID uint64, now time.Time, immediate bool) (*releasePlan, error) {
	fields := map[string]string{}
	p := &releasePlan{revID: revID}
	switch {
	case immediate:
		p.publicAt = now
	case in.PublicAt == nil:
		fields["public_at"] = "Every release needs a public date: everything becomes free eventually."
	default:
		p.publicAt = in.PublicAt.UTC().Truncate(time.Second)
	}
	if revWords == 0 {
		fields["markdown"] = "The text is empty. Write something before scheduling it."
	}
	if !immediate && in.EarlyAt != nil {
		early := in.EarlyAt.UTC().Truncate(time.Second)
		p.earlyAt = &early
	}
	if p.earlyAt != nil && len(fields) == 0 {
		switch {
		case p.earlyAt.After(p.publicAt):
			fields["early_at"] = "Early access must start before the public date."
		case p.publicAt.Sub(*p.earlyAt) > time.Duration(s.MaxEarlyDays)*24*time.Hour:
			fields["early_at"] = fmt.Sprintf("Early access can start at most %d days before the public date.", s.MaxEarlyDays)
		case p.earlyAt.Equal(p.publicAt):
			p.earlyAt = nil
		}
	}
	if p.earlyAt != nil {
		if in.MinRank == nil || *in.MinRank < 1 || *in.MinRank > 100 {
			fields["min_access_rank"] = "Choose the supporter tier rank (1 or higher) that unlocks early access."
		} else {
			p.minRank = *in.MinRank
		}
	}
	if !immediate && in.PublicAt != nil && p.publicAt.Before(now.Add(-5*time.Minute)) {
		fields["public_at"] = "That date is in the past. Use “Publish now” to release immediately."
	}
	if len(fields) > 0 {
		return nil, httpx.Validation(fields)
	}

	// Once readers (or supporters) could access the entry, its public
	// deadline may only move earlier, and an early window cannot be
	// reshaped after it opened: no perpetual premium (§3.2 item 2).
	var curEarly, curPublic *time.Time
	if e.EarlyAt.Valid {
		curEarly = &e.EarlyAt.Time
	}
	if e.PublicAt.Valid {
		curPublic = &e.PublicAt.Time
	}
	state := catalog.StateAt(catalog.Release{Workflow: e.Workflow, EarlyAt: curEarly, PublicAt: curPublic, HasPublished: e.PublishedRevID.Valid}, now)
	switch state {
	case catalog.AccessPublic:
		return nil, conflict("ALREADY_PUBLIC", "This entry is already public. Use “Update published text” to publish corrections.")
	case catalog.AccessEarly:
		if p.publicAt.After(*curPublic) {
			return nil, conflict("PUBLIC_DATE_LOCKED", "Early access has started, so the public date can only move earlier.")
		}
		if p.earlyAt == nil || !p.earlyAt.Equal(*curEarly) {
			if !immediate {
				return nil, conflict("EARLY_WINDOW_LOCKED", "Early access has already started; its start time cannot change.")
			}
		}
	}
	return p, nil
}

// applyRelease freezes the revision as published, sets dates and tier,
// refreshes rollups and enqueues follow-up jobs, all in tx.
func (s *Service) applyRelease(ctx context.Context, tx *sql.Tx, e *entryRow, p *releasePlan, now time.Time) error {
	workflow := "scheduled"
	if !p.publicAt.After(now) {
		workflow = "published"
	}
	if _, err := tx.ExecContext(ctx, "UPDATE "+s.T.T("entries")+" SET published_revision_id = ?, workflow = ?, early_at = ?, public_at = ? WHERE id = ?",
		p.revID, workflow, p.earlyAt, p.publicAt, e.ID); err != nil {
		return err
	}
	if p.earlyAt != nil {
		if _, err := tx.ExecContext(ctx, "INSERT INTO "+s.T.T("entry_access")+" (entry_id, owner_user_id, minimum_access_rank) VALUES (?,?,?)"+
			" ON DUPLICATE KEY UPDATE minimum_access_rank = VALUES(minimum_access_rank)", e.ID, e.OwnerID, p.minRank); err != nil {
			return err
		}
	} else if _, err := tx.ExecContext(ctx, "DELETE FROM "+s.T.T("entry_access")+" WHERE entry_id = ?", e.ID); err != nil {
		return err
	}
	if err := catalog.RefreshRollups(ctx, tx, s.T, e.WorkID, now); err != nil {
		return err
	}
	return s.enqueueReleaseJobs(ctx, tx, e.WorkID, e.ID, p.publicAt, now)
}

// enqueueReleaseJobs schedules the rollup refresh (and reader notification)
// for the public moment. Access itself never waits for these jobs.
func (s *Service) enqueueReleaseJobs(ctx context.Context, tx *sql.Tx, workID, entryID uint64, publicAt, now time.Time) error {
	if s.Jobs == nil {
		return nil
	}
	runAt := publicAt
	if runAt.Before(now) {
		runAt = now
	}
	stamp := publicAt.UTC().Format("20060102T150405Z")
	_, err := s.Jobs.EnqueueTx(ctx, tx, KindUpdateWorkRollups, map[string]string{"work_id": idStr(workID)},
		collar.EnqueueOptions{RunAt: runAt, UniqueKey: "work:" + idStr(workID) + ":public:" + stamp, MaxAttempts: 10})
	_ = entryID // reader notifications are enqueued here once the library phase lands
	return err
}

func releaseSnapshot(e *entryRow) map[string]any {
	m := map[string]any{"workflow": e.Workflow, "early_at": nil, "public_at": nil, "published_revision_id": nil, "min_access_rank": nil}
	if e.EarlyAt.Valid {
		m["early_at"] = e.EarlyAt.Time
	}
	if e.PublicAt.Valid {
		m["public_at"] = e.PublicAt.Time
	}
	if e.PublishedRevID.Valid {
		m["published_revision_id"] = strconv.FormatInt(e.PublishedRevID.Int64, 10)
	}
	if e.MinRank.Valid {
		m["min_access_rank"] = e.MinRank.Int64
	}
	return m
}

// revisionFor resolves the revision to freeze (default: current draft) and
// its word count.
func (s *Service) revisionFor(ctx context.Context, tx *sql.Tx, e *entryRow, raw *string) (uint64, int, error) {
	revID := uint64(e.DraftRevID.Int64)
	if raw != nil && *raw != "" {
		id, err := strconv.ParseUint(*raw, 10, 64)
		if err != nil {
			return 0, 0, httpx.Validation(map[string]string{"revision_id": "Choose a revision."})
		}
		revID = id
	}
	var words int
	err := tx.QueryRowContext(ctx, "SELECT word_count FROM "+s.T.T("entry_revisions")+" WHERE id = ? AND entry_id = ?", revID, e.ID).Scan(&words)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, httpx.Validation(map[string]string{"revision_id": "That revision does not belong to this entry."})
	}
	return revID, words, err
}

func (s *Service) release(w http.ResponseWriter, r *http.Request, immediate bool) {
	ctx := r.Context()
	v := auth.ViewerFrom(ctx)
	id, err := pathID(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	var in scheduleInput
	if r.ContentLength != 0 {
		if err := httpx.DecodeJSON(w, r, &in, httpx.MaxJSONBody); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
	}
	var out *Draft
	err = db.InTx(ctx, s.DB, func(tx *sql.Tx) error {
		e, err := s.lockEntry(ctx, tx, id, v.UserID)
		if err != nil {
			return err
		}
		revID, words, err := s.revisionFor(ctx, tx, e, in.RevisionID)
		if err != nil {
			return err
		}
		now := s.now()
		plan, err := s.planRelease(e, in, words, revID, now, immediate)
		if err != nil {
			return err
		}
		before := releaseSnapshot(e)
		if err := s.applyRelease(ctx, tx, e, plan, now); err != nil {
			return err
		}
		action := "entry.schedule"
		if immediate {
			action = "entry.publish_now"
		}
		after := map[string]any{"public_at": plan.publicAt, "early_at": plan.earlyAt, "min_access_rank": plan.minRank, "published_revision_id": idStr(plan.revID)}
		if err := s.audit(ctx, tx, r, w, action, "entry", e.ID, before, after); err != nil {
			return err
		}
		out, err = s.loadDraft(ctx, tx, e.ID, v.UserID)
		return err
	})
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out.Markdown = ""
	httpx.WriteData(w, r, http.StatusOK, out, nil)
}

func (s *Service) schedule(w http.ResponseWriter, r *http.Request)   { s.release(w, r, false) }
func (s *Service) publishNow(w http.ResponseWriter, r *http.Request) { s.release(w, r, true) }

// publishRevision swaps the published text of an already released entry
// (corrections after release, §11.1 step 9). Dates are untouched, so it does
// not bump "recently updated" (PUB-08); the swap is audited.
func (s *Service) publishRevision(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	v := auth.ViewerFrom(ctx)
	id, err := pathID(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	var in scheduleInput
	if r.ContentLength != 0 {
		if err := httpx.DecodeJSON(w, r, &in, httpx.MaxJSONBody); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
	}
	var out *Draft
	err = db.InTx(ctx, s.DB, func(tx *sql.Tx) error {
		e, err := s.lockEntry(ctx, tx, id, v.UserID)
		if err != nil {
			return err
		}
		if e.Workflow != "scheduled" && e.Workflow != "published" {
			return conflict("NOT_RELEASED", "Schedule or publish the entry first.")
		}
		revID, words, err := s.revisionFor(ctx, tx, e, in.RevisionID)
		if err != nil {
			return err
		}
		if words == 0 {
			return httpx.Validation(map[string]string{"markdown": "The text is empty."})
		}
		if _, err := tx.ExecContext(ctx, "UPDATE "+s.T.T("entries")+" SET published_revision_id = ? WHERE id = ?", revID, e.ID); err != nil {
			return err
		}
		if err := catalog.RefreshRollups(ctx, tx, s.T, e.WorkID, s.now()); err != nil {
			return err
		}
		if err := s.audit(ctx, tx, r, w, "entry.publish_revision", "entry", e.ID,
			map[string]any{"published_revision_id": e.PublishedRevID.Int64}, map[string]any{"published_revision_id": idStr(revID)}); err != nil {
			return err
		}
		out, err = s.loadDraft(ctx, tx, e.ID, v.UserID)
		return err
	})
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out.Markdown = ""
	httpx.WriteData(w, r, http.StatusOK, out, nil)
}

// unpublish is the exceptional, audited withdrawal of a released entry
// (§3.2 item 8). The entry, its revisions and readers' progress remain.
func (s *Service) unpublish(w http.ResponseWriter, r *http.Request) {
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
		e, err := s.lockEntry(ctx, tx, id, v.UserID)
		if err != nil {
			return err
		}
		if e.Workflow != "scheduled" && e.Workflow != "published" {
			return conflict("NOT_RELEASED", "Only released entries can be unpublished.")
		}
		if _, err := tx.ExecContext(ctx, "UPDATE "+s.T.T("entries")+" SET workflow = 'archived' WHERE id = ?", e.ID); err != nil {
			return err
		}
		if err := catalog.RefreshRollups(ctx, tx, s.T, e.WorkID, s.now()); err != nil {
			return err
		}
		return s.audit(ctx, tx, r, w, "entry.unpublish", "entry", e.ID, releaseSnapshot(e), map[string]string{"workflow": "archived", "reason": in.Reason})
	})
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- bulk scheduling -------------------------------------------------------------

type bulkInput struct {
	EntryIDs      []string  `json:"entry_ids"`
	Anchor        time.Time `json:"anchor"`
	IntervalDays  int       `json:"interval_days"`
	EarlyLeadDays int       `json:"early_lead_days"`
	MinRank       *int      `json:"min_access_rank"`
	Apply         bool      `json:"apply"`
}

type bulkItem struct {
	EntryID  string     `json:"entry_id"`
	Ordinal  int        `json:"ordinal"`
	Title    string     `json:"title"`
	PublicAt time.Time  `json:"public_at"`
	EarlyAt  *time.Time `json:"early_at"`
	Problem  string     `json:"problem,omitempty"`
}

// bulkSchedule previews (apply=false) or applies, all or nothing, a cadence
// of release dates for a novel's chapters (§11.2). Empty drafts are refused.
func (s *Service) bulkSchedule(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	v := auth.ViewerFrom(ctx)
	workID, err := pathID(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	var in bulkInput
	if err := httpx.DecodeJSON(w, r, &in, httpx.MaxJSONBody); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	fields := map[string]string{}
	if in.Anchor.IsZero() {
		fields["anchor"] = "Choose the first public date."
	}
	if in.IntervalDays < 1 || in.IntervalDays > 60 {
		fields["interval_days"] = "Use 1 to 60 days between releases."
	}
	if in.EarlyLeadDays < 0 || in.EarlyLeadDays > s.MaxEarlyDays {
		fields["early_lead_days"] = fmt.Sprintf("Use 0 to %d days of early access.", s.MaxEarlyDays)
	}
	if len(in.EntryIDs) > 200 {
		fields["entry_ids"] = "Schedule at most 200 chapters at once."
	}
	if len(fields) > 0 {
		httpx.WriteError(w, r, httpx.Validation(fields))
		return
	}
	var items []bulkItem
	err = db.InTx(ctx, s.DB, func(tx *sql.Tx) error {
		wr, err := s.lockWork(ctx, tx, workID, v.UserID)
		if err != nil {
			return err
		}
		if wr.Kind != "novel" {
			return conflict("NOT_A_NOVEL", "Bulk scheduling is for novel chapters.")
		}
		want := map[uint64]bool{}
		for _, raw := range in.EntryIDs {
			id, err := strconv.ParseUint(raw, 10, 64)
			if err != nil {
				return httpx.Validation(map[string]string{"entry_ids": "Invalid chapter id."})
			}
			want[id] = true
		}
		q := "SELECT id FROM " + s.T.T("entries") + " WHERE work_id = ? AND workflow IN ('draft','archived') ORDER BY ordinal"
		ids, err := s.orderedIDs(ctx, tx, q, wr.ID)
		if err != nil {
			return err
		}
		now := s.now()
		problems := 0
		at := in.Anchor.UTC().Truncate(time.Second)
		for _, id := range ids {
			if len(want) > 0 && !want[id] {
				continue
			}
			e, err := s.lockEntry(ctx, tx, id, v.UserID)
			if err != nil {
				return err
			}
			revID, words, err := s.revisionFor(ctx, tx, e, nil)
			if err != nil {
				return err
			}
			pub := at
			si := scheduleInput{PublicAt: &pub, MinRank: in.MinRank}
			if in.EarlyLeadDays > 0 {
				early := pub.Add(-time.Duration(in.EarlyLeadDays) * 24 * time.Hour)
				si.EarlyAt = &early
			}
			item := bulkItem{EntryID: idStr(id), Ordinal: e.Ordinal, Title: e.Title, PublicAt: pub, EarlyAt: si.EarlyAt}
			plan, perr := s.planRelease(e, si, words, revID, now, false)
			if perr != nil {
				var ae *httpx.Error
				if errors.As(perr, &ae) {
					item.Problem = ae.Message
					if f, ok := ae.Details.(map[string]any); ok {
						if ff, ok := f["fields"].(map[string]string); ok {
							for _, m := range ff {
								item.Problem = m
								break
							}
						}
					}
				}
				problems++
			} else if in.Apply {
				before := releaseSnapshot(e)
				if err := s.applyRelease(ctx, tx, e, plan, now); err != nil {
					return err
				}
				if err := s.audit(ctx, tx, r, w, "entry.schedule.bulk", "entry", e.ID, before, map[string]any{"public_at": plan.publicAt, "early_at": plan.earlyAt}); err != nil {
					return err
				}
			}
			items = append(items, item)
			at = at.Add(time.Duration(in.IntervalDays) * 24 * time.Hour)
		}
		if len(items) == 0 {
			return httpx.Validation(map[string]string{"entry_ids": "There are no unreleased chapters to schedule."})
		}
		if in.Apply && problems > 0 {
			return httpx.NewError(http.StatusUnprocessableEntity, "BULK_SCHEDULE_INVALID", "Some chapters cannot be scheduled; nothing was changed.").
				WithDetails(map[string]any{"items": items})
		}
		return nil
	})
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteData(w, r, http.StatusOK, map[string]any{"applied": in.Apply, "items": items}, nil)
}
