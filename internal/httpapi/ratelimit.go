package httpapi

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/httprate"

	"github.com/Nanako1900/linksPage/internal/netx"
)

// Per-IP rate limits (doc 4.11). /go/* is deliberately never limited.
const (
	PublicAPILimit  = 120
	MediaProxyLimit = 300
	RateLimitWindow = time.Minute
	// EdgeRenderLimit is one shared bucket for authenticated Worker render
	// requests: they come from a few shared edge addresses (so per-IP
	// limits do not fit), but cache-busting URLs must not turn into
	// unbounded origin renders. Over the limit the Worker serves its
	// static shell and the SPA loads the page itself.
	EdgeRenderLimit = 1200
	edgeBucketKey   = "edge-render"

	publicAPIPrefix  = "/api/v1/public/"
	mediaProxyPrefix = "/media/p/"
)

// clientKey keys limits by the trusted client IP (IPv6 bucketed by /64).
func clientKey(r *http.Request) (string, error) {
	if info, ok := netx.FromContext(r.Context()); ok && info.IP.IsValid() {
		return httprate.CanonicalizeIP(info.IP.Unmap().String()), nil
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return httprate.CanonicalizeIP(host), nil
}

// noRateHeaders keeps per-client counters out of cacheable responses.
var noRateHeaders = httprate.ResponseHeaders{}

func newLimiter(limit int, onLimit http.HandlerFunc) *httprate.RateLimiter {
	return httprate.NewRateLimiter(limit, RateLimitWindow,
		httprate.WithKeyFuncs(clientKey),
		httprate.WithResponseHeaders(noRateHeaders),
		httprate.WithLimitHandler(onLimit))
}

func setRetryAfter(w http.ResponseWriter) {
	w.Header().Set("Retry-After", strconv.Itoa(int(RateLimitWindow.Seconds())))
}

func apiLimited(w http.ResponseWriter, r *http.Request) {
	setRetryAfter(w)
	writeProblem(w, r, http.StatusTooManyRequests, CodeRateLimited, "too many requests")
}

func mediaLimited(w http.ResponseWriter, _ *http.Request) {
	setRetryAfter(w)
	w.Header().Set("Cache-Control", "no-store")
	http.Error(w, http.StatusText(http.StatusTooManyRequests), http.StatusTooManyRequests)
}

// edgeKey puts every authenticated edge request into one bucket.
func edgeKey(*http.Request) (string, error) {
	return edgeBucketKey, nil
}

// rateLimits applies the public read API and image proxy limits by path
// prefix. Edge requests (the authenticated Worker) use one shared bucket of
// edgeLimit per window instead of the per-IP API limit.
func rateLimits(edge func(*http.Request) bool, edgeLimit int) func(http.Handler) http.Handler {
	apiLimiter := newLimiter(PublicAPILimit, apiLimited)
	mediaLimiter := newLimiter(MediaProxyLimit, mediaLimited)
	edgeLimiter := httprate.NewRateLimiter(edgeLimit, RateLimitWindow,
		httprate.WithKeyFuncs(edgeKey),
		httprate.WithResponseHeaders(noRateHeaders),
		httprate.WithLimitHandler(apiLimited))
	return func(next http.Handler) http.Handler {
		api := apiLimiter.Handler(next)
		media := mediaLimiter.Handler(next)
		edgeAPI := edgeLimiter.Handler(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case strings.HasPrefix(r.URL.Path, publicAPIPrefix) && edge(r):
				edgeAPI.ServeHTTP(w, r)
			case strings.HasPrefix(r.URL.Path, publicAPIPrefix):
				api.ServeHTTP(w, r)
			case strings.HasPrefix(r.URL.Path, mediaProxyPrefix):
				media.ServeHTTP(w, r)
			default:
				next.ServeHTTP(w, r)
			}
		})
	}
}

// edgeRequest reports authenticated Worker render requests. Without a
// configured secret no request counts as an edge request.
func edgeRequest(secret []byte) func(*http.Request) bool {
	return func(r *http.Request) bool {
		return len(secret) > 0 && r.URL.Path == RenderPath && proxyAuthOK(secret, r.Header.Get(ProxyAuthHeader))
	}
}
