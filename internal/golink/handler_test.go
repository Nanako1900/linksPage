package golink

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Nanako1900/linksPage/internal/provider"
	"github.com/Nanako1900/linksPage/internal/site"
)

const testBaseURL = "https://links.example.com"

var (
	inlineStyleRe  = regexp.MustCompile(`(?s)<style>(.*?)</style>`)
	inlineScriptRe = regexp.MustCompile(`(?s)<script>(.*?)</script>`)
	styleAttrRe    = regexp.MustCompile(`(?i)\sstyle\s*=`)
	externalRe     = regexp.MustCompile(`(?i)<(script|link)[^>]+(src|href)=`)
)

type recordingHook struct {
	mu     sync.Mutex
	events []ClickEvent
}

func (h *recordingHook) OnClick(_ context.Context, ev ClickEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.events = append(h.events, ev)
}

func joinPath(slug string) *string {
	s := site.PathGo + slug
	return &s
}

// testSnapshot builds a page with the communities of testSource().
func testSnapshot(t *testing.T, mutate func(*site.Settings)) *site.Snapshot {
	t.Helper()
	settings := site.Default()
	if mutate != nil {
		mutate(&settings)
	}
	snap, err := site.NewSnapshot(3, site.Page{ID: 1, Slug: "default"}, settings, testNow)
	if err != nil {
		t.Fatal(err)
	}
	page := *snap.Public
	page.Communities = map[string]site.CommunityView{
		"id-discord":  {ID: "id-discord", Slug: "discord", Platform: "discord", SharePath: "/c/discord", Name: site.LocalizedText{"zh-CN": "主服务器", "en": "Main server"}, Live: site.LiveView{State: provider.StateLive, JoinURL: joinPath("discord")}},
		"id-telegram": {ID: "id-telegram", Slug: "telegram", Platform: "telegram", SharePath: "/c/telegram", Name: site.LocalizedText{"en": "TG <chat>"}, Live: site.LiveView{State: provider.StateStatic}},
		"id-gone":     {ID: "id-gone", Slug: "gone", Platform: "discord", SharePath: "/c/gone", Name: site.LocalizedText{"en": "Gone"}, Live: site.LiveView{State: provider.StateUnavailable}},
		"id-wechat": {
			ID: "id-wechat", Slug: "wechat", Platform: "wechat-group", SharePath: "/c/wechat", Name: site.LocalizedText{"en": "WeChat fans"},
			QR:      &site.QRView{URL: "/media/q/11111111-1111-1111-1111-111111111111", Width: 430, Height: 430, Note: site.LocalizedText{"en": "Valid until 10-12"}},
			Contact: &site.ContactView{Label: site.LocalizedText{}, Value: "admin_wx"},
			Live:    site.LiveView{State: provider.StateQROnly},
		},
	}
	page.Platforms = map[string]site.PlatformView{
		"discord":  {ID: "discord", Name: site.LocalizedText{"en": "Discord"}},
		"telegram": {ID: "telegram", Name: site.LocalizedText{"en": "Telegram"}},
	}
	page.Blocks = []site.BlockView{
		{ID: "b1", Kind: site.BlockCommunity, CommunityID: "id-discord"},
		{ID: "b2", Kind: site.BlockCommunity, CommunityID: "id-telegram"},
		{ID: "b3", Kind: site.BlockCommunity, CommunityID: "id-gone"},
		{ID: "b4", Kind: site.BlockCommunity, CommunityID: "id-discord"},
		{ID: "b5", Kind: site.BlockCommunity, CommunityID: "id-missing"},
		{ID: "b6", Kind: site.BlockCommunity, CommunityID: "id-dead"},
		{ID: "b7", Kind: site.BlockHeading, Text: site.LocalizedText{"en": "x"}},
	}
	snap.Public = &page
	return snap
}

func newTestHandler(t *testing.T, src Source, snap *site.Snapshot, hook ClickHook) http.Handler {
	t.Helper()
	h, err := NewHandler(HandlerOptions{
		Resolver: NewResolver(src, func() time.Time { return testNow }),
		Snapshot: func() *site.Snapshot { return snap },
		BaseURL:  testBaseURL + "/",
		Hook:     hook,
	})
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func serve(h http.Handler, target, ua, acceptLang string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	slug := strings.TrimPrefix(req.URL.Path, "/go/")
	req.SetPathValue("slug", slug)
	req.Header.Set("User-Agent", ua)
	if acceptLang != "" {
		req.Header.Set("Accept-Language", acceptLang)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

const (
	weChatUA  = "Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/15E148 MicroMessenger/8.0.49(0x18003130) NetType/WIFI Language/zh_CN"
	qqUA      = "Mozilla/5.0 (iPhone; CPU iPhone OS 16_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/15E148 QQ/8.9.20.618 V1_IPH_SQ_8.9.20_1_APP_A"
	desktopUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0.0.0 Safari/537.36"
)

func TestHandlerResponses(t *testing.T) {
	tests := []struct {
		name     string
		target   string
		ua       string
		accept   string
		status   int
		location string
		contains []string
		absent   []string
	}{
		{"redirect discord", "/go/discord", desktopUA, "", 302, "https://discord.gg/abc", nil, nil},
		{"redirect link", "/go/blog", weChatUA, "", 302, "https://blog.example.com", nil, nil},
		{"open in browser zh", "/go/discord", weChatUA, "", 200, "", []string{`lang="zh-CN"`, "点击右上角 ··· → 在浏览器打开", "主服务器", `data-copy="https://links.example.com/go/discord"`, "复制链接", "lp-arrow"}, []string{`<ul class="lp-list"`}},
		{"open in browser en via accept", "/go/discord", qqUA, "en-US,en;q=0.9", 200, "", []string{`lang="en"`, "Tap ··· in the top-right corner", "Main server", "Copy link"}, nil},
		{"open in browser lang param", "/go/telegram?lang=en", qqUA, "zh-CN", 200, "", []string{`lang="en"`, "TG &lt;chat&gt;"}, []string{"TG <chat>"}},
		{"qq group in wechat", "/go/qq-link", weChatUA, "", 200, "", []string{"123456789", "复制群号", "QQ 群号"}, []string{"qm.qq.com"}},
		{"qq group plain", "/go/qq-plain", desktopUA, "", 200, "", []string{"987654", "打开 QQ，搜索群号"}, nil},
		{"wechat qr from page", "/go/wechat", desktopUA, "en", 200, "", []string{`src="/media/q/11111111-1111-1111-1111-111111111111"`, `width="430"`, "Valid until 10-12", "admin_wx", "Copy WeChat ID", "Press and hold"}, nil},
		{"wechat qr off page", "/go/wechat2", desktopUA, "", 200, "", []string{`src="/media/q/22222222-2222-2222-2222-222222222222"`, `width="240"`, "群满或二维码失效时"}, nil},
		{"wechat without qr", "/go/wechat-noqr", desktopUA, "", 200, "", []string{"二维码暂未上传"}, []string{"<img"}},
		{"unavailable lists others", "/go/dead", desktopUA, "en", 200, "", []string{"This invite is unavailable", "Other communities", `href="/go/discord"`, `href="/c/telegram"`, "Telegram"}, []string{`href="/c/gone"`, "/go/dead\""}},
		{"not found", "/go/nope", desktopUA, "", 404, "", []string{"链接不存在", `href="/"`}, nil},
		{"invalid slug", "/go/NOPE", desktopUA, "en", 404, "", []string{"Link not found"}, nil},
	}
	h := newTestHandler(t, handlerSource(), testSnapshot(t, nil), nil)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := serve(h, tt.target, tt.ua, tt.accept)
			assertCommonHeaders(t, rec)
			if rec.Code != tt.status {
				t.Fatalf("status = %d, want %d; body: %s", rec.Code, tt.status, rec.Body.String())
			}
			if got := rec.Header().Get("Location"); got != tt.location {
				t.Errorf("Location = %q, want %q", got, tt.location)
			}
			if tt.status == http.StatusFound {
				return
			}
			body := rec.Body.String()
			assertSelfContained(t, body, rec.Header().Get("Content-Security-Policy"))
			for _, s := range tt.contains {
				if !strings.Contains(body, s) {
					t.Errorf("body missing %q", s)
				}
			}
			for _, s := range tt.absent {
				if strings.Contains(body, s) {
					t.Errorf("body unexpectedly contains %q", s)
				}
			}
		})
	}
}

func handlerSource() *fakeSource {
	src := testSource()
	qq := src.communities["qq-link"]
	qq.QQGroupNumber = "123456789"
	src.communities["qq-link"] = qq
	plain := src.communities["qq-plain"]
	plain.QQGroupNumber = "987654"
	src.communities["qq-plain"] = plain
	w := src.communities["wechat"]
	w.ID = "id-wechat"
	src.communities["wechat"] = w
	w2 := staticTarget("wechat2", "wechat-group", provider.CardWeChatGroup, false, provider.JoinInput{})
	w2.HasQR, w2.QRID = true, "22222222-2222-2222-2222-222222222222"
	w2.Name = site.LocalizedText{"zh-CN": "粉丝群"}
	w2.Contact = &site.ContactView{Value: "wx_2"}
	src.communities["wechat2"] = w2
	src.communities["wechat-noqr"] = staticTarget("wechat-noqr", "wechat-group", provider.CardWeChatGroup, false, provider.JoinInput{})
	return src
}

func assertCommonHeaders(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if rec.Code == http.StatusTooManyRequests {
		t.Fatal("/go must never return 429")
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q", got)
	}
	if got := rec.Header().Get("X-Robots-Tag"); got != "noindex" {
		t.Errorf("X-Robots-Tag = %q", got)
	}
}

// assertSelfContained checks the hash CSP against the inline blocks and
// that the page has no style attributes or external scripts/styles.
func assertSelfContained(t *testing.T, body, csp string) {
	t.Helper()
	styles := inlineStyleRe.FindAllStringSubmatch(body, -1)
	if len(styles) != 2 {
		t.Fatalf("want 2 inline styles, got %d", len(styles))
	}
	for _, s := range styles {
		if !strings.Contains(csp, site.CSPHash(s[1])) {
			t.Errorf("CSP %q misses style hash", csp)
		}
	}
	scripts := inlineScriptRe.FindAllStringSubmatch(body, -1)
	if len(scripts) != 1 || !strings.Contains(csp, "script-src "+site.CSPHash(scripts[0][1])) {
		t.Errorf("script/CSP mismatch (%d scripts) in %q", len(scripts), csp)
	}
	for _, part := range []string{"default-src 'none'", "frame-ancestors 'none'", "object-src 'none'", "base-uri 'none'", "img-src 'self'"} {
		if !strings.Contains(csp, part) {
			t.Errorf("CSP missing %q", part)
		}
	}
	if strings.Contains(csp, "unsafe-inline") {
		t.Error("CSP allows unsafe-inline")
	}
	if styleAttrRe.MatchString(body) {
		t.Error("page contains a style attribute")
	}
	if externalRe.MatchString(body) {
		t.Error("page references external scripts or stylesheets")
	}
	for _, s := range []string{`name="viewport"`, "viewport-fit=cover", `name="robots" content="noindex`} {
		if !strings.Contains(body, s) {
			t.Errorf("head missing %q", s)
		}
	}
}

func TestHandlerHookAndErrors(t *testing.T) {
	hook := &recordingHook{}
	h := newTestHandler(t, handlerSource(), testSnapshot(t, nil), hook)
	serve(h, "/go/discord", weChatUA, "")
	serve(h, "/go/blog", desktopUA, "")
	if len(hook.events) != 2 {
		t.Fatalf("hook got %d events", len(hook.events))
	}
	ev := hook.events[0]
	if ev.Slug != "discord" || ev.CommunityID != "id-discord" || ev.Action != ActionOpenInBrowser || !ev.UA.InWeChat || !ev.At.Equal(testNow) {
		t.Errorf("event = %+v", ev)
	}
	if hook.events[1].CommunityID != "" || hook.events[1].Action != ActionRedirect {
		t.Errorf("link event = %+v", hook.events[1])
	}

	failing := newTestHandler(t, &fakeSource{commErr: errors.New("db down")}, testSnapshot(t, nil), hook)
	rec := serve(failing, "/go/discord", desktopUA, "")
	assertCommonHeaders(t, rec)
	if rec.Code != http.StatusServiceUnavailable || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("status = %d, Retry-After = %q", rec.Code, rec.Header().Get("Retry-After"))
	}
	if !strings.Contains(rec.Body.String(), "暂时无法打开") {
		t.Error("error page text missing")
	}
	if len(hook.events) != 2 {
		t.Error("hook must not run for failed lookups")
	}
}

func TestHandlerCopyOverridesAndAppearance(t *testing.T) {
	tests := []struct {
		name       string
		appearance string
		contains   []string
		absent     []string
	}{
		{"auto follows system", site.AppearanceAuto, []string{"@media (prefers-color-scheme:dark){:root{--lp-bg:#111216;", `<html lang="zh-CN">`}, nil},
		{"dark fixed", site.AppearanceDark, []string{`data-appearance="dark"`, `name="theme-color" content="#111216"`}, []string{"prefers-color-scheme:dark"}},
		{"light fixed", site.AppearanceLight, []string{`data-appearance="light"`, `content="#f9f7f1"`}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snap := testSnapshot(t, func(s *site.Settings) {
				s.Appearance = tt.appearance
				s.Copy = site.CopyOverrides{
					"zh-CN": {site.CopyOpenInBrowser: "请用浏览器打开", site.CopyInviteUnavailable: "暂时没有邀请"},
				}
			})
			h := newTestHandler(t, handlerSource(), snap, nil)
			body := serve(h, "/go/discord", weChatUA, "").Body.String()
			for _, s := range append([]string{"请用浏览器打开"}, tt.contains...) {
				if !strings.Contains(body, s) {
					t.Errorf("body missing %q", s)
				}
			}
			for _, s := range tt.absent {
				if strings.Contains(body, s) {
					t.Errorf("body unexpectedly contains %q", s)
				}
			}
			if !strings.Contains(serve(h, "/go/dead", desktopUA, "").Body.String(), "暂时没有邀请") {
				t.Error("inviteUnavailable override not used")
			}
		})
	}
}

func TestHandlerNotFoundUsesSettings(t *testing.T) {
	snap := testSnapshot(t, func(s *site.Settings) {
		s.NotFound = site.LocalizedText{"zh-CN": "这里什么都没有"}
	})
	h := newTestHandler(t, handlerSource(), snap, nil)
	rec := serve(h, "/go/missing", desktopUA, "")
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "这里什么都没有") {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestHandlerNilSnapshotFallsBack(t *testing.T) {
	h, err := NewHandler(HandlerOptions{
		Resolver: NewResolver(handlerSource(), nil),
		Snapshot: func() *site.Snapshot { return nil },
		BaseURL:  testBaseURL,
	})
	if err != nil {
		t.Fatal(err)
	}
	rec := serve(h, "/go/dead", desktopUA, "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "邀请暂不可用") {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestHandlerStyleCacheReuse(t *testing.T) {
	snap := testSnapshot(t, nil)
	var c styleCache
	a, b := c.get(snap), c.get(snap)
	if a != b {
		t.Fatal("style not cached for the same theme")
	}
	dark := testSnapshot(t, func(s *site.Settings) { s.Appearance = site.AppearanceDark })
	if c.get(dark) == a {
		t.Fatal("style cache ignores appearance")
	}
}

func TestNewHandlerValidation(t *testing.T) {
	snapFn := func() *site.Snapshot { return nil }
	r := NewResolver(&fakeSource{}, nil)
	tests := []struct {
		name string
		opts HandlerOptions
	}{
		{"no resolver", HandlerOptions{Snapshot: snapFn, BaseURL: testBaseURL}},
		{"resolver without source", HandlerOptions{Resolver: NewResolver(nil, nil), Snapshot: snapFn, BaseURL: testBaseURL}},
		{"no snapshot", HandlerOptions{Resolver: r, BaseURL: testBaseURL}},
		{"empty base", HandlerOptions{Resolver: r, Snapshot: snapFn}},
		{"relative base", HandlerOptions{Resolver: r, Snapshot: snapFn, BaseURL: "/x"}},
		{"ftp base", HandlerOptions{Resolver: r, Snapshot: snapFn, BaseURL: "ftp://x"}},
		{"credentials", HandlerOptions{Resolver: r, Snapshot: snapFn, BaseURL: "https://u:p@x"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewHandler(tt.opts); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestNoopClickHook(t *testing.T) {
	var hook ClickHook = NoopClickHook{}
	hook.OnClick(context.Background(), ClickEvent{Slug: "x"})
}
