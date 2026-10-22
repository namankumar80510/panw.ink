package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/xmfunew/collar"

	"github.com/namankumar80510/panw.ink/internal/db"
)

// Job kinds.
const (
	KindUpdateWorkRollups = "update_work_rollups" // one work, enqueued at each release's public_at
	KindRefreshRollups    = "refresh_rollups"     // periodic sweep, backstop for missed jobs
)

// RollupJobs refreshes listing rollups in the worker. Listings stay
// time-correct without them (D-016); these keep word counts and "recently
// updated" ordering fresh.
type RollupJobs struct {
	DB  *sql.DB
	T   db.Tables
	Now func() time.Time
}

func (j *RollupJobs) now() time.Time {
	if j.Now != nil {
		return j.Now().UTC()
	}
	return time.Now().UTC()
}

// UpdateWork handles KindUpdateWorkRollups ({"work_id": "…"}).
func (j *RollupJobs) UpdateWork(ctx context.Context, job *collar.Job) error {
	var p struct {
		WorkID string `json:"work_id"`
	}
	if err := json.Unmarshal(job.Payload, &p); err != nil {
		return fmt.Errorf("rollups: payload: %w", err)
	}
	id, err := strconv.ParseUint(p.WorkID, 10, 64)
	if err != nil {
		return nil // malformed: nothing to retry
	}
	return RefreshRollups(ctx, j.DB, j.T, id, j.now())
}

// Sweep handles KindRefreshRollups: every work with an entry that became
// public in the last hour, or whose rollups are older than its newest public
// entry, is refreshed.
func (j *RollupJobs) Sweep(ctx context.Context, _ *collar.Job) error {
	now := j.now()
	rows, err := j.DB.QueryContext(ctx, "SELECT DISTINCT e.work_id FROM "+j.T.T("entries")+" e JOIN "+j.T.T("works")+" w ON w.id = e.work_id"+
		" WHERE e.workflow IN ('scheduled','published') AND e.public_at <= ? AND (e.public_at > ? OR w.rollups_at IS NULL OR w.rollups_at < e.public_at) LIMIT 1000",
		now, now.Add(-time.Hour))
	if err != nil {
		return fmt.Errorf("rollups: sweep: %w", err)
	}
	var ids []uint64
	for rows.Next() {
		var id uint64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	for _, id := range ids {
		if err := RefreshRollups(ctx, j.DB, j.T, id, now); err != nil {
			return err
		}
	}
	return nil
}
