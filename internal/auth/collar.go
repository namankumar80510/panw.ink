package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
)

// Native Collar auth routes the bridge dispatches to internally. Browser
// ingress blocks all of them (internal/server), so token-bearing responses
// never reach a browser.
const (
	collarLoginPath          = "/api/v1/pawn/auth/content/users/login"
	collarRefreshPath        = "/api/v1/auth/refresh"
	collarLogoutPath         = "/api/v1/auth/logout"
	collarMePath             = "/api/v1/auth/me"
	collarChangePasswordPath = "/api/v1/auth/change-password"
)

// collarLogin is Collar's login/refresh response (README §6.9, PLAN A12.4).
type collarLogin struct {
	Token        string `json:"token"`
	RefreshToken string `json:"refreshToken"`
	ExpiresIn    int    `json:"expiresIn"`
	Identity     struct {
		Kind       string `json:"kind"`
		ID         any    `json:"id"`
		App        string `json:"app"`
		Collection string `json:"collection"`
	} `json:"identity"`
}

type collarMe struct {
	SessionID string `json:"sessionId"`
}

// collarError is Collar's error envelope.
type collarError struct {
	Error struct {
		Code    string              `json:"code"`
		Message string              `json:"message"`
		Details map[string][]string `json:"details"`
	} `json:"error"`
}

// collarResponse is a buffered response of an internal dispatch.
type collarResponse struct {
	Status int
	Body   []byte
}

func (r collarResponse) decode(v any) error {
	if err := json.Unmarshal(r.Body, v); err != nil {
		return fmt.Errorf("auth: decode collar response (%d): %w", r.Status, err)
	}
	return nil
}

func (r collarResponse) errorCode() (string, map[string][]string) {
	var e collarError
	_ = json.Unmarshal(r.Body, &e)
	return e.Error.Code, e.Error.Details
}

// bufferWriter is a minimal http.ResponseWriter that buffers one response.
type bufferWriter struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (b *bufferWriter) Header() http.Header { return b.header }
func (b *bufferWriter) WriteHeader(code int) {
	if b.status == 0 {
		b.status = code
	}
}
func (b *bufferWriter) Write(p []byte) (int, error) {
	if b.status == 0 {
		b.status = http.StatusOK
	}
	return b.body.Write(p)
}

// dispatcher calls Collar's handler in process, using documented routes and
// contracts only. clientIP is the resolved client address, so Collar's
// per-IP login throttling sees the real client.
type dispatcher struct {
	handler http.Handler
}

var errNoCollar = errors.New("auth: collar handler not set")

func (d *dispatcher) do(ctx context.Context, method, path, bearer string, body any, clientIP, userAgent string) (collarResponse, error) {
	if d.handler == nil {
		return collarResponse{}, errNoCollar
	}
	var rdr *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return collarResponse{}, fmt.Errorf("auth: marshal: %w", err)
		}
		rdr = bytes.NewReader(raw)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://collar.internal"+path, rdr)
	if err != nil {
		return collarResponse{}, fmt.Errorf("auth: request: %w", err)
	}
	if clientIP == "" {
		clientIP = "127.0.0.1"
	}
	req.RemoteAddr = net.JoinHostPort(clientIP, "0")
	req.Header.Set("Content-Type", "application/json")
	if userAgent != "" {
		req.Header.Set("User-Agent", userAgent)
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	w := &bufferWriter{header: http.Header{}}
	d.handler.ServeHTTP(w, req)
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return collarResponse{Status: w.status, Body: w.body.Bytes()}, nil
}
