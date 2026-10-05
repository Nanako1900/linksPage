package seed

import (
	"encoding/json"
	"maps"
	"slices"

	"github.com/Nanako1900/linksPage/internal/content"
	"github.com/Nanako1900/linksPage/internal/site"
)

// placeholderKey stands in for not-yet-ingested images during validation.
const placeholderKey = "00000000000000000000000000000000.webp"

// planSite maps the site section onto site_settings.data keys (only keys
// present in the seed) and validates the result against the defaults.
func planSite(s SiteSeed, md *content.Markdown, p *problems, out *plan) {
	m := map[string]any{}
	setIf(m, "defaultLocale", s.DefaultLocale, s.DefaultLocale != "")
	setIf(m, "locales", s.Locales, s.Locales != nil)
	plainTexts := map[string]Text{
		"title": s.Title, "description": s.Description, "displayName": s.DisplayName, "notFound": s.NotFound,
	}
	yamlNames := map[string]string{"title": "title", "description": "description", "displayName": "display_name", "notFound": "not_found"}
	for _, key := range slices.Sorted(maps.Keys(plainTexts)) {
		t := plainTexts[key]
		setIf(m, key, p.text("site."+yamlNames[key], t, MaxPlainTextRunes, false), t != nil)
	}
	setIf(m, "bio", p.markdown("site.bio", s.Bio, md, false), s.Bio != nil)
	setIf(m, "footer", p.markdown("site.footer", s.Footer, md, false), s.Footer != nil)
	if s.ShowPoweredBy != nil {
		m["showPoweredBy"] = *s.ShowPoweredBy
	}
	setIf(m, "appearance", s.Appearance, s.Appearance != "")
	setIf(m, "searchIndexing", s.SearchIndexing, s.SearchIndexing != "")
	if s.Copy != nil {
		m["copy"] = planCopy(s.Copy)
	}
	out.avatar = p.imagePath("site.avatar", s.Avatar)
	out.ogImage = p.imagePath("site.og.image", s.OG.Image)
	if s.OG.Title != nil || s.OG.Description != nil || s.OG.Image != "" {
		m["og"] = site.OGSettings{
			Title:       p.text("site.og.title", s.OG.Title, MaxPlainTextRunes, false),
			Description: p.text("site.og.description", s.OG.Description, MaxPlainTextRunes, false),
		}
	}
	out.settings = m
	validateSettings(m, out, p)
}

func setIf(m map[string]any, key string, v any, ok bool) {
	if ok {
		m[key] = v
	}
}

// planCopy cleans copy overrides; keys and lengths are checked by
// site.Settings.Validate.
func planCopy(c map[string]map[string]string) site.CopyOverrides {
	out := site.CopyOverrides{}
	for l, kv := range c {
		out[l] = map[string]string{}
		for k, v := range kv {
			out[l][k] = content.CleanText(v, 0)
		}
	}
	return out
}

// validateSettings overlays the seeded keys (with placeholder media keys)
// on the defaults and runs the settings validation.
func validateSettings(m map[string]any, out *plan, p *problems) {
	probe := withMediaKeys(m, keyIf(out.avatar), keyIf(out.ogImage))
	raw, err := json.Marshal(probe)
	if err != nil {
		p.add("site", "%v", err)
		return
	}
	if _, err := site.ParseSettings(raw); err != nil {
		p.add("site", "%v", err)
	}
}

func keyIf(path string) string {
	if path == "" {
		return ""
	}
	return placeholderKey
}

// withMediaKeys returns a copy of m with avatarKey / og.imageKey set.
func withMediaKeys(m map[string]any, avatarKey, ogKey string) map[string]any {
	out := maps.Clone(m)
	if avatarKey != "" {
		out["avatarKey"] = avatarKey
	}
	if ogKey != "" {
		og, _ := out["og"].(site.OGSettings)
		og.ImageKey = ogKey
		if og.Title == nil {
			og.Title = site.LocalizedText{}
		}
		if og.Description == nil {
			og.Description = site.LocalizedText{}
		}
		out["og"] = og
	}
	return out
}
