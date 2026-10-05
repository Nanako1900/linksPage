package site

import (
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"unicode/utf8"
)

// Search indexing modes (settings.searchIndexing).
const (
	SearchIndex   = "index"
	SearchNoIndex = "noindex"
)

// Copy keys that admins may override per locale (settings.copy). The
// frontend and the server-rendered guide pages read the same keys.
const (
	// CopyOpenInBrowser is the WeChat/QQ overlay text ("tap ··· → open in browser").
	CopyOpenInBrowser = "openInBrowser"
	// CopyOpenOnDesktop is the hint in the "open on desktop" dialog.
	CopyOpenOnDesktop = "openOnDesktop"
	// CopyInviteUnavailable is shown when a community has no usable join target.
	CopyInviteUnavailable = "inviteUnavailable"
	// CopyCommunityUnavailable is the default text of an unavailable card.
	CopyCommunityUnavailable = "communityUnavailable"
)

// CopyKeys lists every overridable copy key.
func CopyKeys() []string {
	return []string{CopyOpenInBrowser, CopyOpenOnDesktop, CopyInviteUnavailable, CopyCommunityUnavailable}
}

// Text length limits (in runes) for admin-provided settings text.
const (
	MaxPlainTextRunes    = 300
	MaxMarkdownTextBytes = 4096
	MaxCopyTextRunes     = 300
)

// MediaKeyRe matches a content-addressed media key (internal/media).
var MediaKeyRe = regexp.MustCompile(`^[0-9a-f]{32}\.(webp|png|jpg)$`)

// OGSettings override link-preview metadata. Empty values fall back to
// title/description and the generated OG image.
type OGSettings struct {
	Title       LocalizedText `json:"title"`
	Description LocalizedText `json:"description"`
	// ImageKey is an uploaded 1200x630 media key or "" (generated image).
	ImageKey string `json:"imageKey"`
}

func (o OGSettings) clone() OGSettings {
	return OGSettings{Title: maps.Clone(o.Title), Description: maps.Clone(o.Description), ImageKey: o.ImageKey}
}

// CopyOverrides maps locale → copy key → text.
type CopyOverrides map[string]map[string]string

// Clone returns a deep copy.
func (c CopyOverrides) Clone() CopyOverrides {
	out := make(CopyOverrides, len(c))
	for l, m := range c {
		out[l] = maps.Clone(m)
	}
	return out
}

// Get returns the override for key in locale, falling back to fallback
// and then "en"; "" means "use the built-in text".
func (c CopyOverrides) Get(locale, fallback, key string) string {
	for _, l := range []string{locale, fallback, "en"} {
		if v := c[l][key]; v != "" {
			return v
		}
	}
	return ""
}

func (s Settings) validateM1() []error {
	var errs []error
	errs = append(errs, validateTexts("displayName", s.DisplayName, MaxPlainTextRunes, false)...)
	errs = append(errs, validateTexts("bio", s.Bio, MaxMarkdownTextBytes, true)...)
	errs = append(errs, validateTexts("footer", s.Footer, MaxMarkdownTextBytes, true)...)
	errs = append(errs, validateTexts("notFound", s.NotFound, MaxPlainTextRunes, false)...)
	errs = append(errs, validateTexts("og.title", s.OG.Title, MaxPlainTextRunes, false)...)
	errs = append(errs, validateTexts("og.description", s.OG.Description, MaxPlainTextRunes, false)...)
	errs = append(errs, validateTexts("title", s.Title, MaxPlainTextRunes, false)...)
	errs = append(errs, validateTexts("description", s.Description, MaxPlainTextRunes, false)...)
	if s.AvatarKey != "" && !MediaKeyRe.MatchString(s.AvatarKey) {
		errs = append(errs, errors.New("avatarKey: must be a media key"))
	}
	if s.OG.ImageKey != "" && !MediaKeyRe.MatchString(s.OG.ImageKey) {
		errs = append(errs, errors.New("og.imageKey: must be a media key"))
	}
	if s.SearchIndexing != SearchIndex && s.SearchIndexing != SearchNoIndex {
		errs = append(errs, errors.New("searchIndexing: must be index or noindex"))
	}
	return append(errs, s.Copy.validate()...)
}

// validateTexts checks locale keys and lengths. Markdown limits are in
// bytes, plain text limits in runes.
func validateTexts(field string, t LocalizedText, limit int, markdown bool) []error {
	var errs []error
	for _, l := range slices.Sorted(maps.Keys(t)) {
		v := t[l]
		switch {
		case !localeRe.MatchString(l):
			errs = append(errs, fmt.Errorf("%s: invalid locale key", field))
		case !utf8.ValidString(v):
			errs = append(errs, fmt.Errorf("%s.%s: invalid UTF-8", field, l))
		case markdown && len(v) > limit:
			errs = append(errs, fmt.Errorf("%s.%s: longer than %d bytes", field, l, limit))
		case !markdown && utf8.RuneCountInString(v) > limit:
			errs = append(errs, fmt.Errorf("%s.%s: longer than %d characters", field, l, limit))
		}
	}
	return errs
}

func (c CopyOverrides) validate() []error {
	var errs []error
	known := CopyKeys()
	for _, l := range slices.Sorted(maps.Keys(c)) {
		if !localeRe.MatchString(l) {
			errs = append(errs, errors.New("copy: invalid locale key"))
			continue
		}
		for _, k := range slices.Sorted(maps.Keys(c[l])) {
			if !slices.Contains(known, k) {
				errs = append(errs, fmt.Errorf("copy.%s: unknown key %q", l, k))
				continue
			}
			if v := c[l][k]; !utf8.ValidString(v) || utf8.RuneCountInString(v) > MaxCopyTextRunes {
				errs = append(errs, fmt.Errorf("copy.%s.%s: invalid or longer than %d characters", l, k, MaxCopyTextRunes))
			}
		}
	}
	return errs
}

// ValidLocale reports whether l is an accepted locale key (BCP-47 style,
// e.g. "zh-CN"). Writers such as the seed importer use it for
// LocalizedText keys.
func ValidLocale(l string) bool { return localeRe.MatchString(l) }
