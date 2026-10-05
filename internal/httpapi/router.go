// Package httpapi wires the HTTP server: chi routing in the documented
// order, huma operations, problem+json errors, middleware and lifecycle.
package httpapi

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/Nanako1900/linksPage/internal/netx"
	"github.com/Nanako1900/linksPage/internal/webui"
)

// compressLevel is the gzip/deflate level for dynamic responses.
const compressLevel = 5

// Deps are the dependencies of the HTTP handler.
type Deps struct {
	BaseURL   string
	Version   string
	Logger    *slog.Logger
	Ready     *Readiness
	Snapshots SnapshotSource
	Web       *webui.Renderer
	Resolver  *netx.Resolver
	// HSTS sends Strict-Transport-Security (only when BaseURL is https).
	HSTS bool
}

func (d Deps) validate() error {
	if d.Logger == nil || d.Ready == nil || d.Snapshots == nil || d.Web == nil || d.Resolver == nil {
		return errors.New("httpapi: Logger, Ready, Snapshots, Web and Resolver are required")
	}
	return nil
}

// NewHandler builds the root handler. Routes are mounted in the order of
// doc 4.2: /api/*, static assets, health endpoints, /admin, public HTML,
// then the 404 page. CrossOriginProtection (trusting base_url) runs after
// the common middleware so rejections carry the request ID and security
// headers and are access-logged.
func NewHandler(d Deps) (http.Handler, error) {
	if err := d.validate(); err != nil {
		return nil, err
	}
	cop, err := crossOriginProtection(d.BaseURL)
	if err != nil {
		return nil, err
	}
	r := chi.NewRouter()
	r.Use(
		requestContext(d.Logger),
		recoverer,
		netx.Middleware(d.Resolver, d.Logger),
		accessLog(d.Logger),
		securityHeaders(d.HSTS && strings.HasPrefix(d.BaseURL, "https://")),
		cop.Handler,
		identityRanges,
		middleware.Compress(compressLevel, "text/html", "text/css", "text/javascript", "application/javascript",
			"application/json", ProblemContentType, "application/openapi+json", "image/svg+xml"),
		middleware.GetHead,
	)

	// 1. API (huma). Unknown /api/* paths are problem+json, never HTML.
	api := newAPI(r, d.Version, publicAPI{snapshots: d.Snapshots, ready: d.Ready})
	r.Get(OpenAPIPath, openAPIHandler(api))

	// 2. Static assets from the embedded build.
	r.Handle("/assets/*", d.Web.AssetsHandler())

	// 3. Health endpoints.
	r.Get("/healthz", healthz)
	r.Get("/readyz", readyz(d.Ready))

	// 4. Admin shell.
	r.Get("/admin", d.Web.Admin().ServeHTTP)
	r.Get("/admin/*", d.Web.Admin().ServeHTTP)

	// 5. Public HTML (only these paths).
	r.Get("/", d.Web.Public(webui.PageHome).ServeHTTP)
	r.Get("/privacy", d.Web.Public(webui.PagePrivacy).ServeHTTP)
	r.Get("/c/{slug}", d.Web.Public(webui.PageCommunity).ServeHTTP)

	notFound := d.Web.NotFound()
	r.NotFound(func(w http.ResponseWriter, req *http.Request) {
		if isAPIPath(req.URL.Path) {
			writeProblem(w, req, http.StatusNotFound, CodeNotFound, "no such API endpoint")
			return
		}
		notFound.ServeHTTP(w, req)
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, req *http.Request) {
		if isAPIPath(req.URL.Path) {
			writeProblem(w, req, http.StatusMethodNotAllowed, CodeMethodNotAllowed, "method not allowed")
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
	})

	return r, nil
}

// crossOriginProtection rejects cross-origin unsafe requests, trusting
// base_url's origin.
func crossOriginProtection(baseURL string) (*http.CrossOriginProtection, error) {
	cop := http.NewCrossOriginProtection()
	if baseURL != "" {
		if err := cop.AddTrustedOrigin(baseURL); err != nil {
			return nil, fmt.Errorf("trust base_url origin: %w", err)
		}
	}
	cop.SetDenyHandler(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		loggerFrom(req.Context()).Warn("cross-origin request rejected",
			slog.String("method", req.Method), slog.String("path", req.URL.Path),
			slog.String("request_id", RequestIDFrom(req.Context())))
		if isAPIPath(req.URL.Path) {
			writeProblem(w, req, http.StatusForbidden, CodeCrossOrigin, "cross-origin request rejected")
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		http.Error(w, "cross-origin request rejected", http.StatusForbidden)
	}))
	return cop, nil
}
