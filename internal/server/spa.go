package server

import (
	"bytes"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"regexp"
	"strings"
	"time"
)

// hashedAsset matches Parcel's content-hashed output names such as
// index.1a2b3c4d.js or bootstrap-icons.5f6e7d8c.woff2.
var hashedAsset = regexp.MustCompile(`\.[0-9a-f]{8,}\.[A-Za-z0-9]+$`)

// SPA serves the two embedded single-page apps (IMPLEMENTATION.md §4):
// existing files are served as assets; /studio and /admin document paths get
// dashboard.html; any other extension-less path gets index.html. A missing
// asset (a path whose last segment has an extension) is a plain 404, never
// an HTML shell.
type SPA struct {
	files fs.FS
}

// NewSPA returns an SPA over the Parcel output directory.
func NewSPA(files fs.FS) *SPA {
	return &SPA{files: files}
}

func (s *SPA) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "Method not allowed.", http.StatusMethodNotAllowed)
		return
	}
	p := path.Clean("/" + r.URL.Path)
	name := strings.TrimPrefix(p, "/")

	if name != "" && !strings.HasSuffix(name, ".html") {
		if f, err := fs.Stat(s.files, name); err == nil && !f.IsDir() {
			s.serveFile(w, r, name, cacheFor(name))
			return
		}
	}
	last := path.Base(p)
	if strings.Contains(last, ".") && !strings.HasSuffix(last, ".html") {
		w.Header().Set("Cache-Control", "no-store")
		http.Error(w, "Not found.", http.StatusNotFound)
		return
	}
	entry := "index.html"
	if isDashboardPath(p) {
		entry = "dashboard.html"
	}
	s.serveFile(w, r, entry, "no-cache")
}

func isDashboardPath(p string) bool {
	return p == "/studio" || p == "/admin" || strings.HasPrefix(p, "/studio/") || strings.HasPrefix(p, "/admin/")
}

func cacheFor(name string) string {
	if hashedAsset.MatchString(name) {
		return "public, max-age=31536000, immutable"
	}
	return "public, max-age=3600"
}

func (s *SPA) serveFile(w http.ResponseWriter, r *http.Request, name, cache string) {
	f, err := s.files.Open(name)
	if err != nil {
		w.Header().Set("Cache-Control", "no-store")
		if name == "index.html" || name == "dashboard.html" {
			http.Error(w, "The web UI is not built. Run `make build-web` (or `make build`).", http.StatusServiceUnavailable)
			return
		}
		http.Error(w, "Not found.", http.StatusNotFound)
		return
	}
	defer f.Close()
	if ct := mime.TypeByExtension(path.Ext(name)); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.Header().Set("Cache-Control", cache)
	if rs, ok := f.(io.ReadSeeker); ok {
		http.ServeContent(w, r, name, time.Time{}, rs)
		return
	}
	b, err := io.ReadAll(f)
	if err != nil {
		http.Error(w, "Internal error.", http.StatusInternalServerError)
		return
	}
	http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(b))
}
