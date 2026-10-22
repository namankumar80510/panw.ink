// Command seed loads the deterministic development fixtures of
// IMPLEMENTATION.md §16. It refuses to run unless APP_ENV is development
// or test. The catalogue fixtures arrive with Phase 2.
package main

import (
	"fmt"
	"os"

	"github.com/namankumar80510/panw.ink/internal/config"
)

func main() {
	if err := config.LoadDotEnv(".env"); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if env := os.Getenv("APP_ENV"); env != config.EnvDevelopment && env != config.EnvTest {
		fmt.Fprintf(os.Stderr, "seed: refusing to run with APP_ENV=%q (development or test only)\n", env)
		os.Exit(1)
	}
	fmt.Println("seed: no fixtures yet (catalogue fixtures are added in Phase 2)")
}
