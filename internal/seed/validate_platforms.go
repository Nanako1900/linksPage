package seed

import (
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/Nanako1900/linksPage/internal/provider"
	"github.com/Nanako1900/linksPage/internal/store/dbq"
)

// Custom platform limits (custom_platforms CHECKs).
const (
	MaxPlatformNameRunes = 64
	MaxURLPatternBytes   = 512
)

// planPlatforms validates custom platforms and returns the catalog that
// communities are checked against (presets plus the valid custom ones).
func planPlatforms(ps []PlatformSeed, presets *provider.Catalog, p *problems, out *plan) *provider.Catalog {
	custom := make([]provider.Platform, 0, len(ps))
	seen := map[string]bool{}
	for i, s := range ps {
		field := fmt.Sprintf("platforms[%d]", i)
		pp, pl, ok := planPlatform(field, s, p)
		if _, preset := presets.Get(s.ID); preset {
			p.add(field+".id", "%q is a built-in platform", s.ID)
			ok = false
		}
		if seen[s.ID] {
			p.add(field+".id", "duplicate id %q", s.ID)
			ok = false
		}
		seen[s.ID] = true
		if ok {
			custom = append(custom, pl)
			out.platforms = append(out.platforms, pp)
		}
	}
	if len(custom) == 0 {
		return presets
	}
	cat, err := presets.WithCustom(custom)
	if err != nil {
		p.add("platforms", "%v", err)
		return presets
	}
	return cat
}

func planPlatform(field string, s PlatformSeed, p *problems) (platformPlan, provider.Platform, bool) {
	before := len(p.list)
	if !provider.PlatformIDRe.MatchString(s.ID) {
		p.add(field+".id", "must match %s", provider.PlatformIDRe)
	}
	name := p.text(field+".name", s.Name, MaxPlatformNameRunes, true)
	pp := platformPlan{params: dbq.InsertCustomPlatformParams{ID: s.ID, NeedsExternalBrowser: s.NeedsExternalBrowser}}
	if s.URLPattern != "" {
		pattern := s.URLPattern
		pp.params.UrlPattern = &pattern
		if _, err := regexp.Compile(pattern); err != nil || len(pattern) > MaxURLPatternBytes {
			p.add(field+".url_pattern", "must be a valid RE2 pattern of at most %d bytes", MaxURLPatternBytes)
		}
	}
	pl := provider.Platform{
		ID: s.ID, Name: name, Card: provider.CardStatic, URLPattern: s.URLPattern,
		NeedsExternalBrowser: s.NeedsExternalBrowser, Custom: true,
	}
	switch {
	case provider.IconRefRe.MatchString(s.Icon):
		icon := s.Icon
		pp.params.Icon = &icon
		pl.Icon = icon
	case s.Icon != "":
		pp.icon = p.imagePath(field+".icon", s.Icon)
	}
	raw, err := json.Marshal(name)
	if err != nil {
		p.add(field+".name", "%v", err)
	}
	pp.params.Name = raw
	return pp, pl, len(p.list) == before
}
