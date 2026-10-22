// Package studio implements the publishing studio's business commands
// (IMPLEMENTATION.md §7.5): creates, ordering, drafts and revisions,
// scheduling, completion, collections and asset validation. Dashboard
// listings and safe metadata edits use Collar's native API directly; only
// operations that need multi-row transactions, locking, sanitization or audit
// live here.
//
// Every command loads its target with owner_user_id = caller under a row
// lock; another author's work is indistinguishable from a missing one (404).
package studio

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/namankumar80510/panw.ink/internal/auth"
	"github.com/namankumar80510/panw.ink/internal/db"
	"github.com/namankumar80510/panw.ink/internal/httpx"
	"github.com/namankumar80510/panw.ink/internal/mail"
	"github.com/namankumar80510/panw.ink/internal/objstore"
)

// Service holds the studio's collaborators.
type Service struct {
	DB           *sql.DB
	T            db.Tables
	Jobs         mail.Enqueuer
	Store        *objstore.Client
	Now          func() time.Time
	MaxEarlyDays int
	Log          *slog.Logger
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

// Base of studio commands.
const Base = auth.Base + "/studio"

// Editor bodies may carry up to 2 MB of Markdown plus JSON escaping.
const maxDraftBody = 3 << 20

// Mount registers the commands.
func (s *Service) Mount(r auth.Router) error {
	routes := map[string]http.HandlerFunc{
		"POST " + Base + "/pen-names":                     s.createPenName,
		"POST " + Base + "/works":                         s.createWork,
		"POST " + Base + "/works/{id}/initialize-story":   s.initializeStory,
		"PUT " + Base + "/works/{id}/tags":                s.setTags,
		"POST " + Base + "/works/{id}/complete":           s.completeWork,
		"POST " + Base + "/works/{id}/archive":            s.archiveWork,
		"POST " + Base + "/works/{id}/volumes":            s.createVolume,
		"POST " + Base + "/works/{id}/entries":            s.createEntry,
		"POST " + Base + "/works/{id}/bulk-schedule":      s.bulkSchedule,
		"POST " + Base + "/volumes/{id}/reorder":          s.reorderVolume,
		"GET " + Base + "/entries/{id}/draft":             s.getDraft,
		"PUT " + Base + "/entries/{id}/draft":             s.saveDraft,
		"POST " + Base + "/entries/{id}/preview":          s.preview,
		"POST " + Base + "/entries/{id}/restore":          s.restore,
		"POST " + Base + "/entries/{id}/schedule":         s.schedule,
		"POST " + Base + "/entries/{id}/publish-now":      s.publishNow,
		"POST " + Base + "/entries/{id}/publish-revision": s.publishRevision,
		"POST " + Base + "/entries/{id}/unpublish":        s.unpublish,
		"POST " + Base + "/entries/{id}/reorder":          s.reorderEntry,
		"POST " + Base + "/collections":                   s.createCollection,
		"PUT " + Base + "/collections/{id}/items":         s.setCollectionItems,
		"POST " + Base + "/assets":                        s.registerAsset,
		"POST " + Base + "/assets/{id}/validate":          s.validateAsset,
	}
	for pattern, fn := range routes {
		if err := r.HandleFunc(pattern, s.requireStudio(fn)); err != nil {
			return err
		}
	}
	return r.HandleFunc("GET "+auth.Base+"/assets/{id}/{variant}", s.serveAsset)
}

// requireStudio admits authors and owners with a verified email.
func (s *Service) requireStudio(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		v := auth.ViewerFrom(r.Context())
		switch {
		case v == nil:
			httpx.WriteError(w, r, httpx.ErrUnauthenticated)
		case !v.CanUseStudio():
			httpx.WriteError(w, r, httpx.ErrForbidden)
		case !v.EmailVerified:
			httpx.WriteError(w, r, httpx.ErrEmailUnverified)
		default:
			w.Header().Set("Cache-Control", "private, no-store")
			next(w, r)
		}
	}
}

// ---- helpers ----------------------------------------------------------------

func pathID(r *http.Request) (uint64, error) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		return 0, httpx.ErrNotFound
	}
	return id, nil
}

// conflict is a 409 with a stable code.
func conflict(code, msg string) *httpx.Error {
	return httpx.NewError(http.StatusConflict, code, msg)
}

var (
	errWorkHidden = httpx.NewError(http.StatusForbidden, "WORK_HIDDEN", "This work is hidden by moderation and cannot be changed.")
	slugRE        = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	langRE        = regexp.MustCompile(`^[a-z]{2,3}(-[A-Za-z0-9]{2,8})?$`)
)

// reservedSlugs cannot name works, pen names or collections (they would
// shadow routes or confuse readers).
var reservedSlugs = map[string]bool{
	"new": true, "edit": true, "admin": true, "studio": true, "api": true, "read": true, "ch": true,
	"login": true, "register": true, "settings": true, "search": true, "discover": true, "about": true,
	"support": true, "policies": true, "library": true, "history": true, "notifications": true,
}

func checkSlug(fields map[string]string, key, slug string, minLen, maxLen int) {
	switch {
	case !slugRE.MatchString(slug) || len(slug) < minLen || len(slug) > maxLen:
		fields[key] = "Use " + strconv.Itoa(minLen) + "–" + strconv.Itoa(maxLen) + " lowercase letters, digits and single hyphens."
	case reservedSlugs[slug]:
		fields[key] = "This address is reserved."
	}
}

func checkText(fields map[string]string, key, v string, minLen, maxLen int) string {
	v = strings.TrimSpace(v)
	if n := utf8.RuneCountInString(v); n < minLen || n > maxLen {
		if minLen > 0 {
			fields[key] = "Use " + strconv.Itoa(minLen) + " to " + strconv.Itoa(maxLen) + " characters."
		} else {
			fields[key] = "Use at most " + strconv.Itoa(maxLen) + " characters."
		}
	}
	return v
}

// Slugify makes a URL slug from a title (ASCII letters and digits).
func Slugify(title string, maxLen int) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(title) {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		case unicode.IsSpace(r) || r == '-' || r == '_' || unicode.IsPunct(r):
			if !dash && b.Len() > 0 {
				b.WriteByte('-')
				dash = true
			}
		}
		if b.Len() >= maxLen {
			break
		}
	}
	return strings.Trim(b.String(), "-")
}

func isDuplicate(err error, key string) bool {
	return err != nil && strings.Contains(err.Error(), "Duplicate entry") && (key == "" || strings.Contains(err.Error(), key))
}

func jsonOf(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

func idStr(n uint64) string { return strconv.FormatUint(n, 10) }

// audit writes an audit row for the caller.
func (s *Service) audit(ctx context.Context, tx *sql.Tx, r *http.Request, w http.ResponseWriter, action, objectType string, objectID uint64, before, after any) error {
	v := auth.ViewerFrom(ctx)
	var b, a []byte
	if before != nil {
		b = jsonOf(before)
	}
	if after != nil {
		a = jsonOf(after)
	}
	uid := v.UserID
	return db.Audit(ctx, tx, s.T, &uid, action, objectType, &objectID, b, a, httpx.RequestID(w, r))
}

// workRow is a locked, owned work.
type workRow struct {
	ID, PenNameID, OwnerID uint64
	Kind, Slug, Title      string
	Visibility, Status     string
	Deleted                bool
}

// lockWork loads one of the caller's works FOR UPDATE.
func (s *Service) lockWork(ctx context.Context, tx *sql.Tx, workID, owner uint64) (*workRow, error) {
	var w workRow
	var deleted sql.NullString
	err := tx.QueryRowContext(ctx, "SELECT id, pen_name_id, owner_user_id, kind, slug, title, visibility, publication_status, deleted_at FROM "+s.T.T("works")+
		" WHERE id = ? AND owner_user_id = ? FOR UPDATE", workID, owner).
		Scan(&w.ID, &w.PenNameID, &w.OwnerID, &w.Kind, &w.Slug, &w.Title, &w.Visibility, &w.Status, &deleted)
	if errors.Is(err, sql.ErrNoRows) || err == nil && deleted.Valid {
		return nil, httpx.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if w.Visibility == "hidden" {
		return nil, errWorkHidden
	}
	return &w, nil
}

// entryRow is a locked, owned entry with its work.
type entryRow struct {
	ID, WorkID, OwnerID        uint64
	WorkKind, Slug, Title      string
	Ordinal                    int
	Workflow                   string
	EarlyAt, PublicAt          db.Time
	DraftRevID, PublishedRevID sql.NullInt64
	MinRank                    sql.NullInt64
	Work                       *workRow
}

// lockEntry locks the entry's work first, then the entry (consistent lock
// order with every work-level command, so commands on one work serialize).
func (s *Service) lockEntry(ctx context.Context, tx *sql.Tx, entryID, owner uint64) (*entryRow, error) {
	var workID uint64
	err := tx.QueryRowContext(ctx, "SELECT work_id FROM "+s.T.T("entries")+" WHERE id = ? AND owner_user_id = ?", entryID, owner).Scan(&workID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, httpx.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	wr, err := s.lockWork(ctx, tx, workID, owner)
	if err != nil {
		return nil, err
	}
	e := entryRow{Work: wr}
	err = tx.QueryRowContext(ctx, "SELECT e.id, e.work_id, e.owner_user_id, e.work_kind, e.slug, e.title, e.ordinal, e.workflow, e.early_at, e.public_at,"+
		" e.current_draft_revision_id, e.published_revision_id, a.minimum_access_rank FROM "+s.T.T("entries")+" e LEFT JOIN "+s.T.T("entry_access")+
		" a ON a.entry_id = e.id WHERE e.id = ? AND e.work_id = ? FOR UPDATE", entryID, workID).
		Scan(&e.ID, &e.WorkID, &e.OwnerID, &e.WorkKind, &e.Slug, &e.Title, &e.Ordinal, &e.Workflow, &e.EarlyAt, &e.PublicAt, &e.DraftRevID, &e.PublishedRevID, &e.MinRank)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, httpx.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}
