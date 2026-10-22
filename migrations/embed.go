// Package migrations embeds Pawn.Ink's Goose migrations for the pawn_ink
// application database. Collar's own system database is migrated separately
// by `collar migrate` (cmd/collar).
package migrations

import (
	"embed"
	"strconv"
	"strings"
)

// FS holds every *.sql migration.
//
//go:embed *.sql
var FS embed.FS

// Latest returns the highest migration version embedded in this binary,
// used by /readyz to check schema compatibility.
func Latest() int64 {
	entries, err := FS.ReadDir(".")
	if err != nil {
		return 0
	}
	var latest int64
	for _, e := range entries {
		name := e.Name()
		i := strings.IndexByte(name, '_')
		if i <= 0 || !strings.HasSuffix(name, ".sql") {
			continue
		}
		if v, err := strconv.ParseInt(name[:i], 10, 64); err == nil && v > latest {
			latest = v
		}
	}
	return latest
}
