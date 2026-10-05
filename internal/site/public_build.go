package site

import (
	"maps"
	"slices"
	"time"
)

// PublicSiteFrom derives the public site view from settings. avatar is the
// resolved avatar image (nil when none).
func PublicSiteFrom(s Settings, baseURL string, avatar *ImageView) PublicSite {
	return PublicSite{
		BaseURL:       baseURL,
		DefaultLocale: s.DefaultLocale,
		Locales:       nonNil(slices.Clone(s.Locales)),
		Title:         cloneText(s.Title),
		Description:   cloneText(s.Description),
		DisplayName:   cloneText(s.DisplayName),
		Bio:           cloneText(s.Bio),
		Avatar:        avatar,
		Appearance:    s.Appearance,
		Theme:         s.Theme,
		Footer:        cloneText(s.Footer),
		ShowPoweredBy: s.ShowPoweredBy,
		NotFound:      cloneText(s.NotFound),
		Copy:          s.Copy.Clone(),
	}
}

// EmptyPublicPage is the page served before content is built: settings
// only, no blocks.
func EmptyPublicPage(version int64, page Page, s Settings, baseURL string, now time.Time) *PublicPage {
	return &PublicPage{
		Version:     version,
		Revision:    "",
		Page:        page,
		Site:        PublicSiteFrom(s, baseURL, nil),
		Blocks:      []BlockView{},
		Communities: map[string]CommunityView{},
		Links:       map[string]LinkView{},
		Platforms:   map[string]PlatformView{},
		GeneratedAt: now.UTC(),
	}
}

func cloneText(t LocalizedText) LocalizedText {
	if t == nil {
		return LocalizedText{}
	}
	return maps.Clone(t)
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
