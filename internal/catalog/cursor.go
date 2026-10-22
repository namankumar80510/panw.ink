package catalog

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
)

// cursorPayload is an opaque keyset position. The server signs it, so clients
// cannot forge positions or reuse a cursor with other filters (§7, §12.4).
type cursorPayload struct {
	Sort   string `json:"s"`
	Filter string `json:"f"` // hash of the normalized filters
	Key    string `json:"k"` // last sort value
	ID     uint64 `json:"i"` // last id (tie-breaker)
}

var errBadCursor = errors.New("catalog: invalid cursor")

type cursorCodec struct{ key []byte }

func (c cursorCodec) mac(b []byte) []byte {
	m := hmac.New(sha256.New, c.key)
	m.Write([]byte("cursor\x00"))
	m.Write(b)
	return m.Sum(nil)[:16]
}

func (c cursorCodec) encode(p cursorPayload) string {
	raw, _ := json.Marshal(p)
	enc := base64.RawURLEncoding
	return enc.EncodeToString(raw) + "." + enc.EncodeToString(c.mac(raw))
}

func (c cursorCodec) decode(s, sort, filter string) (*cursorPayload, error) {
	if s == "" {
		return nil, nil
	}
	body, sig, ok := strings.Cut(s, ".")
	if !ok || len(s) > 512 {
		return nil, errBadCursor
	}
	enc := base64.RawURLEncoding
	raw, err1 := enc.DecodeString(body)
	mac, err2 := enc.DecodeString(sig)
	if err1 != nil || err2 != nil || !hmac.Equal(mac, c.mac(raw)) {
		return nil, errBadCursor
	}
	var p cursorPayload
	if err := json.Unmarshal(raw, &p); err != nil || p.Sort != sort || p.Filter != filter {
		return nil, errBadCursor
	}
	return &p, nil
}

func filterHash(parts ...string) string {
	h := sha256.Sum256([]byte(strings.Join(parts, "\x1f")))
	return base64.RawURLEncoding.EncodeToString(h[:9])
}
