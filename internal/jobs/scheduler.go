// Package jobs holds Pawn.Ink's recurring scheduler and housekeeping jobs.
//
// Collar provides the durable queue (claiming, retries, reaping) but no cron
// API. The scheduler turns UTC time buckets into Collar jobs exactly once per
// bucket: a scheduler_runs row and the job are inserted in one transaction,
// so restarts and multiple workers never double-enqueue, and a restart
// catches up on the current bucket (IMPLEMENTATION.md §12.3).
package jobs

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/xmfunew/collar"

	"github.com/namankumar80510/panw.ink/internal/db"
	"github.com/namankumar80510/panw.ink/internal/mail"
)

// Schedule is one recurring job.
type Schedule struct {
	Name  string        // scheduler_runs.name
	Kind  string        // Collar job kind
	Every time.Duration // UTC bucket width
	// Key formats the bucket into the job's unique key and scheduler_runs.bucket.
	Key func(bucket time.Time) string
}

// Scheduler enqueues due buckets.
type Scheduler struct {
	DB        *sql.DB
	Tables    db.Tables
	Jobs      mail.Enqueuer
	Schedules []Schedule
	Log       *slog.Logger
	Now       func() time.Time
	Tick      time.Duration
}

// Run enqueues the current bucket of every schedule at start and then on
// every tick, until ctx ends.
func (s *Scheduler) Run(ctx context.Context) {
	tick := s.Tick
	if tick == 0 {
		tick = 10 * time.Second
	}
	now := s.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	t := time.NewTicker(tick)
	defer t.Stop()
	for {
		for _, sc := range s.Schedules {
			if err := s.enqueue(ctx, sc, now().Truncate(sc.Every)); err != nil && ctx.Err() == nil {
				s.Log.Error("scheduler enqueue", "schedule", sc.Name, "err", err)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (s *Scheduler) enqueue(ctx context.Context, sc Schedule, bucket time.Time) error {
	key := sc.Key(bucket)
	return db.InTx(ctx, s.DB, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, "INSERT IGNORE INTO "+s.Tables.T("scheduler_runs")+" (name, bucket) VALUES (?, ?)", sc.Name, key)
		if err != nil {
			return fmt.Errorf("scheduler_runs: %w", err)
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return nil // this bucket was already enqueued
		}
		id, err := s.Jobs.EnqueueTx(ctx, tx, sc.Kind, map[string]string{"bucket": key}, collar.EnqueueOptions{UniqueKey: key})
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "UPDATE "+s.Tables.T("scheduler_runs")+" SET job_id = ? WHERE name = ? AND bucket = ?", id, sc.Name, key)
		return err
	})
}

// Daily is a 24-hour schedule keyed "<prefix>:<YYYY-MM-DD>".
func Daily(name, kind, prefix string) Schedule {
	return Schedule{Name: name, Kind: kind, Every: 24 * time.Hour, Key: func(b time.Time) string {
		return prefix + ":" + b.Format("2006-01-02")
	}}
}
