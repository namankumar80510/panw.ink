// Command server is the Pawn.Ink API process: embedded Collar, custom reader
// and command routes, the browser-session bridge and both embedded SPAs.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/namankumar80510/panw.ink/internal/boot"
	"github.com/namankumar80510/panw.ink/internal/config"
	"github.com/namankumar80510/panw.ink/internal/logx"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "pawn-ink server:", err)
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

	api, err := boot.NewAPI(ctx, cfg, boot.Options{Logger: log})
	if err != nil {
		return err
	}

	ln, err := net.Listen("tcp", cfg.HTTPAddr)
	if err != nil {
		_ = api.Close()
		return fmt.Errorf("listen %s: %w", cfg.HTTPAddr, err)
	}
	srv := &http.Server{
		Handler:           api.Handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    64 << 10,
		// No overall WriteTimeout: Collar's realtime SSE streams are long-lived.
	}
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()
	log.Info("pawn-ink api listening", "addr", ln.Addr().String(), "env", cfg.Env, "version", boot.Version)

	select {
	case err := <-errc:
		_ = api.Close()
		return err
	case <-ctx.Done():
	}
	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	err = srv.Shutdown(shutdownCtx)
	if cerr := api.Close(); err == nil {
		err = cerr
	}
	if errors.Is(err, http.ErrServerClosed) {
		err = nil
	}
	return err
}
