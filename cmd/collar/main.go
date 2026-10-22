// Command collar is Collar's public CLI entrypoint for Pawn.Ink
// (collar.Main): `migrate` (the pawn_ink_collar system database only),
// `config check`, `superuser create` for restricted operator accounts, etc.
// Application owner bootstrap is separate: see cmd/maintenance.
package main

import (
	"fmt"
	"os"

	"github.com/xmfunew/collar"

	"github.com/namankumar80510/panw.ink/internal/boot"
	"github.com/namankumar80510/panw.ink/internal/config"
)

func main() {
	if err := config.LoadDotEnv(".env"); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	config.ExportCollarDefaults()
	path := os.Getenv("COLLAR_CONFIG")
	if path == "" {
		path = "collar.yaml"
	}
	os.Exit(collar.Main(os.Args[1:], collar.Options{ConfigPath: path, Version: boot.Version}))
}
