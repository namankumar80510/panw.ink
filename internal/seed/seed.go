// Package seed loads the deterministic development fixtures of
// IMPLEMENTATION.md §16: fictional metadata and synthetic prose written for
// this repository (no external copyrighted text). All release dates are
// relative to the supplied clock, so tests never depend on wall-clock time.
//
// It is used by cmd/seed (development/test only) and the integration tests.
package seed

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/namankumar80510/panw.ink/internal/auth"
	"github.com/namankumar80510/panw.ink/internal/catalog"
	"github.com/namankumar80510/panw.ink/internal/db"
)

// Fixture accounts. Their shared development password is documented in
// tests/fixtures/README.md and can be overridden with SEED_PASSWORD.
const (
	OwnerEmail      = "owner@example.invalid"
	ReaderEmail     = "reader1@example.invalid"
	UnverifiedEmail = "reader2@example.invalid"
	DefaultPassword = "pawn-dev-fixture-passphrase"
)

// ErrAlreadySeeded is returned when the fixtures are already present.
var ErrAlreadySeeded = errors.New("seed: fixtures already present")

// Times are the fixture release times relative to now.
type Times struct {
	Now time.Time
}

func (t Times) at(d time.Duration) *time.Time {
	x := t.Now.Add(d).UTC().Truncate(time.Second)
	return &x
}

const day = 24 * time.Hour

type entryFixture struct {
	slug, title      string
	ordinal          int
	volume           int // 0 = none
	workflow         string
	early, public    *time.Time
	paragraphs, salt int
	extra            string // appended Markdown (sanitization fixture)
}

type workFixture struct {
	slug, title, kind, pen, genre, status, visibility, synopsis, rating string
	featured                                                            bool
	warnings                                                            []string
	tags                                                                []string
	volumes                                                             []string
	entries                                                             []entryFixture
}

// Run inserts the fixtures. It refuses (ErrAlreadySeeded) when the
// raven-vale pen name exists, so it is safe to run repeatedly.
func Run(ctx context.Context, database *sql.DB, t db.Tables, now time.Time, password string) error {
	if password == "" {
		password = DefaultPassword
	}
	var n int
	if err := database.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+t.T("pen_names")+" WHERE slug = 'raven-vale'").Scan(&n); err != nil {
		return fmt.Errorf("seed: %w", err)
	}
	if n > 0 {
		return ErrAlreadySeeded
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	tm := Times{Now: now.UTC()}

	return db.InTx(ctx, database, func(tx *sql.Tx) error {
		var owners int
		if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+t.T("users")+" WHERE role = 'owner' AND state <> 'deleted'").Scan(&owners); err != nil {
			return err
		}
		ownerRole := auth.RoleOwner
		if owners > 0 {
			ownerRole = auth.RoleAuthor // never create a second owner
		}
		ownerID, err := user(ctx, tx, t, OwnerEmail, "Fixture Owner", ownerRole, hash, true)
		if err != nil {
			return err
		}
		if _, err := user(ctx, tx, t, ReaderEmail, "Reader One", auth.RoleReader, hash, true); err != nil {
			return err
		}
		if _, err := user(ctx, tx, t, UnverifiedEmail, "Reader Two", auth.RoleReader, hash, false); err != nil {
			return err
		}

		pens := map[string]uint64{}
		for _, p := range [][3]string{
			{"raven-vale", "Raven Vale", "Raven Vale writes long-form fantasy about archives, debts and the people who keep them."},
			{"a-n-other", "A. N. Other", "Short, strange fiction: locked rooms, wrong doors and the occasional unreliable narrator."},
		} {
			res, err := tx.ExecContext(ctx, "INSERT INTO "+t.T("pen_names")+" (owner_user_id, slug, name, bio) VALUES (?,?,?,?)", ownerID, p[0], p[1], p[2])
			if err != nil {
				return fmt.Errorf("seed: pen name %s: %w", p[0], err)
			}
			id, _ := res.LastInsertId()
			pens[p[0]] = uint64(id)
		}

		tags := map[string]uint64{}
		for _, tg := range [][3]string{
			{"found-family", "Found family", "approved"}, {"slow-burn", "Slow burn", "approved"},
			{"locked-room", "Locked room", "approved"}, {"unreliable-narrator", "Unreliable narrator", "approved"},
			{"gothic", "Gothic", "approved"}, {"archives", "Archives", "approved"}, {"proposed-tag", "Proposed tag", "proposed"},
		} {
			res, err := tx.ExecContext(ctx, "INSERT INTO "+t.T("tags")+" (slug, label, state) VALUES (?,?,?)", tg[0], tg[1], tg[2])
			if err != nil {
				return fmt.Errorf("seed: tag %s: %w", tg[0], err)
			}
			id, _ := res.LastInsertId()
			tags[tg[0]] = uint64(id)
		}

		works := []workFixture{
			{
				slug: "holunkai", title: "Holunkai", kind: "novel", pen: "raven-vale", genre: "fantasy", status: "ongoing", visibility: "listed", featured: true,
				synopsis: "When the drowned archive of Holunkai surfaces for a single night, an apprentice cataloguer steals the one book that was never meant to be read — and discovers it is reading her back.",
				rating:   "teen", warnings: []string{"peril"}, tags: []string{"found-family", "archives", "slow-burn", "proposed-tag"},
				volumes: []string{"The Drowned Archive"},
				entries: []entryFixture{
					{slug: "the-last-breath", title: "The Last Breath", ordinal: 1, volume: 1, workflow: "published", public: tm.at(-10 * day), paragraphs: 9, salt: 1},
					{slug: "the-wrong-savior", title: "The Wrong Savior", ordinal: 2, volume: 1, workflow: "published", public: tm.at(-3 * day), paragraphs: 8, salt: 2,
						// Sanitization fixture (CAT-06): raw HTML in Markdown must never execute.
						extra: "\n\n<script>alert('fixture')</script>\n\n[a link](javascript:alert(1))"},
					{slug: "the-first-specimen", title: "The First Specimen", ordinal: 3, volume: 1, workflow: "scheduled", early: tm.at(-1 * time.Hour), public: tm.at(6 * day), paragraphs: 8, salt: 3},
					{slug: "an-unfinished-thing", title: "An Unfinished Thing", ordinal: 4, workflow: "draft", paragraphs: 3, salt: 4},
					{slug: "a-cut-scene", title: "A Cut Scene", ordinal: 5, workflow: "archived", public: tm.at(-2 * day), paragraphs: 3, salt: 5},
				},
			},
			{
				slug: "the-locked-study", title: "The Locked Study", kind: "short_story", pen: "a-n-other", genre: "mystery", status: "completed", visibility: "listed",
				synopsis: "A professor is found in a study locked from the inside. The only witness is the study itself, and it has opinions.",
				rating:   "general", tags: []string{"locked-room", "unreliable-narrator"},
				entries: []entryFixture{{slug: "full-story", title: "The Locked Study", ordinal: 1, workflow: "published", public: tm.at(-20 * day), paragraphs: 12, salt: 6}},
			},
			{
				slug: "beyond-the-door", title: "Beyond the Door", kind: "short_story", pen: "a-n-other", genre: "horror", status: "completed", visibility: "listed",
				synopsis: "Every house on Calder Street has one door too many. Tonight, Mira opens hers.",
				rating:   "teen", warnings: []string{"body horror"}, tags: []string{"gothic"},
				entries: []entryFixture{{slug: "full-story", title: "Beyond the Door", ordinal: 1, workflow: "scheduled", early: tm.at(-2 * time.Hour), public: tm.at(5 * day), paragraphs: 10, salt: 7}},
			},
			{
				// Hidden unpublished draft work: its released-looking entry must
				// never be reachable (CAT-05).
				slug: "the-quiet-manuscript", title: "The Quiet Manuscript", kind: "short_story", pen: "raven-vale", genre: "literary", status: "ongoing", visibility: "draft",
				synopsis: "Not ready yet.", rating: "general",
				entries: []entryFixture{{slug: "full-story", title: "The Quiet Manuscript", ordinal: 1, workflow: "published", public: tm.at(-5 * day), paragraphs: 2, salt: 8}},
			},
			{
				slug: "a-withdrawn-tale", title: "A Withdrawn Tale", kind: "short_story", pen: "a-n-other", genre: "drama", status: "completed", visibility: "hidden",
				synopsis: "Hidden by moderation.", rating: "general",
				entries: []entryFixture{{slug: "full-story", title: "A Withdrawn Tale", ordinal: 1, workflow: "published", public: tm.at(-6 * day), paragraphs: 2, salt: 9}},
			},
		}

		workIDs := map[string]uint64{}
		for _, wf := range works {
			id, err := insertWork(ctx, tx, t, ownerID, pens[wf.pen], tags, wf)
			if err != nil {
				return err
			}
			workIDs[wf.slug] = id
		}

		res, err := tx.ExecContext(ctx, "INSERT INTO "+t.T("collections")+" (pen_name_id, owner_user_id, slug, title, description, visibility) VALUES (?,?,?,?,?, 'listed')",
			pens["a-n-other"], ownerID, "unsettling-tales", "Unsettling Tales", "Short stories for reading with the lights on.")
		if err != nil {
			return fmt.Errorf("seed: collection: %w", err)
		}
		cid, _ := res.LastInsertId()
		for i, slug := range []string{"the-locked-study", "beyond-the-door"} {
			if _, err := tx.ExecContext(ctx, "INSERT INTO "+t.T("collection_items")+" (collection_id, work_id, ordinal) VALUES (?,?,?)", cid, workIDs[slug], i+1); err != nil {
				return fmt.Errorf("seed: collection item: %w", err)
			}
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO "+t.T("slug_redirects")+" (entity_type, old_path, new_path) VALUES ('work', '/fiction/holunkai-old', '/fiction/holunkai')"); err != nil {
			return fmt.Errorf("seed: redirect: %w", err)
		}
		for _, id := range workIDs {
			if err := catalog.RefreshRollups(ctx, tx, t, id, tm.Now); err != nil {
				return err
			}
		}
		return nil
	})
}

func user(ctx context.Context, tx *sql.Tx, t db.Tables, email, name, role, hash string, verified bool) (uint64, error) {
	var id uint64
	err := tx.QueryRowContext(ctx, "SELECT id FROM "+t.T("users")+" WHERE email = ?", email).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	var verifiedAt any
	if verified {
		verifiedAt = time.Now().UTC()
	}
	res, err := tx.ExecContext(ctx, "INSERT INTO "+t.T("users")+" (email, display_name, password_hash, role, email_verified_at) VALUES (?,?,?,?,?)",
		email, name, hash, role, verifiedAt)
	if err != nil {
		return 0, fmt.Errorf("seed: user %s: %w", email, err)
	}
	n, _ := res.LastInsertId()
	return uint64(n), nil
}

func insertWork(ctx context.Context, tx *sql.Tx, t db.Tables, ownerID, penID uint64, tags map[string]uint64, wf workFixture) (uint64, error) {
	var genreID uint64
	if err := tx.QueryRowContext(ctx, "SELECT id FROM "+t.T("genres")+" WHERE slug = ?", wf.genre).Scan(&genreID); err != nil {
		return 0, fmt.Errorf("seed: genre %s: %w", wf.genre, err)
	}
	var warnings any
	if len(wf.warnings) > 0 {
		warnings = `["` + strings.Join(wf.warnings, `","`) + `"]`
	}
	var completedAt any
	if wf.status == "completed" {
		completedAt = time.Now().UTC()
	}
	res, err := tx.ExecContext(ctx, "INSERT INTO "+t.T("works")+
		" (pen_name_id, owner_user_id, kind, slug, title, synopsis, primary_genre_id, publication_status, visibility, age_rating, content_warnings_json, is_featured, completed_at)"+
		" VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)",
		penID, ownerID, wf.kind, wf.slug, wf.title, wf.synopsis, genreID, wf.status, wf.visibility, wf.rating, warnings, wf.featured, completedAt)
	if err != nil {
		return 0, fmt.Errorf("seed: work %s: %w", wf.slug, err)
	}
	n, _ := res.LastInsertId()
	workID := uint64(n)
	for _, tg := range wf.tags {
		if _, err := tx.ExecContext(ctx, "INSERT INTO "+t.T("work_tags")+" (work_id, tag_id) VALUES (?,?)", workID, tags[tg]); err != nil {
			return 0, err
		}
	}
	volumeIDs := map[int]uint64{}
	for i, title := range wf.volumes {
		res, err := tx.ExecContext(ctx, "INSERT INTO "+t.T("volumes")+" (work_id, owner_user_id, number, title) VALUES (?,?,?,?)", workID, ownerID, i+1, title)
		if err != nil {
			return 0, fmt.Errorf("seed: volume: %w", err)
		}
		v, _ := res.LastInsertId()
		volumeIDs[i+1] = uint64(v)
	}
	for _, ef := range wf.entries {
		var volume any
		if ef.volume > 0 {
			volume = volumeIDs[ef.volume]
		}
		res, err := tx.ExecContext(ctx, "INSERT INTO "+t.T("entries")+" (work_id, work_kind, owner_user_id, volume_id, ordinal, slug, title) VALUES (?,?,?,?,?,?,?)",
			workID, wf.kind, ownerID, volume, ef.ordinal, ef.slug, ef.title)
		if err != nil {
			return 0, fmt.Errorf("seed: entry %s/%s: %w", wf.slug, ef.slug, err)
		}
		eid, _ := res.LastInsertId()
		rev, err := catalog.CreateRevision(ctx, tx, t, uint64(eid), ownerID, Prose(ef.title, ef.paragraphs, ef.salt)+ef.extra, "fixture")
		if err != nil {
			return 0, err
		}
		var published any
		if ef.workflow != "draft" {
			published = rev.ID
		}
		_, err = tx.ExecContext(ctx, "UPDATE "+t.T("entries")+" SET current_draft_revision_id = ?, published_revision_id = ?, workflow = ?, early_at = ?, public_at = ? WHERE id = ?",
			rev.ID, published, ef.workflow, ef.early, ef.public, eid)
		if err != nil {
			return 0, fmt.Errorf("seed: release %s/%s: %w", wf.slug, ef.slug, err)
		}
	}
	return workID, nil
}
