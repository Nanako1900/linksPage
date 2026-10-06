package golink

import (
	"slices"
	"sort"
	"strconv"
	"strings"
)

// maxAcceptLanguageTags bounds Accept-Language parsing work.
const maxAcceptLanguageTags = 16

// negotiateLocale picks the page language: ?lang= when it is a site
// locale, then the best Accept-Language match (exact tag, then primary
// subtag), then the default locale.
func negotiateLocale(locales []string, defaultLocale, langParam, acceptLanguage string) string {
	if langParam != "" && slices.Contains(locales, langParam) {
		return langParam
	}
	for _, tag := range parseAcceptLanguage(acceptLanguage) {
		if l, ok := matchLocale(locales, tag); ok {
			return l
		}
	}
	return defaultLocale
}

// matchLocale matches tag against locales case-insensitively: exact
// first, then by primary subtag ("zh-TW" or "zh" → "zh-CN").
func matchLocale(locales []string, tag string) (string, bool) {
	for _, l := range locales {
		if strings.EqualFold(l, tag) {
			return l, true
		}
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

type weightedTag struct {
	tag string
	q   float64
}

// parseAcceptLanguage returns language tags ordered by descending
// quality (stable for equal q). "*", q=0 and malformed entries are
// dropped.
func parseAcceptLanguage(header string) []string {
	parts := strings.Split(header, ",")
	if len(parts) > maxAcceptLanguageTags {
		parts = parts[:maxAcceptLanguageTags]
	}
	tags := make([]weightedTag, 0, len(parts))
	for _, part := range parts {
		if wt, ok := parseLanguageRange(part); ok {
			tags = append(tags, wt)
		}
	}
	sort.SliceStable(tags, func(i, j int) bool { return tags[i].q > tags[j].q })
	out := make([]string, len(tags))
	for i, t := range tags {
		out[i] = t.tag
	}
	return out
}

func parseLanguageRange(part string) (weightedTag, bool) {
	tag, params, _ := strings.Cut(strings.TrimSpace(part), ";")
	tag = strings.TrimSpace(tag)
	if tag == "" || tag == "*" || !validTag(tag) {
		return weightedTag{}, false
	}
	q := 1.0
	if v, ok := strings.CutPrefix(strings.TrimSpace(params), "q="); ok {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil || f < 0 || f > 1 {
			return weightedTag{}, false
		}
		q = f
	}
	if q == 0 {
		return weightedTag{}, false
	}
	return weightedTag{tag: tag, q: q}, true
}

// validTag accepts ASCII letters, digits and hyphens (≤ 35 bytes).
func validTag(tag string) bool {
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
