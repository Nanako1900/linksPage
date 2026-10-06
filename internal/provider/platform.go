package provider

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"maps"
	"regexp"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Card kinds (must equal site.CardKind values).
const (
	CardDiscord     = "discord"
	CardKOOK        = "kook"
	CardQQGroup     = "qq-group"
	CardWeChatGroup = "wechat-group"
	CardStatic      = "static"
)

// PlatformIDRe matches preset and custom platform ids.
var PlatformIDRe = regexp.MustCompile(`^[a-z][a-z0-9-]{1,31}$`)

// IconRefRe matches icon references "si:<simple-icons slug>" and
// "builtin:<name>". Uploaded icons are media keys stored separately.
var IconRefRe = regexp.MustCompile(`^(si|builtin):[a-z0-9-]{1,64}$`)

//go:embed platforms.yaml
var presetsYAML []byte

// Platform is one platform preset (platforms.yaml) or custom platform.
type Platform struct {
	ID string `yaml:"id"`
	// Name maps locale → display name (at least "en").
	Name map[string]string `yaml:"name"`
	// Icon is "si:<slug>", "builtin:<name>" or "" (custom platforms may
	// use an uploaded icon instead, held outside this struct).
	Icon string `yaml:"icon"`
	// Provider is "discord", "kook" or "" for static platforms.
	Provider string `yaml:"provider"`
	// Card is one of the Card* constants.
	Card string `yaml:"card"`
	// URLPattern is an RE2 pattern that invite/fallback URLs of this
	// platform must match ("" = any URL allowed by internal/content).
	URLPattern string `yaml:"url_pattern"`
	// NeedsExternalBrowser: inside WeChat/QQ show "open in browser"
	// instead of navigating (doc 5.7).
	NeedsExternalBrowser bool `yaml:"needs_external_browser"`
	// Custom is true for rows of custom_platforms.
	Custom bool `yaml:"-"`

	// re is URLPattern compiled by NewCatalog / WithCustom.
	re *regexp.Regexp
}

// platformsFile is the platforms.yaml document.
type platformsFile struct {
	Version   int        `yaml:"version"`
	Platforms []Platform `yaml:"platforms"`
}

// Catalog is an immutable set of platforms in display order.
type Catalog struct {
	order []Platform
	byID  map[string]Platform
}

// LoadPresets parses the embedded platforms.yaml.
func LoadPresets() (*Catalog, error) {
	ps, err := ParsePlatforms(presetsYAML)
	if err != nil {
		return nil, fmt.Errorf("embedded platforms.yaml: %w", err)
	}
	return NewCatalog(ps)
}

// ParsePlatforms strictly decodes a platforms.yaml document (unknown keys
// are errors). Semantic validation happens in NewCatalog.
func ParsePlatforms(data []byte) ([]Platform, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var f platformsFile
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("decode platforms: %w", err)
	}
	if f.Version != 1 {
		return nil, errors.New("platforms: version must be 1")
	}
	return f.Platforms, nil
}

// NewCatalog validates platforms (ids, icons, cards, patterns, duplicates).
// Presets must have an English name.
func NewCatalog(platforms []Platform) (*Catalog, error) {
	return (&Catalog{}).with(platforms, false)
}

// WithCustom returns a new catalog with custom platforms appended; ids
// must not collide with existing ones. Custom platforms are always static
// cards without a provider (Custom is set on the copies).
func (c *Catalog) WithCustom(custom []Platform) (*Catalog, error) {
	return c.with(custom, true)
}

func (c *Catalog) with(add []Platform, custom bool) (*Catalog, error) {
	if c == nil {
		c = &Catalog{}
	}
	out := &Catalog{
		order: make([]Platform, 0, len(c.order)+len(add)),
		byID:  make(map[string]Platform, len(c.order)+len(add)),
	}
	for _, p := range c.order {
		out.order = append(out.order, p)
		out.byID[p.ID] = p
	}
	for i, p := range add {
		p.Custom = custom
		compiled, err := validatePlatform(p)
		if err != nil {
			return nil, fmt.Errorf("platforms[%d]: %w", i, err)
		}
		if _, dup := out.byID[p.ID]; dup {
			return nil, fmt.Errorf("platforms[%d]: duplicate id %q", i, p.ID)
		}
		p.Name = maps.Clone(p.Name)
		p.re = compiled
		out.order = append(out.order, p)
		out.byID[p.ID] = p
	}
	return out, nil
}

// validCards maps card kinds to the provider they require ("" = static).
var validCards = map[string]string{
	CardDiscord:     "discord",
	CardKOOK:        "kook",
	CardQQGroup:     "",
	CardWeChatGroup: "",
	CardStatic:      "",
}

// maxPatternLen matches custom_platforms.url_pattern.
const maxPatternLen = 512

func validatePlatform(p Platform) (*regexp.Regexp, error) {
	if !PlatformIDRe.MatchString(p.ID) {
		return nil, fmt.Errorf("invalid id %q", p.ID)
	}
	if err := validateName(p.Name, !p.Custom); err != nil {
		return nil, err
	}
	if p.Icon != "" && !IconRefRe.MatchString(p.Icon) {
		return nil, fmt.Errorf("%s: invalid icon %q", p.ID, p.Icon)
	}
	wantProvider, ok := validCards[p.Card]
	if !ok {
		return nil, fmt.Errorf("%s: invalid card %q", p.ID, p.Card)
	}
	if p.Provider != wantProvider {
		return nil, fmt.Errorf("%s: card %q requires provider %q", p.ID, p.Card, wantProvider)
	}
	if p.Custom && p.Card != CardStatic {
		return nil, fmt.Errorf("%s: custom platforms must use the static card", p.ID)
	}
	if len(p.URLPattern) > maxPatternLen {
		return nil, fmt.Errorf("%s: url_pattern too long", p.ID)
	}
	if p.URLPattern == "" {
		return nil, nil
	}
	re, err := regexp.Compile(p.URLPattern)
	if err != nil {
		return nil, fmt.Errorf("%s: url_pattern: %w", p.ID, err)
	}
	return re, nil
}

func validateName(name map[string]string, requireEnglish bool) error {
	if requireEnglish && strings.TrimSpace(name["en"]) == "" {
		return errors.New("name.en is required")
	}
	for locale, v := range name {
		if locale == "" || strings.TrimSpace(v) == "" {
			return fmt.Errorf("name[%q] is empty", locale)
		}
	}
	if len(name) == 0 {
		return errors.New("name is required")
	}
	return nil
}

// MatchURL reports whether raw matches the platform's url_pattern; an
// empty pattern matches everything (content.SafeURL still applies).
func (p Platform) MatchURL(raw string) bool {
	if p.URLPattern == "" {
		return true
	}
	re := p.re
	if re == nil {
		compiled, err := regexp.Compile(p.URLPattern)
		if err != nil {
			return false
		}
		re = compiled
	}
	return re.MatchString(raw)
}

// Get returns the platform with id.
func (c *Catalog) Get(id string) (Platform, bool) {
	if c == nil {
		return Platform{}, false
	}
	p, ok := c.byID[id]
	if ok {
		p.Name = maps.Clone(p.Name)
	}
	return p, ok
}

// All returns the platforms in display order (a copy).
func (c *Catalog) All() []Platform {
	if c == nil {
		return []Platform{}
	}
	out := make([]Platform, len(c.order))
	for i, p := range c.order {
		p.Name = maps.Clone(p.Name)
		out[i] = p
	}
	return out
}
