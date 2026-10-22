package auth

import (
	"crypto/subtle"
	"encoding/base64"
	"strings"
	"testing"

	"golang.org/x/crypto/argon2"
)

func TestHashPasswordPHC(t *testing.T) {
	h, err := HashPassword("correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(h, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" || parts[3] != "m=65536,t=2,p=1" {
		t.Fatalf("unexpected PHC string %q", h)
	}
	salt, _ := base64.RawStdEncoding.DecodeString(parts[4])
	key, _ := base64.RawStdEncoding.DecodeString(parts[5])
	got := argon2.IDKey([]byte("correct horse battery"), salt, 2, 64*1024, 1, uint32(len(key)))
	if subtle.ConstantTimeCompare(got, key) != 1 {
		t.Fatal("hash does not verify")
	}
}

func TestCheckPassword(t *testing.T) {
	for pw, ok := range map[string]bool{
		"short":                    false,
		"password1234":             false,
		strings.Repeat("x", 129):   false,
		"reader@example.invalid":   false,
		"a long enough passphrase": true,
		"ünïcødé-pässwörd":         true,
	} {
		if (CheckPassword(pw, "reader@example.invalid") == "") != ok {
			t.Errorf("CheckPassword(%q) ok=%v", pw, !ok)
		}
	}
}

func TestNormalizeEmail(t *testing.T) {
	for in, want := range map[string]string{
		" Reader@Example.COM ": "reader@example.com",
		"no-at-sign":           "",
		"a@b":                  "",
		"x <a@b.com>":          "",
		"a@b.co\r\nBcc: c@d":   "",
	} {
		if got := NormalizeEmail(in); got != want {
			t.Errorf("NormalizeEmail(%q)=%q want %q", in, got, want)
		}
	}
}
