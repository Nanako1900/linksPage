package provider

import (
	"strings"
	"testing"
)

func TestLoadPresets(t *testing.T) {
	c, err := LoadPresets()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"discord", "kook", "telegram", "qq-group", "qq-channel", "wechat-group", "bilibili",
		"steam-group", "teamspeak", "matrix", "revolt", "link",
	}
	all := c.All()
	if len(all) != len(want) {
		t.Fatalf("presets = %d, want %d", len(all), len(want))
	}
	external := map[string]bool{"discord": true, "telegram": true, "matrix": true, "revolt": true}
	for i, id := range want {
		p := all[i]
		if p.ID != id || p.Custom {
			t.Errorf("preset %d = %s", i, p.ID)
		}
		if p.NeedsExternalBrowser != external[id] {
			t.Errorf("%s needs_external_browser = %v", id, p.NeedsExternalBrowser)
		}
	}
	matches := []struct {
		id, url string
		ok      bool
	}{
		{"discord", "https://discord.gg/KwdRuAkT", true},
		{"discord", "https://discord.com/invite/KwdRuAkT", true},
		{"discord", "https://evil.example/discord.gg/x", false},
		{"kook", "https://kook.top/abcd", true},
		{"telegram", "https://t.me/joinchat", true},
		{"qq-group", "https://qm.qq.com/q/abc", true},
		{"qq-group", "https://evil.example", false},
		{"link", "https://anything.example", true},
		{"wechat-group", "https://anything.example", true},
	}
	for _, m := range matches {
		p, ok := c.Get(m.id)
		if !ok || p.MatchURL(m.url) != m.ok {
			t.Errorf("%s.MatchURL(%s) != %v", m.id, m.url, m.ok)
		}
	}
	if p, _ := c.Get("discord"); ProviderFor(p) != "discord" || StaticState(p.Card) != StateStatic {
		t.Error("discord provider mapping")
	}
	if p, _ := c.Get("wechat-group"); ProviderFor(p) != ProviderStatic || StaticState(p.Card) != StateQROnly {
		t.Error("wechat mapping")
	}
}

func preset(mod func(p *Platform)) Platform {
	p := Platform{ID: "heybox", Name: map[string]string{"en": "Heybox"}, Icon: "builtin:heybox", Card: CardStatic, URLPattern: "^https://"}
	mod(&p)
	return p
}

func TestNewCatalogValidation(t *testing.T) {
	tests := []struct {
		name string
		p    Platform
		want string
	}{
		{"bad id", preset(func(p *Platform) { p.ID = "X" }), "invalid id"},
		{"missing en", preset(func(p *Platform) { p.Name = map[string]string{"zh-CN": "x"} }), "name.en"},
		{"empty name value", preset(func(p *Platform) { p.Name = map[string]string{"en": "x", "de": " "} }), "empty"},
		{"bad icon", preset(func(p *Platform) { p.Icon = "http://x" }), "invalid icon"},
		{"bad card", preset(func(p *Platform) { p.Card = "x" }), "invalid card"},
		{"card needs provider", preset(func(p *Platform) { p.Card = CardDiscord }), "requires provider"},
		{"static with provider", preset(func(p *Platform) { p.Provider = "kook" }), "requires provider"},
		{"bad pattern", preset(func(p *Platform) { p.URLPattern = "(" }), "url_pattern"},
		{"long pattern", preset(func(p *Platform) { p.URLPattern = strings.Repeat("a", 513) }), "too long"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewCatalog([]Platform{tt.p})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want %q", err, tt.want)
			}
		})
	}
	ok := preset(func(*Platform) {})
	if _, err := NewCatalog([]Platform{ok, ok}); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("duplicate err = %v", err)
	}
}

func TestCatalogWithCustom(t *testing.T) {
	base, err := LoadPresets()
	if err != nil {
		t.Fatal(err)
	}
	custom := preset(func(p *Platform) { p.Name = map[string]string{"zh-CN": "黑盒语音"}; p.URLPattern = "" })
	c, err := base.WithCustom([]Platform{custom})
	if err != nil {
		t.Fatal(err)
	}
	got, ok := c.Get("heybox")
	if !ok || !got.Custom || len(c.All()) != len(base.All())+1 || !got.MatchURL("https://x") {
		t.Errorf("custom = %+v", got)
	}
	if _, ok := base.Get("heybox"); ok {
		t.Error("WithCustom must not modify the receiver")
	}
	got.Name["zh-CN"] = "mutated"
	if again, _ := c.Get("heybox"); again.Name["zh-CN"] != "黑盒语音" {
		t.Error("Get must return a copy")
	}
	all := c.All()
	all[0].Name["en"] = "mutated"
	if p, _ := c.Get("discord"); p.Name["en"] != "Discord" {
		t.Error("All must return copies")
	}
	if _, err := base.WithCustom([]Platform{preset(func(p *Platform) { p.ID = "discord" })}); err == nil {
		t.Error("collision with preset must fail")
	}
	if _, err := base.WithCustom([]Platform{preset(func(p *Platform) { p.Card = CardQQGroup })}); err == nil {
		t.Error("custom non-static card must fail")
	}
	if _, err := base.WithCustom([]Platform{preset(func(p *Platform) { p.Name = nil })}); err == nil {
		t.Error("custom without name must fail")
	}
	var nilCat *Catalog
	if _, ok := nilCat.Get("x"); ok || len(nilCat.All()) != 0 {
		t.Error("nil catalog must be empty")
	}
	if c2, err := nilCat.WithCustom([]Platform{custom}); err != nil || len(c2.All()) != 1 {
		t.Errorf("nil WithCustom = %v", err)
	}
	unvalidated := Platform{URLPattern: "^https://a"}
	if !unvalidated.MatchURL("https://a/b") || unvalidated.MatchURL("https://b") {
		t.Error("lazy MatchURL")
	}
	if (Platform{URLPattern: "("}).MatchURL("x") {
		t.Error("invalid pattern must not match")
	}
}
