// Package boot composes the API and worker processes from configuration, so
// cmd/server, cmd/worker and the integration tests share one wiring.
package boot

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/xmfunew/collar"

	"github.com/namankumar80510/panw.ink/internal/auth"
	"github.com/namankumar80510/panw.ink/internal/catalog"
	"github.com/namankumar80510/panw.ink/internal/config"
	"github.com/namankumar80510/panw.ink/internal/db"
	"github.com/namankumar80510/panw.ink/internal/objstore"
	"github.com/namankumar80510/panw.ink/internal/redis"
	"github.com/namankumar80510/panw.ink/internal/server"
	"github.com/namankumar80510/panw.ink/internal/site"
	"github.com/namankumar80510/panw.ink/internal/studio"
	"github.com/namankumar80510/panw.ink/migrations"
	"github.com/namankumar80510/panw.ink/web"
)

// Version is set at build time with -ldflags.
var Version = "dev"

// API is a composed API process.
type API struct {
	Handler http.Handler
	Collar  *collar.App
	Bridge  *auth.Bridge
	Studio  *studio.Service
	Tables  db.Tables
	Redis   *redis.Client
}

// Close releases Collar and Redis. Call after the HTTP server has shut down.
func (a *API) Close() error {
	if a.Redis != nil {
		_ = a.Redis.Close()
	}
	return a.Collar.Close()
}

// Options tune NewAPI for tests.
type Options struct {
	Logger    *slog.Logger
	WebFS     fs.FS        // nil → WEB_DIST_DIR in development, else the embedded UI
	AuthLimit *auth.Limits // nil → auth.DefaultLimits()
	// Clock replaces time.Now for release decisions (tests). Production
	// always uses the real clock.
	Clock func() time.Time
}

// NewAPI builds Collar, registers every custom route and wraps it.
func NewAPI(ctx context.Context, cfg *config.Config, opts Options) (*API, error) {
	log := opts.Logger
	if log == nil {
		log = slog.Default()
	}
	capp, err := collar.New(ctx, collar.Options{ConfigPath: cfg.CollarConfigPath, Logger: log, Version: Version})
	if err != nil {
		return nil, fmt.Errorf("boot: collar: %w", err)
	}
	physical, err := capp.Database("pawn", "content")
	if err != nil {
		_ = capp.Close()
		return nil, fmt.Errorf("boot: %w", err)
	}
	if physical != cfg.AppDatabase {
		_ = capp.Close()
		return nil, fmt.Errorf("boot: collar.yaml maps pawn.content to %q but APP_DATABASE is %q", physical, cfg.AppDatabase)
	}
	t := db.NewTables(physical)

	rc, err := redis.Open(cfg.RedisURL)
	if err != nil {
		_ = capp.Close()
		return nil, err
	}

	bridge, err := auth.NewBridge(cfg, capp.DB(), t, log)
	if err != nil {
		_ = capp.Close()
		return nil, err
	}
	bridge.SetCollar(capp.Handler())

	limits := auth.DefaultLimits()
	if opts.AuthLimit != nil {
		limits = *opts.AuthLimit
	}
	ah := &auth.Handlers{
		Bridge:   bridge,
		Accounts: &auth.Accounts{Bridge: bridge, Jobs: capp.Jobs()},
		Limits:   limits,
		ClientIP: server.ClientIP,
	}
	routes := server.NewRoutes(capp)
	if err := ah.Mount(routes); err != nil {
		_ = capp.Close()
		return nil, fmt.Errorf("boot: mount auth routes: %w", err)
	}
	store, err := objstore.New(objstore.Config{Endpoint: cfg.S3Endpoint, Region: cfg.S3Region, Bucket: cfg.S3Bucket, AccessKey: cfg.S3AccessKey, SecretKey: cfg.S3SecretKey})
	if err != nil {
		_ = capp.Close()
		return nil, err
	}
	st := &studio.Service{DB: capp.DB(), T: t, Jobs: capp.Jobs(), Store: store, Now: opts.Clock, MaxEarlyDays: cfg.MaxEarlyDays, Log: log}
	if err := st.Mount(routes); err != nil {
		_ = capp.Close()
		return nil, fmt.Errorf("boot: mount studio routes: %w", err)
	}
	pub := &catalog.Public{DB: capp.DB(), T: t, Now: opts.Clock, CursorKey: cfg.RankingHMACKey}
	if err := pub.Mount(routes); err != nil {
		_ = capp.Close()
		return nil, fmt.Errorf("boot: mount catalogue routes: %w", err)
	}
	if err := mountPublic(routes, cfg); err != nil {
		_ = capp.Close()
		return nil, err
	}

	webFS := opts.WebFS
	if webFS == nil {
		webFS = web.Dist()
		if cfg.WebDistDir != "" {
			webFS = os.DirFS(cfg.WebDistDir)
		}
	}

	h := server.New(server.Deps{
		Config: cfg,
		Collar: capp.Handler(),
		Bridge: bridge,
		SPA:    server.NewSPA(webFS),
		Custom: routes,
		Health: healthFunc(capp.DB(), t, rc),
		Log:    log,
	})
	return &API{Handler: h, Collar: capp, Bridge: bridge, Studio: st, Tables: t, Redis: rc}, nil
}

// healthFunc: the application database must be reachable and migrated to at
// least this binary's schema; Redis is reported but never fails readiness.
func healthFunc(database *sql.DB, t db.Tables, rc *redis.Client) server.Health {
	want := migrations.Latest()
	return func(ctx context.Context) (bool, map[string]string) {
		ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		checks := map[string]string{}
		ok := true
		if err := database.PingContext(ctx); err != nil {
			checks["database"], ok = "unreachable", false
		} else {
			checks["database"] = "ok"
			var have sql.NullInt64
			err := database.QueryRowContext(ctx, "SELECT MAX(version_id) FROM "+t.T("goose_db_version")+" WHERE is_applied = 1").Scan(&have)
			switch {
			case err != nil:
				checks["schema"], ok = "unknown", false
			case have.Int64 < want:
				checks["schema"], ok = fmt.Sprintf("behind (have %d, need %d)", have.Int64, want), false
			default:
				checks["schema"] = "ok"
			}
		}
		checks["redis"] = redis.Health(ctx, rc)
		return ok, checks
	}
}

func mountPublic(r auth.Router, cfg *config.Config) error {
	if err := site.Mount(r, cfg); err != nil {
		return fmt.Errorf("boot: mount site routes: %w", err)
	}
	return nil
}
