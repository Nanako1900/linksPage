// Package imgproxy implements the opaque-key external image proxy
// /media/p/{key}.{ext} (doc 5.2). Only URLs registered by providers are
// served; unknown keys are 404 without contacting the upstream.
package imgproxy

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"time"

	"github.com/hashicorp/golang-lru/v2/expirable"

	"github.com/Nanako1900/linksPage/internal/provider"
	"github.com/Nanako1900/linksPage/internal/store/dbq"
)

// ErrHostNotAllowed is returned when a URL's host/path prefix is not in
// the provider's ImageHosts.
var ErrHostNotAllowed = errors.New("imgproxy: host or path not allowed")

// ErrInvalidURL is returned for URLs that fail normalization.
var ErrInvalidURL = errors.New("imgproxy: invalid image url")

// ErrInvalidKind is returned for unknown provider or image kinds.
var ErrInvalidKind = errors.New("imgproxy: invalid provider or image kind")

// PathPrefix is the public route prefix.
const PathPrefix = "/media/p/"

// KeyLen is the length of a proxy key: base64url(HMAC-SHA256(K_media,
// normalized URL)) truncated to 22 characters.
const KeyLen = 22

// HKDFInfo is the HKDF info used to derive K_media from secret_key
// (config.DeriveKey).
const HKDFInfo = "media-proxy"

// MinKeyBytes is the minimum K_media length.
const MinKeyBytes = 32

// Registration cache: a key is upserted at most once per
// registrationTTL (which also refreshes media_proxy.last_seen_at).
const (
	registrationCacheSize = 4096
	registrationTTL       = time.Hour
)

// FileRe matches "{key}.{ext}" in /media/p/{file}.
var FileRe = regexp.MustCompile(`^([A-Za-z0-9_-]{22})\.(png|jpg|webp|gif)$`)

// Store is the subset of dbq.Queries used by the registrar and handler.
type Store interface {
	UpsertMediaProxy(ctx context.Context, arg dbq.UpsertMediaProxyParams) error
	GetMediaProxy(ctx context.Context, key string) (dbq.MediaProxy, error)
	TouchMediaProxy(ctx context.Context, keys []string) error
}

// Registrar registers upstream image URLs (implements
// provider.ImageRegistrar). URLs are rebuilt with url.URL, path.Clean'ed
// and rejected if they contain "%", "\\" or "..", then checked against
// hosts[provider][host] path prefixes. The extension comes from the URL
// path; extension-less Discord widget avatars are registered as "png".
// Registered keys are cached in memory to avoid repeated upserts.
type Registrar struct {
	store Store
	key   []byte
	hosts map[string]map[string][]string
	seen  *expirable.LRU[string, struct{}]
}

var _ provider.ImageRegistrar = (*Registrar)(nil)

// NewRegistrar returns a registrar. mediaKey is K_media (32 bytes from
// config.DeriveKey(HKDFInfo)); hosts is provider.Registry.ImageHosts().
func NewRegistrar(store Store, mediaKey []byte, hosts map[string]map[string][]string) (*Registrar, error) {
	if store == nil {
		return nil, errors.New("imgproxy: store is required")
	}
	if len(mediaKey) < MinKeyBytes {
		return nil, fmt.Errorf("imgproxy: media key must be at least %d bytes", MinKeyBytes)
	}
	return &Registrar{
		store: store,
		key:   slices.Clone(mediaKey),
		hosts: cloneHosts(hosts),
		seen:  expirable.NewLRU[string, struct{}](registrationCacheSize, nil, registrationTTL),
	}, nil
}

// cloneHosts deep-copies an allow-list (provider kind → host → prefixes).
func cloneHosts(hosts map[string]map[string][]string) map[string]map[string][]string {
	copied := make(map[string]map[string][]string, len(hosts))
	for kind, byHost := range hosts {
		inner := make(map[string][]string, len(byHost))
		for host, prefixes := range byHost {
			inner[host] = slices.Clone(prefixes)
		}
		copied[kind] = inner
	}
	return copied
}

// Register implements provider.ImageRegistrar and returns
// "/media/p/{key}.{ext}".
func (r *Registrar) Register(ctx context.Context, providerKind, imageKind, rawURL string) (string, error) {
	if !validImageKind(imageKind) {
		return "", fmt.Errorf("%w: image kind %q", ErrInvalidKind, imageKind)
	}
	hosts, ok := r.hosts[providerKind]
	if !ok {
		return "", fmt.Errorf("%w: provider %q", ErrInvalidKind, providerKind)
	}
	n, err := Normalize(rawURL, hosts)
	if err != nil {
		return "", err
	}
	key := Key(r.key, n.URL)
	publicPath := PathPrefix + key + "." + n.Ext
	if _, cached := r.seen.Get(key); cached {
		return publicPath, nil
	}
	err = r.store.UpsertMediaProxy(ctx, dbq.UpsertMediaProxyParams{
		Key: key, Provider: providerKind, Url: n.URL, Ext: n.Ext, Kind: imageKind,
	})
	if err != nil {
		return "", fmt.Errorf("imgproxy: register: %w", err)
	}
	r.seen.Add(key, struct{}{})
	return publicPath, nil
}

func validImageKind(kind string) bool {
	switch kind {
	case provider.ImageIcon, provider.ImageBanner, provider.ImageSplash, provider.ImageAvatar:
		return true
	}
	return false
}

// Key computes the opaque key for a normalized URL.
func Key(mediaKey []byte, normalizedURL string) string {
	mac := hmac.New(sha256.New, mediaKey)
	mac.Write([]byte(normalizedURL))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))[:KeyLen]
}

var _ Store = (*dbq.Queries)(nil)
