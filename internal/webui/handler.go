package webui

import (
	"io"
	"io/fs"
	"log/slog"
	"mime"
	"net/http"
	"path"
	"regexp"
	"strings"
)

// Cache-Control values.
const (
	CacheHTML      = "no-cache, no-transform"
	CacheImmutable = "public, max-age=31536000, immutable"
	CacheNoStore   = "no-store"
)

var slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

// Public returns the handler for a public page kind. For PageCommunity
// the slug is read from r.PathValue("slug").
func (rd *Renderer) Public(kind PageKind) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if kind == PageCommunity && !slugRe.MatchString(r.PathValue("slug")) {
			rd.NotFound().ServeHTTP(w, r)
			return
		}
		p, err := rd.renderPublic(kind, r.URL.Path, r.URL.Query().Get("lang"))
		rd.write(w, r, p, err, http.StatusOK, true)
	})
}

// NotFound returns the 404 HTML handler.
func (rd *Renderer) NotFound() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, err := rd.renderPublic(PageNotFound, "/", r.URL.Query().Get("lang"))
		rd.write(w, r, p, err, http.StatusNotFound, true)
	})
}

// Admin returns the admin shell handler (/admin and /admin/*).
func (rd *Renderer) Admin() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, err := rd.renderAdmin()
		w.Header().Set("X-Robots-Tag", "noindex, nofollow")
		rd.write(w, r, p, err, http.StatusOK, false)
	})
}

func (rd *Renderer) write(w http.ResponseWriter, r *http.Request, p page, err error, status int, public bool) {
	if err != nil {
		rd.opts.Logger.Error("render html", slog.Any("error", err))
		w.Header().Set("Cache-Control", CacheNoStore)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	h := w.Header()
	// Set on every HTML response (200, 304, 404) so the 304 carries the
	// same Vary as the 200 regardless of whether it was compressed.
	h.Add("Vary", "Accept-Encoding")
	h.Set("Content-Security-Policy", p.csp)
	if public {
		h.Set("Content-Security-Policy-Report-Only", TrustedTypesReportOnly)
	}
	h.Set("Cache-Control", CacheHTML)
	if status == http.StatusOK {
		h.Set("ETag", p.etag)
		if etagMatches(r.Header.Get("If-None-Match"), p.etag) {
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}
	h.Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if r.Method == http.MethodHead {
		return
	}
	if _, err := w.Write(p.body); err != nil {
		rd.opts.Logger.Debug("write html", slog.Any("error", err))
	}
}

// etagMatches implements weak comparison for If-None-Match.
func etagMatches(header, etag string) bool {
	if header == "" {
		return false
	}
	want := strings.TrimPrefix(etag, "W/")
	for _, cand := range strings.Split(header, ",") {
		cand = strings.TrimSpace(cand)
		if cand == "*" || strings.TrimPrefix(cand, "W/") == want {
			return true
		}
	}
	return false
}

// extraTypes covers extensions missing from minimal (distroless) systems.
var extraTypes = map[string]string{
	".js":    "text/javascript; charset=utf-8",
	".mjs":   "text/javascript; charset=utf-8",
	".css":   "text/css; charset=utf-8",
	".map":   "application/json",
	".json":  "application/json",
	".svg":   "image/svg+xml",
	".woff2": "font/woff2",
	".woff":  "font/woff",
	".png":   "image/png",
	".webp":  "image/webp",
	".avif":  "image/avif",
	".ico":   "image/x-icon",
	".wasm":  "application/wasm",
	".txt":   "text/plain; charset=utf-8",
}

func contentType(name string) string {
	ext := strings.ToLower(path.Ext(name))
	if t, ok := extraTypes[ext]; ok {
		return t
	}
	if t := mime.TypeByExtension(ext); t != "" {
		return t
	}
	return "application/octet-stream"
}

// AssetsHandler serves /assets/* from the embedded build with immutable
// caching; misses are 404 with no-store and never fall back to HTML.
func (rd *Renderer) AssetsHandler() http.Handler {
	fsys := rd.opts.Assets.fsys
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/")
		if !strings.HasPrefix(name, assetsPrefix) || !fs.ValidPath(name) {
			assetNotFound(w)
			return
		}
		f, err := fsys.Open(name)
		if err != nil {
			assetNotFound(w)
			return
		}
		defer func() { _ = f.Close() }()
		fi, err := f.Stat()
		if err != nil || fi.IsDir() {
			assetNotFound(w)
			return
		}
		rs, ok := f.(io.ReadSeeker)
		if !ok {
			rd.opts.Logger.Error("embedded asset is not seekable", slog.String("path", name))
			assetNotFound(w)
			return
		}
		h := w.Header()
		h.Set("Content-Type", contentType(name))
		h.Set("Cache-Control", CacheImmutable)
		h.Add("Vary", "Accept-Encoding")
		http.ServeContent(w, r, name, fi.ModTime(), rs)
	})
}

func assetNotFound(w http.ResponseWriter) {
	h := w.Header()
	h.Set("Cache-Control", CacheNoStore)
	h.Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	_, _ = io.WriteString(w, "404 not found\n")
}
