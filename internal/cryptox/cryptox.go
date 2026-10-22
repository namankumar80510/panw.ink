// Package cryptox holds the small set of cryptographic helpers Pawn.Ink uses:
// AEAD encryption of stored provider/Collar credentials, random tokens and
// keyed hashes for lookup of opaque tokens.
package cryptox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
)

// keyVersion prefixes every ciphertext so keys can be rotated later
// (docs/RUNBOOK.md, "Rotate secrets").
const keyVersion byte = 1

// ErrDecrypt is returned for any ciphertext that cannot be opened. It never
// carries key or plaintext material.
var ErrDecrypt = errors.New("cryptox: cannot decrypt")

// Sealer encrypts and decrypts small secrets with AES-256-GCM.
type Sealer struct {
	aead cipher.AEAD
}

// NewSealer returns a Sealer for a 32-byte key.
func NewSealer(key []byte) (*Sealer, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("cryptox: key must be 32 bytes, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("cryptox: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("cryptox: %w", err)
	}
	return &Sealer{aead: aead}, nil
}

// Seal encrypts plaintext bound to additional data ad (for example the row
// purpose and id) as version || nonce || ciphertext.
func (s *Sealer) Seal(plaintext, ad []byte) ([]byte, error) {
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("cryptox: nonce: %w", err)
	}
	out := make([]byte, 0, 1+len(nonce)+len(plaintext)+s.aead.Overhead())
	out = append(out, keyVersion)
	out = append(out, nonce...)
	return s.aead.Seal(out, nonce, plaintext, ad), nil
}

// Open reverses Seal.
func (s *Sealer) Open(ciphertext, ad []byte) ([]byte, error) {
	ns := s.aead.NonceSize()
	if len(ciphertext) < 1+ns+s.aead.Overhead() || ciphertext[0] != keyVersion {
		return nil, ErrDecrypt
	}
	pt, err := s.aead.Open(nil, ciphertext[1:1+ns], ciphertext[1+ns:], ad)
	if err != nil {
		return nil, ErrDecrypt
	}
	return pt, nil
}

// RandomToken returns n random bytes encoded as unpadded base64url.
func RandomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("cryptox: random: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// KeyedHash is HMAC-SHA256(key, purpose || 0x00 || value): the stored lookup
// form of opaque tokens. The purpose separates domains (session cookie,
// email token, CSRF).
func KeyedHash(key []byte, purpose, value string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(purpose))
	m.Write([]byte{0})
	m.Write([]byte(value))
	return m.Sum(nil)
}

// Equal compares two byte slices in constant time.
func Equal(a, b []byte) bool {
	return hmac.Equal(a, b)
}

// SHA256 returns the SHA-256 digest of b.
func SHA256(b []byte) []byte {
	sum := sha256.Sum256(b)
	return sum[:]
}
