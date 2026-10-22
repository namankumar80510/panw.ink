// Package config loads Pawn.Ink's process settings from the environment
// (optionally seeded from ./.env, matching Collar's own loading rules).
//
// COLLAR_* variables are Collar's; this package validates only the ones
// Pawn.Ink depends on and leaves the rest to collar.New. APP_DATABASE,
// COLLAR_SYSTEM_DATABASE, Redis and HTTP settings are Pawn.Ink settings that
// collar.yaml references through ${VAR} expansion.
package config

import (
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Environments.
const (
	EnvDevelopment = "development"
	EnvTest        = "test"
	EnvProduction  = "production"
)

// RequiredFlushInterval is the fixed production analytics flush cadence
// (IMPLEMENTATION.md §10.1).
const RequiredFlushInterval = 120 * time.Second

// MaxSessionTTL bounds a browser session (and therefore the Collar refresh
// TTL it relies on).
const MaxSessionTTL = 30 * 24 * time.Hour

// Config is every Pawn.Ink setting.
type Config struct {
	Env     string
	AppName string
	BaseURL *url.URL
	// BaseOrigin is scheme://host[:port] of BaseURL, the only accepted Origin
	// for cookie-authenticated mutations (plus loopback aliases in development).
	BaseOrigin     string
	SecureCookies  bool
	HTTPAddr       string
	TrustedProxies []netip.Prefix

	CollarConfigPath string
	AppDatabase      string
	SystemDatabase   string
	RuntimeDSN       string
	MigrateDSN       string
	SessionTTL       time.Duration

	RedisURL               string
	RedisKeyPrefix         string
	AnalyticsFlushInterval time.Duration
	AnalyticsEnabled       bool

	SessionHMACKey     []byte
	CSRFHMACKey        []byte
	TokenEncryptionKey []byte // exactly 32 bytes (AES-256-GCM)
	RankingHMACKey     []byte
	AnalyticsHMACKey   []byte

	OwnerBootstrapEmail string

	SMTPHost     string
	SMTPPort     int
	SMTPUsername string
	SMTPPassword string
	MailFrom     string

	PatreonEnabled           bool
	PatreonClientID          string
	PatreonClientSecret      string
	PatreonRedirectURL       string
	PatreonCreatorCampaignID string
	MaxEarlyDays             int
	AllowTestEntitlements    bool
	CSPReportOnly            bool
	WebDistDir               string   // development only: serve the Parcel output from disk
	AssetOrigins             []string // extra img-src/connect-src origins; the S3 origin is added automatically
	S3Endpoint               string
	S3Region                 string
	S3Bucket                 string
	S3AccessKey              string
	S3SecretKey              string
	LogLevel                 string
}

// IsProduction reports whether this is the production environment.
func (c *Config) IsProduction() bool { return c.Env == EnvProduction }

// IsDevelopment reports whether this is the development environment.
func (c *Config) IsDevelopment() bool { return c.Env == EnvDevelopment }

// Load reads ./.env (real environment variables win) and then the
// environment. It also exports defaults for the variables collar.yaml
// expands, so Collar and Pawn.Ink agree on database names.
func Load() (*Config, error) {
	if err := LoadDotEnv(".env"); err != nil {
		return nil, err
	}
	cfg, err := Parse(os.LookupEnv)
	if err != nil {
		return nil, err
	}
	for k, v := range map[string]string{
		"APP_DATABASE":           cfg.AppDatabase,
		"COLLAR_SYSTEM_DATABASE": cfg.SystemDatabase,
		"S3_REGION":              cfg.S3Region,
	} {
		if _, ok := os.LookupEnv(k); !ok {
			_ = os.Setenv(k, v)
		}
	}
	return cfg, nil
}

type parser struct {
	lookup func(string) (string, bool)
	errs   []error
}

func (p *parser) str(key, def string) string {
	if v, ok := p.lookup(key); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return def
}

func (p *parser) fail(key, msg string) {
	p.errs = append(p.errs, fmt.Errorf("%s: %s", key, msg))
}

func (p *parser) boolean(key string, def bool) bool {
	v := p.str(key, strconv.FormatBool(def))
	b, err := strconv.ParseBool(v)
	if err != nil {
		p.fail(key, "want true or false")
	}
	return b
}

func (p *parser) integer(key string, def, lo, hi int) int {
	v := p.str(key, strconv.Itoa(def))
	n, err := strconv.Atoi(v)
	if err != nil || n < lo || n > hi {
		p.fail(key, fmt.Sprintf("want an integer between %d and %d", lo, hi))
	}
	return n
}

func (p *parser) duration(key string, def time.Duration) time.Duration {
	v := p.str(key, def.String())
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		p.fail(key, "want a positive Go duration such as 120s")
	}
	return d
}

// secret reads a secret of at least minLen bytes. Placeholders containing
// CHANGE_ME are rejected everywhere: generate real values (scripts/dev.sh
// does so for development).
func (p *parser) secret(key string, minLen int) []byte {
	v := p.str(key, "")
	switch {
	case v == "":
		p.fail(key, "is required")
		return nil
	case strings.Contains(v, "CHANGE_ME"):
		p.fail(key, "still holds the .env.example placeholder; generate a random value")
		return nil
	case len(v) < minLen:
		// Never echo secret material.
		p.fail(key, fmt.Sprintf("must be at least %d bytes (got %d)", minLen, len(v)))
		return nil
	}
	return []byte(v)
}

// encryptionKey decodes a 32-byte key given as 64 hex characters or
// standard/URL base64.
func (p *parser) encryptionKey(key string) []byte {
	v := p.str(key, "")
	if v == "" {
		p.fail(key, "is required (32 bytes as 64 hex characters or base64)")
		return nil
	}
	if strings.Contains(v, "CHANGE_ME") {
		p.fail(key, "still holds the .env.example placeholder; generate a random value")
		return nil
	}
	if b, err := hex.DecodeString(v); err == nil && len(b) == 32 {
		return b
	}
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if b, err := enc.DecodeString(v); err == nil && len(b) == 32 {
			return b
		}
	}
	p.fail(key, "must decode to exactly 32 bytes (64 hex characters or base64)")
	return nil
}

// Parse builds a Config from lookup, reporting every problem at once.
func Parse(lookup func(string) (string, bool)) (*Config, error) {
	p := &parser{lookup: lookup}
	c := &Config{}

	c.Env = p.str("APP_ENV", EnvDevelopment)
	switch c.Env {
	case EnvDevelopment, EnvTest, EnvProduction:
	default:
		p.fail("APP_ENV", "want development, test or production")
	}
	c.AppName = p.str("APP_NAME", "Pawn.Ink")

	base := p.str("APP_BASE_URL", "http://localhost:14021")
	u, err := url.Parse(base)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || (u.Path != "" && u.Path != "/") {
		p.fail("APP_BASE_URL", "want an absolute http(s) origin such as https://pawn.ink")
	} else {
		c.BaseURL = u
		c.BaseOrigin = u.Scheme + "://" + u.Host
		c.SecureCookies = u.Scheme == "https"
	}

	c.HTTPAddr = p.str("HTTP_ADDR", "127.0.0.1:14021")
	if host, port, err := net.SplitHostPort(c.HTTPAddr); err != nil || port == "" {
		p.fail("HTTP_ADDR", "want host:port such as 127.0.0.1:14021")
	} else if c.Env == EnvProduction && !isLoopbackHost(host) {
		p.fail("HTTP_ADDR", "production must bind a loopback address behind Nginx")
	}
	if collarAddr := p.str("COLLAR_ADDR", ""); collarAddr != "" && collarAddr != c.HTTPAddr {
		p.fail("COLLAR_ADDR", "must equal HTTP_ADDR (the wrapper serves Collar's handler on that listener)")
	}

	for _, raw := range splitCSV(p.str("TRUSTED_PROXY_CIDRS", "")) {
		pfx, err := netip.ParsePrefix(raw)
		if err != nil {
			addr, aerr := netip.ParseAddr(raw)
			if aerr != nil {
				p.fail("TRUSTED_PROXY_CIDRS", fmt.Sprintf("%q is not a CIDR or IP", raw))
				continue
			}
			pfx = netip.PrefixFrom(addr, addr.BitLen())
		}
		c.TrustedProxies = append(c.TrustedProxies, pfx.Masked())
	}
	if v := p.str("COLLAR_TRUST_PROXY", "false"); v != "false" && v != "0" {
		// The wrapper resolves the client address from TRUSTED_PROXY_CIDRS and
		// hands Collar a clean RemoteAddr; Collar must not trust X-Forwarded-For.
		p.fail("COLLAR_TRUST_PROXY", "must be false; configure TRUSTED_PROXY_CIDRS instead")
	}
	if v := p.str("COLLAR_ADMIN_DIR", ""); v != "" {
		p.fail("COLLAR_ADMIN_DIR", "must be unset: Pawn.Ink owns /, /studio and /admin")
	}
	if v := p.str("COLLAR_GLOBAL_REGISTRATION", "false"); v != "false" && v != "0" {
		p.fail("COLLAR_GLOBAL_REGISTRATION", "must be false")
	}
	if v := p.str("COLLAR_AUTO_MIGRATE", "false"); v != "false" && v != "0" {
		p.fail("COLLAR_AUTO_MIGRATE", "must be false; run `make migrate` explicitly")
	}

	c.CollarConfigPath = p.str("COLLAR_CONFIG", "./collar.yaml")
	c.AppDatabase = p.str("APP_DATABASE", "pawn_ink")
	c.SystemDatabase = p.str("COLLAR_SYSTEM_DATABASE", "pawn_ink_collar")
	if !validDBName(c.AppDatabase) {
		p.fail("APP_DATABASE", "want [A-Za-z0-9_]{1,64}")
	}
	if !validDBName(c.SystemDatabase) || c.SystemDatabase == c.AppDatabase {
		p.fail("COLLAR_SYSTEM_DATABASE", "want [A-Za-z0-9_]{1,64}, different from APP_DATABASE")
	}
	c.RuntimeDSN = p.str("COLLAR_DSN", "")
	if c.RuntimeDSN == "" {
		p.fail("COLLAR_DSN", "is required")
	}
	c.MigrateDSN = p.str("COLLAR_ADMIN_DSN", "")

	refresh := p.duration("COLLAR_REFRESH_TTL", 720*time.Hour)
	c.SessionTTL = min(refresh, MaxSessionTTL)

	c.RedisURL = p.str("REDIS_URL", "redis://127.0.0.1:6379/0")
	c.RedisKeyPrefix = p.str("REDIS_KEY_PREFIX", "pawn:dev:analytics")
	c.AnalyticsFlushInterval = p.duration("ANALYTICS_FLUSH_INTERVAL", RequiredFlushInterval)
	if c.Env == EnvProduction && c.AnalyticsFlushInterval != RequiredFlushInterval {
		p.fail("ANALYTICS_FLUSH_INTERVAL", "is fixed at 120s in production")
	}
	c.AnalyticsEnabled = p.boolean("ANALYTICS_ENABLED", true)

	c.SessionHMACKey = p.secret("SESSION_HMAC_KEY", 32)
	c.CSRFHMACKey = p.secret("CSRF_HMAC_KEY", 32)
	c.TokenEncryptionKey = p.encryptionKey("TOKEN_ENCRYPTION_KEY")
	c.RankingHMACKey = p.secret("RANKING_HMAC_KEY", 32)
	c.AnalyticsHMACKey = p.secret("ANALYTICS_HMAC_KEY", 32)
	if jwt := p.str("COLLAR_JWT_SECRET", ""); strings.Contains(jwt, "CHANGE_ME") {
		p.fail("COLLAR_JWT_SECRET", "still holds the .env.example placeholder; generate a random value")
	} else if c.Env == EnvProduction && len(jwt) < 32 {
		p.fail("COLLAR_JWT_SECRET", "must be set (at least 32 bytes) in production")
	}

	c.OwnerBootstrapEmail = strings.ToLower(p.str("OWNER_BOOTSTRAP_EMAIL", ""))

	c.SMTPHost = p.str("SMTP_HOST", "localhost")
	c.SMTPPort = p.integer("SMTP_PORT", 1025, 1, 65535)
	c.SMTPUsername = p.str("SMTP_USERNAME", "")
	c.SMTPPassword = p.str("SMTP_PASSWORD", "")
	c.MailFrom = p.str("MAIL_FROM", "no-reply@example.invalid")

	c.PatreonEnabled = p.boolean("PATREON_ENABLED", false)
	c.PatreonClientID = p.str("PATREON_CLIENT_ID", "")
	c.PatreonClientSecret = p.str("PATREON_CLIENT_SECRET", "")
	c.PatreonRedirectURL = p.str("PATREON_REDIRECT_URL", "")
	c.PatreonCreatorCampaignID = p.str("PATREON_CREATOR_CAMPAIGN_ID", "")
	if c.PatreonEnabled && (c.PatreonClientID == "" || c.PatreonClientSecret == "" || c.PatreonRedirectURL == "") {
		p.fail("PATREON_ENABLED", "requires PATREON_CLIENT_ID, PATREON_CLIENT_SECRET and PATREON_REDIRECT_URL")
	}
	c.MaxEarlyDays = p.integer("MAX_EARLY_DAYS", 30, 1, 30)

	c.AllowTestEntitlements = p.boolean("ALLOW_TEST_ENTITLEMENTS", false)
	if c.AllowTestEntitlements {
		host, _, _ := net.SplitHostPort(c.HTTPAddr)
		if c.Env != EnvDevelopment && c.Env != EnvTest {
			p.fail("ALLOW_TEST_ENTITLEMENTS", "is only allowed when APP_ENV=development (or test)")
		} else if !isLoopbackHost(host) {
			p.fail("ALLOW_TEST_ENTITLEMENTS", "requires HTTP_ADDR bound to loopback")
		}
	}
	c.CSPReportOnly = p.boolean("CSP_REPORT_ONLY", false)
	c.WebDistDir = p.str("WEB_DIST_DIR", "")
	if c.WebDistDir != "" && c.Env == EnvProduction {
		p.fail("WEB_DIST_DIR", "must be unset in production (the embedded UI is always used)")
	}
	c.AssetOrigins = splitCSV(p.str("ASSET_ORIGINS", ""))
	c.S3Endpoint = strings.TrimRight(p.str("S3_ENDPOINT", ""), "/")
	c.S3Region = p.str("S3_REGION", "us-east-1")
	c.S3Bucket = p.str("S3_BUCKET", "")
	c.S3AccessKey = p.str("S3_ACCESS_KEY", "")
	c.S3SecretKey = p.str("S3_SECRET_KEY", "")
	if u, err := url.Parse(c.S3Endpoint); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || (u.Path != "" && u.Path != "/") {
		p.fail("S3_ENDPOINT", "want the object store origin, e.g. https://s3.eu-west-1.amazonaws.com")
	} else {
		if c.Env == EnvProduction && u.Scheme != "https" {
			p.fail("S3_ENDPOINT", "production requires https")
		}
		c.AssetOrigins = append(c.AssetOrigins, u.Scheme+"://"+u.Host)
	}
	if c.S3Bucket == "" || c.S3AccessKey == "" || c.S3SecretKey == "" {
		p.fail("S3_BUCKET", "S3_BUCKET, S3_ACCESS_KEY and S3_SECRET_KEY are required (development uses the Compose MinIO)")
	}
	c.LogLevel = strings.ToLower(p.str("LOG_LEVEL", "info"))
	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		p.fail("LOG_LEVEL", "want debug, info, warn or error")
	}

	if c.Env == EnvProduction && !c.SecureCookies {
		p.fail("APP_BASE_URL", "production requires https")
	}

	if len(p.errs) > 0 {
		return nil, errors.Join(p.errs...)
	}
	return c, nil
}

func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	addr, err := netip.ParseAddr(host)
	return err == nil && addr.IsLoopback()
}

func validDBName(s string) bool {
	if s == "" || len(s) > 64 {
		return false
	}
	for _, r := range s {
		if !(r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}

func splitCSV(s string) []string {
	var out []string
	for part := range strings.SplitSeq(s, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}
