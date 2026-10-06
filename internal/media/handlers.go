package media

import (
	"context"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/Nanako1900/linksPage/internal/store/dbq"
)

// Cache-Control values.
const (
	CacheUploads = "public, max-age=31536000, immutable"
	CacheQR      = "public, max-age=300"
	CacheGone    = "public, max-age=300"
	CacheNoStore = "no-store"
	// MediaCSP neutralises any active content a stored file might carry.
	MediaCSP = "default-src 'none'; sandbox"
)

// MetaStore is the subset of dbq.Queries used by the handlers.
type MetaStore interface {
	GetMedia(ctx context.Context, key string) (dbq.Medium, error)
	GetQRCode(ctx context.Context, id pgtype.UUID) (dbq.GetQRCodeRow, error)
}

// UploadsHandler serves GET /media/u/{key} (key from r.PathValue("key")):
// invalid or unknown keys → 404 (no-store); otherwise the stored bytes
// with the Content-Type implied by the key (the DB guarantees it matches
// the row), CacheUploads, nosniff and Content-Security-Policy:
// default-src 'none'; sandbox. QR codes are only served by /media/q so
// that replacing one takes effect (404 here).
func UploadsHandler(store Store, meta MetaStore, logger *slog.Logger) http.Handler {
	mustDeps(store, meta)
	logger = orDiscard(logger)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.PathValue("key")
		ct := ContentTypeForKey(key)
		if ct == "" {
			notFound(w)
			return
		}
		row, err := meta.GetMedia(r.Context(), key)
		if isMissing(err) || (err == nil && row.Kind == string(KindQR)) {
			notFound(w)
			return
		}
		if err != nil {
			internalError(w, r, logger, "media: lookup failed", err, slog.String("key", key))
			return
		}
		serveStored(w, r, store, logger, key, ct, CacheUploads)
	})
}

// QRHandler serves GET /media/q/{id}: a malformed id → 404; a well-formed
// UUID without a row (replaced or deleted) → 410 Gone with CacheGone;
// otherwise the QR image with CacheQR.
func QRHandler(store Store, meta MetaStore, logger *slog.Logger) http.Handler {
	mustDeps(store, meta)
	logger = orDiscard(logger)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseUUID(r.PathValue("id"))
		if !ok {
			notFound(w)
			return
		}
		row, err := meta.GetQRCode(r.Context(), id)
		if isMissing(err) {
			gone(w)
			return
		}
		if err != nil {
			internalError(w, r, logger, "media: qr lookup failed", err)
			return
		}
		ct := ContentTypeForKey(row.MediaKey)
		if ct == "" {
			internalError(w, r, logger, "media: qr row has an invalid media key", errors.New(row.MediaKey))
			return
		}
		serveStored(w, r, store, logger, row.MediaKey, ct, CacheQR)
	})
}

func serveStored(w http.ResponseWriter, r *http.Request, store Store, logger *slog.Logger, key, ct, cache string) {
	f, err := store.Open(r.Context(), key)
	if errors.Is(err, ErrNotFound) {
		logger.WarnContext(r.Context(), "media: row without file", slog.String("key", key))
		notFound(w)
		return
	}
	if err != nil {
		internalError(w, r, logger, "media: open failed", err, slog.String("key", key))
		return
	}
	defer func() { _ = f.Close() }()
	h := w.Header()
	h.Set("Content-Type", ct)
	h.Set("Cache-Control", cache)
	h.Set("ETag", `"`+strings.TrimSuffix(key, path.Ext(key))+`"`)
	setSafetyHeaders(h)
	http.ServeContent(w, r, "", time.Time{}, f)
}

func setSafetyHeaders(h http.Header) {
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Security-Policy", MediaCSP)
}

func notFound(w http.ResponseWriter) {
	writeText(w, http.StatusNotFound, CacheNoStore, "not found")
}

func gone(w http.ResponseWriter) {
	writeText(w, http.StatusGone, CacheGone, "gone")
}

func internalError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, msg string, err error, attrs ...any) {
	logger.ErrorContext(r.Context(), msg, append(attrs, slog.Any("err", err))...)
	writeText(w, http.StatusInternalServerError, CacheNoStore, "internal error")
}

func writeText(w http.ResponseWriter, status int, cache, body string) {
	h := w.Header()
	h.Set("Content-Type", "text/plain; charset=utf-8")
	h.Set("Cache-Control", cache)
	setSafetyHeaders(h)
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body+"\n")
}

func isMissing(err error) bool {
	return errors.Is(err, pgx.ErrNoRows) || errors.Is(err, ErrNotFound)
}

// parseUUID accepts only the canonical 8-4-4-4-12 hex form.
func parseUUID(s string) (pgtype.UUID, bool) {
	if len(s) != 36 || s[8] != '-' || s[13] != '-' || s[18] != '-' || s[23] != '-' {
		return pgtype.UUID{}, false
	}
	var u pgtype.UUID
	n, err := hex.Decode(u.Bytes[:], []byte(strings.ReplaceAll(s, "-", "")))
	if err != nil || n != len(u.Bytes) {
		return pgtype.UUID{}, false
	}
	u.Valid = true
	return u, true
}

func mustDeps(store Store, meta MetaStore) {
	if store == nil || meta == nil {
		panic("media: handler requires a Store and a MetaStore")
	}
}

func orDiscard(logger *slog.Logger) *slog.Logger {
	if logger == nil {
		return slog.New(slog.DiscardHandler)
	}
	return logger
}

var _ MetaStore = (*dbq.Queries)(nil)
