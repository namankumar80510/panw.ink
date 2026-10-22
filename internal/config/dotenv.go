package config

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
)

// LoadDotEnv sets variables from a KEY=VALUE file without overriding the real
// environment. A missing file is not an error. Values may be single- or
// double-quoted; lines starting with # are comments.
func LoadDotEnv(path string) error {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, val, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			return fmt.Errorf("config: %s:%d: want KEY=VALUE", path, n)
		}
		val = strings.TrimSpace(val)
		if len(val) >= 2 && (val[0] == '"' && val[len(val)-1] == '"' || val[0] == '\'' && val[len(val)-1] == '\'') {
			val = val[1 : len(val)-1]
		} else if i := strings.Index(val, " #"); i >= 0 {
			val = strings.TrimSpace(val[:i])
		}
		if _, set := os.LookupEnv(key); !set {
			if err := os.Setenv(key, val); err != nil {
				return fmt.Errorf("config: %s:%d: %w", path, n, err)
			}
		}
	}
	return sc.Err()
}

// ExportCollarDefaults sets the variables collar.yaml expands, when unset,
// so CLI tools that run without a complete Pawn.Ink config still agree with
// the server on database names.
func ExportCollarDefaults() {
	for k, v := range map[string]string{"APP_DATABASE": "pawn_ink", "COLLAR_SYSTEM_DATABASE": "pawn_ink_collar", "S3_REGION": "us-east-1"} {
		if s, ok := os.LookupEnv(k); !ok || strings.TrimSpace(s) == "" {
			_ = os.Setenv(k, v)
		}
	}
}
