// Package logx builds the structured JSON logger. Callers never log
// passwords, tokens, cookies, email bodies or reader content.
package logx

import (
	"log/slog"
	"os"
)

// New returns a JSON logger at level (debug, info, warn, error).
func New(level string) *slog.Logger {
	var l slog.Level
	_ = l.UnmarshalText([]byte(level))
	return slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: l}))
}
