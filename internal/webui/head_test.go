package webui

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/Nanako1900/linksPage/internal/site"
)

func TestHomeHeadMetadata(t *testing.T) {
	env := fixtureEnv(t, fakeMarkdown{}, withHeadAssets)
	w := do(env.rd.Public(PageHome), http.MethodGet, "/", nil)
	body := w.Body.String()
	for _, want := range []string{
		`<html lang="zh-CN" data-appearance="auto">`,
		`<title>猎人小屋</title>`,
		`<link rel="canonical" href="https://links.example.com/">`,
		`<link rel="alternate" hreflang="zh-CN" href="https://links.example.com/">`,
		`<link rel="alternate" hreflang="en" href="https://links.example.com/?lang=en">`,
		`<link rel="alternate" hreflang="x-default" href="https://links.example.com/">`,
		`<meta property="og:title" content="猎人小屋 · 社区">`,
		`<meta property="og:locale" content="zh_CN">`, `<meta property="og:locale:alternate" content="en">`,
		`<meta property="og:image" content="https://links.example.com/media/u/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.png">`,
		`<meta property="og:image:width" content="1200">`, `<meta property="og:image:height" content="630">`,
		`<meta name="twitter:card" content="summary_large_image">`,
		`<meta name="twitter:image" content="https://links.example.com/media/u/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.png">`,
		`<meta itemprop="name" content="猎人小屋 · 社区">`,
		`<meta itemprop="image" content="https://links.example.com/media/u/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.png">`,
		`<link rel="icon" type="image/png" sizes="32x32" href="/media/u/bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb.png">`,
		`<link rel="apple-touch-icon" type="image/png" sizes="180x180" href="/media/u/cccccccccccccccccccccccccccccccc.png">`,
		`<link rel="manifest" href="/site.webmanifest">`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("head missing %q", want)
		}
	}
	if strings.Contains(body, "dddddddddddddddd") || strings.Contains(body, `name="robots"`) {
		t.Error("192px icon belongs to the manifest; indexable pages emit no robots meta")
	}
	if !strings.Contains(w.Header().Get("Content-Security-Policy"), "frame-src https://discord.com;") {
		t.Errorf("fixture has a discord embed; CSP = %q", w.Header().Get("Content-Security-Policy"))
	}
	assertCSPMatchesInline(t, body, w.Header().Get("Content-Security-Policy"))
}

func TestCommunityPage(t *testing.T) {
	env := fixtureEnv(t, fakeMarkdown{}, withHeadAssets)
	mux := http.NewServeMux()
	mux.Handle("GET /c/{slug}", env.rd.Public(PageCommunity))

	w := do(mux, http.MethodGet, "/c/discord?lang=en", nil)
	body := w.Body.String()
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	for _, want := range []string{
		`<title>Main Discord · Hunter&#39;s Lodge</title>`,
		`<meta name="description" content="News, squads and voice.">`,
		`<link rel="canonical" href="https://links.example.com/c/discord?lang=en">`,
		`<link rel="alternate" hreflang="x-default" href="https://links.example.com/c/discord">`,
		`<meta property="og:title" content="Main Discord · Hunter&#39;s Lodge">`,
		`<meta property="og:image" content="https://links.example.com/media/p/Q2xvdWRmbGFyZUljb24xMg.png">`,
		`<meta name="twitter:card" content="summary">`,
		`<section class="lp-pinned" aria-label="Shared community"><article class="lp-card" data-state="live" data-card="discord">`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("community page missing %q", want)
		}
	}
	root := rootOf(t, body)
	if strings.Count(root, `href="/go/discord"`) != 1 {
		t.Error("pinned community must appear exactly once")
	}
	if strings.Index(root, "lp-pinned") > strings.Index(root, `<h2 class="lp-h">`) {
		t.Error("pinned community must come before the blocks")
	}

	// A community without an icon falls back to the site OG image.
	noIcon := do(mux, http.MethodGet, "/c/wechat", nil).Body.String()
	if !strings.Contains(noIcon, `<meta name="twitter:card" content="summary_large_image">`) ||
		!strings.Contains(noIcon, `<title>微信群 · 猎人小屋</title>`) ||
		!strings.Contains(noIcon, `<meta name="description" content="我们的 Discord、KOOK、QQ 和微信社区。">`) {
		t.Error("community without icon/description should use site OG image and description")
	}

	nf := do(mux, http.MethodGet, "/c/unknown", nil)
	if nf.Code != http.StatusNotFound || !strings.Contains(nf.Body.String(), `<p class="lp-lede">这个地址不存在。</p>`) ||
		strings.Contains(nf.Body.String(), "lp-data") || strings.Contains(nf.Header().Get("Content-Security-Policy"), "discord.com") {
		t.Errorf("unknown slug: %d", nf.Code)
	}
}

func TestNoIndexAndNotFoundText(t *testing.T) {
	env := fixtureEnv(t, fakeMarkdown{}, func(s *site.Snapshot) { s.Head.Robots = site.SearchNoIndex })
	body := do(env.rd.Public(PagePrivacy), http.MethodGet, "/privacy", nil).Body.String()
	if !strings.Contains(body, `<meta name="robots" content="noindex">`) || !strings.Contains(body, `<title>隐私政策 · 猎人小屋</title>`) {
		t.Error("noindex settings must emit meta robots")
	}
	nf := do(env.rd.NotFound(), http.MethodGet, "/x?lang=en", nil)
	nb := nf.Body.String()
	for _, want := range []string{`<p class="lp-lede">Nothing here.</p>`, `<a class="lp-link" href="/?lang=en">Back to home →</a>`, `<html lang="en"`} {
		if !strings.Contains(nb, want) {
			t.Errorf("404 missing %q", want)
		}
	}
	if strings.Contains(nb, "og:title") || strings.Contains(nb, "canonical") || nf.Header().Get("Vary") == "" {
		t.Error("404 must not carry canonical/OG metadata")
	}
}

func TestLocaleNegotiationOverHTTP(t *testing.T) {
	env := fixtureEnv(t, fakeMarkdown{}, nil)
	h := env.rd.Public(PageHome)
	zh := do(h, http.MethodGet, "/", nil)
	en := do(h, http.MethodGet, "/", map[string]string{"Accept-Language": "en-US,en;q=0.9,zh;q=0.5"})
	if !strings.Contains(en.Body.String(), `<html lang="en"`) || !strings.Contains(en.Body.String(), `<title>Hunter&#39;s Lodge</title>`) {
		t.Error("Accept-Language must select English")
	}
	if zh.Header().Get("ETag") == en.Header().Get("ETag") {
		t.Error("ETag must differ per locale")
	}
	if vary := strings.Join(en.Header().Values("Vary"), ","); !strings.Contains(vary, "Accept-Language") || !strings.Contains(vary, "Accept-Encoding") {
		t.Errorf("Vary = %q", vary)
	}
	again := do(h, http.MethodGet, "/", map[string]string{"Accept-Language": "en", "If-None-Match": en.Header().Get("ETag")})
	if again.Code != http.StatusNotModified || again.Header().Get("Content-Security-Policy") != en.Header().Get("Content-Security-Policy") ||
		!strings.Contains(strings.Join(again.Header().Values("Vary"), ","), "Accept-Language") {
		t.Errorf("304 = %d", again.Code)
	}
	if w := do(h, http.MethodGet, "/", map[string]string{"Accept-Language": "zh", "If-None-Match": en.Header().Get("ETag")}); w.Code != http.StatusOK {
		t.Error("an English ETag must not validate the Chinese page")
	}
	param := do(h, http.MethodGet, "/?lang=zh-CN", map[string]string{"Accept-Language": "en"})
	if !strings.Contains(param.Body.String(), `<html lang="zh-CN"`) {
		t.Error("?lang= must win over Accept-Language")
	}
}

func TestNegotiateLocale(t *testing.T) {
	locales := []string{"zh-CN", "en"}
	tests := []struct {
		name, param, accept, want string
	}{
		{"default", "", "", "zh-CN"},
		{"param exact", "en", "zh", "en"},
		{"param case-insensitive", "ZH-cn", "", "zh-CN"},
		{"param unsupported falls through", "fr", "en", "en"},
		{"param base language is not matched", "zh", "en", "en"},
		{"accept exact", "", "en", "en"},
		{"accept base match", "", "en-GB", "en"},
		{"accept zh-TW base match", "", "zh-TW", "zh-CN"},
		{"accept q order", "", "zh;q=0.4, en;q=0.8", "en"},
		{"accept ties keep order", "", "en, zh-CN", "en"},
		{"accept skips unknown", "", "fr, de;q=0.9, en;q=0.1", "en"},
		{"accept q=0 excluded", "", "en;q=0", "zh-CN"},
		{"accept wildcard ignored", "", "*", "zh-CN"},
		{"accept malformed q", "", "en;q=abc", "zh-CN"},
		{"accept q out of range", "", "en;q=2", "zh-CN"},
		{"accept unknown param", "", "en;level=1", "zh-CN"},
		{"accept bad tag", "", "e n,<x>", "zh-CN"},
		{"accept long tag", "", strings.Repeat("a", 40), "zh-CN"},
		{"accept empty entries", "", " , ,en", "en"},
		{"accept too many entries", "", strings.Repeat("fr,", 20) + "en", "zh-CN"},
		{"accept too long", "", strings.Repeat("x", 600) + ",en", "zh-CN"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := negotiateLocale(locales, "zh-CN", tt.param, tt.accept); got != tt.want {
				t.Errorf("negotiateLocale(%q, %q) = %q, want %q", tt.param, tt.accept, got, tt.want)
			}
		})
	}
}

func TestPublicCSP(t *testing.T) {
	off := PublicCSP("'b'", "'c'", "'t'", false)
	on := PublicCSP("'b'", "'c'", "'t'", true)
	if !strings.Contains(off, "frame-src 'none'") || strings.Contains(off, "discord") {
		t.Errorf("CSP without frame = %q", off)
	}
	if !strings.Contains(on, "frame-src https://discord.com;") || !strings.Contains(on, "img-src 'self' data: blob:") ||
		!strings.Contains(on, "script-src 'self' 'b';") || !strings.Contains(on, "style-src 'self' 'c' 't';") {
		t.Errorf("CSP with frame = %q", on)
	}
}

func TestLPDataMatchesFixture(t *testing.T) {
	env := fixtureEnv(t, fakeMarkdown{}, nil)
	body := do(env.rd.Public(PageHome), http.MethodGet, "/", nil).Body.String()
	m := lpDataRe.FindStringSubmatch(body)
	if m == nil {
		t.Fatal("lp-data missing")
	}
	var got struct {
		Data site.PublicPage `json:"data"`
	}
	if err := site.DecodeStrict([]byte(m[1]), &got); err != nil {
		t.Fatal(err)
	}
	if err := got.Data.Validate(); err != nil {
		t.Fatalf("embedded page violates the contract: %v", err)
	}
	want, _ := json.Marshal(env.snap.Public)
	have, _ := json.Marshal(&got.Data)
	if !reflect.DeepEqual(want, have) {
		t.Error("lp-data differs from the snapshot's PublicPage")
	}
}
