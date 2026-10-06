// Package site holds the public site model: settings stored in the
// database, their built-in defaults and validation, the theme CSS derived
// from them and the immutable snapshot served to visitors.
package site

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
)

// Appearance modes.
const (
	AppearanceLight         = "light"
	AppearanceDark          = "dark"
	AppearanceAuto          = "auto"
	AppearanceVisitorChoice = "visitor-choice"
)

// LocalizedText maps a BCP-47 locale to text.
type LocalizedText map[string]string

// Get returns the text for locale, falling back to fallback and then to
// any "en" value.
func (t LocalizedText) Get(locale, fallback string) string {
	for _, l := range []string{locale, fallback, "en"} {
		if v, ok := t[l]; ok && v != "" {
			return v
		}
	}
	return ""
}

// Settings are the DB-editable site settings (site_settings.data). The
// stored JSON only needs to contain values that differ from Default().
// Settings are never sent to browsers as-is: the public view is PublicSite.
type Settings struct {
	DefaultLocale string        `json:"defaultLocale"`
	Locales       []string      `json:"locales"`
	Title         LocalizedText `json:"title"`
	Description   LocalizedText `json:"description"`
	Appearance    string        `json:"appearance"`
	Theme         Theme         `json:"theme"`

	// M1 additions (doc 9). See settings_m1.go for validation.

	// DisplayName is the name shown in the identity column (falls back to
	// Title when empty).
	DisplayName LocalizedText `json:"displayName"`
	// Bio is a short introduction in the Markdown subset of internal/content.
	Bio LocalizedText `json:"bio"`
	// AvatarKey is a media key ("<32 hex>.webp|png|jpg") or "".
	AvatarKey string `json:"avatarKey"`
	// Footer is footer text in the Markdown subset.
	Footer LocalizedText `json:"footer"`
	// ShowPoweredBy shows "Powered by LinksPage" in the footer.
	ShowPoweredBy bool `json:"showPoweredBy"`
	// OG overrides link-preview metadata.
	OG OGSettings `json:"og"`
	// SearchIndexing is SearchIndex or SearchNoIndex (meta robots and robots.txt).
	SearchIndexing string `json:"searchIndexing"`
	// NotFound is the lede of the 404 page (plain text).
	NotFound LocalizedText `json:"notFound"`
	// Copy overrides built-in guide texts per locale (see CopyKeys).
	Copy CopyOverrides `json:"copy"`
}

// Default returns fresh built-in settings ("Signal Paper").
func Default() Settings {
	return Settings{
		DefaultLocale:  "zh-CN",
		Locales:        []string{"zh-CN", "en"},
		Title:          LocalizedText{"zh-CN": "我的社区", "en": "My Communities"},
		Description:    LocalizedText{"zh-CN": "加入我们的社区。", "en": "Join our communities."},
		Appearance:     AppearanceAuto,
		Theme:          DefaultTheme(),
		DisplayName:    LocalizedText{},
		Bio:            LocalizedText{},
		Footer:         LocalizedText{},
		ShowPoweredBy:  true,
		OG:             OGSettings{Title: LocalizedText{}, Description: LocalizedText{}},
		SearchIndexing: SearchIndex,
		NotFound:       LocalizedText{},
		Copy:           CopyOverrides{},
	}
}

// ParseSettings overlays stored JSON on the defaults and validates the
// result. Empty input yields the defaults.
func ParseSettings(data []byte) (Settings, error) {
	s := Default()
	if len(data) > 0 {
		if err := json.Unmarshal(data, &s); err != nil {
			return Settings{}, fmt.Errorf("decode site settings: %w", err)
		}
	}
	if err := s.Validate(); err != nil {
		return Settings{}, err
	}
	return s, nil
}

var localeRe = regexp.MustCompile(`^[a-z]{2,3}(-[A-Za-z0-9]{2,8})*$`)

// Validate checks every field.
func (s Settings) Validate() error {
	var errs []error
	if len(s.Locales) == 0 {
		errs = append(errs, errors.New("locales: must not be empty"))
	}
	for i, l := range s.Locales {
		if !localeRe.MatchString(l) {
			errs = append(errs, fmt.Errorf("locales[%d]: invalid locale", i))
		}
	}
	if !slices.Contains(s.Locales, s.DefaultLocale) {
		errs = append(errs, errors.New("defaultLocale: must be one of locales"))
	}
	switch s.Appearance {
	case AppearanceLight, AppearanceDark, AppearanceAuto, AppearanceVisitorChoice:
	default:
		errs = append(errs, errors.New("appearance: must be light, dark, auto or visitor-choice"))
	}
	if err := s.Theme.Validate(); err != nil {
		errs = append(errs, err)
	}
	errs = append(errs, s.validateM1()...)
	return errors.Join(errs...)
}

// Clone returns a deep copy.
func (s Settings) Clone() Settings {
	out := s
	out.Locales = slices.Clone(s.Locales)
	out.Title = maps.Clone(s.Title)
	out.Description = maps.Clone(s.Description)
	out.DisplayName = maps.Clone(s.DisplayName)
	out.Bio = maps.Clone(s.Bio)
	out.Footer = maps.Clone(s.Footer)
	out.OG = s.OG.clone()
	out.NotFound = maps.Clone(s.NotFound)
	out.Copy = s.Copy.Clone()
	return out
}

// ResolveLocale returns requested when it is enabled, otherwise the
// default locale.
func (s Settings) ResolveLocale(requested string) string {
	if requested != "" && slices.Contains(s.Locales, requested) {
		return requested
	}
	return s.DefaultLocale
}
