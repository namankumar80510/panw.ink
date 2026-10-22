package config

import (
	"strings"
	"testing"
)

func env(over map[string]string) func(string) (string, bool) {
	base := map[string]string{
		"APP_ENV":              "development",
		"COLLAR_DSN":           "u:p@tcp(127.0.0.1:3306)/",
		"SESSION_HMAC_KEY":     strings.Repeat("a", 64),
		"CSRF_HMAC_KEY":        strings.Repeat("b", 64),
		"TOKEN_ENCRYPTION_KEY": strings.Repeat("0f", 32),
		"RANKING_HMAC_KEY":     strings.Repeat("c", 64),
		"ANALYTICS_HMAC_KEY":   strings.Repeat("d", 64),
	}
	for k, v := range over {
		base[k] = v
	}
	return func(k string) (string, bool) { v, ok := base[k]; return v, ok }
}

func TestParseDefaults(t *testing.T) {
	c, err := Parse(env(nil))
	if err != nil {
		t.Fatal(err)
	}
	if c.AppDatabase != "pawn_ink" || c.SystemDatabase != "pawn_ink_collar" {
		t.Fatalf("databases: %q %q", c.AppDatabase, c.SystemDatabase)
	}
	if len(c.TokenEncryptionKey) != 32 || c.SecureCookies || c.AnalyticsFlushInterval != RequiredFlushInterval {
		t.Fatalf("unexpected config: %+v", c)
	}
	if c.SessionTTL > MaxSessionTTL {
		t.Fatalf("session ttl %v exceeds max", c.SessionTTL)
	}
}

func TestParseRejects(t *testing.T) {
	cases := map[string]map[string]string{
		"placeholder secret":         {"SESSION_HMAC_KEY": "CHANGE_ME_LONG_RANDOM_HEX"},
		"short secret":               {"CSRF_HMAC_KEY": "short"},
		"bad encryption key":         {"TOKEN_ENCRYPTION_KEY": "not-32-bytes"},
		"production over http":       {"APP_ENV": "production", "COLLAR_JWT_SECRET": strings.Repeat("j", 40)},
		"production public bind":     {"APP_ENV": "production", "APP_BASE_URL": "https://pawn.ink", "HTTP_ADDR": "0.0.0.0:14021", "COLLAR_JWT_SECRET": strings.Repeat("j", 40)},
		"production flush interval":  {"APP_ENV": "production", "APP_BASE_URL": "https://pawn.ink", "ANALYTICS_FLUSH_INTERVAL": "60s", "COLLAR_JWT_SECRET": strings.Repeat("j", 40)},
		"test entitlements in prod":  {"APP_ENV": "production", "APP_BASE_URL": "https://pawn.ink", "ALLOW_TEST_ENTITLEMENTS": "true", "COLLAR_JWT_SECRET": strings.Repeat("j", 40)},
		"test entitlements public":   {"ALLOW_TEST_ENTITLEMENTS": "true", "HTTP_ADDR": "0.0.0.0:14021"},
		"collar trusts proxy":        {"COLLAR_TRUST_PROXY": "true"},
		"collar admin dir":           {"COLLAR_ADMIN_DIR": "/srv/admin"},
		"collar addr mismatch":       {"COLLAR_ADDR": "127.0.0.1:9999"},
		"same databases":             {"COLLAR_SYSTEM_DATABASE": "pawn_ink"},
		"early days unlimited":       {"MAX_EARLY_DAYS": "90"},
		"web dist dir in production": {"APP_ENV": "production", "APP_BASE_URL": "https://pawn.ink", "WEB_DIST_DIR": "./web/dist", "COLLAR_JWT_SECRET": strings.Repeat("j", 40)},
	}
	for name, over := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse(env(over)); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestProductionOK(t *testing.T) {
	c, err := Parse(env(map[string]string{
		"APP_ENV": "production", "APP_BASE_URL": "https://pawn.ink", "COLLAR_JWT_SECRET": strings.Repeat("j", 40),
		"TRUSTED_PROXY_CIDRS": "127.0.0.1/32, ::1",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !c.SecureCookies || c.BaseOrigin != "https://pawn.ink" || len(c.TrustedProxies) != 2 {
		t.Fatalf("unexpected: %+v", c)
	}
}
