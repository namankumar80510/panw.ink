package jobs

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/xmfunew/collar"

	"github.com/namankumar80510/panw.ink/internal/db"
)

// KindCleanupExpiredAuth removes expired auth artifacts daily.
const KindCleanupExpiredAuth = "cleanup_expired_auth"

// Retention of auth artifacts after they stop being usable.
const (
	authTokenRetention   = 7 * 24 * time.Hour
	sessionRetention     = 30 * 24 * time.Hour
	schedulerRetention   = 30 * 24 * time.Hour
	emailOutboxRetention = 30 * 24 * time.Hour
	deleteBatch          = 1000
)

// Cleanup implements the housekeeping job handlers.
type Cleanup struct {
	DB     *sql.DB
	Tables db.Tables
}

// ExpiredAuth deletes long-expired one-time tokens, ended browser sessions,
// old scheduler buckets and delivered mail rows, in bounded batches. It is
// idempotent.
func (c *Cleanup) ExpiredAuth(ctx context.Context, _ *collar.Job) error {
	now := time.Now().UTC()
	steps := []struct {
		table, where string
		arg          time.Time
	}{
		{"auth_tokens", "expires_at < ?", now.Add(-authTokenRetention)},
		{"browser_sessions", "(expires_at < ? OR revoked_at < ?)", now.Add(-sessionRetention)},
		{"scheduler_runs", "enqueued_at < ?", now.Add(-schedulerRetention)},
		{"email_outbox", "state <> 'queued' AND created_at < ?", now.Add(-emailOutboxRetention)},
	}
	for _, s := range steps {
		args := []any{s.arg}
		if s.table == "browser_sessions" {
			args = append(args, s.arg)
		}
		for {
			res, err := c.DB.ExecContext(ctx, "DELETE FROM "+c.Tables.T(s.table)+" WHERE "+s.where+" LIMIT ?", append(args, deleteBatch)...)
			if err != nil {
				return fmt.Errorf("cleanup %s: %w", s.table, err)
			}
			if n, _ := res.RowsAffected(); n < deleteBatch {
				break
			}
		}
	}
	return nil
}
