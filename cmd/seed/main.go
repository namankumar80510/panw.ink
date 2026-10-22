// Command seed loads the deterministic development fixtures of
// IMPLEMENTATION.md §16 (see internal/seed and tests/fixtures/README.md).
// It refuses to run unless APP_ENV is development or test.
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/go-sql-driver/mysql"

	"github.com/namankumar80510/panw.ink/internal/config"
	"github.com/namankumar80510/panw.ink/internal/db"
	"github.com/namankumar80510/panw.ink/internal/seed"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "seed:", err)
		os.Exit(1)
	}
}

func run() error {
	if err := config.LoadDotEnv(".env"); err != nil {
		return err
	}
	config.ExportCollarDefaults()
	if env := os.Getenv("APP_ENV"); env != config.EnvDevelopment && env != config.EnvTest {
		return fmt.Errorf("refusing to run with APP_ENV=%q (development or test only)", env)
	}
	c, err := mysql.ParseDSN(os.Getenv("COLLAR_DSN"))
	if err != nil {
		return fmt.Errorf("COLLAR_DSN: %w", err)
	}
	c.ParseTime, c.Loc = true, time.UTC
	c.Params = map[string]string{"time_zone": "'+00:00'"}
	database, err := sql.Open("mysql", c.FormatDSN())
	if err != nil {
		return err
	}
	defer database.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	err = seed.Run(ctx, database, db.NewTables(os.Getenv("APP_DATABASE")), time.Now(), os.Getenv("SEED_PASSWORD"))
	if errors.Is(err, seed.ErrAlreadySeeded) {
		fmt.Println("fixtures already present; nothing to do")
		return nil
	}
	if err != nil {
		return err
	}
	fmt.Println("seeded fixtures (accounts and password: tests/fixtures/README.md)")
	return nil
}
