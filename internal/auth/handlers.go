package auth

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/namankumar80510/panw.ink/internal/httpx"
)

// Base is the custom API prefix (§1.1 item 4).
const Base = "/api/v1/pawn/x"

// Limits groups the rate limiters of the auth routes (§13.2).
type Limits struct {
	Register *httpx.Limiter // per IP prefix
	Login    *httpx.Limiter // per IP + email
	Reset    *httpx.Limiter // per IP prefix + email
	Resend   *httpx.Limiter // per IP prefix + email
	Verify   *httpx.Limiter // per IP prefix
}

// DefaultLimits are the initial knobs from §13.2.
func DefaultLimits() Limits {
	return Limits{
		Register: httpx.NewLimiter(5, time.Hour),
		Login:    httpx.NewLimiter(10, 15*time.Minute),
		Reset:    httpx.NewLimiter(3, time.Hour),
		Resend:   httpx.NewLimiter(3, time.Hour),
		Verify:   httpx.NewLimiter(30, time.Hour),
	}
}

// Router is the subset of *collar.App used to register custom routes.
type Router interface {
	HandleFunc(pattern string, h http.HandlerFunc) error
}

// ClientIPFunc returns the resolved client address of a request.
type ClientIPFunc func(r *http.Request) string

// Handlers serves the custom auth/profile routes (§7.3).
type Handlers struct {
	Bridge   *Bridge
	Accounts *Accounts
	Limits   Limits
	ClientIP ClientIPFunc
}

// Mount registers the routes.
func (h *Handlers) Mount(r Router) error {
	routes := map[string]http.HandlerFunc{
		"POST " + Base + "/auth/register":            h.register,
		"POST " + Base + "/auth/login":               h.login,
		"POST " + Base + "/auth/logout":              h.logout,
		"POST " + Base + "/auth/logout-all":          h.requireViewer(h.logoutAll),
		"GET " + Base + "/auth/me":                   h.requireViewer(h.me),
		"POST " + Base + "/auth/verify-email":        h.verifyEmail,
		"POST " + Base + "/auth/resend-verification": h.resendVerification,
		"POST " + Base + "/auth/password/forgot":     h.forgotPassword,
		"POST " + Base + "/auth/password/reset":      h.resetPassword,
		"POST " + Base + "/auth/password/change":     h.requireViewer(h.changePassword),
		"PATCH " + Base + "/users/me":                h.requireViewer(h.updateProfile),
		"GET " + Base + "/users/me/sessions":         h.requireViewer(h.sessions),
		"DELETE " + Base + "/users/me/sessions/{id}": h.requireViewer(h.deleteSession),
	}
	for pattern, fn := range routes {
		if err := r.HandleFunc(pattern, fn); err != nil {
			return err
		}
	}
	return nil
}

func (h *Handlers) requireViewer(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if ViewerFrom(r.Context()) == nil {
			httpx.WriteError(w, r, httpx.ErrUnauthenticated)
			return
		}
		next(w, r)
	}
}

// UserDTO is the public account shape returned to its owner.
type UserDTO struct {
	ID            string          `json:"id"`
	Email         string          `json:"email"`
	Handle        *string         `json:"handle"`
	DisplayName   string          `json:"display_name"`
	Role          string          `json:"role"`
	EmailVerified bool            `json:"email_verified"`
	Timezone      string          `json:"timezone"`
	Settings      json.RawMessage `json:"settings"`
	Permissions   Permissions     `json:"permissions"`
}

// Permissions are UI hints; the API enforces every permission itself.
type Permissions struct {
	CanComment bool `json:"can_comment"`
	CanReview  bool `json:"can_review"`
	Studio     bool `json:"studio"`
	Admin      bool `json:"admin"`
}

func (h *Handlers) dto(r *http.Request, v *Viewer) UserDTO {
	d := UserDTO{
		ID: strconv.FormatUint(v.UserID, 10), Email: v.Email, DisplayName: v.DisplayName, Role: v.Role,
		EmailVerified: v.EmailVerified, Timezone: v.Timezone, Settings: json.RawMessage("null"),
		Permissions: Permissions{
			CanComment: v.EmailVerified, CanReview: v.EmailVerified,
			Studio: v.CanUseStudio(), Admin: v.IsStaff(),
		},
	}
	if v.Handle != "" {
		hd := v.Handle
		d.Handle = &hd
	}
	if raw, err := h.Accounts.Settings(r.Context(), v.UserID); err == nil && len(raw) > 0 && json.Valid(raw) {
		d.Settings = raw
	}
	return d
}

type sessionPayload struct {
	User      UserDTO `json:"user"`
	CSRFToken string  `json:"csrf_token"`
}

func (h *Handlers) register(w http.ResponseWriter, r *http.Request) {
	var in RegisterInput
	if err := httpx.DecodeJSON(w, r, &in, httpx.MaxJSONBody); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if !h.Limits.Register.Allow("reg:" + httpx.IPPrefix(h.ClientIP(r))) {
		httpx.WriteError(w, r, httpx.ErrRateLimited)
		return
	}
	if err := h.Accounts.Register(r.Context(), in, httpx.RequestID(w, r)); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteData(w, r, http.StatusAccepted, map[string]string{
		"status":  "verification_sent",
		"message": "Check your inbox to confirm your email address, then sign in.",
	}, nil)
}

type loginInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *Handlers) login(w http.ResponseWriter, r *http.Request) {
	var in loginInput
	if err := httpx.DecodeJSON(w, r, &in, httpx.MaxJSONBody); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	email := NormalizeEmail(in.Email)
	ip := h.ClientIP(r)
	if !h.Limits.Login.Allow("login:" + ip + "|" + email) {
		httpx.WriteError(w, r, httpx.ErrRateLimited)
		return
	}
	if email == "" || in.Password == "" || len(in.Password) > MaxPasswordLen {
		httpx.WriteError(w, r, ErrInvalidCredentials)
		return
	}
	res, err := h.Bridge.Login(r.Context(), email, in.Password, ip, r.UserAgent())
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	// A pre-existing session cookie on this browser is replaced; end it.
	if old := ViewerFrom(r.Context()); old != nil {
		h.Bridge.Revoke(r.Context(), old.SessionID, old.UserID, ip, r.UserAgent())
	}
	h.Bridge.SetCookie(w, res.RawCookie, res.Expires)
	httpx.WriteData(w, r, http.StatusOK, sessionPayload{User: h.dto(r, res.Viewer), CSRFToken: res.Viewer.CSRFToken}, nil)
}

func (h *Handlers) logout(w http.ResponseWriter, r *http.Request) {
	if v := ViewerFrom(r.Context()); v != nil {
		h.Bridge.Revoke(r.Context(), v.SessionID, v.UserID, h.ClientIP(r), r.UserAgent())
	}
	h.Bridge.ClearCookie(w)
	w.Header().Set("Cache-Control", "private, no-store")
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) logoutAll(w http.ResponseWriter, r *http.Request) {
	v := ViewerFrom(r.Context())
	if err := h.Bridge.RevokeAllForUser(r.Context(), v.UserID, 0, h.ClientIP(r)); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	h.Bridge.ClearCookie(w)
	w.Header().Set("Cache-Control", "private, no-store")
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) me(w http.ResponseWriter, r *http.Request) {
	v := ViewerFrom(r.Context())
	httpx.WriteData(w, r, http.StatusOK, sessionPayload{User: h.dto(r, v), CSRFToken: v.CSRFToken}, nil)
}

type tokenInput struct {
	Token string `json:"token"`
}

func (h *Handlers) verifyEmail(w http.ResponseWriter, r *http.Request) {
	var in tokenInput
	if err := httpx.DecodeJSON(w, r, &in, httpx.MaxJSONBody); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if !h.Limits.Verify.Allow("verify:" + httpx.IPPrefix(h.ClientIP(r))) {
		httpx.WriteError(w, r, httpx.ErrRateLimited)
		return
	}
	if err := h.Accounts.VerifyEmail(r.Context(), in.Token, httpx.RequestID(w, r)); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteData(w, r, http.StatusOK, map[string]string{"status": "verified"}, nil)
}

type emailInput struct {
	Email string `json:"email"`
}

// generic is the single response of non-enumerating endpoints.
func generic(w http.ResponseWriter, r *http.Request) {
	httpx.WriteData(w, r, http.StatusAccepted, map[string]string{
		"status":  "accepted",
		"message": "If an account matches that address, we have sent it an email.",
	}, nil)
}

func (h *Handlers) resendVerification(w http.ResponseWriter, r *http.Request) {
	var in emailInput
	if err := httpx.DecodeJSON(w, r, &in, httpx.MaxJSONBody); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if !h.Limits.Resend.Allow("resend:" + httpx.IPPrefix(h.ClientIP(r)) + "|" + NormalizeEmail(in.Email)) {
		httpx.WriteError(w, r, httpx.ErrRateLimited)
		return
	}
	if err := h.Accounts.ResendVerification(r.Context(), in.Email); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	generic(w, r)
}

func (h *Handlers) forgotPassword(w http.ResponseWriter, r *http.Request) {
	var in emailInput
	if err := httpx.DecodeJSON(w, r, &in, httpx.MaxJSONBody); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if !h.Limits.Reset.Allow("reset:" + httpx.IPPrefix(h.ClientIP(r)) + "|" + NormalizeEmail(in.Email)) {
		httpx.WriteError(w, r, httpx.ErrRateLimited)
		return
	}
	if err := h.Accounts.ForgotPassword(r.Context(), in.Email); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	generic(w, r)
}

type resetInput struct {
	Token       string `json:"token"`
	NewPassword string `json:"new_password"`
}

func (h *Handlers) resetPassword(w http.ResponseWriter, r *http.Request) {
	var in resetInput
	if err := httpx.DecodeJSON(w, r, &in, httpx.MaxJSONBody); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if !h.Limits.Verify.Allow("verify:" + httpx.IPPrefix(h.ClientIP(r))) {
		httpx.WriteError(w, r, httpx.ErrRateLimited)
		return
	}
	if err := h.Accounts.ResetPassword(r.Context(), in.Token, in.NewPassword, h.ClientIP(r), httpx.RequestID(w, r)); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	// Any session this browser had is now revoked.
	h.Bridge.ClearCookie(w)
	httpx.WriteData(w, r, http.StatusOK, map[string]string{"status": "password_reset"}, nil)
}

type changeInput struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

func (h *Handlers) changePassword(w http.ResponseWriter, r *http.Request) {
	v := ViewerFrom(r.Context())
	var in changeInput
	if err := httpx.DecodeJSON(w, r, &in, httpx.MaxJSONBody); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if msg := CheckPassword(in.NewPassword, v.Email); msg != "" {
		httpx.WriteError(w, r, httpx.Validation(map[string]string{"new_password": msg}))
		return
	}
	ip := h.ClientIP(r)
	if !h.Limits.Login.Allow("login:" + ip + "|" + v.Email) {
		httpx.WriteError(w, r, httpx.ErrRateLimited)
		return
	}
	access := AccessTokenFrom(r.Context())
	if err := h.Bridge.ChangePassword(r.Context(), access, in.OldPassword, in.NewPassword, ip, r.UserAgent()); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if err := h.Bridge.RevokeAllForUser(r.Context(), v.UserID, v.SessionID, ip); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	raw, expires, err := h.Bridge.Rotate(r.Context(), v.SessionID)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	h.Bridge.SetCookie(w, raw, expires)
	httpx.WriteData(w, r, http.StatusOK, map[string]string{"status": "password_changed", "csrf_token": h.Bridge.CSRFFor(raw)}, nil)
}

func (h *Handlers) updateProfile(w http.ResponseWriter, r *http.Request) {
	v := ViewerFrom(r.Context())
	var in ProfileInput
	if err := httpx.DecodeJSON(w, r, &in, httpx.MaxJSONBody); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	var settings []byte
	if in.Settings != nil {
		raw, err := json.Marshal(*in.Settings)
		if err != nil {
			httpx.WriteError(w, r, httpx.Validation(map[string]string{"settings": "Must be a JSON object."}))
			return
		}
		settings = raw
	}
	if err := h.Accounts.UpdateProfile(r.Context(), v.UserID, in, settings); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	fresh, err := h.Bridge.loadViewer(r.Context(), v.UserID)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	fresh.SessionID, fresh.CSRFToken = v.SessionID, v.CSRFToken
	httpx.WriteData(w, r, http.StatusOK, h.dto(r, fresh), nil)
}

func (h *Handlers) sessions(w http.ResponseWriter, r *http.Request) {
	v := ViewerFrom(r.Context())
	items, err := h.Bridge.ListSessions(r.Context(), v.UserID, v.SessionID)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteData(w, r, http.StatusOK, items, nil)
}

func (h *Handlers) deleteSession(w http.ResponseWriter, r *http.Request) {
	v := ViewerFrom(r.Context())
	id, err := strconv.ParseUint(strings.TrimSpace(r.PathValue("id")), 10, 64)
	if err != nil {
		httpx.WriteError(w, r, httpx.ErrNotFound)
		return
	}
	ok, err := h.Bridge.OwnsSession(r.Context(), v.UserID, id)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if !ok {
		httpx.WriteError(w, r, httpx.ErrNotFound)
		return
	}
	h.Bridge.Revoke(r.Context(), id, v.UserID, h.ClientIP(r), r.UserAgent())
	if id == v.SessionID {
		h.Bridge.ClearCookie(w)
	}
	w.WriteHeader(http.StatusNoContent)
}
