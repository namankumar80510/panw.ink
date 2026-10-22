package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/namankumar80510/panw.ink/internal/auth"
	"github.com/namankumar80510/panw.ink/internal/db"
	"github.com/namankumar80510/panw.ink/internal/httpx"
)

// Entitlement decides early access for a verified viewer (Patreon, later
// phase). nil means nobody is entitled: early bodies stay locked.
type Entitlement func(ctx context.Context, v *auth.Viewer, entryID uint64) bool

// Public serves the public catalogue routes.
type Public struct {
	DB          *sql.DB
	T           db.Tables
	Now         func() time.Time
	CursorKey   []byte
	Entitlement Entitlement
}

func (p *Public) now() time.Time {
	if p.Now != nil {
		return p.Now().UTC()
	}
	return time.Now().UTC()
}

// Mount registers the routes.
func (p *Public) Mount(r auth.Router) error {
	routes := map[string]http.HandlerFunc{
		"GET " + auth.Base + "/genres":                           p.genres,
		"GET " + auth.Base + "/tags":                             p.tags,
		"GET " + auth.Base + "/home":                             p.home,
		"GET " + auth.Base + "/works":                            p.works,
		"GET " + auth.Base + "/works/{slug}":                     p.work,
		"GET " + auth.Base + "/works/{slug}/toc":                 p.toc,
		"GET " + auth.Base + "/works/{slug}/entries/{entrySlug}": p.entry,
		"GET " + auth.Base + "/stories/{slug}/read":              p.storyRead,
		"GET " + auth.Base + "/authors/{slug}":                   p.author,
		"GET " + auth.Base + "/collections":                      p.collections,
		"GET " + auth.Base + "/collections/{slug}":               p.collection,
		"GET " + auth.Base + "/search":                           p.search,
	}
	for pattern, fn := range routes {
		if err := r.HandleFunc(pattern, fn); err != nil {
			return err
		}
	}
	return nil
}

func publicCache(w http.ResponseWriter, seconds int) {
	w.Header().Set("Cache-Control", "public, max-age="+strconv.Itoa(seconds))
}

func privateCache(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "private, no-store")
}

// ---- DTOs -----------------------------------------------------------------

// Ref is a slug + display name pair.
type Ref struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
}

// TagRef is an approved tag.
type TagRef struct {
	Slug  string `json:"slug"`
	Label string `json:"label"`
}

// WorkCard is a work in listings (§4.4 WorkCard). No invented scores.
type WorkCard struct {
	ID                 string     `json:"id"`
	Slug               string     `json:"slug"`
	Kind               string     `json:"kind"`
	Title              string     `json:"title"`
	Subtitle           *string    `json:"subtitle"`
	Synopsis           string     `json:"synopsis"`
	Author             Ref        `json:"author"`
	Genre              Ref        `json:"genre"`
	Tags               []TagRef   `json:"tags"`
	PublicationStatus  string     `json:"publication_status"`
	AgeRating          string     `json:"age_rating"`
	ContentWarnings    []string   `json:"content_warnings"`
	WordCount          int        `json:"word_count"`
	ReadMinutes        int        `json:"read_minutes"`
	PublicEntryCount   int        `json:"public_entry_count"`
	FirstPublicAt      *time.Time `json:"first_public_at"`
	LastPublicUpdateAt *time.Time `json:"last_public_update_at"`
	Featured           bool       `json:"featured"`
	CoverURL           *string    `json:"cover_url"`
}

// workVisible: the work and its pen name are publicly visible (listed or
// unlisted). Listings additionally require visibility = 'listed'.
const workVisible = "w.deleted_at IS NULL AND w.visibility IN ('listed','unlisted') AND p.state = 'active'"

const cardColumns = "w.id, w.slug, w.kind, w.title, w.subtitle, w.synopsis, w.publication_status, w.age_rating," +
	" w.content_warnings_json, w.public_word_count, w.public_entry_count, w.first_public_at, w.last_public_update_at," +
	" w.is_featured, p.slug, p.name, g.slug, g.name, w.cover_asset_id"

func (p *Public) cardFrom() string {
	return " FROM " + p.T.T("works") + " w JOIN " + p.T.T("pen_names") + " p ON p.id = w.pen_name_id JOIN " + p.T.T("genres") + " g ON g.id = w.primary_genre_id"
}

// coverURLs keeps only covers whose asset is ready.
func (p *Public) coverURLs(ctx context.Context, cards []*WorkCard, covers []sql.NullInt64) error {
	for i, c := range covers {
		if !c.Valid {
			continue
		}
		var state string
		if err := p.DB.QueryRowContext(ctx, "SELECT state FROM "+p.T.T("assets")+" WHERE id = ?", c.Int64).Scan(&state); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if state == "ready" {
			u := auth.Base + "/assets/" + strconv.FormatInt(c.Int64, 10) + "/card"
			cards[i].CoverURL = &u
		}
	}
	return nil
}

func scanCard(rows interface{ Scan(...any) error }) (*WorkCard, uint64, sql.NullInt64, error) {
	var c WorkCard
	var id uint64
	var warnings sql.NullString
	var first, last db.Time
	var cover sql.NullInt64
	err := rows.Scan(&id, &c.Slug, &c.Kind, &c.Title, &c.Subtitle, &c.Synopsis, &c.PublicationStatus, &c.AgeRating,
		&warnings, &c.WordCount, &c.PublicEntryCount, &first, &last, &c.Featured, &c.Author.Slug, &c.Author.Name, &c.Genre.Slug, &c.Genre.Name, &cover)
	if err != nil {
		return nil, 0, cover, err
	}
	return finishCard(&c, id, warnings, first, last), id, cover, nil
}

// cards runs a card query and attaches approved tags.
func (p *Public) cards(ctx context.Context, query string, args ...any) ([]*WorkCard, []uint64, error) {
	rows, err := p.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, nil, fmt.Errorf("catalog: cards: %w", err)
	}
	defer rows.Close()
	out, ids, covers := []*WorkCard{}, []uint64{}, []sql.NullInt64{}
	for rows.Next() {
		c, id, cover, err := scanCard(rows)
		if err != nil {
			return nil, nil, fmt.Errorf("catalog: scan card: %w", err)
		}
		out, ids, covers = append(out, c), append(ids, id), append(covers, cover)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	rows.Close()
	if err := p.coverURLs(ctx, out, covers); err != nil {
		return nil, nil, err
	}
	return out, ids, p.attachTags(ctx, out, ids)
}

// ---- taxonomy ---------------------------------------------------------------

func (p *Public) genres(w http.ResponseWriter, r *http.Request) {
	rows, err := p.DB.QueryContext(r.Context(), "SELECT slug, name FROM "+p.T.T("genres")+" WHERE enabled = 1 ORDER BY display_order, name")
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	defer rows.Close()
	out := []Ref{}
	for rows.Next() {
		var g Ref
		if err := rows.Scan(&g.Slug, &g.Name); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		out = append(out, g)
	}
	publicCache(w, 300)
	httpx.WriteData(w, r, http.StatusOK, out, nil)
}

func (p *Public) tags(w http.ResponseWriter, r *http.Request) {
	q := "SELECT slug, label FROM " + p.T.T("tags") + " WHERE state = 'approved'"
	args := []any{}
	if s := strings.TrimSpace(r.URL.Query().Get("q")); s != "" {
		if utf8.RuneCountInString(s) > 50 {
			httpx.WriteError(w, r, httpx.Validation(map[string]string{"q": "Use at most 50 characters."}))
			return
		}
		q += " AND label LIKE ? ESCAPE '!'"
		args = append(args, "%"+likeEscape(s)+"%")
	}
	rows, err := p.DB.QueryContext(r.Context(), q+" ORDER BY label LIMIT 200", args...)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	defer rows.Close()
	out := []TagRef{}
	for rows.Next() {
		var t TagRef
		if err := rows.Scan(&t.Slug, &t.Label); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		out = append(out, t)
	}
	publicCache(w, 300)
	httpx.WriteData(w, r, http.StatusOK, out, nil)
}

func likeEscape(s string) string {
	return strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(s)
}

// ---- catalogue -------------------------------------------------------------

type listQuery struct {
	kind, genre, status, sort, q string
	tags                         []string
	minWords, maxWords           int
	limit                        int
}

func parseList(r *http.Request) (*listQuery, error) {
	v := r.URL.Query()
	lq := &listQuery{kind: v.Get("kind"), genre: v.Get("genre"), status: v.Get("status"), sort: v.Get("sort"), q: strings.TrimSpace(v.Get("q")), limit: 20}
	fields := map[string]string{}
	switch lq.kind {
	case "", "novel", "short_story":
	default:
		fields["kind"] = "Use novel or short_story."
	}
	switch lq.status {
	case "", "ongoing", "completed", "hiatus", "cancelled":
	default:
		fields["status"] = "Unknown status."
	}
	if lq.sort == "" {
		lq.sort = "latest"
	}
	switch lq.sort {
	case "latest", "newest", "title":
	default:
		fields["sort"] = "Use latest, newest or title."
	}
	if utf8.RuneCountInString(lq.q) > 100 {
		fields["q"] = "Use at most 100 characters."
	}
	if t := v.Get("tags"); t != "" {
		for _, s := range strings.Split(t, ",") {
			if s = strings.TrimSpace(s); s != "" {
				lq.tags = append(lq.tags, s)
			}
		}
		if len(lq.tags) > 5 {
			fields["tags"] = "Use at most 5 tags."
		}
	}
	num := func(key string) int {
		s := v.Get(key)
		if s == "" {
			return 0
		}
		n, err := strconv.Atoi(s)
		if err != nil || n < 0 || n > 10_000_000 {
			fields[key] = "Use a non-negative whole number."
		}
		return n
	}
	lq.minWords, lq.maxWords = num("min_words"), num("max_words")
	if mm := num("max_minutes"); mm > 0 && (lq.maxWords == 0 || mm*230 < lq.maxWords) {
		lq.maxWords = mm * 230
	}
	if l := num("limit"); l > 0 {
		lq.limit = min(l, 50)
	}
	if len(fields) > 0 {
		return nil, httpx.Validation(fields)
	}
	return lq, nil
}

// booleanQuery turns user text into a safe InnoDB boolean-mode query: every
// term required, with prefix matching; operators are stripped.
func booleanQuery(s string) string {
	var terms []string
	for _, f := range strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if len(terms) == 8 {
			break
		}
		terms = append(terms, "+"+f+"*")
	}
	return strings.Join(terms, " ")
}

func (p *Public) works(w http.ResponseWriter, r *http.Request) {
	lq, err := parseList(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	now := p.now()
	where := []string{workVisible, "w.visibility = 'listed'", "w.first_public_at <= ?"}
	args := []any{now}
	if lq.kind != "" {
		where, args = append(where, "w.kind = ?"), append(args, lq.kind)
	}
	if lq.genre != "" {
		where, args = append(where, "g.slug = ? AND g.enabled = 1"), append(args, lq.genre)
	}
	if lq.status != "" {
		where, args = append(where, "w.publication_status = ?"), append(args, lq.status)
	}
	if lq.minWords > 0 {
		where, args = append(where, "w.public_word_count >= ?"), append(args, lq.minWords)
	}
	if lq.maxWords > 0 {
		where, args = append(where, "w.public_word_count <= ?"), append(args, lq.maxWords)
	}
	if bq := booleanQuery(lq.q); bq != "" {
		where, args = append(where, "MATCH(w.title, w.synopsis) AGAINST (? IN BOOLEAN MODE)"), append(args, bq)
	}
	if len(lq.tags) > 0 {
		ph := strings.TrimSuffix(strings.Repeat("?,", len(lq.tags)), ",")
		where = append(where, "w.id IN (SELECT wt.work_id FROM "+p.T.T("work_tags")+" wt JOIN "+p.T.T("tags")+
			" t ON t.id = wt.tag_id WHERE t.state = 'approved' AND t.slug IN ("+ph+") GROUP BY wt.work_id HAVING COUNT(DISTINCT t.id) = ?)")
		for _, t := range lq.tags {
			args = append(args, t)
		}
		args = append(args, len(lq.tags))
	}

	fh := filterHash(lq.kind, lq.genre, lq.status, lq.q, strings.Join(lq.tags, ","), strconv.Itoa(lq.minWords), strconv.Itoa(lq.maxWords))
	codec := cursorCodec{key: p.CursorKey}
	cur, err := codec.decode(r.URL.Query().Get("cursor"), lq.sort, fh)
	if err != nil {
		httpx.WriteError(w, r, httpx.NewError(http.StatusBadRequest, "INVALID_CURSOR", "This page link has expired. Start again from the first page."))
		return
	}
	var key, order string
	switch lq.sort {
	case "latest":
		key, order = "COALESCE(w.last_public_update_at, w.first_public_at)", "DESC"
	case "newest":
		key, order = "w.first_public_at", "DESC"
	case "title":
		key, order = "w.title", "ASC"
	}
	if cur != nil {
		cmp := "<"
		if order == "ASC" {
			cmp = ">"
		}
		var kv any = cur.Key
		if lq.sort != "title" {
			t, err := time.Parse(time.RFC3339Nano, cur.Key)
			if err != nil {
				httpx.WriteError(w, r, httpx.NewError(http.StatusBadRequest, "INVALID_CURSOR", "Invalid cursor."))
				return
			}
			kv = t
		}
		where = append(where, "("+key+" "+cmp+" ? OR ("+key+" = ? AND w.id "+cmp+" ?))")
		args = append(args, kv, kv, cur.ID)
	}
	q := "SELECT " + cardColumns + ", " + key + p.cardFrom() + " WHERE " + strings.Join(where, " AND ") +
		" ORDER BY " + key + " " + order + ", w.id " + order + " LIMIT ?"
	args = append(args, lq.limit+1)

	rows, err := p.DB.QueryContext(r.Context(), q, args...)
	if err != nil {
		httpx.WriteError(w, r, fmt.Errorf("catalog: list: %w", err))
		return
	}
	type rowT struct {
		card  *WorkCard
		id    uint64
		key   string
		cover sql.NullInt64
	}
	var got []rowT
	for rows.Next() {
		var c WorkCard
		var id uint64
		var warnings sql.NullString
		var first, last db.Time
		var sortVal sql.RawBytes
		var cover sql.NullInt64
		err := rows.Scan(&id, &c.Slug, &c.Kind, &c.Title, &c.Subtitle, &c.Synopsis, &c.PublicationStatus, &c.AgeRating,
			&warnings, &c.WordCount, &c.PublicEntryCount, &first, &last, &c.Featured, &c.Author.Slug, &c.Author.Name, &c.Genre.Slug, &c.Genre.Name, &cover, &sortVal)
		if err != nil {
			rows.Close()
			httpx.WriteError(w, r, err)
			return
		}
		k := string(sortVal)
		if lq.sort != "title" {
			var t db.Time
			_ = t.Scan(k)
			k = t.Time.Format(time.RFC3339Nano)
		}
		got = append(got, rowT{card: finishCard(&c, id, warnings, first, last), id: id, key: k, cover: cover})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	var next *string
	if len(got) > lq.limit {
		got = got[:lq.limit]
		last := got[len(got)-1]
		s := codec.encode(cursorPayload{Sort: lq.sort, Filter: fh, Key: last.key, ID: last.id})
		next = &s
	}
	cards := make([]*WorkCard, len(got))
	ids := make([]uint64, len(got))
	covers := make([]sql.NullInt64, len(got))
	for i, g := range got {
		cards[i], ids[i], covers[i] = g.card, g.id, g.cover
	}
	if err := p.coverURLs(r.Context(), cards, covers); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if err := p.attachTags(r.Context(), cards, ids); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	publicCache(w, 60)
	httpx.WriteData(w, r, http.StatusOK, cards, next)
}

// finishCard fills the derived card fields.
func finishCard(c *WorkCard, id uint64, warnings sql.NullString, first, last db.Time) *WorkCard {
	c.ID = strconv.FormatUint(id, 10)
	c.ContentWarnings = []string{}
	if warnings.Valid {
		_ = json.Unmarshal([]byte(warnings.String), &c.ContentWarnings)
	}
	if first.Valid {
		c.FirstPublicAt = &first.Time
	}
	if last.Valid {
		c.LastPublicUpdateAt = &last.Time
	}
	c.ReadMinutes = max(1, (c.WordCount+229)/230)
	c.Tags = []TagRef{}
	if utf8.RuneCountInString(c.Synopsis) > 300 {
		c.Synopsis = string([]rune(c.Synopsis)[:297]) + "…"
	}
	return c
}

func (p *Public) attachTags(ctx context.Context, cards []*WorkCard, ids []uint64) error {
	if len(ids) == 0 {
		return nil
	}
	byID := map[uint64]*WorkCard{}
	targs := make([]any, len(ids))
	for i, id := range ids {
		byID[id], targs[i] = cards[i], id
	}
	ph := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	rows, err := p.DB.QueryContext(ctx, "SELECT wt.work_id, t.slug, t.label FROM "+p.T.T("work_tags")+" wt JOIN "+p.T.T("tags")+
		" t ON t.id = wt.tag_id WHERE t.state = 'approved' AND wt.work_id IN ("+ph+") ORDER BY t.label", targs...)
	if err != nil {
		return fmt.Errorf("catalog: tags: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id uint64
		var t TagRef
		if err := rows.Scan(&id, &t.Slug, &t.Label); err != nil {
			return err
		}
		if c := byID[id]; c != nil {
			c.Tags = append(c.Tags, t)
		}
	}
	return rows.Err()
}

// ---- home ------------------------------------------------------------------

// Home is the home page payload (§4.1). Rails are honest: empty when the
// catalogue is small, never padded with fake works or stats.
type Home struct {
	Featured        []*WorkCard `json:"featured"`
	ShortReads      []*WorkCard `json:"short_reads"`
	Completed       []*WorkCard `json:"completed"`
	OngoingNovels   []*WorkCard `json:"ongoing_novels"`
	RecentlyUpdated []*WorkCard `json:"recently_updated"`
}

func (p *Public) home(w http.ResponseWriter, r *http.Request) {
	now := p.now()
	base := "SELECT " + cardColumns + p.cardFrom() + " WHERE " + workVisible + " AND w.visibility = 'listed' AND w.first_public_at <= ?"
	latest := " ORDER BY COALESCE(w.last_public_update_at, w.first_public_at) DESC, w.id DESC LIMIT 8"
	var h Home
	var err error
	if h.Featured, _, err = p.cards(r.Context(), base+" AND w.is_featured = 1"+latest, now); err == nil {
		if h.ShortReads, _, err = p.cards(r.Context(), base+" AND w.kind = 'short_story'"+latest, now); err == nil {
			if h.Completed, _, err = p.cards(r.Context(), base+" AND w.publication_status = 'completed'"+latest, now); err == nil {
				if h.OngoingNovels, _, err = p.cards(r.Context(), base+" AND w.kind = 'novel' AND w.publication_status = 'ongoing'"+latest, now); err == nil {
					h.RecentlyUpdated, _, err = p.cards(r.Context(), base+latest, now)
				}
			}
		}
	}
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	publicCache(w, 60)
	httpx.WriteData(w, r, http.StatusOK, h, nil)
}

// ---- work ------------------------------------------------------------------

// WorkDetail is the work overview (§4.1 /fiction/:workSlug).
type WorkDetail struct {
	WorkCard
	FullSynopsis string        `json:"full_synopsis"`
	Language     string        `json:"language"`
	CompletedAt  *time.Time    `json:"completed_at"`
	Author       AuthorSummary `json:"author"`
	Rating       RatingSummary `json:"rating"`
	Start        *EntryRef     `json:"start"`        // first entry readable now, if any
	NextRelease  *EntryRef     `json:"next_release"` // soonest early/upcoming entry, if any
	Collections  []Ref         `json:"collections"`
}

// AuthorSummary is the pen name shown on a work.
type AuthorSummary struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
	Bio  string `json:"bio"`
}

// RatingSummary is filled by the reviews phase; until then it honestly
// reports insufficient data rather than a zero score (§0.4).
type RatingSummary struct {
	State   string   `json:"state"` // insufficient_data | ok
	Average *float64 `json:"average"`
	Count   int      `json:"count"`
}

// EntryRef is a compact entry reference.
type EntryRef struct {
	ID       string     `json:"id"`
	Slug     string     `json:"slug"`
	Title    string     `json:"title"`
	Ordinal  int        `json:"ordinal"`
	Access   Access     `json:"access"`
	EarlyAt  *time.Time `json:"early_at"`
	PublicAt *time.Time `json:"public_at"`
}

type workRow struct {
	id       uint64
	kind     string
	slug     string
	title    string
	listed   bool
	authorID uint64
}

// findWork loads a publicly visible work (listed or unlisted) that has at
// least one released entry. Missing, draft, hidden and deleted works are all
// the same 404 (CAT-05).
func (p *Public) findWork(ctx context.Context, slug string) (*WorkDetail, *workRow, error) {
	if !validSlug(slug) {
		return nil, nil, httpx.ErrNotFound
	}
	q := "SELECT " + cardColumns + ", w.synopsis, w.language_code, w.completed_at, w.visibility, p.id, p.bio" + p.cardFrom() +
		" WHERE w.slug = ? AND " + workVisible + " AND w.first_public_at IS NOT NULL"
	var d WorkDetail
	var id, authorID uint64
	var warnings sql.NullString
	var first, last, completed db.Time
	var visibility string
	var cover sql.NullInt64
	err := p.DB.QueryRowContext(ctx, q, slug).Scan(&id, &d.Slug, &d.Kind, &d.Title, &d.Subtitle, &d.Synopsis, &d.PublicationStatus, &d.AgeRating,
		&warnings, &d.WordCount, &d.PublicEntryCount, &first, &last, &d.Featured, &d.WorkCard.Author.Slug, &d.WorkCard.Author.Name, &d.Genre.Slug, &d.Genre.Name, &cover,
		&d.FullSynopsis, &d.Language, &completed, &visibility, &authorID, &d.Author.Bio)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, p.notFoundOrMoved(ctx, "work", "/fiction/"+slug)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("catalog: work: %w", err)
	}
	finishCard(&d.WorkCard, id, warnings, first, last)
	if err := p.coverURLs(ctx, []*WorkCard{&d.WorkCard}, []sql.NullInt64{cover}); err != nil {
		return nil, nil, err
	}
	if completed.Valid {
		d.CompletedAt = &completed.Time
	}
	d.Author.Slug, d.Author.Name = d.WorkCard.Author.Slug, d.WorkCard.Author.Name
	d.Rating = RatingSummary{State: "insufficient_data"}
	d.Collections = []Ref{}
	return &d, &workRow{id: id, kind: d.Kind, slug: d.Slug, title: d.Title, listed: visibility == "listed", authorID: authorID}, nil
}

// notFoundOrMoved returns 404, or 404 MOVED with the canonical location when
// a slug redirect exists (the SPA replaces its URL).
func (p *Public) notFoundOrMoved(ctx context.Context, _ string, oldPath string) error {
	var to string
	err := p.DB.QueryRowContext(ctx, "SELECT new_path FROM "+p.T.T("slug_redirects")+" WHERE old_path = ?", oldPath).Scan(&to)
	if err == nil && strings.HasPrefix(to, "/") && !strings.HasPrefix(to, "//") {
		return httpx.NewError(http.StatusNotFound, "MOVED", "This page has moved.").WithDetails(map[string]string{"location": to})
	}
	return httpx.ErrNotFound
}

func validSlug(s string) bool {
	if s == "" || len(s) > 160 {
		return false
	}
	for _, r := range s {
		if !(r == '-' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}

func (p *Public) work(w http.ResponseWriter, r *http.Request) {
	d, wr, err := p.findWork(r.Context(), r.PathValue("slug"))
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if err := p.attachTags(r.Context(), []*WorkCard{&d.WorkCard}, []uint64{wr.id}); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	toc, err := p.loadTOC(r.Context(), wr.id)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	for i := range toc {
		e := &toc[i]
		if d.Start == nil && e.Access == AccessPublic {
			ref := e.EntryRef
			d.Start = &ref
		}
		if d.NextRelease == nil && (e.Access == AccessEarly || e.Access == AccessUpcoming) {
			ref := e.EntryRef
			d.NextRelease = &ref
		}
	}
	crow, err := p.DB.QueryContext(r.Context(), "SELECT c.slug, c.title FROM "+p.T.T("collection_items")+" ci JOIN "+p.T.T("collections")+
		" c ON c.id = ci.collection_id WHERE ci.work_id = ? AND c.visibility = 'listed' ORDER BY c.title LIMIT 20", wr.id)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	for crow.Next() {
		var c Ref
		if err := crow.Scan(&c.Slug, &c.Name); err == nil {
			d.Collections = append(d.Collections, c)
		}
	}
	crow.Close()
	publicCache(w, 60)
	httpx.WriteData(w, r, http.StatusOK, d, nil)
}

// TOCEntry is one released entry in a table of contents. Titles, ordinals
// and dates are public metadata; bodies never appear here.
type TOCEntry struct {
	EntryRef
	Volume      *VolumeRef `json:"volume"`
	WordCount   int        `json:"word_count"`
	ReadSeconds int        `json:"read_seconds"`
}

// VolumeRef is an optional novel volume.
type VolumeRef struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
}

func (p *Public) loadTOC(ctx context.Context, workID uint64) ([]TOCEntry, error) {
	now := p.now()
	rows, err := p.DB.QueryContext(ctx, "SELECT e.id, e.slug, e.title, e.ordinal, e.workflow, e.early_at, e.public_at, v.number, v.title, r.word_count, r.estimated_read_seconds"+
		" FROM "+p.T.T("entries")+" e JOIN "+p.T.T("entry_revisions")+" r ON r.id = e.published_revision_id"+
		" LEFT JOIN "+p.T.T("volumes")+" v ON v.id = e.volume_id"+
		" WHERE e.work_id = ? AND "+released+" ORDER BY e.ordinal LIMIT 5000", workID)
	if err != nil {
		return nil, fmt.Errorf("catalog: toc: %w", err)
	}
	defer rows.Close()
	out := []TOCEntry{}
	for rows.Next() {
		var t TOCEntry
		var id uint64
		var workflow string
		var early, public db.Time
		var vnum sql.NullInt64
		var vtitle sql.NullString
		if err := rows.Scan(&id, &t.Slug, &t.Title, &t.Ordinal, &workflow, &early, &public, &vnum, &vtitle, &t.WordCount, &t.ReadSeconds); err != nil {
			return nil, err
		}
		t.ID = strconv.FormatUint(id, 10)
		if early.Valid {
			t.EarlyAt = &early.Time
		}
		if public.Valid {
			t.PublicAt = &public.Time
		}
		t.Access = StateAt(Release{Workflow: workflow, EarlyAt: t.EarlyAt, PublicAt: t.PublicAt, HasPublished: true}, now)
		if vnum.Valid {
			t.Volume = &VolumeRef{Number: int(vnum.Int64), Title: vtitle.String}
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (p *Public) toc(w http.ResponseWriter, r *http.Request) {
	_, wr, err := p.findWork(r.Context(), r.PathValue("slug"))
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	toc, err := p.loadTOC(r.Context(), wr.id)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	// Access states depend only on time, not on the viewer, so a short
	// shared cache is safe (§12.5).
	publicCache(w, 30)
	httpx.WriteData(w, r, http.StatusOK, map[string]any{"work": map[string]string{"slug": wr.slug, "title": wr.title, "kind": wr.kind}, "entries": toc}, nil)
}

// ---- entry body ------------------------------------------------------------

// EntryBody is a readable entry (§7.2 sample). body_html is present only when
// the viewer may read it.
type EntryBody struct {
	ID                string     `json:"id"`
	WorkID            string     `json:"work_id"`
	WorkKind          string     `json:"work_kind"`
	WorkSlug          string     `json:"work_slug"`
	WorkTitle         string     `json:"work_title"`
	Author            Ref        `json:"author"`
	Title             string     `json:"title"`
	Slug              string     `json:"slug"`
	Ordinal           int        `json:"ordinal"`
	Volume            *VolumeRef `json:"volume"`
	PublicAt          *time.Time `json:"public_at"`
	EarlyAt           *time.Time `json:"early_at"`
	Access            AccessInfo `json:"access"`
	BodyHTML          string     `json:"body_html"`
	WordCount         int        `json:"word_count"`
	ReadSeconds       int        `json:"read_seconds"`
	Previous          *EntryRef  `json:"previous"`
	Next              *EntryRef  `json:"next"`
	PublishedRevision int        `json:"published_revision"`
	RevisedAt         *time.Time `json:"revised_at"`
	TOCCount          int        `json:"toc_count"`
}

// AccessInfo is the access state.
type AccessInfo struct {
	State Access `json:"state"`
}

type entryMeta struct {
	EntryBody
	id, workID, revisionID uint64
	workflow               string
}

func (p *Public) entry(w http.ResponseWriter, r *http.Request) {
	p.serveEntry(w, r, r.PathValue("slug"), r.PathValue("entrySlug"), "")
}

func (p *Public) storyRead(w http.ResponseWriter, r *http.Request) {
	p.serveEntry(w, r, r.PathValue("slug"), "", "short_story")
}

func (p *Public) serveEntry(w http.ResponseWriter, r *http.Request, workSlug, entrySlug, requireKind string) {
	ctx := r.Context()
	if !validSlug(workSlug) || (entrySlug != "" && !validSlug(entrySlug)) {
		httpx.WriteError(w, r, httpx.ErrNotFound)
		return
	}
	cond, args := "e.slug = ?", []any{workSlug, entrySlug}
	if entrySlug == "" {
		cond, args = "e.ordinal = 1", []any{workSlug}
	}
	q := "SELECT e.id, e.slug, e.title, e.ordinal, e.workflow, e.early_at, e.public_at, e.published_revision_id," +
		" w.id, w.slug, w.kind, w.title, p.slug, p.name, v.number, v.title" +
		" FROM " + p.T.T("works") + " w JOIN " + p.T.T("pen_names") + " p ON p.id = w.pen_name_id" +
		" JOIN " + p.T.T("entries") + " e ON e.work_id = w.id LEFT JOIN " + p.T.T("volumes") + " v ON v.id = e.volume_id" +
		" WHERE w.slug = ? AND " + cond + " AND " + workVisible
	var m entryMeta
	var early, public db.Time
	var rev sql.NullInt64
	var vnum sql.NullInt64
	var vtitle sql.NullString
	err := p.DB.QueryRowContext(ctx, q, args...).Scan(&m.id, &m.Slug, &m.Title, &m.Ordinal, &m.workflow, &early, &public, &rev,
		&m.workID, &m.WorkSlug, &m.WorkKind, &m.WorkTitle, &m.Author.Slug, &m.Author.Name, &vnum, &vtitle)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, r, p.notFoundOrMoved(ctx, "entry", "/fiction/"+workSlug+"/ch/"+entrySlug))
		return
	}
	if err != nil {
		httpx.WriteError(w, r, fmt.Errorf("catalog: entry: %w", err))
		return
	}
	if requireKind != "" && m.WorkKind != requireKind {
		httpx.WriteError(w, r, httpx.ErrNotFound)
		return
	}
	if early.Valid {
		m.EarlyAt = &early.Time
	}
	if public.Valid {
		m.PublicAt = &public.Time
	}
	state := StateAt(Release{Workflow: m.workflow, EarlyAt: m.EarlyAt, PublicAt: m.PublicAt, HasPublished: rev.Valid}, p.now())
	if state == AccessUnavailable {
		// Drafts, archived and unpublished entries do not exist publicly:
		// no title, no dates, no body (CAT-05, §3.2 items 6–7).
		httpx.WriteError(w, r, httpx.ErrNotFound)
		return
	}
	m.ID, m.WorkID = strconv.FormatUint(m.id, 10), strconv.FormatUint(m.workID, 10)
	m.revisionID = uint64(rev.Int64)
	m.Access = AccessInfo{State: state}
	if vnum.Valid {
		m.Volume = &VolumeRef{Number: int(vnum.Int64), Title: vtitle.String}
	}
	if err := p.neighbours(ctx, &m); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	viewer := auth.ViewerFrom(ctx)
	allowed := state == AccessPublic
	if state == AccessEarly && viewer != nil && viewer.EmailVerified && p.Entitlement != nil && p.Entitlement(ctx, viewer, m.id) {
		allowed = true
	}
	if !allowed {
		privateCache(w)
		code, msg := "EARLY_ACCESS_REQUIRED", "Available to supporters now and free for everyone on its public date."
		if state == AccessUpcoming {
			code, msg = "ENTRY_UPCOMING", "This chapter is scheduled and not readable yet."
		}
		m.BodyHTML = ""
		httpx.WriteError(w, r, httpx.NewError(http.StatusForbidden, code, msg).WithDetails(lockedDetails(&m)))
		return
	}

	var revisedAt db.Time
	err = p.DB.QueryRowContext(ctx, "SELECT rendered_html, word_count, estimated_read_seconds, version, created_at FROM "+p.T.T("entry_revisions")+
		" WHERE id = ? AND entry_id = ?", m.revisionID, m.id).Scan(&m.BodyHTML, &m.WordCount, &m.ReadSeconds, &m.PublishedRevision, &revisedAt)
	if err != nil {
		httpx.WriteError(w, r, fmt.Errorf("catalog: body: %w", err))
		return
	}
	if m.PublishedRevision > 1 && revisedAt.Valid && m.PublicAt != nil && revisedAt.Time.After(*m.PublicAt) {
		m.RevisedAt = &revisedAt.Time
	}
	if state == AccessPublic {
		publicCache(w, 60)
	} else {
		privateCache(w) // entitlement-dependent: never shared (§9.3)
	}
	httpx.WriteData(w, r, http.StatusOK, m.EntryBody, nil)
}

// lockedDetails is everything the SPA needs to render a lock page; no body.
func lockedDetails(m *entryMeta) map[string]any {
	return map[string]any{
		"state":           m.Access.State,
		"public_at":       m.PublicAt,
		"early_at":        m.EarlyAt,
		"min_access_rank": nil,
		"entry": map[string]any{
			"id": m.ID, "slug": m.Slug, "title": m.Title, "ordinal": m.Ordinal, "volume": m.Volume,
			"work_slug": m.WorkSlug, "work_title": m.WorkTitle, "work_kind": m.WorkKind, "author": m.Author,
			"previous": m.Previous, "next": m.Next, "toc_count": m.TOCCount,
		},
	}
}

func (p *Public) neighbours(ctx context.Context, m *entryMeta) error {
	now := p.now()
	one := func(cmp, order string) (*EntryRef, error) {
		var ref EntryRef
		var id uint64
		var workflow string
		var early, public db.Time
		err := p.DB.QueryRowContext(ctx, "SELECT e.id, e.slug, e.title, e.ordinal, e.workflow, e.early_at, e.public_at FROM "+p.T.T("entries")+
			" e WHERE e.work_id = ? AND "+released+" AND e.ordinal "+cmp+" ? ORDER BY e.ordinal "+order+" LIMIT 1", m.workID, m.Ordinal).
			Scan(&id, &ref.Slug, &ref.Title, &ref.Ordinal, &workflow, &early, &public)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		if err != nil {
			return nil, fmt.Errorf("catalog: neighbour: %w", err)
		}
		ref.ID = strconv.FormatUint(id, 10)
		if early.Valid {
			ref.EarlyAt = &early.Time
		}
		if public.Valid {
			ref.PublicAt = &public.Time
		}
		ref.Access = StateAt(Release{Workflow: workflow, EarlyAt: ref.EarlyAt, PublicAt: ref.PublicAt, HasPublished: true}, now)
		return &ref, nil
	}
	var err error
	if m.Previous, err = one("<", "DESC"); err != nil {
		return err
	}
	if m.Next, err = one(">", "ASC"); err != nil {
		return err
	}
	return p.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+p.T.T("entries")+" e WHERE e.work_id = ? AND "+released, m.workID).Scan(&m.TOCCount)
}

// ---- authors ---------------------------------------------------------------

func (p *Public) author(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if !validSlug(slug) {
		httpx.WriteError(w, r, httpx.ErrNotFound)
		return
	}
	var id uint64
	var a struct {
		Slug    string      `json:"slug"`
		Name    string      `json:"name"`
		Bio     string      `json:"bio"`
		Website *string     `json:"website_url"`
		Works   []*WorkCard `json:"works"`
	}
	err := p.DB.QueryRowContext(r.Context(), "SELECT id, slug, name, bio, website_url FROM "+p.T.T("pen_names")+" WHERE slug = ? AND state = 'active'", slug).
		Scan(&id, &a.Slug, &a.Name, &a.Bio, &a.Website)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, r, p.notFoundOrMoved(r.Context(), "author", "/authors/"+slug))
		return
	}
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	a.Works, _, err = p.cards(r.Context(), "SELECT "+cardColumns+p.cardFrom()+" WHERE "+workVisible+" AND w.visibility = 'listed' AND w.first_public_at <= ? AND w.pen_name_id = ?"+
		" ORDER BY COALESCE(w.last_public_update_at, w.first_public_at) DESC, w.id DESC LIMIT 100", p.now(), id)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	publicCache(w, 60)
	httpx.WriteData(w, r, http.StatusOK, a, nil)
}

// ---- collections -------------------------------------------------------------

// CollectionCard is an anthology in listings.
type CollectionCard struct {
	Slug        string `json:"slug"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Author      Ref    `json:"author"`
	StoryCount  int    `json:"story_count"`
}

// storyPublic: a collection item counts only when its story is publicly visible now.
func (p *Public) storyPublic() string {
	return " JOIN " + p.T.T("works") + " w ON w.id = ci.work_id JOIN " + p.T.T("pen_names") + " p ON p.id = w.pen_name_id" +
		" WHERE " + workVisible + " AND w.first_public_at <= ?"
}

func (p *Public) collections(w http.ResponseWriter, r *http.Request) {
	now := p.now()
	rows, err := p.DB.QueryContext(r.Context(), "SELECT c.slug, c.title, c.description, cp.slug, cp.name,"+
		" (SELECT COUNT(*) FROM "+p.T.T("collection_items")+" ci"+p.storyPublic()+" AND ci.collection_id = c.id) AS n"+
		" FROM "+p.T.T("collections")+" c JOIN "+p.T.T("pen_names")+" cp ON cp.id = c.pen_name_id"+
		" WHERE c.visibility = 'listed' AND cp.state = 'active' HAVING n > 0 ORDER BY c.updated_at DESC, c.id DESC LIMIT 50", now)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	defer rows.Close()
	out := []CollectionCard{}
	for rows.Next() {
		var c CollectionCard
		if err := rows.Scan(&c.Slug, &c.Title, &c.Description, &c.Author.Slug, &c.Author.Name, &c.StoryCount); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		out = append(out, c)
	}
	publicCache(w, 60)
	httpx.WriteData(w, r, http.StatusOK, out, nil)
}

func (p *Public) collection(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if !validSlug(slug) {
		httpx.WriteError(w, r, httpx.ErrNotFound)
		return
	}
	var id uint64
	var c struct {
		CollectionCard
		Stories []*WorkCard `json:"stories"`
	}
	err := p.DB.QueryRowContext(r.Context(), "SELECT c.id, c.slug, c.title, c.description, cp.slug, cp.name FROM "+p.T.T("collections")+" c JOIN "+
		p.T.T("pen_names")+" cp ON cp.id = c.pen_name_id WHERE c.slug = ? AND c.visibility IN ('listed','unlisted') AND cp.state = 'active'", slug).
		Scan(&id, &c.Slug, &c.Title, &c.Description, &c.Author.Slug, &c.Author.Name)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, r, p.notFoundOrMoved(r.Context(), "collection", "/collections/"+slug))
		return
	}
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	c.Stories, _, err = p.cards(r.Context(), "SELECT "+cardColumns+p.cardFrom()+" JOIN "+p.T.T("collection_items")+" ci ON ci.work_id = w.id"+
		" WHERE ci.collection_id = ? AND "+workVisible+" AND w.first_public_at <= ? ORDER BY ci.ordinal LIMIT 200", id, p.now())
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	c.StoryCount = len(c.Stories)
	publicCache(w, 60)
	httpx.WriteData(w, r, http.StatusOK, c, nil)
}

// ---- search -----------------------------------------------------------------

func (p *Public) search(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	typ := r.URL.Query().Get("type")
	if typ == "" {
		typ = "all"
	}
	fields := map[string]string{}
	if q == "" {
		fields["q"] = "Enter something to search for."
	} else if utf8.RuneCountInString(q) > 100 {
		fields["q"] = "Use at most 100 characters."
	}
	if typ != "all" && typ != "works" && typ != "authors" && typ != "collections" {
		fields["type"] = "Use all, works, authors or collections."
	}
	if len(fields) > 0 {
		httpx.WriteError(w, r, httpx.Validation(fields))
		return
	}
	limit := 20
	if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && l > 0 {
		limit = min(l, 50)
	}
	ctx, now := r.Context(), p.now()
	res := struct {
		Works       []*WorkCard `json:"works"`
		Authors     []Ref       `json:"authors"`
		Collections []Ref       `json:"collections"`
	}{Works: []*WorkCard{}, Authors: []Ref{}, Collections: []Ref{}}
	like := "%" + likeEscape(q) + "%"
	var err error
	if typ == "all" || typ == "works" {
		bq := booleanQuery(q)
		// FULLTEXT ignores very short tokens, so titles also match by substring.
		cond, args := "w.title LIKE ? ESCAPE '!'", []any{now, like}
		if bq != "" {
			cond, args = "(MATCH(w.title, w.synopsis) AGAINST (? IN BOOLEAN MODE) OR w.title LIKE ? ESCAPE '!')", []any{now, bq, like}
		}
		args = append(args, likeEscape(q)+"%", limit)
		res.Works, _, err = p.cards(ctx, "SELECT "+cardColumns+p.cardFrom()+" WHERE "+workVisible+" AND w.visibility = 'listed' AND w.first_public_at <= ? AND "+cond+
			" ORDER BY w.title LIKE ? ESCAPE '!' DESC, COALESCE(w.last_public_update_at, w.first_public_at) DESC LIMIT ?", args...)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
	}
	scanRefs := func(query string, args ...any) ([]Ref, error) {
		rows, err := p.DB.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		out := []Ref{}
		for rows.Next() {
			var x Ref
			if err := rows.Scan(&x.Slug, &x.Name); err != nil {
				return nil, err
			}
			out = append(out, x)
		}
		return out, rows.Err()
	}
	if typ == "all" || typ == "authors" {
		if res.Authors, err = scanRefs("SELECT slug, name FROM "+p.T.T("pen_names")+" WHERE state = 'active' AND name LIKE ? ESCAPE '!' ORDER BY name LIMIT ?", like, limit); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
	}
	if typ == "all" || typ == "collections" {
		if res.Collections, err = scanRefs("SELECT c.slug, c.title FROM "+p.T.T("collections")+" c JOIN "+p.T.T("pen_names")+
			" cp ON cp.id = c.pen_name_id WHERE c.visibility = 'listed' AND cp.state = 'active' AND c.title LIKE ? ESCAPE '!' ORDER BY c.title LIMIT ?", like, limit); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
	}
	publicCache(w, 30)
	httpx.WriteData(w, r, http.StatusOK, res, nil)
}
