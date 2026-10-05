package imgproxy

import (
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strings"
)

// MaxURLLength matches the media_proxy.url CHECK.
const MaxURLLength = 2048

// Extension-less upstream images registered as PNG (Discord widget
// avatars, doc 5.2 / spikes/providers.md).
const (
	discordCDNHost       = "cdn.discordapp.com"
	discordAvatarsPrefix = "/widget-avatars/"
)

// sizeRe bounds the only accepted query parameter (?size=N).
var sizeRe = regexp.MustCompile(`^[1-9][0-9]{0,3}$`)

// Normalized is a canonical upstream URL plus its proxy extension.
type Normalized struct {
	URL string
	Ext string
}

// Normalize rebuilds rawURL canonically and checks it against hosts (host
// → allowed path prefixes). Only https, no credentials, port or fragment,
// no "%", "\\" or "..", a clean path and at most a numeric "size" query
// parameter are accepted.
func Normalize(rawURL string, hosts map[string][]string) (Normalized, error) {
	if len(rawURL) > MaxURLLength || strings.ContainsAny(rawURL, "%\\#") || strings.Contains(rawURL, "..") {
		return Normalized{}, fmt.Errorf("%w: forbidden characters or length", ErrInvalidURL)
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return Normalized{}, fmt.Errorf("%w: parse", ErrInvalidURL)
	}
	if u.Scheme != "https" || u.User != nil || u.Opaque != "" || u.Port() != "" || u.Hostname() == "" {
		return Normalized{}, fmt.Errorf("%w: scheme, credentials or port", ErrInvalidURL)
	}
	host := strings.ToLower(u.Hostname())
	cleanPath := path.Clean("/" + u.Path)
	if !allowed(hosts[host], cleanPath) {
		return Normalized{}, fmt.Errorf("%w: %s%s", ErrHostNotAllowed, host, cleanPath)
	}
	query, err := normalizeQuery(u.RawQuery)
	if err != nil {
		return Normalized{}, err
	}
	ext, err := extension(host, cleanPath)
	if err != nil {
		return Normalized{}, err
	}
	out := url.URL{Scheme: "https", Host: host, Path: cleanPath, RawQuery: query}
	return Normalized{URL: out.String(), Ext: ext}, nil
}

func allowed(prefixes []string, p string) bool {
	for _, prefix := range prefixes {
		if prefix != "" && strings.HasPrefix(p, prefix) && len(p) > len(prefix) {
			return true
		}
	}
	return false
}

func normalizeQuery(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	q, err := url.ParseQuery(raw)
	if err != nil {
		return "", fmt.Errorf("%w: query", ErrInvalidURL)
	}
	for k, v := range q {
		if k != "size" || len(v) != 1 || !sizeRe.MatchString(v[0]) {
			return "", fmt.Errorf("%w: query parameter %q", ErrInvalidURL, k)
		}
	}
	return q.Encode(), nil
}

func extension(host, p string) (string, error) {
	switch strings.ToLower(path.Ext(p)) {
	case ".png":
		return "png", nil
	case ".jpg", ".jpeg":
		return "jpg", nil
	case ".webp":
		return "webp", nil
	case ".gif":
		return "gif", nil
	case "":
		if host == discordCDNHost && strings.HasPrefix(p, discordAvatarsPrefix) {
			return "png", nil
		}
	}
	return "", fmt.Errorf("%w: unsupported extension", ErrInvalidURL)
}
