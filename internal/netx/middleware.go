package netx

import (
	"context"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"
)

type ctxKey struct{}

// FromContext returns the ClientInfo stored by Middleware.
func FromContext(ctx context.Context) (ClientInfo, bool) {
	info, ok := ctx.Value(ctxKey{}).(ClientInfo)
	return info, ok
}

// WithClientInfo returns a context carrying info (useful in tests).
func WithClientInfo(ctx context.Context, info ClientInfo) context.Context {
	return context.WithValue(ctx, ctxKey{}, info)
}

// missingHeaderWarnInterval rate-limits the "missing client IP header"
// warning.
const missingHeaderWarnInterval = time.Minute

// Middleware resolves the client, stores it in the request context and
// strips fine-grained location headers. The original request is not
// modified; a shallow clone with a copied header map is passed on.
func Middleware(rv *Resolver, logger *slog.Logger) func(http.Handler) http.Handler {
	var lastWarn atomic.Int64
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			info := rv.Resolve(r)
			if info.MissingHeader {
				warnMissing(logger, &lastWarn, rv.header)
			}
			r2 := r.WithContext(WithClientInfo(r.Context(), info))
			r2.Header = stripLocationHeaders(r.Header)
			next.ServeHTTP(w, r2)
		})
	}
}

func warnMissing(logger *slog.Logger, last *atomic.Int64, header string) {
	now := time.Now().UnixNano()
	prev := last.Load()
	if now-prev < int64(missingHeaderWarnInterval) || !last.CompareAndSwap(prev, now) {
		return
	}
	logger.Warn("request from trusted proxy is missing a usable client IP header; check trusted_proxies and client_ip_header",
		slog.String("header", header))
}

func stripLocationHeaders(h http.Header) http.Header {
	out := h.Clone()
	if out == nil {
		out = http.Header{}
	}
	for _, name := range strippedHeaders {
		out.Del(name)
	}
	return out
}
