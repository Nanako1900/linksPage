package golink

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Nanako1900/linksPage/internal/provider"
	"github.com/Nanako1900/linksPage/internal/uaclass"
)

// fakeSource is an in-memory Source.
type fakeSource struct {
	communities map[string]CommunityTarget
	links       map[string]LinkTarget
	commErr     error
	linkErr     error
	calls       int
}

func (f *fakeSource) Community(_ context.Context, slug string) (CommunityTarget, error) {
	f.calls++
	if f.commErr != nil {
		return CommunityTarget{}, f.commErr
	}
	c, ok := f.communities[slug]
	if !ok {
		return CommunityTarget{}, ErrNotFound
	}
	return c, nil
}

func (f *fakeSource) Link(_ context.Context, slug string) (LinkTarget, error) {
	if f.linkErr != nil {
		return LinkTarget{}, f.linkErr
	}
	l, ok := f.links[slug]
	if !ok {
		return LinkTarget{}, ErrNotFound
	}
	return l, nil
}

func discordTarget(slug string, join provider.JoinInput) CommunityTarget {
	join.Provider, join.Card = "discord", provider.CardDiscord
	return CommunityTarget{ID: "id-" + slug, Slug: slug, Platform: "discord", NeedsExternalBrowser: true, Join: join}
}

func staticTarget(slug, platform, card string, needsExternal bool, join provider.JoinInput) CommunityTarget {
	join.Provider, join.Card, join.State = "static", card, provider.StateStatic
	return CommunityTarget{ID: "id-" + slug, Slug: slug, Platform: platform, NeedsExternalBrowser: needsExternal, Join: join}
}

func testSource() *fakeSource {
	future := testNow.Add(time.Hour)
	return &fakeSource{
		communities: map[string]CommunityTarget{
			"discord":   discordTarget("discord", provider.JoinInput{State: provider.StateLive, InviteURL: "https://discord.gg/abc"}),
			"degraded":  discordTarget("degraded", provider.JoinInput{State: provider.StateDegraded, InviteURL: "https://discord.gg/dead", InviteInvalid: true, InstantInviteURL: "https://discord.com/invite/tmp", InstantInviteExpiresAt: &future}),
			"dead":      discordTarget("dead", provider.JoinInput{State: provider.StateDegraded, InviteURL: "https://discord.gg/dead", InviteInvalid: true}),
			"gone":      discordTarget("gone", provider.JoinInput{State: provider.StateUnavailable, InviteURL: "https://discord.gg/abc"}),
			"pending":   discordTarget("pending", provider.JoinInput{FallbackURL: "https://example.com/wait"}),
			"kook":      {ID: "id-kook", Slug: "kook", Platform: "kook", Join: provider.JoinInput{Provider: "kook", Card: provider.CardKOOK, State: provider.StateLive, InviteURL: "https://kook.top/abc"}},
			"qq-link":   staticTarget("qq-link", "qq-group", provider.CardQQGroup, false, provider.JoinInput{InviteURL: "https://qm.qq.com/q/abc"}),
			"qq-plain":  staticTarget("qq-plain", "qq-group", provider.CardQQGroup, false, provider.JoinInput{}),
			"wechat":    staticTarget("wechat", "wechat-group", provider.CardWeChatGroup, false, provider.JoinInput{FallbackURL: "https://example.com"}),
			"telegram":  staticTarget("telegram", "telegram", provider.CardStatic, true, provider.JoinInput{InviteURL: "https://t.me/abc"}),
			"bilibili":  staticTarget("bilibili", "bilibili", provider.CardStatic, false, provider.JoinInput{InviteURL: "https://space.bilibili.com/1"}),
			"no-target": staticTarget("no-target", "link", provider.CardStatic, false, provider.JoinInput{}),
		},
		links: map[string]LinkTarget{
			"blog":   {ID: "l1", Slug: "blog", URL: "https://blog.example.com"},
			"mail":   {ID: "l2", Slug: "mail", URL: "mailto:me@example.com"},
			"unsafe": {ID: "l3", Slug: "unsafe", URL: "javascript:alert(1)"},
		},
	}
}

var (
	uaDesktop = uaclass.Class{}
	uaMobile  = uaclass.Class{Mobile: true}
	uaWeChat  = uaclass.Class{InWeChat: true, Mobile: true}
	uaQQ      = uaclass.Class{InQQ: true, Mobile: true}
)

func TestResolveDecisionTable(t *testing.T) {
	tests := []struct {
		name   string
		slug   string
		ua     uaclass.Class
		action Action
		url    string
	}{
		{"invalid slug", "Bad_Slug", uaDesktop, ActionNotFound, ""},
		{"too long slug", "a234567890123456789012345678901234567890123456789012345678901234x", uaDesktop, ActionNotFound, ""},
		{"unknown slug", "nope", uaDesktop, ActionNotFound, ""},
		{"link desktop", "blog", uaDesktop, ActionRedirect, "https://blog.example.com"},
		{"link ignores wechat", "blog", uaWeChat, ActionRedirect, "https://blog.example.com"},
		{"mailto link", "mail", uaDesktop, ActionRedirect, "mailto:me@example.com"},
		{"unsafe link", "unsafe", uaDesktop, ActionNotFound, ""},
		{"wechat group desktop", "wechat", uaDesktop, ActionQRCode, ""},
		{"wechat group in qq", "wechat", uaQQ, ActionQRCode, ""},
		{"qq link in wechat", "qq-link", uaWeChat, ActionQQGroup, ""},
		{"qq link in qq", "qq-link", uaQQ, ActionRedirect, "https://qm.qq.com/q/abc"},
		{"qq link desktop", "qq-link", uaDesktop, ActionRedirect, "https://qm.qq.com/q/abc"},
		{"qq without link", "qq-plain", uaMobile, ActionQQGroup, ""},
		{"qq without link in wechat", "qq-plain", uaWeChat, ActionQQGroup, ""},
		{"discord desktop", "discord", uaDesktop, ActionRedirect, "https://discord.gg/abc"},
		{"discord mobile", "discord", uaMobile, ActionRedirect, "https://discord.gg/abc"},
		{"discord wechat", "discord", uaWeChat, ActionOpenInBrowser, ""},
		{"discord qq", "discord", uaQQ, ActionOpenInBrowser, ""},
		{"discord degraded uses instant", "degraded", uaDesktop, ActionRedirect, "https://discord.com/invite/tmp"},
		{"discord dead invite", "dead", uaDesktop, ActionUnavailable, ""},
		{"discord dead invite in wechat", "dead", uaWeChat, ActionUnavailable, ""},
		{"discord unavailable", "gone", uaMobile, ActionUnavailable, ""},
		{"discord pending fallback", "pending", uaDesktop, ActionRedirect, "https://example.com/wait"},
		{"kook in wechat redirects", "kook", uaWeChat, ActionRedirect, "https://kook.top/abc"},
		{"telegram in qq", "telegram", uaQQ, ActionOpenInBrowser, ""},
		{"telegram mobile", "telegram", uaMobile, ActionRedirect, "https://t.me/abc"},
		{"bilibili in wechat", "bilibili", uaWeChat, ActionRedirect, "https://space.bilibili.com/1"},
		{"static without target", "no-target", uaDesktop, ActionUnavailable, ""},
	}
	r := NewResolver(testSource(), func() time.Time { return testNow })
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, err := r.Resolve(context.Background(), tt.slug, tt.ua)
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			if d.Action != tt.action || d.URL != tt.url || d.Slug != tt.slug {
				t.Errorf("Resolve() = %+v, want action %q url %q", d, tt.action, tt.url)
			}
			isCommunity := d.Community != nil
			if isCommunity != (d.CommunityID != "") {
				t.Errorf("Community/CommunityID mismatch: %+v", d)
			}
		})
	}
}

func TestResolveSkipsSourceForInvalidSlug(t *testing.T) {
	src := testSource()
	r := NewResolver(src, nil)
	if _, err := r.Resolve(context.Background(), "../etc", uaDesktop); err != nil {
		t.Fatal(err)
	}
	if src.calls != 0 {
		t.Fatalf("source called %d times for an invalid slug", src.calls)
	}
}

func TestResolveErrors(t *testing.T) {
	boom := errors.New("db down")
	tests := []struct {
		name string
		src  *fakeSource
	}{
		{"community error", &fakeSource{commErr: boom}},
		{"link error", &fakeSource{linkErr: boom}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewResolver(tt.src, nil).Resolve(context.Background(), "x", uaDesktop)
			if !errors.Is(err, boom) {
				t.Fatalf("err = %v, want wrapped %v", err, boom)
			}
		})
	}
}

func TestNewResolverDefaultsClock(t *testing.T) {
	r := NewResolver(&fakeSource{}, nil)
	if r.now == nil || r.now().IsZero() {
		t.Fatal("default clock not set")
	}
}
