package httpx

import (
	"net/netip"
	"sync"
	"time"
)

// Limiter is a fixed-window rate limiter keyed by arbitrary strings. It is
// per process: V1 runs a single API process per host (docs/DECISIONS.md D-008).
// Limits are initial knobs, not scientific values (§13.2).
type Limiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	buckets map[string]*bucket
	now     func() time.Time
	sweepAt time.Time
}

type bucket struct {
	start time.Time
	count int
}

// NewLimiter allows limit events per window per key.
func NewLimiter(limit int, window time.Duration) *Limiter {
	return &Limiter{limit: limit, window: window, buckets: map[string]*bucket{}, now: time.Now}
}

// Allow records one event for key and reports whether it is within the limit.
func (l *Limiter) Allow(key string) bool {
	if l == nil || l.limit <= 0 {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if now.After(l.sweepAt) {
		for k, b := range l.buckets {
			if now.Sub(b.start) >= l.window {
				delete(l.buckets, k)
			}
		}
		l.sweepAt = now.Add(l.window)
	}
	b, ok := l.buckets[key]
	if !ok || now.Sub(b.start) >= l.window {
		l.buckets[key] = &bucket{start: now, count: 1}
		return true
	}
	if b.count >= l.limit {
		return false
	}
	b.count++
	return true
}

// IPPrefix returns a coarse key for an address: /24 for IPv4 and /56 for
// IPv6, so rotating addresses inside one allocation share a bucket. It is a
// short-lived abuse signal, never a stored identity.
func IPPrefix(ip string) string {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return ip
	}
	bits := 24
	if addr.Is6() && !addr.Is4In6() {
		bits = 56
	}
	p, err := addr.Unmap().Prefix(bits)
	if err != nil {
		return ip
	}
	return p.String()
}

// ClientIP resolves the client address: the connection's peer, or — only
// when that peer is a configured trusted proxy — the right-most untrusted
// address of X-Forwarded-For.
func ClientIP(remoteAddr, xff string, trusted []netip.Prefix) string {
	peer := remoteAddr
	if ap, err := netip.ParseAddrPort(remoteAddr); err == nil {
		peer = ap.Addr().Unmap().String()
	}
	if !isTrusted(peer, trusted) || xff == "" {
		return peer
	}
	parts := splitComma(xff)
	for i := len(parts) - 1; i >= 0; i-- {
		addr, err := netip.ParseAddr(parts[i])
		if err != nil {
			return peer
		}
		s := addr.Unmap().String()
		if !isTrusted(s, trusted) {
			return s
		}
	}
	return peer
}

func isTrusted(ip string, trusted []netip.Prefix) bool {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return false
	}
	addr = addr.Unmap()
	for _, p := range trusted {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

func splitComma(s string) []string {
	var out []string
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ',' {
			part := s[start:i]
			for len(part) > 0 && part[0] == ' ' {
				part = part[1:]
			}
			for len(part) > 0 && part[len(part)-1] == ' ' {
				part = part[:len(part)-1]
			}
			if part != "" {
				out = append(out, part)
			}
			start = i + 1
		}
	}
	return out
}
