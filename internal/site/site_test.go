package site

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/Nanako1900/linksPage/internal/store/dbq"
)

func TestParseSettings(t *testing.T) {
	s, err := ParseSettings(nil)
	if err != nil || s.DefaultLocale != "zh-CN" {
		t.Fatalf("defaults: %+v %v", s, err)
	}
	s, err = ParseSettings([]byte(`{"title":{"en":"Hi"},"appearance":"dark","theme":{"light":{"accent":"#ABC"},"radius":"8px"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if s.Title["en"] != "Hi" || s.Title["zh-CN"] != "我的社区" || s.Appearance != "dark" || s.Theme.Light.Bg != "#f9f7f1" {
		t.Errorf("overlay wrong: %+v", s)
	}
	css, err := s.Theme.CSS()
	if err != nil || !strings.Contains(css, "--lp-accent:#aabbcc;") || !strings.Contains(css, "--lp-radius:8px;") {
		t.Errorf("css = %s err=%v", css, err)
	}

	bad := []struct{ name, in, want string }{
		{"json", `{`, "decode site settings"},
		{"appearance", `{"appearance":"neon"}`, "appearance"},
		{"locales", `{"locales":[]}`, "locales: must not be empty"},
		{"locale tag", `{"locales":["zh-CN","EN_us"]}`, "locales[1]"},
		{"default locale", `{"defaultLocale":"ja"}`, "defaultLocale"},
		{"color", `{"theme":{"dark":{"bg":"red"}}}`, "theme.dark.bg"},
		{"css injection", `{"theme":{"light":{"fg":"#fff;}body{x:y"}}}`, "theme.light.fg"},
		{"oklch", `{"theme":{"light":{"fg":"oklch(50% 0.1 20)"}}}`, "theme.light.fg"},
		{"radius big", `{"theme":{"radius":"3rem"}}`, "between 0 and 2rem"},
		{"radius px big", `{"theme":{"radius":"40px"}}`, "between 0 and 2rem"},
		{"radius unit", `{"theme":{"radius":"1em"}}`, "rem or px"},
		{"font", `{"theme":{"fontSans":"Comic Sans"}}`, "fontSans"},
		{"display font", `{"theme":{"fontDisplay":"x"}}`, "fontDisplay"},
		{"preset", `{"theme":{"preset":"Bad Preset"}}`, "theme.preset"},
	}
	for _, tt := range bad {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseSettings([]byte(tt.in))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestThemeHelpers(t *testing.T) {
	if _, err := (Theme{}).CSS(); err == nil {
		t.Error("invalid theme CSS should fail")
	}
	th := DefaultTheme()
	if th.ThemeColor(false) != "#f9f7f1" || th.ThemeColor(true) != "#111216" {
		t.Error("theme colors wrong")
	}
	if (Theme{}).ThemeColor(false) != "" {
		t.Error("invalid theme color should be empty")
	}
	if got := CSPHash("abc"); got != "'sha256-ungWv48Bz+pBQUDeXa4iI7ADYaOWF3qctBD/YfIAFa0='" {
		t.Errorf("CSPHash = %s", got)
	}
	lt := LocalizedText{"en": "E"}
	if lt.Get("ja", "zh-CN") != "E" || (LocalizedText{}).Get("en", "en") != "" {
		t.Error("LocalizedText.Get fallback wrong")
	}
	s := Default()
	if s.ResolveLocale("en") != "en" || s.ResolveLocale("ja") != "zh-CN" || s.ResolveLocale("") != "zh-CN" {
		t.Error("ResolveLocale wrong")
	}
}

func TestSnapshotAndHolder(t *testing.T) {
	d := DefaultSnapshot()
	if d.Version != 0 || d.ThemeHash != CSPHash(d.ThemeCSS) {
		t.Fatal("default snapshot wrong")
	}
	h := NewHolder(d)
	if h.Current() != d {
		t.Fatal("holder wrong")
	}
	b := d.Bootstrap()
	b.Site.Title["en"] = "mutated"
	if d.Settings.Title["en"] == "mutated" {
		t.Error("Bootstrap must return a copy")
	}
	bad := Default()
	bad.Appearance = "x"
	if _, err := NewSnapshot(1, Page{}, bad, time.Now()); err == nil {
		t.Error("invalid settings should fail")
	}
	s2, _ := NewSnapshot(2, Page{ID: 1, Slug: "default"}, Default(), time.Now())
	h.Set(s2)
	if h.Current().Version != 2 {
		t.Error("Set failed")
	}
}

type fakeQuerier struct {
	settings dbq.SiteSetting
	page     dbq.Page
	sErr     error
	pErr     error
}

func (f fakeQuerier) GetSiteSettings(context.Context) (dbq.SiteSetting, error) {
	return f.settings, f.sErr
}
func (f fakeQuerier) GetDefaultPage(context.Context) (dbq.Page, error) { return f.page, f.pErr }

func TestLoadSnapshot(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx := context.Background()
	q := fakeQuerier{settings: dbq.SiteSetting{Version: 3, Data: []byte(`{"appearance":"light"}`)}, page: dbq.Page{ID: 1, Slug: "default"}}
	s, err := LoadSnapshot(ctx, q, logger)
	if err != nil || s.Version != 3 || s.Settings.Appearance != "light" || s.Page.Slug != "default" {
		t.Fatalf("snapshot = %+v err=%v", s, err)
	}
	q.settings.Data = []byte(`{"appearance":"neon"}`)
	s, err = LoadSnapshot(ctx, q, logger)
	if err != nil || s.Settings.Appearance != AppearanceAuto || s.Version != 3 {
		t.Fatalf("invalid settings should fall back to defaults: %+v %v", s, err)
	}
	if _, err := LoadSnapshot(ctx, fakeQuerier{sErr: errors.New("x")}, logger); err == nil {
		t.Error("settings error should propagate")
	}
	if _, err := LoadSnapshot(ctx, fakeQuerier{pErr: errors.New("x")}, logger); err == nil {
		t.Error("page error should propagate")
	}
}
