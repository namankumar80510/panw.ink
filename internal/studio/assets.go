package studio

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/namankumar80510/panw.ink/internal/auth"
	"github.com/namankumar80510/panw.ink/internal/cryptox"
	"github.com/namankumar80510/panw.ink/internal/db"
	"github.com/namankumar80510/panw.ink/internal/httpx"
	"github.com/namankumar80510/panw.ink/internal/media"
)

// Asset lifecycle (§7.5, §12.2, §13.2 item 8):
//
//  1. The browser presigns with Collar (POST /api/v1/pawn/storage/uploads/presign),
//     PUTs the file to the bucket and confirms (Collar checks size and type).
//  2. POST /studio/assets {key} registers the upload as a pending asset; the
//     key must lie under the caller's own prefix uploads/<user id>/.
//  3. Validation downloads it, decodes and re-encodes it (media.Process),
//     stores the variants under media/<user>/<asset>/ and marks it ready —
//     or quarantines it and deletes the upload. No DB transaction is held
//     during object-store I/O.
//  4. Only variants of ready assets are ever served, via short-lived
//     presigned redirects from GET /api/v1/pawn/x/assets/{id}/{variant}.

var uploadKeyRE = regexp.MustCompile(`^uploads/[0-9]+/[0-9]{4}/[0-9]{2}/[A-Za-z0-9-]+(\.[a-z0-9]{1,10})?$`)

type assetInput struct {
	Key     string `json:"key"`
	AltText string `json:"alt_text"`
}

// AssetDTO is an asset in command responses.
type AssetDTO struct {
	ID        string  `json:"id"`
	State     string  `json:"state"`
	ErrorCode *string `json:"error_code"`
	Width     *int    `json:"width"`
	Height    *int    `json:"height"`
	AltText   string  `json:"alt_text"`
	CardURL   *string `json:"card_url"`
	FullURL   *string `json:"full_url"`
}

func (s *Service) registerAsset(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	v := auth.ViewerFrom(ctx)
	if s.Store == nil {
		httpx.WriteError(w, r, httpx.NewError(http.StatusServiceUnavailable, "STORAGE_UNAVAILABLE", "Image uploads are not configured."))
		return
	}
	var in assetInput
	if err := httpx.DecodeJSON(w, r, &in, httpx.MaxJSONBody); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	fields := map[string]string{}
	in.AltText = checkText(fields, "alt_text", in.AltText, 0, 300)
	if !uploadKeyRE.MatchString(in.Key) || !strings.HasPrefix(in.Key, "uploads/"+idStr(v.UserID)+"/") {
		fields["key"] = "Upload the file first; only your own uploads can be registered."
	}
	if len(fields) > 0 {
		httpx.WriteError(w, r, httpx.Validation(fields))
		return
	}
	var id uint64
	err := db.InTx(ctx, s.DB, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, "INSERT INTO "+s.T.T("assets")+" (owner_user_id, storage_key, mime, byte_size, alt_text, state) VALUES (?,?, 'image/jpeg', 0, ?, 'pending')",
			v.UserID, in.Key, in.AltText)
		if isDuplicate(err, "") {
			return conflict("ASSET_EXISTS", "This upload is already registered.")
		}
		if err != nil {
			return err
		}
		n, _ := res.LastInsertId()
		id = uint64(n)
		return s.audit(ctx, tx, r, w, "asset.register", "asset", id, nil, nil)
	})
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	s.respondValidate(w, r, id, v.UserID)
}

func (s *Service) validateAsset(w http.ResponseWriter, r *http.Request) {
	v := auth.ViewerFrom(r.Context())
	id, err := pathID(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	s.respondValidate(w, r, id, v.UserID)
}

func (s *Service) respondValidate(w http.ResponseWriter, r *http.Request, id, owner uint64) {
	dto, err := s.ValidateAsset(r.Context(), id, owner)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if dto.State == "quarantined" {
		httpx.WriteError(w, r, httpx.NewError(http.StatusUnprocessableEntity, "IMAGE_REJECTED",
			"This file is not a usable JPEG, PNG or WebP image (32–5000 px, up to 5 MB).").WithDetails(dto))
		return
	}
	httpx.WriteData(w, r, http.StatusOK, dto, nil)
}

// ValidateAsset runs the decode/re-encode pipeline for a pending asset.
// Ready and quarantined assets are returned unchanged (idempotent).
func (s *Service) ValidateAsset(ctx context.Context, id, owner uint64) (*AssetDTO, error) {
	var key, state string
	err := s.DB.QueryRowContext(ctx, "SELECT storage_key, state FROM "+s.T.T("assets")+" WHERE id = ? AND owner_user_id = ?", id, owner).Scan(&key, &state)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, httpx.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if state != "pending" {
		return s.assetDTO(ctx, id)
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	raw, _, err := s.Store.Get(ctx, key, media.MaxBytes)
	reject := func(code string) (*AssetDTO, error) {
		_ = s.Store.Delete(ctx, key)
		if _, err := s.DB.ExecContext(ctx, "UPDATE "+s.T.T("assets")+" SET state = 'quarantined', error_code = ? WHERE id = ? AND state = 'pending'", code, id); err != nil {
			return nil, err
		}
		return s.assetDTO(ctx, id)
	}
	if err != nil {
		return reject("unreadable_upload")
	}
	format, variants, err := media.Process(raw)
	if err != nil {
		return reject(err.Error())
	}
	nonce, err := cryptox.RandomToken(9)
	if err != nil {
		return nil, err
	}
	nonce = strings.NewReplacer("_", "x", "-", "y").Replace(nonce)
	type stored struct {
		name, key string
		w, h, n   int
	}
	var done []stored
	for _, vr := range variants {
		k := "media/" + idStr(owner) + "/" + idStr(id) + "/" + vr.Name + "-" + nonce + ".jpg"
		if err := s.Store.Put(ctx, k, vr.Data, "image/jpeg"); err != nil {
			for _, d := range done {
				_ = s.Store.Delete(ctx, d.key)
			}
			return nil, err // transient: the asset stays pending and can be retried
		}
		done = append(done, stored{vr.Name, k, vr.Width, vr.Height, len(vr.Data)})
	}
	sum := sha256.Sum256(raw)
	err = db.InTx(ctx, s.DB, func(tx *sql.Tx) error {
		for _, d := range done {
			if _, err := tx.ExecContext(ctx, "INSERT INTO "+s.T.T("asset_variants")+" (asset_id, variant, owner_user_id, storage_key, mime, width, height, byte_size) VALUES (?,?,?,?, 'image/jpeg', ?,?,?)",
				id, d.name, owner, d.key, d.w, d.h, d.n); err != nil {
				return err
			}
		}
		_, err := tx.ExecContext(ctx, "UPDATE "+s.T.T("assets")+" SET state = 'ready', error_code = NULL, mime = ?, byte_size = ?, width = ?, height = ?, sha256 = ? WHERE id = ? AND state = 'pending'",
			"image/"+map[string]string{"jpeg": "jpeg", "png": "png", "webp": "webp"}[format], len(raw), done[0].w, done[0].h, sum[:], id)
		return err
	})
	if err != nil {
		for _, d := range done {
			_ = s.Store.Delete(ctx, d.key)
		}
		return nil, err
	}
	_ = s.Store.Delete(ctx, key) // the original (with its metadata) is never kept
	return s.assetDTO(ctx, id)
}

func (s *Service) assetDTO(ctx context.Context, id uint64) (*AssetDTO, error) {
	d := AssetDTO{ID: idStr(id)}
	var w, h sql.NullInt64
	err := s.DB.QueryRowContext(ctx, "SELECT state, error_code, width, height, alt_text FROM "+s.T.T("assets")+" WHERE id = ?", id).Scan(&d.State, &d.ErrorCode, &w, &h, &d.AltText)
	if err != nil {
		return nil, err
	}
	if w.Valid {
		x, y := int(w.Int64), int(h.Int64)
		d.Width, d.Height = &x, &y
	}
	if d.State == "ready" {
		card, full := AssetURL(id, "card"), AssetURL(id, "full")
		d.CardURL, d.FullURL = &card, &full
	}
	return &d, nil
}

// AssetURL is the public path of an asset variant.
func AssetURL(id uint64, variant string) string {
	return auth.Base + "/assets/" + idStr(id) + "/" + variant
}

// serveAsset redirects to a short-lived presigned URL of a ready variant.
// Publicly referenced images (covers of visible works and collections,
// avatars of active pen names) are visible to everyone; other ready assets
// only to their owner.
func (s *Service) serveAsset(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := pathID(r)
	variant := r.PathValue("variant")
	if err != nil || (variant != "card" && variant != "full") || s.Store == nil {
		httpx.WriteError(w, r, httpx.ErrNotFound)
		return
	}
	var key string
	var owner uint64
	var public bool
	err = s.DB.QueryRowContext(ctx, "SELECT v.storage_key, a.owner_user_id,"+
		" (EXISTS (SELECT 1 FROM "+s.T.T("works")+" w JOIN "+s.T.T("pen_names")+" p ON p.id = w.pen_name_id WHERE w.cover_asset_id = a.id AND w.visibility IN ('listed','unlisted') AND w.deleted_at IS NULL AND p.state = 'active')"+
		"  OR EXISTS (SELECT 1 FROM "+s.T.T("collections")+" c WHERE c.cover_asset_id = a.id AND c.visibility IN ('listed','unlisted'))"+
		"  OR EXISTS (SELECT 1 FROM "+s.T.T("pen_names")+" p WHERE p.avatar_asset_id = a.id AND p.state = 'active'))"+
		" FROM "+s.T.T("assets")+" a JOIN "+s.T.T("asset_variants")+" v ON v.asset_id = a.id AND v.variant = ?"+
		" WHERE a.id = ? AND a.state = 'ready'", variant, id).Scan(&key, &owner, &public)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, r, httpx.ErrNotFound)
		return
	}
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	viewer := auth.ViewerFrom(ctx)
	if !public && (viewer == nil || viewer.UserID != owner) {
		httpx.WriteError(w, r, httpx.ErrNotFound)
		return
	}
	if public {
		w.Header().Set("Cache-Control", "public, max-age=600")
	} else {
		w.Header().Set("Cache-Control", "private, no-store")
	}
	w.Header().Set("Location", s.Store.PresignGet(key, time.Hour))
	w.WriteHeader(http.StatusFound)
}
