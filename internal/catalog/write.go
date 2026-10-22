package catalog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/namankumar80510/panw.ink/internal/db"
	"github.com/namankumar80510/panw.ink/internal/render"
)

// Execer is *sql.DB or *sql.Tx.
type Execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// ErrEntryNotFound is returned when a revision targets a missing entry.
var ErrEntryNotFound = errors.New("catalog: entry not found")

// Revision is a newly created immutable revision.
type Revision struct {
	ID      uint64
	Version int
	Render  *render.Result
}

// CreateRevision renders markdown and inserts the entry's next immutable
// revision inside tx. The entry row is locked first, so concurrent writers
// get consecutive versions. Revisions are never updated afterwards.
func CreateRevision(ctx context.Context, tx *sql.Tx, t db.Tables, entryID, editorID uint64, markdown, note string) (*Revision, error) {
	var owner uint64
	err := tx.QueryRowContext(ctx, "SELECT owner_user_id FROM "+t.T("entries")+" WHERE id = ? FOR UPDATE", entryID).Scan(&owner)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrEntryNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("catalog: lock entry: %w", err)
	}
	r, err := render.Markdown(markdown)
	if err != nil {
		return nil, err
	}
	var version int
	if err := tx.QueryRowContext(ctx, "SELECT COALESCE(MAX(version), 0) + 1 FROM "+t.T("entry_revisions")+" WHERE entry_id = ?", entryID).Scan(&version); err != nil {
		return nil, fmt.Errorf("catalog: next version: %w", err)
	}
	var notePtr any
	if note != "" {
		notePtr = note
	}
	res, err := tx.ExecContext(ctx, "INSERT INTO "+t.T("entry_revisions")+
		" (entry_id, owner_user_id, version, markdown, rendered_html, plain_text, word_count, estimated_read_seconds, checksum, editor_user_id, change_note)"+
		" VALUES (?,?,?,?,?,?,?,?,?,?,?)",
		entryID, owner, version, markdown, r.HTML, r.PlainText, r.WordCount, r.ReadSeconds, r.Checksum, editorID, notePtr)
	if err != nil {
		return nil, fmt.Errorf("catalog: insert revision: %w", err)
	}
	id, _ := res.LastInsertId()
	return &Revision{ID: uint64(id), Version: version, Render: r}, nil
}

// released is the SQL condition for entries that have been released.
const released = "e.workflow IN ('scheduled','published') AND e.published_revision_id IS NOT NULL"

// RefreshRollups recomputes a work's listing rollups as of now. Rollups
// order and annotate listings only; access is always computed per request
// from entry timestamps. Corrections to released text do not change
// public_at, so they never bump "recently updated" (PUB-08).
func RefreshRollups(ctx context.Context, x Execer, t db.Tables, workID uint64, now time.Time) error {
	e, r := t.T("entries"), t.T("entry_revisions")
	q := "UPDATE " + t.T("works") + " w SET" +
		" first_public_at = (SELECT MIN(e.public_at) FROM " + e + " e WHERE e.work_id = w.id AND " + released + ")," +
		" last_public_update_at = (SELECT MAX(e.public_at) FROM " + e + " e WHERE e.work_id = w.id AND " + released + " AND e.public_at <= ?)," +
		" public_entry_count = (SELECT COUNT(*) FROM " + e + " e WHERE e.work_id = w.id AND " + released + " AND e.public_at <= ?)," +
		" public_word_count = (SELECT COALESCE(SUM(r.word_count), 0) FROM " + e + " e JOIN " + r + " r ON r.id = e.published_revision_id WHERE e.work_id = w.id AND " + released + " AND e.public_at <= ?)," +
		" rollups_at = ? WHERE w.id = ?"
	if _, err := x.ExecContext(ctx, q, now, now, now, now, workID); err != nil {
		return fmt.Errorf("catalog: rollups for work %d: %w", workID, err)
	}
	return nil
}
