// Command worker is the Pawn.Ink background process: Collar's worker (job
// queue, subscriptions, webhook delivery, retention, reaper) plus Pawn.Ink's
// job handlers and recurring scheduler. The 120-second Redis analytics flush
// joins this process in Phase 6.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/xmfunew/collar"

	"github.com/namankumar80510/panw.ink/internal/boot"
	"github.com/namankumar80510/panw.ink/internal/catalog"
	"github.com/namankumar80510/panw.ink/internal/config"
	"github.com/namankumar80510/panw.ink/internal/db"
	"github.com/namankumar80510/panw.ink/internal/jobs"
	"github.com/namankumar80510/panw.ink/internal/logx"
	"github.com/namankumar80510/panw.ink/internal/mail"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "pawn-ink worker:", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := logx.New(cfg.LogLevel)
	slog.SetDefault(log)

	w, err := collar.NewWorker(ctx, collar.Options{ConfigPath: cfg.CollarConfigPath, Logger: log, Version: boot.Version})
	if err != nil {
		return err
	}
	physical, err := w.Database("pawn", "content")
	if err != nil {
		_ = w.Close()
		return err
	}
	t := db.NewTables(physical)

	mailer := &mail.Handler{
		DB: w.DB(), Tables: t, From: cfg.MailFrom, AppName: cfg.AppName,
		Sender: mail.SMTPSender{Host: cfg.SMTPHost, Port: cfg.SMTPPort, Username: cfg.SMTPUsername, Password: cfg.SMTPPassword},
	}
	cleanup := &jobs.Cleanup{DB: w.DB(), Tables: t}
	w.HandleJob(mail.JobKind, mailer.HandleJob)
	w.HandleJob(jobs.KindCleanupExpiredAuth, cleanup.ExpiredAuth)
	rollups := &catalog.RollupJobs{DB: w.DB(), T: t}
	w.HandleJob(catalog.KindUpdateWorkRollups, rollups.UpdateWork)
	w.HandleJob(catalog.KindRefreshRollups, rollups.Sweep)

	sched := &jobs.Scheduler{
		DB: w.DB(), Tables: t, Jobs: w.Jobs(), Log: log,
		Schedules: []jobs.Schedule{
			jobs.Daily("cleanup_expired_auth", jobs.KindCleanupExpiredAuth, "authcleanup"),
			jobs.Every("refresh_rollups", catalog.KindRefreshRollups, "rollups", 10*time.Minute),
		},
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); sched.Run(ctx) }()

	log.Info("pawn-ink worker started", "version", boot.Version)
	err = w.Run(ctx) // returns after ctx ends and in-flight jobs finish (COLLAR_SHUTDOWN_TIMEOUT)
	stop()
	wg.Wait()
	return err
}
