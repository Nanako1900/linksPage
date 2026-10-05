package webui

import (
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/Nanako1900/linksPage/internal/provider"
	"github.com/Nanako1900/linksPage/internal/site"
)

func rootOf(t *testing.T, body string) string {
	t.Helper()
	start := strings.Index(body, `<div id="root">`)
	end := strings.Index(body, `</div>`+"\n"+`<script id="lp-data"`)
	if start < 0 || end < 0 {
		t.Fatalf("no #root in body:\n%s", body)
	}
	return body[start+len(`<div id="root">`) : end]
}

func TestFallbackRendersEveryBlockAndCardState(t *testing.T) {
	env := fixtureEnv(t, fakeMarkdown{}, nil)
	w := do(env.rd.Public(PageHome), http.MethodGet, "/", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	root := rootOf(t, w.Body.String())
	for _, want := range []string{
		// identity
		`<img class="lp-avatar" src="/media/u/0123456789abcdef0123456789abcdef.webp" width="256" height="256" alt="">`,
		`<h1 class="lp-title">猎人小屋</h1>`,
		`<div class="lp-md lp-lede"><p class="md">欢迎来玩，**周五晚上**一起开黑。</p></div>`,
		// heading with count, text, link, social row
		`<h2 class="lp-h">社区 <span class="lp-count">— 08</span></h2>`,
		`<h2 class="lp-h">链接</h2>`,
		`<a class="lp-row" href="/go/blog">博客 <span aria-hidden="true">→</span></a>`,
		`<div class="lp-md"><p class="md">活动日历见 [博客](https://blog.example.com)。</p></div>`,
		`<li><a href="/go/github">GitHub</a></li>`,
		`<li><a href="https://mastodon.social/@example" rel="me noopener">Mastodon</a></li>`,
		// live discord card
		`<article class="lp-card" data-state="live" data-card="discord">`,
		`<img class="lp-icon" src="/media/p/Q2xvdWRmbGFyZUljb24xMg.png" width="128" height="128" alt="">`,
		`<h3 class="lp-name">Discord 主服务器 <span class="lp-tag">Discord</span></h3>`,
		`<p class="lp-stat">● 13 在线 · 125 成员</p>`,
		`<ul class="lp-chips"><li>#General</li><li>#开黑 1 号</li></ul>`,
		`<p class="lp-desc">在线：user1、猎人二号、user3</p>`,
		`<a class="lp-btn" href="/go/discord">加入 →</a>`,
		// stale
		`<p class="lp-stat">● 4 在线 · 52 成员 · <time datetime="2026-10-05T12:10:00Z">数据更新于 2026-10-05 12:10 UTC</time></p>`,
		// degraded without target
		`<article class="lp-card" data-state="degraded" data-card="discord">`,
		`<p class="lp-notice">邀请暂不可用，请稍后再试。</p>`,
		// kook badge
		`<p class="lp-stat">● 10350 在线 · 107345 成员</p>`,
		// qq group with link and QR, qq group without link
		`<p class="lp-qq">群号 <strong class="lp-mono">123456789</strong></p>`,
		`<img src="/media/q/01920000-0000-7000-8000-000000000301" width="480" height="480" alt="QQ 粉丝群 二维码" loading="lazy">`,
		`<a class="lp-btn" href="/go/qq-fans">加群 →</a>`,
		`<p class="lp-qq">群号 <strong class="lp-mono">87654321</strong></p>`,
		// wechat group
		`<article class="lp-card" data-state="qr-only" data-card="wechat-group">`,
		`<img src="/media/q/01920000-0000-7000-8000-000000000302" width="430" height="430" alt="微信群 二维码" loading="lazy">`,
		`<figcaption>长按或扫码加入 · 更新于 10-05，7 天内有效</figcaption>`,
		`<p class="lp-desc">群满或二维码失效时，加我拉你进群 <strong class="lp-mono">lodge_admin</strong></p>`,
		// unavailable with custom text
		`<article class="lp-card" data-state="unavailable" data-card="kook">`,
		`<p class="lp-notice">服务器已关闭，请加入新的 KOOK。</p>`,
		// footer
		`<footer class="lp-shell lp-foot">`, `<p class="md">联系：[邮件](mailto:hi@example.com)</p>`,
		`<a href="https://github.com/Nanako1900/linksPage" rel="noopener">由 LinksPage 驱动</a>`,
		`<noscript><p class="lp-note">`,
	} {
		if !strings.Contains(root, want) {
			t.Errorf("fallback missing %q", want)
		}
	}
	for _, unwanted := range []string{"/go/discord-old", "/go/kook-old", "/go/wechat", "/go/qq-chat", "lp-note\">正在加载", "style="} {
		if strings.Contains(root, unwanted) {
			t.Errorf("fallback must not contain %q", unwanted)
		}
	}
	if got := strings.Count(root, `<article class="lp-card"`); got != 8 {
		t.Errorf("cards = %d, want 8", got)
	}
}

func TestFallbackEnglishAndCopyFallbacks(t *testing.T) {
	env := fixtureEnv(t, fakeMarkdown{}, func(s *site.Snapshot) {
		c := s.Public.Communities["01920000-0000-7000-8000-000000000008"]
		c.UnavailableText = site.LocalizedText{}
		s.Public.Communities[c.ID] = c
		s.Public.Site.Copy = site.CopyOverrides{"en": {site.CopyInviteUnavailable: "Invite is gone."}}
	})
	root := rootOf(t, do(env.rd.Public(PageHome), http.MethodGet, "/?lang=en", nil).Body.String())
	for _, want := range []string{
		`<h2 class="lp-h">Communities <span class="lp-count">— 08</span></h2>`,
		`<p class="lp-stat">● 13 online · 125 members</p>`, `Online: user1, 猎人二号, user3`,
		`<p class="lp-notice">Invite is gone.</p>`,
		`<p class="lp-notice">This community is currently unavailable.</p>`,
		`<a class="lp-btn" href="/go/discord">Join →</a>`, `Join group →`, `Scan to join`,
	} {
		if !strings.Contains(root, want) {
			t.Errorf("english fallback missing %q", want)
		}
	}
}

func TestCardHelpers(t *testing.T) {
	one, two := 1, 2
	en := uiText["en"]
	for _, tt := range []struct {
		live site.LiveView
		want string
	}{
		{site.LiveView{Online: &one, Members: &two}, "● 1 online · 2 members"},
		{site.LiveView{Online: &one}, "● 1 online"},
		{site.LiveView{Members: &two}, "2 members"},
		{site.LiveView{}, ""},
	} {
		if got := statLine(en, tt.live); got != tt.want {
			t.Errorf("statLine = %q, want %q", got, tt.want)
		}
	}
	chs := make([]site.ChannelView, 20)
	if got := channelNames(chs); len(got) != maxFallbackChannels {
		t.Errorf("channels = %d", len(got))
	}
	name := "n"
	users := make([]site.UserView, 13)
	for i := range users {
		users[i] = site.UserView{Name: &name, Status: "online"}
	}
	c := site.CommunityView{MemberDisplay: site.MemberAvatarsNames, Live: site.LiveView{Users: users}}
	if got := userSummary(en, c); !strings.HasSuffix(got, " and 3 more") {
		t.Errorf("userSummary = %q", got)
	}
	c.Live.Users = []site.UserView{{Status: "online"}}
	if got := userSummary(en, c); got != "" {
		t.Errorf("nameless users summary = %q", got)
	}
	c.MemberDisplay = site.MemberAvatars
	if userSummary(en, c) != "" {
		t.Error("avatars mode must not list names")
	}
}

func TestPendingCardNotice(t *testing.T) {
	env := fixtureEnv(t, fakeMarkdown{}, func(s *site.Snapshot) {
		c := s.Public.Communities["01920000-0000-7000-8000-000000000004"]
		c.Live = site.LiveView{State: provider.StatePending, Channels: []site.ChannelView{}, Users: []site.UserView{}, JoinURL: c.Live.JoinURL}
		s.Public.Communities[c.ID] = c
	})
	root := rootOf(t, do(env.rd.Public(PageHome), http.MethodGet, "/", nil).Body.String())
	if !strings.Contains(root, `<p class="lp-notice">正在获取最新数据…</p>`) {
		t.Error("pending card must show the pending hint")
	}
}

func TestFallbackSkipsDanglingAndEmptyBlocks(t *testing.T) {
	env := fixtureEnv(t, fakeMarkdown{}, func(s *site.Snapshot) {
		s.Public.Blocks = []site.BlockView{
			{ID: "a", Kind: site.BlockCommunity, CommunityID: "missing"},
			{ID: "b", Kind: site.BlockLink, LinkID: "missing"},
			{ID: "c", Kind: site.BlockSocialRow, LinkIDs: []string{"missing"}},
			{ID: "d", Kind: site.BlockHeading, Text: site.LocalizedText{}},
			{ID: "e", Kind: site.BlockText, Markdown: site.LocalizedText{}},
			{ID: "f", Kind: "future"},
		}
		s.Public.Site.ShowPoweredBy = false
		s.Public.Site.Footer = site.LocalizedText{}
	})
	fv := env.rd.fallbackView(env.rd.resolve(PageHome, "", "", ""))
	if len(fv.Blocks) != 0 || fv.Note == "" || fv.PoweredBy != "" || fv.Footer != "" {
		t.Errorf("fallback view = %+v", fv)
	}
	root := rootOf(t, do(env.rd.Public(PageHome), http.MethodGet, "/", nil).Body.String())
	if strings.Contains(root, "<footer") {
		t.Error("empty footer must be omitted")
	}
}

func TestMarkdownFailureFallsBackToText(t *testing.T) {
	env := fixtureEnv(t, fakeMarkdown{err: errMarkdown}, func(s *site.Snapshot) {
		s.Public.Site.Bio = site.LocalizedText{"zh-CN": "<b>hi</b>"}
	})
	root := rootOf(t, do(env.rd.Public(PageHome), http.MethodGet, "/", nil).Body.String())
	if !strings.Contains(root, `<div class="lp-md lp-lede"><p>&lt;b&gt;hi&lt;/b&gt;</p></div>`) {
		t.Errorf("markdown fallback not escaped:\n%s", root)
	}
	if !strings.Contains(env.logs.String(), "markdown broke") {
		t.Error("markdown error must be logged")
	}
	// The default renderer is content.Markdown.
	def := fixtureEnv(t, nil, nil)
	if _, ok := def.rd.opts.Markdown.(fakeMarkdown); ok || def.rd.opts.Markdown == nil {
		t.Error("nil Markdown must default to content.NewMarkdown")
	}
}

func TestFallbackEscapesAdminText(t *testing.T) {
	env := fixtureEnv(t, fakeMarkdown{}, func(s *site.Snapshot) {
		c := s.Public.Communities["01920000-0000-7000-8000-000000000001"]
		c.Name = site.LocalizedText{"zh-CN": `</h3><script>alert(1)</script>`}
		s.Public.Communities[c.ID] = c
	})
	body := do(env.rd.Public(PageHome), http.MethodGet, "/", nil).Body.String()
	if strings.Contains(body, "<script>alert(1)") {
		t.Fatal("community name not escaped")
	}
	assertCSPMatchesInline(t, body, PublicCSP(env.rd.bootHash, env.rd.criticalHash, env.snap.ThemeHash, true))
}

func TestFallbackViewOnPrivacy(t *testing.T) {
	env := fixtureEnv(t, fakeMarkdown{}, nil)
	fv := env.rd.fallbackView(env.rd.resolve(PagePrivacy, "", "en", ""))
	want := fallbackView{
		Heading: "Privacy", Lede: "Our Discord, KOOK, QQ and WeChat communities.", Note: "Loading…",
		NoScript: uiText["en"].NoScript, LinkText: "Back to home →", LinkHref: "/?lang=en",
	}
	if !reflect.DeepEqual(fv, want) {
		t.Errorf("privacy view = %+v", fv)
	}
}
