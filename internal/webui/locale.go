package webui

import (
	"cmp"
	"slices"
	"strconv"
	"strings"
)

// Locale negotiation for server-rendered HTML (M0 follow-up): ?lang= →
// Accept-Language (q-values, exact then base-language match) → default.

// maxAcceptLanguageEntries bounds the work done on hostile headers.
const maxAcceptLanguageEntries = 16

// maxAcceptLanguageBytes bounds the header length considered.
const maxAcceptLanguageBytes = 512

type langPref struct {
	tag string
	q   float64
}

// negotiateLocale picks the response locale among enabled locales.
func negotiateLocale(locales []string, defaultLocale, langParam, acceptLanguage string) string {
	if l, ok := matchLocale(locales, langParam, false); ok {
		return l
	}
	for _, p := range parseAcceptLanguage(acceptLanguage) {
		if l, ok := matchLocale(locales, p.tag, true); ok {
			return l
		}
	}
	return defaultLocale
}

// matchLocale finds tag among locales case-insensitively. With base set,
// a tag also matches the first locale sharing its primary subtag
// ("zh" → "zh-CN", "en-US" → "en").
func matchLocale(locales []string, tag string, base bool) (string, bool) {
	if tag == "" {
		return "", false
	}
	for _, l := range locales {
		if strings.EqualFold(l, tag) {
			return l, true
		}
	}
	if !base {
		return "", false
	}
	primary := primarySubtag(tag)
	for _, l := range locales {
		if primarySubtag(l) == primary {
			return l, true
		}
	}
	return "", false
}

func primarySubtag(tag string) string {
	p, _, _ := strings.Cut(strings.ToLower(tag), "-")
	return p
}

// parseAcceptLanguage returns acceptable language ranges ordered by
// descending q (ties keep header order). Wildcards, q=0 and malformed
// entries are dropped.
func parseAcceptLanguage(header string) []langPref {
	if len(header) > maxAcceptLanguageBytes {
		header = header[:maxAcceptLanguageBytes]
	}
	parts := strings.Split(header, ",")
	if len(parts) > maxAcceptLanguageEntries {
		parts = parts[:maxAcceptLanguageEntries]
	}
	prefs := make([]langPref, 0, len(parts))
	for _, part := range parts {
		if p, ok := parseLangRange(part); ok {
			prefs = append(prefs, p)
		}
	}
	slices.SortStableFunc(prefs, func(a, b langPref) int { return cmp.Compare(b.q, a.q) })
	return prefs
}

func parseLangRange(part string) (langPref, bool) {
	tag, params, _ := strings.Cut(strings.TrimSpace(part), ";")
	tag = strings.TrimSpace(tag)
	if tag == "" || tag == "*" || !validLangTag(tag) {
		return langPref{}, false
	}
	q := 1.0
	if params != "" {
		name, val, ok := strings.Cut(strings.TrimSpace(params), "=")
		if !ok || strings.TrimSpace(name) != "q" {
			return langPref{}, false
		}
		f, err := strconv.ParseFloat(strings.TrimSpace(val), 64)
		if err != nil || f < 0 || f > 1 {
			return langPref{}, false
		}
		q = f
	}
	if q == 0 {
		return langPref{}, false
	}
	return langPref{tag: tag, q: q}, true
}

// validLangTag accepts letters, digits and hyphens (1–35 bytes).
func validLangTag(tag string) bool {
	if len(tag) > 35 {
		return false
	}
	for _, r := range tag {
		ok := r == '-' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
		if !ok {
			return false
		}
	}
	return true
}
