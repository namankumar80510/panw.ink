// Package site serves site-wide public metadata (GET /public-config).
package site

import (
	"net/http"

	"github.com/namankumar80510/panw.ink/internal/auth"
	"github.com/namankumar80510/panw.ink/internal/config"
	"github.com/namankumar80510/panw.ink/internal/httpx"
)

// PublicConfig is safe, non-secret configuration for the SPAs.
type PublicConfig struct {
	AppName        string   `json:"app_name"`
	BaseURL        string   `json:"base_url"`
	Environment    string   `json:"environment"`
	NonProduction  bool     `json:"non_production"`
	Features       Features `json:"features"`
	Limits         Limits   `json:"limits"`
	PatreonURL     *string  `json:"patreon_url"`
	SupportContact *string  `json:"support_contact"`
}

// Features are public feature flags.
type Features struct {
	Registration bool `json:"registration"`
	EarlyAccess  bool `json:"early_access"`
	Analytics    bool `json:"analytics"`
	PublicSignup bool `json:"public_author_signup"` // always false in V1
	TestEntitled bool `json:"test_entitlements"`
}

// Limits are client-visible input limits.
type Limits struct {
	PasswordMin    int `json:"password_min"`
	PasswordMax    int `json:"password_max"`
	MaxEarlyDays   int `json:"max_early_days"`
	PageSizeMax    int `json:"page_size_max"`
	SynopsisMax    int `json:"synopsis_max"`
	CommentMax     int `json:"comment_max"`
	UploadMaxBytes int `json:"upload_max_bytes"`
}

// Mount registers GET /public-config.
func Mount(r auth.Router, cfg *config.Config) error {
	pc := PublicConfig{
		AppName:       cfg.AppName,
		BaseURL:       cfg.BaseOrigin,
		Environment:   cfg.Env,
		NonProduction: !cfg.IsProduction(),
		Features: Features{
			Registration: true,
			EarlyAccess:  cfg.PatreonEnabled || cfg.AllowTestEntitlements,
			Analytics:    cfg.AnalyticsEnabled,
			TestEntitled: cfg.AllowTestEntitlements,
		},
		Limits: Limits{
			PasswordMin: auth.MinPasswordLen, PasswordMax: auth.MaxPasswordLen, MaxEarlyDays: cfg.MaxEarlyDays,
			PageSizeMax: 50, SynopsisMax: 4000, CommentMax: 10000, UploadMaxBytes: 5 << 20,
		},
	}
	return r.HandleFunc("GET "+auth.Base+"/public-config", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=60")
		httpx.WriteData(w, r, http.StatusOK, pc, nil)
	})
}
