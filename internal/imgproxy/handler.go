package imgproxy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sync/semaphore"
	"golang.org/x/sync/singleflight"
)

// Upstream limits (doc 5.2 / 4.11).
const (
	MaxUpstreamBytes    = 2 << 20
	UpstreamTimeout     = 5 * time.Second
	UpstreamConcurrency = 4
	NegativeCacheTTL    = 10 * time.Minute
	// CacheBytes bounds the in-memory LRU of small images.
	CacheBytes = 16 << 20
)

// Cache-Control by media_proxy.kind.
const (
	CacheIconBanner = "public, max-age=86400, stale-while-revalidate=604800"
	CacheAvatar     = "public, max-age=3600"
	cacheNotFound   = "public, max-age=60"
	cacheNoStore    = "no-store"
)

// Fixed security headers on every response.
const (
	headerCSP = "default-src 'none'; sandbox"
	// unknownKeyTTL caches database misses briefly (keys are registered
	// before pages reference them).
	unknownKeyTTL = time.Minute
	// lookupTimeout bounds the database lookup plus the upstream fetch.
	lookupTimeout = UpstreamTimeout + 2*time.Second
)

// allowedTypes are the DetectContentType results that may be served.
var allowedTypes = map[string]bool{
	"image/png":  true,
	"image/jpeg": true,
	"image/webp": true,
	"image/gif":  true,
}

// Handler errors mapped to statuses.
var (
	errNotFound = errors.New("imgproxy: not found")
	errUpstream = errors.New("imgproxy: upstream failure")
	errBusy     = errors.New("imgproxy: busy")
)

// HandlerOptions configure the /media/p handler.
type HandlerOptions struct {
	Store Store
	// Client is the provider HTTP client (no redirects, proxy aware).
	Client *http.Client
	// Hosts is the registration allow-list (provider kind → host → path
	// prefixes, as passed to NewRegistrar). Every stored row is checked
	// against it again before dialing, so a row written by anything other
	// than the Registrar can never make the proxy fetch an arbitrary URL.
	Hosts  map[string]map[string][]string
	Logger *slog.Logger
}

// cachedImage is one cached upstream response (or a negative entry).
type cachedImage struct {
	contentType  string
	body         []byte
	etag         string
	cacheControl string
	ext          string
	notFound     bool
	expires      time.Time // zero: the LRU TTL applies
}

// Handler serves GET /media/p/{file}. Responses carry only Content-Type,
// Content-Length, Cache-Control and ETag, plus X-Content-Type-Options:
// nosniff and Content-Security-Policy: default-src 'none'; sandbox.
// Upstream: no redirects, LimitReader(MaxUpstreamBytes), UpstreamTimeout,
// status 200 only, DetectContentType ∈ png/jpeg/webp/gif. Concurrent
// requests for one key share a singleflight call; upstream 404 is
// negatively cached for NegativeCacheTTL. Per-IP rate limiting is applied
// by the router.
type Handler struct {
	opts   HandlerOptions
	cache  *imageCache
	group  singleflight.Group
	sem    *semaphore.Weighted
	logger *slog.Logger
	now    func() time.Time
}

// NewHandler returns the handler (file from r.PathValue("file")).
func NewHandler(opts HandlerOptions) (*Handler, error) {
	if opts.Store == nil || opts.Client == nil || opts.Hosts == nil {
		return nil, errors.New("imgproxy: store, client and hosts are required")
	}
	opts.Hosts = cloneHosts(opts.Hosts)
	logger := opts.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &Handler{
		opts:   opts,
		cache:  newImageCache(CacheBytes, time.Now),
		sem:    semaphore.NewWeighted(UpstreamConcurrency),
		logger: logger,
		now:    time.Now,
	}, nil
}

// ServeHTTP implements http.Handler.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		writeError(w, http.StatusMethodNotAllowed, cacheNoStore)
		return
	}
	file := r.PathValue("file")
	if file == "" {
		file = strings.TrimPrefix(r.URL.Path, PathPrefix)
	}
	m := FileRe.FindStringSubmatch(file)
	if m == nil {
		writeError(w, http.StatusNotFound, cacheNotFound)
		return
	}
	img, err := h.load(r.Context(), m[1])
	switch {
	case err != nil:
		writeFailure(w, err)
	case img.ext != m[2]:
		writeError(w, http.StatusNotFound, cacheNotFound)
	default:
		writeImage(w, r, img)
	}
}

// writeFailure maps load errors: unknown → 404, busy → 503, else 502.
func writeFailure(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errNotFound):
		writeError(w, http.StatusNotFound, cacheNotFound)
	case errors.Is(err, errBusy):
		w.Header().Set("Retry-After", "1")
		writeError(w, http.StatusServiceUnavailable, cacheNoStore)
	default:
		writeError(w, http.StatusBadGateway, cacheNoStore)
	}
}

// load returns the cached image or fetches it once for all concurrent
// callers. Negative entries come back as errNotFound.
func (h *Handler) load(ctx context.Context, key string) (cachedImage, error) {
	if img, ok := h.cache.get(key); ok {
		return entryResult(img)
	}
	ch := h.group.DoChan(key, func() (any, error) {
		// Detached from the first caller so its cancellation does not fail
		// the other waiters; lookupTimeout bounds the work.
		fetchCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), lookupTimeout)
		defer cancel()
		return h.fetch(fetchCtx, key)
	})
	select {
	case <-ctx.Done():
		return cachedImage{}, ctx.Err()
	case res := <-ch:
		if res.Err != nil {
			return cachedImage{}, res.Err
		}
		img, ok := res.Val.(cachedImage)
		if !ok {
			return cachedImage{}, errUpstream
		}
		return entryResult(img)
	}
}

func entryResult(img cachedImage) (cachedImage, error) {
	if img.notFound {
		return cachedImage{}, errNotFound
	}
	return img, nil
}

func writeImage(w http.ResponseWriter, r *http.Request, img cachedImage) {
	hdr := w.Header()
	setSecurityHeaders(hdr)
	hdr.Set("Cache-Control", img.cacheControl)
	hdr.Set("ETag", img.etag)
	if etagMatches(r.Header.Get("If-None-Match"), img.etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	hdr.Set("Content-Type", img.contentType)
	hdr.Set("Content-Length", strconv.Itoa(len(img.body)))
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write(img.body)
}

func writeError(w http.ResponseWriter, status int, cacheControl string) {
	hdr := w.Header()
	setSecurityHeaders(hdr)
	hdr.Set("Cache-Control", cacheControl)
	hdr.Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(http.StatusText(status)))
}

func setSecurityHeaders(hdr http.Header) {
	hdr.Set("X-Content-Type-Options", "nosniff")
	hdr.Set("Content-Security-Policy", headerCSP)
}

// etagMatches implements the If-None-Match list check (weak comparison).
func etagMatches(header, etag string) bool {
	if header == "" {
		return false
	}
	for _, candidate := range strings.Split(header, ",") {
		c := strings.TrimPrefix(strings.TrimSpace(candidate), "W/")
		if c == "*" || c == etag {
			return true
		}
	}
	return false
}

// etagFor returns a strong ETag over the body.
func etagFor(body []byte) string {
	sum := sha256.Sum256(body)
	return `"` + hex.EncodeToString(sum[:])[:32] + `"`
}
