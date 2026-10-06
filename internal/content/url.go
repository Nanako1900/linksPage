package content

import (
	"errors"
	"net/url"
	"slices"
	"strings"
	"unicode"
)

// AllowedSchemes is the URL scheme allow-list for links (doc 9).
var AllowedSchemes = []string{"https", "http", "mailto"}

// MaxURLLength bounds stored URLs (matches the DB CHECKs).
const MaxURLLength = 2048

// ErrUnsafeURL is returned for URLs outside the allow-list.
var ErrUnsafeURL = errors.New("content: URL scheme or form not allowed")

// SafeURL validates raw against AllowedSchemes (no credentials, no
// control characters, ≤ MaxURLLength) and returns its normalized form
// (lowercase scheme and host).
func SafeURL(raw string) (string, error) {
	if raw == "" || len(raw) > MaxURLLength || strings.ContainsRune(raw, '\\') || hasUnsafeRune(raw) {
		return "", ErrUnsafeURL
	}
	u, err := url.Parse(raw)
	if err != nil || u.User != nil {
		return "", ErrUnsafeURL
	}
	u.Scheme = strings.ToLower(u.Scheme)
	if !slices.Contains(AllowedSchemes, u.Scheme) {
		return "", ErrUnsafeURL
	}
	if u.Scheme == "mailto" {
		return safeMailto(u)
	}
	if u.Opaque != "" || u.Hostname() == "" {
		return "", ErrUnsafeURL
	}
	u.Host = strings.ToLower(u.Host)
	out := u.String()
	if len(out) > MaxURLLength {
		return "", ErrUnsafeURL
	}
	return out, nil
}

// safeMailto accepts "mailto:local@domain" (no host form, no credentials).
func safeMailto(u *url.URL) (string, error) {
	if u.Host != "" || u.Opaque == "" || !strings.Contains(u.Opaque, "@") {
		return "", ErrUnsafeURL
	}
	return u.String(), nil
}

// hasUnsafeRune reports spaces, control characters and invisible/bidi
// characters, none of which may appear in a stored URL.
func hasUnsafeRune(s string) bool {
	for _, r := range s {
		if unicode.IsSpace(r) || unicode.IsControl(r) || isInvisible(r) || r == unicode.ReplacementChar {
			return true
		}
	}
	return false
}

// HTTPSURL validates an https URL whose host is one of hosts (exact,
// case-insensitive; empty hosts = any host). Used for QQ join links
// (qm.qq.com, qun.qq.com) and provider invites.
func HTTPSURL(raw string, hosts ...string) (string, error) {
	out, err := SafeURL(raw)
	if err != nil {
		return "", err
	}
	u, err := url.Parse(out)
	if err != nil || u.Scheme != "https" {
		return "", ErrUnsafeURL
	}
	if len(hosts) == 0 {
		return out, nil
	}
	host := u.Hostname()
	for _, h := range hosts {
		if strings.EqualFold(h, host) {
			return out, nil
		}
	}
	return "", ErrUnsafeURL
}
