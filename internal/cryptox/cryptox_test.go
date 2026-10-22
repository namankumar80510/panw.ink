package cryptox

import (
	"bytes"
	"testing"
)

func TestSealOpen(t *testing.T) {
	s, err := NewSealer(bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal(err)
	}
	ct, err := s.Seal([]byte("refresh-token"), []byte("ad:1"))
	if err != nil {
		t.Fatal(err)
	}
	pt, err := s.Open(ct, []byte("ad:1"))
	if err != nil || string(pt) != "refresh-token" {
		t.Fatalf("open: %q %v", pt, err)
	}
	if _, err := s.Open(ct, []byte("ad:2")); err == nil {
		t.Fatal("wrong additional data must fail")
	}
	ct[len(ct)-1] ^= 1
	if _, err := s.Open(ct, []byte("ad:1")); err == nil {
		t.Fatal("tampered ciphertext must fail")
	}
	again, _ := s.Seal([]byte("refresh-token"), []byte("ad:1"))
	if bytes.Equal(again[1:13], ct[1:13]) {
		t.Fatal("nonces must differ")
	}
}

func TestKeyedHashDomains(t *testing.T) {
	k := []byte("key")
	if bytes.Equal(KeyedHash(k, "session", "x"), KeyedHash(k, "csrf", "x")) {
		t.Fatal("purposes must separate domains")
	}
	if a, _ := RandomToken(32); len(a) != 43 {
		t.Fatalf("token length %d", len(a))
	}
}
