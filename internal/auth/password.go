package auth

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

// Password policy (§13.2): 12–128 characters. Collar's auth collection is
// configured with the same minimum (collar.yaml minPasswordLength: 12) and
// argon2id's 128-byte maximum.
const (
	MinPasswordLen = 12
	MaxPasswordLen = 128
)

// argon2id parameters. They MUST match the parameters Collar uses for new
// hashes at the pinned revision (internal/passwd/argon2.go in
// github.com/xmfunew/collar v0.1.1): otherwise Collar's rehash-on-login would
// silently rewrite hashes, and its verifier must accept ours. The integration
// test TestRegisterThenCollarLogin proves Collar verifies these hashes.
// Calibration notes for production hardware live in docs/DECISIONS.md D-004.
const (
	argonMemory      uint32 = 64 * 1024
	argonTime        uint32 = 2
	argonParallelism uint8  = 1
	argonSaltLen            = 16
	argonKeyLen      uint32 = 32
)

// commonPasswords is a small blocklist of the most common long passwords.
// It is a speed bump, not a breach corpus.
var commonPasswords = map[string]bool{
	"123456789012": true, "1234567890123": true, "password1234": true, "passwordpassword": true,
	"qwertyuiopas": true, "qwerty123456": true, "iloveyou1234": true, "letmein12345": true,
	"aaaaaaaaaaaa": true, "000000000000": true, "111111111111": true, "abcdefghijkl": true,
	"administrator": true, "changemechangeme": true, "welcome12345": true, "pawninkpawnink": true,
}

// CheckPassword validates a new password; the message is user-facing.
func CheckPassword(pw, email string) string {
	n := utf8.RuneCountInString(pw)
	switch {
	case n < MinPasswordLen:
		return fmt.Sprintf("Use at least %d characters.", MinPasswordLen)
	case len(pw) > MaxPasswordLen:
		return fmt.Sprintf("Use at most %d characters.", MaxPasswordLen)
	case commonPasswords[strings.ToLower(pw)]:
		return "This password is too common. Choose another."
	case email != "" && strings.EqualFold(pw, email):
		return "Do not use your email address as your password."
	}
	return ""
}

// HashPassword returns a PHC-format argon2id hash
// ($argon2id$v=19$m=65536,t=2,p=1$<salt>$<key>).
func HashPassword(pw string) (string, error) {
	if len(pw) > MaxPasswordLen {
		return "", fmt.Errorf("auth: password too long")
	}
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("auth: salt: %w", err)
	}
	key := argon2.IDKey([]byte(pw), salt, argonTime, argonMemory, argonParallelism, argonKeyLen)
	enc := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonParallelism,
		enc.EncodeToString(salt), enc.EncodeToString(key)), nil
}
