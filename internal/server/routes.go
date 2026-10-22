package server

import (
	"net/http"
	"strings"

	"github.com/namankumar80510/panw.ink/internal/auth"
)

// Routes registers custom routes with Collar and remembers their patterns,
// so the ingress can answer unknown /api/v1/pawn/x paths itself instead of
// letting them fall through to Collar's generic /{app}/{db}/{table} route.
type Routes struct {
	inner auth.Router
	mux   *http.ServeMux
}

// NewRoutes wraps a Collar app (or any auth.Router).
func NewRoutes(inner auth.Router) *Routes {
	return &Routes{inner: inner, mux: http.NewServeMux()}
}

// HandleFunc implements auth.Router.
func (rt *Routes) HandleFunc(pattern string, h http.HandlerFunc) error {
	if err := rt.inner.HandleFunc(pattern, h); err != nil {
		return err
	}
	rt.mux.HandleFunc(pattern, func(http.ResponseWriter, *http.Request) {})
	return nil
}

// match reports whether r matches a custom route; when only the method is
// wrong it returns the allowed methods.
func (rt *Routes) match(r *http.Request) (ok bool, allow []string) {
	if _, p := rt.mux.Handler(r); p != "" {
		return true, nil
	}
	for _, m := range []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		if m == r.Method {
			continue
		}
		c := r.Clone(r.Context())
		c.Method = m
		if _, p := rt.mux.Handler(c); p != "" {
			allow = append(allow, m)
		}
	}
	return false, allow
}

func isCustomPath(p string) bool {
	return p == auth.Base || strings.HasPrefix(p, auth.Base+"/")
}
