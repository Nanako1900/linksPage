package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"runtime/debug"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/Nanako1900/linksPage/internal/netx"
)

type ctxKey int

const (
	ctxRequestID ctxKey = iota
	ctxLogger
)

// RequestIDHeader is the response header carrying the request ID.
const RequestIDHeader = "X-Request-Id"

// RequestIDFrom returns the request ID stored in ctx ("" if none).
func RequestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(ctxRequestID).(string)
	return id
}

func loggerFrom(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(ctxLogger).(*slog.Logger); ok {
		return l
	}
	return slog.Default()
}

func newRequestID() string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("t%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

// requestContext always generates a fresh request ID (client-supplied IDs
// are ignored to keep logs trustworthy) and attaches the logger.
func requestContext(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := newRequestID()
			ctx := context.WithValue(r.Context(), ctxRequestID, id)
			ctx = context.WithValue(ctx, ctxLogger, logger)
			w.Header().Set(RequestIDHeader, id)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// recoverer turns panics into 500 responses (problem+json under /api).
func recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			rec := recover()
			if rec == nil {
				return
			}
			if rec == http.ErrAbortHandler { //nolint:errorlint // sentinel panic value, compared by identity
				panic(rec)
			}
			loggerFrom(r.Context()).Error("panic recovered", slog.Any("panic", rec),
				slog.String("stack", string(debug.Stack())), slog.String("request_id", RequestIDFrom(r.Context())))
			if isAPIPath(r.URL.Path) {
				writeProblem(w, r, http.StatusInternalServerError, CodeInternal, "")
				return
			}
			w.Header().Set("Cache-Control", "no-store")
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		}()
		next.ServeHTTP(w, r)
	})
}

func isAPIPath(p string) bool { return p == "/api" || strings.HasPrefix(p, "/api/") }

// isPrivilegedPath reports admin/auth routes, which keep full access logs.
// Every other route is treated as public and logged without IP, UA,
// Referer or query (doc 4.12).
func isPrivilegedPath(p string) bool {
	for _, prefix := range []string{"/admin", "/api/auth", "/api/v1/admin"} {
		if p == prefix || strings.HasPrefix(p, prefix+"/") {
			return true
		}
	}
	return false
}

// accessLog writes one structured line per request.
func accessLog(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			status := ww.Status()
			if status == 0 {
				status = http.StatusOK
			}
			route := ""
			if rc := chi.RouteContext(r.Context()); rc != nil {
				route = rc.RoutePattern()
			}
			attrs := []slog.Attr{
				slog.String("method", r.Method),
				slog.String("route", route),
				slog.Int("status", status),
				slog.Duration("duration", time.Since(start)),
				slog.String("request_id", RequestIDFrom(r.Context())),
			}
			if isPrivilegedPath(r.URL.Path) {
				ip := ""
				if info, ok := netx.FromContext(r.Context()); ok && info.IP.IsValid() {
					ip = info.IP.String()
				}
				attrs = append(attrs,
					slog.String("path", r.URL.Path), slog.String("query", redactQuery(r.URL.RawQuery)),
					slog.String("ip", ip), slog.String("user_agent", r.UserAgent()), slog.String("referer", r.Referer()))
			}
			level := slog.LevelInfo
			switch {
			case status >= http.StatusInternalServerError:
				level = slog.LevelError
			case r.URL.Path == "/healthz" || r.URL.Path == "/readyz":
				level = slog.LevelDebug
			}
			logger.LogAttrs(r.Context(), level, "http request", attrs...)
		})
	}
}

// HSTSValue is sent when the application itself is configured to send
// HSTS (deployments that do not sit behind Cloudflare; doc 12.4).
const HSTSValue = "max-age=15552000"

// securityHeaders sets headers common to every response. API responses
// additionally get a deny-all CSP and default to no-store.
func securityHeaders(hsts bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
			h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
			h.Set("Cross-Origin-Opener-Policy", "same-origin")
			if hsts {
				h.Set("Strict-Transport-Security", HSTSValue)
			}
			if isAPIPath(r.URL.Path) {
				h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
				h.Set("Cache-Control", "no-store")
			}
			next.ServeHTTP(w, r)
		})
	}
}

// identityRanges disables response compression for Range requests: a
// gzip stream of a byte range would not match its Content-Range.
func identityRanges(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Range") != "" && r.Header.Get("Accept-Encoding") != "" {
			r = r.Clone(r.Context())
			r.Header.Del("Accept-Encoding")
		}
		next.ServeHTTP(w, r)
	})
}

// sensitiveQueryKeys are redacted from privileged access logs (OAuth
// callback parameters, tokens, passwords).
var sensitiveQueryKeys = map[string]bool{
	"code": true, "state": true, "token": true, "id_token": true, "access_token": true,
	"refresh_token": true, "password": true, "secret": true, "reauth": true,
}

// redactQuery returns raw with the values of sensitive keys replaced.
func redactQuery(raw string) string {
	if raw == "" {
		return ""
	}
	q, err := url.ParseQuery(raw)
	if err != nil {
		return redactedValue
	}
	for k := range q {
		if sensitiveQueryKeys[strings.ToLower(k)] {
			q[k] = []string{redactedValue}
		}
	}
	return q.Encode()
}

const redactedValue = "[REDACTED]"
