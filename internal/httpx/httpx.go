// Package httpx is the custom API's response contract (IMPLEMENTATION.md §7.1):
// the data/error envelopes, stable error codes and strict JSON decoding.
//
// Native Collar endpoints keep Collar's own contracts; this package applies to
// /api/v1/pawn/x routes and the wrapper's own routes only.
package httpx

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
)

// Error is a client-visible custom API error with a stable code.
type Error struct {
	Status  int
	Code    string
	Message string
	Details any
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

// NewError returns an Error.
func NewError(status int, code, message string) *Error {
	return &Error{Status: status, Code: code, Message: message}
}

// WithDetails returns a copy of e carrying details.
func (e *Error) WithDetails(d any) *Error {
	c := *e
	c.Details = d
	return &c
}

// Common errors.
var (
	ErrNotFound        = NewError(http.StatusNotFound, "NOT_FOUND", "Not found.")
	ErrUnauthenticated = NewError(http.StatusUnauthorized, "UNAUTHENTICATED", "Please sign in.")
	ErrForbidden       = NewError(http.StatusForbidden, "FORBIDDEN", "You do not have permission to do that.")
	ErrCSRF            = NewError(http.StatusForbidden, "CSRF_REJECTED", "This request could not be verified. Reload the page and try again.")
	ErrRateLimited     = NewError(http.StatusTooManyRequests, "RATE_LIMITED", "Too many requests. Please wait a moment and try again.")
	ErrInternal        = NewError(http.StatusInternalServerError, "INTERNAL", "Something went wrong on our side.")
	ErrEmailUnverified = NewError(http.StatusForbidden, "EMAIL_UNVERIFIED", "Please verify your email address first.")
)

// Validation returns a 422 with per-field messages.
func Validation(fields map[string]string) *Error {
	return &Error{Status: http.StatusUnprocessableEntity, Code: "VALIDATION_FAILED", Message: "Some fields need attention.", Details: map[string]any{"fields": fields}}
}

// Meta is the meta object of a data envelope.
type Meta struct {
	RequestID  string  `json:"request_id"`
	NextCursor *string `json:"next_cursor"`
}

// RequestID returns the request id Collar's middleware (or the wrapper) put on
// the response, creating one when absent.
func RequestID(w http.ResponseWriter, r *http.Request) string {
	if id := w.Header().Get("X-Request-Id"); id != "" {
		return id
	}
	if id := r.Header.Get("X-Request-Id"); validRequestID(id) {
		w.Header().Set("X-Request-Id", id)
		return id
	}
	var b [16]byte
	_, _ = rand.Read(b[:])
	id := hex.EncodeToString(b[:])
	w.Header().Set("X-Request-Id", id)
	return id
}

func validRequestID(s string) bool {
	if s == "" || len(s) > 64 {
		return false
	}
	for _, r := range s {
		if !(r == '-' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}

// WriteData writes {"data":..., "meta":{...}}.
func WriteData(w http.ResponseWriter, r *http.Request, status int, data any, nextCursor *string) {
	writeJSON(w, status, map[string]any{
		"data": data,
		"meta": Meta{RequestID: RequestID(w, r), NextCursor: nextCursor},
	})
}

// WriteError writes the error envelope. Non-*Error values are logged and
// become 500 INTERNAL without leaking details.
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	var ae *Error
	if !errors.As(err, &ae) {
		slog.Default().Error("internal error", "err", err, "path", r.URL.Path, "request_id", RequestID(w, r))
		ae = ErrInternal
	}
	body := map[string]any{"code": ae.Code, "message": ae.Message}
	if ae.Details != nil {
		body["details"] = ae.Details
	}
	writeJSON(w, ae.Status, map[string]any{"error": body, "request_id": RequestID(w, r)})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	h := w.Header()
	h.Set("Content-Type", "application/json; charset=utf-8")
	if h.Get("Cache-Control") == "" {
		h.Set("Cache-Control", "private, no-store")
	}
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(true)
	_ = enc.Encode(v) // a failed write means the client went away
}

// DecodeJSON strictly decodes a JSON object body of at most maxBytes into dst.
// Unknown fields are a 422 naming the field; malformed JSON is 400.
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any, maxBytes int64) error {
	if ct := r.Header.Get("Content-Type"); ct != "" && !strings.HasPrefix(strings.ToLower(ct), "application/json") {
		return NewError(http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE", "Send application/json.")
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		var mbe *http.MaxBytesError
		var ute *json.UnmarshalTypeError
		switch {
		case errors.As(err, &mbe):
			return NewError(http.StatusRequestEntityTooLarge, "BODY_TOO_LARGE", fmt.Sprintf("Request body exceeds %d bytes.", maxBytes))
		case errors.Is(err, io.EOF):
			return NewError(http.StatusBadRequest, "MALFORMED_JSON", "Request body is empty.")
		case errors.As(err, &ute):
			return Validation(map[string]string{ute.Field: "Has the wrong type."})
		case strings.HasPrefix(err.Error(), "json: unknown field "):
			field := strings.Trim(strings.TrimPrefix(err.Error(), "json: unknown field "), `"`)
			return Validation(map[string]string{field: "Unknown field."})
		default:
			return NewError(http.StatusBadRequest, "MALFORMED_JSON", "Request body is not valid JSON.")
		}
	}
	if dec.More() {
		return NewError(http.StatusBadRequest, "MALFORMED_JSON", "Request body must be a single JSON object.")
	}
	return nil
}

// MaxJSONBody is the default custom-API body limit (§12.1).
const MaxJSONBody = 1 << 20
