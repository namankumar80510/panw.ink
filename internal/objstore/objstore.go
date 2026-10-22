// Package objstore is a minimal S3-compatible client (AWS Signature V4, path
// addressing) for Pawn.Ink's server-side image pipeline: read an uploaded
// object, write validated variants, delete, and presign short-lived GETs for
// serving. Browser uploads go through Collar's presign/confirm endpoints.
//
// Only the standard library is used; secrets never appear in errors or logs.
package objstore

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Config is one bucket on an S3-compatible endpoint.
type Config struct {
	Endpoint  string // scheme://host[:port]
	Region    string // default us-east-1
	Bucket    string
	AccessKey string
	SecretKey string
}

// Client talks to one bucket.
type Client struct {
	cfg  Config
	base *url.URL
	http *http.Client
	now  func() time.Time
}

// ErrNotFound is returned for missing objects.
var ErrNotFound = errors.New("objstore: not found")

// New validates cfg and returns a client.
func New(cfg Config) (*Client, error) {
	u, err := url.Parse(cfg.Endpoint)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, errors.New("objstore: endpoint must be an http(s) origin")
	}
	if cfg.Bucket == "" || cfg.AccessKey == "" || cfg.SecretKey == "" {
		return nil, errors.New("objstore: bucket and credentials are required")
	}
	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}
	return &Client{cfg: cfg, base: u, http: &http.Client{Timeout: 30 * time.Second}, now: time.Now}, nil
}

// Origin is the endpoint origin (for the CSP img-src allowlist).
func (c *Client) Origin() string { return c.base.Scheme + "://" + c.base.Host }

func (c *Client) objectURL(key string) *url.URL {
	u := *c.base
	u.Path = "/" + c.cfg.Bucket
	if key != "" {
		u.Path += "/" + key
	}
	u.RawPath = "/" + uriEncode(c.cfg.Bucket, false)
	if key != "" {
		u.RawPath += "/" + uriEncode(key, false)
	}
	return &u
}

// Get downloads at most maxBytes of an object.
func (c *Client) Get(ctx context.Context, key string, maxBytes int64) ([]byte, string, error) {
	res, err := c.do(ctx, http.MethodGet, key, nil, "")
	if err != nil {
		return nil, "", err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(io.LimitReader(res.Body, maxBytes+1))
	if err != nil {
		return nil, "", fmt.Errorf("objstore: read %s: %w", key, err)
	}
	if int64(len(b)) > maxBytes {
		return nil, "", fmt.Errorf("objstore: object %s exceeds %d bytes", key, maxBytes)
	}
	return b, res.Header.Get("Content-Type"), nil
}

// Put uploads an object.
func (c *Client) Put(ctx context.Context, key string, body []byte, contentType string) error {
	res, err := c.do(ctx, http.MethodPut, key, body, contentType)
	if err != nil {
		return err
	}
	res.Body.Close()
	return nil
}

// Delete removes an object; deleting a missing object succeeds.
func (c *Client) Delete(ctx context.Context, key string) error {
	res, err := c.do(ctx, http.MethodDelete, key, nil, "")
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	res.Body.Close()
	return nil
}

// EnsureBucket creates the bucket when it does not exist (development).
func (c *Client) EnsureBucket(ctx context.Context) error {
	res, err := c.do(ctx, http.MethodHead, "", nil, "")
	if err == nil {
		res.Body.Close()
		return nil
	}
	if !errors.Is(err, ErrNotFound) {
		return err
	}
	res, err = c.do(ctx, http.MethodPut, "", nil, "")
	if err != nil {
		return err
	}
	res.Body.Close()
	return nil
}

// PresignGet returns a time-limited GET URL for an object.
func (c *Client) PresignGet(key string, ttl time.Duration) string {
	now := c.now().UTC()
	amz := now.Format("20060102T150405Z")
	date := now.Format("20060102")
	scope := date + "/" + c.cfg.Region + "/s3/aws4_request"
	u := c.objectURL(key)
	q := url.Values{}
	q.Set("X-Amz-Algorithm", "AWS4-HMAC-SHA256")
	q.Set("X-Amz-Credential", c.cfg.AccessKey+"/"+scope)
	q.Set("X-Amz-Date", amz)
	q.Set("X-Amz-Expires", strconv.Itoa(int(ttl.Seconds())))
	q.Set("X-Amz-SignedHeaders", "host")
	canonical := strings.Join([]string{http.MethodGet, u.EscapedPath(), canonicalQuery(q), "host:" + u.Host + "\n", "host", "UNSIGNED-PAYLOAD"}, "\n")
	sig := c.signature(date, amz, scope, canonical)
	q.Set("X-Amz-Signature", sig)
	u.RawQuery = canonicalQuery(q)
	return u.String()
}

func (c *Client) do(ctx context.Context, method, key string, body []byte, contentType string) (*http.Response, error) {
	u := c.objectURL(key)
	req, err := http.NewRequestWithContext(ctx, method, u.String(), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("objstore: %w", err)
	}
	now := c.now().UTC()
	amz := now.Format("20060102T150405Z")
	date := now.Format("20060102")
	scope := date + "/" + c.cfg.Region + "/s3/aws4_request"
	sum := sha256.Sum256(body)
	payload := hex.EncodeToString(sum[:])
	req.Header.Set("X-Amz-Date", amz)
	req.Header.Set("X-Amz-Content-Sha256", payload)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.ContentLength = int64(len(body))
	signed := []string{"host", "x-amz-content-sha256", "x-amz-date"}
	headers := "host:" + u.Host + "\nx-amz-content-sha256:" + payload + "\nx-amz-date:" + amz + "\n"
	if contentType != "" {
		signed = []string{"content-type", "host", "x-amz-content-sha256", "x-amz-date"}
		headers = "content-type:" + contentType + "\n" + headers
	}
	canonical := strings.Join([]string{method, u.EscapedPath(), "", headers, strings.Join(signed, ";"), payload}, "\n")
	sig := c.signature(date, amz, scope, canonical)
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+c.cfg.AccessKey+"/"+scope+", SignedHeaders="+strings.Join(signed, ";")+", Signature="+sig)
	res, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("objstore: %s %s: %w", method, key, err)
	}
	if res.StatusCode == http.StatusNotFound {
		res.Body.Close()
		return nil, ErrNotFound
	}
	if res.StatusCode >= 300 {
		res.Body.Close()
		return nil, fmt.Errorf("objstore: %s %s: status %d", method, key, res.StatusCode)
	}
	return res, nil
}

func (c *Client) signature(date, amz, scope, canonical string) string {
	h := sha256.Sum256([]byte(canonical))
	toSign := "AWS4-HMAC-SHA256\n" + amz + "\n" + scope + "\n" + hex.EncodeToString(h[:])
	k := hmacSHA256([]byte("AWS4"+c.cfg.SecretKey), date)
	k = hmacSHA256(k, c.cfg.Region)
	k = hmacSHA256(k, "s3")
	k = hmacSHA256(k, "aws4_request")
	return hex.EncodeToString(hmacSHA256(k, toSign))
}

func hmacSHA256(key []byte, s string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(s))
	return m.Sum(nil)
}

func canonicalQuery(q url.Values) string {
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, uriEncode(k, true)+"="+uriEncode(q.Get(k), true))
	}
	return strings.Join(parts, "&")
}

// uriEncode is the AWS SigV4 URI encoding (RFC 3986 unreserved characters
// kept; "/" kept in paths).
func uriEncode(s string, encodeSlash bool) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		ch := s[i]
		switch {
		case ch >= 'A' && ch <= 'Z', ch >= 'a' && ch <= 'z', ch >= '0' && ch <= '9', ch == '-', ch == '_', ch == '.', ch == '~':
			b.WriteByte(ch)
		case ch == '/' && !encodeSlash:
			b.WriteByte(ch)
		default:
			fmt.Fprintf(&b, "%%%02X", ch)
		}
	}
	return b.String()
}
