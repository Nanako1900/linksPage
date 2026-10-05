package imgproxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5"

	"github.com/Nanako1900/linksPage/internal/provider"
	"github.com/Nanako1900/linksPage/internal/store/dbq"
)

// fetch resolves key in the registry and downloads the upstream image.
// Unknown keys and upstream 404/410 are cached as negative entries.
func (h *Handler) fetch(ctx context.Context, key string) (cachedImage, error) {
	row, err := h.opts.Store.GetMediaProxy(ctx, key)
	if errors.Is(err, pgx.ErrNoRows) {
		neg := cachedImage{notFound: true, expires: h.now().Add(unknownKeyTTL)}
		h.cache.add(key, neg)
		return neg, nil
	}
	if err != nil {
		h.logger.ErrorContext(ctx, "media proxy lookup failed", slog.String("key", key), slog.Any("error", err))
		return cachedImage{}, fmt.Errorf("%w: lookup: %w", errUpstream, err)
	}
	if !h.registrable(row) {
		h.logger.ErrorContext(ctx, "media proxy row rejected by the allow-list",
			slog.String("key", key), slog.String("provider", row.Provider))
		neg := cachedImage{notFound: true, expires: h.now().Add(unknownKeyTTL)}
		h.cache.add(key, neg)
		return neg, nil
	}
	if err := h.sem.Acquire(ctx, 1); err != nil {
		return cachedImage{}, errBusy
	}
	defer h.sem.Release(1)
	upCtx, cancel := context.WithTimeout(ctx, UpstreamTimeout)
	defer cancel()
	body, status, err := h.download(upCtx, row.Url)
	switch {
	case err != nil:
		h.logger.WarnContext(ctx, "media proxy upstream failed", slog.String("key", key), slog.Any("error", err))
		return cachedImage{}, fmt.Errorf("%w: %w", errUpstream, err)
	case status == http.StatusNotFound || status == http.StatusGone:
		neg := cachedImage{notFound: true}
		h.cache.add(key, neg)
		return neg, nil
	case status != http.StatusOK:
		h.logger.WarnContext(ctx, "media proxy upstream status", slog.String("key", key), slog.Int("status", status))
		return cachedImage{}, fmt.Errorf("%w: status %d", errUpstream, status)
	}
	contentType := http.DetectContentType(body)
	if !allowedTypes[contentType] {
		h.logger.WarnContext(ctx, "media proxy upstream type rejected", slog.String("key", key), slog.String("type", contentType))
		return cachedImage{}, fmt.Errorf("%w: content type %s", errUpstream, contentType)
	}
	img := cachedImage{
		contentType:  contentType,
		body:         body,
		etag:         etagFor(body),
		cacheControl: cacheControlFor(row.Kind),
		ext:          row.Ext,
	}
	h.cache.add(key, img)
	return img, nil
}

// registrable re-applies the registration rules to a stored row: the URL
// must already be in canonical form for its provider's allow-list and keep
// the registered extension.
func (h *Handler) registrable(row dbq.MediaProxy) bool {
	hosts, ok := h.opts.Hosts[row.Provider]
	if !ok {
		return false
	}
	n, err := Normalize(row.Url, hosts)
	return err == nil && n.URL == row.Url && n.Ext == row.Ext
}

// download GETs rawURL without following redirects and reads at most
// MaxUpstreamBytes; the body is returned only for status 200.
func (h *Handler) download(ctx context.Context, rawURL string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "image/png,image/jpeg,image/webp,image/gif")
	resp, err := h.opts.Client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxUpstreamBytes+1))
	if err != nil {
		return nil, 0, fmt.Errorf("read body: %w", err)
	}
	if len(body) > MaxUpstreamBytes {
		return nil, 0, errors.New("body exceeds limit")
	}
	return body, resp.StatusCode, nil
}

func cacheControlFor(kind string) string {
	if kind == provider.ImageAvatar {
		return CacheAvatar
	}
	return CacheIconBanner
}
