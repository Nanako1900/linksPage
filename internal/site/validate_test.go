package site

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/Nanako1900/linksPage/internal/media"
	"github.com/Nanako1900/linksPage/internal/provider"
)

const (
	discordID = "01920000-0000-7000-8000-000000000001"
	staleID   = "01920000-0000-7000-8000-000000000002"
	qqID      = "01920000-0000-7000-8000-000000000005"
	wechatID  = "01920000-0000-7000-8000-000000000007"
	blogID    = "01920000-0000-7000-8000-000000000201"
	mastoID   = "01920000-0000-7000-8000-000000000203"
)

func ptr[T any](v T) *T { return &v }

// mutateCommunity returns a mutation that edits one community.
func mutateCommunity(id string, f func(c *CommunityView)) func(p *PublicPage) {
	return func(p *PublicPage) {
		c := p.Communities[id]
		f(&c)
		p.Communities[id] = c
	}
}

func mutateLink(id string, f func(l *LinkView)) func(p *PublicPage) {
	return func(p *PublicPage) {
		l := p.Links[id]
		f(&l)
		p.Links[id] = l
	}
}

func TestPublicPageValidateRejects(t *testing.T) {
	cases := []struct {
		name, want string
		mutate     func(p *PublicPage)
	}{
		{"null blocks", "must not be null", func(p *PublicPage) { p.Blocks = nil }},
		{"page slug", "page", func(p *PublicPage) { p.Page.Slug = "Bad" }},
		{"base url", "baseUrl", func(p *PublicPage) { p.Site.BaseURL = "https://x/" }},
		{"avatar", "site.avatar", func(p *PublicPage) { p.Site.Avatar.URL = "/media/p/x.png" }},
		{"site null text", "site.bio", func(p *PublicPage) { p.Site.Bio = nil }},
		{"site appearance", "appearance", func(p *PublicPage) { p.Site.Appearance = "neon" }},
		{"block id", "invalid id", func(p *PublicPage) { p.Blocks[0].ID = "x" }},
		{"block kind", "unknown kind", func(p *PublicPage) { p.Blocks[0].Kind = "video" }},
		{"block extra field", "not allowed", func(p *PublicPage) { p.Blocks[1].LinkID = blogID }},
		{"block community ref", "communityId", func(p *PublicPage) { p.Blocks[1].CommunityID = blogID }},
		{"block link ref", "linkId", func(p *PublicPage) { p.Blocks[10].LinkID = discordID }},
		{"heading count", "heading", func(p *PublicPage) { p.Blocks[0].Count = ptr(-1) }},
		{"text empty", "markdown", func(p *PublicPage) { p.Blocks[11].Markdown = LocalizedText{} }},
		{"social empty", "linkIds", func(p *PublicPage) { p.Blocks[12].LinkIDs = []string{} }},
		{"social ref", "linkIds entry", func(p *PublicPage) { p.Blocks[12].LinkIDs = []string{discordID} }},
		{"community key", "invalid id", mutateCommunity(discordID, func(c *CommunityView) { c.ID = staleID })},
		{"provider", "unknown provider", mutateCommunity(discordID, func(c *CommunityView) { c.Provider = "slack" })},
		{"card", "invalid card", mutateCommunity(discordID, func(c *CommunityView) { c.Card = "x" })},
		{"name", "name is required", mutateCommunity(discordID, func(c *CommunityView) { c.Name = LocalizedText{} })},
		{"share path", "sharePath", mutateCommunity(discordID, func(c *CommunityView) { c.SharePath = "/c/x" })},
		{"invite http", "inviteUrl", mutateCommunity(discordID, func(c *CommunityView) { c.InviteURL = ptr("http://discord.gg/x") })},
		{"platform", "platform", mutateCommunity(discordID, func(c *CommunityView) { c.Platform = "slack" })},
		{"icon", "icon", mutateCommunity(discordID, func(c *CommunityView) { c.Icon = &ImageView{URL: "/x.png", Width: 1, Height: 1} })},
		{"embed", "embed", mutateCommunity(qqID, func(c *CommunityView) { c.Embed = &EmbedView{Kind: "discord", Src: "x"} })},
		{"qq missing", "qq must", mutateCommunity(qqID, func(c *CommunityView) { c.QQ = nil })},
		{"qq number", "qq must", mutateCommunity(qqID, func(c *CommunityView) { c.QQ = &QQView{GroupNumber: "12"} })},
		{"wechat qr", "need a qr", mutateCommunity(wechatID, func(c *CommunityView) { c.QR = nil })},
		{"qr url", "qr must", mutateCommunity(wechatID, func(c *CommunityView) { c.QR.URL = "/media/u/x" })},
		{"contact", "contact", mutateCommunity(wechatID, func(c *CommunityView) { c.Contact.Value = "" })},
		{"state", "live.state", mutateCommunity(discordID, func(c *CommunityView) { c.Live.State = "ok" })},
		{"null users", "must not be null", mutateCommunity(discordID, func(c *CommunityView) { c.Live.Users = nil })},
		{"online source", "onlineSource", mutateCommunity(discordID, func(c *CommunityView) { c.Live.OnlineSource = nil })},
		{"online source value", "onlineSource unknown", mutateCommunity(discordID, func(c *CommunityView) { c.Live.OnlineSource = ptr("api") })},
		{"join url", "joinUrl must be /go", mutateCommunity(discordID, func(c *CommunityView) { c.Live.JoinURL = ptr("https://discord.gg/x") })},
		{"join unavailable", "must be null", mutateCommunity(wechatID, func(c *CommunityView) { c.Live.JoinURL = ptr("/go/wechat") })},
		{"hidden users", "hidden", mutateCommunity(discordID, func(c *CommunityView) { c.MemberDisplay = MemberHidden })},
		{"avatars names", "null in avatars", mutateCommunity(discordID, func(c *CommunityView) { c.MemberDisplay = MemberAvatars })},
		{"names required", "required", mutateCommunity(staleID, func(c *CommunityView) { c.MemberDisplay = MemberAvatarsNames })},
		{"user status", "status", mutateCommunity(discordID, func(c *CommunityView) { c.Live.Users[0].Status = "away" })},
		{"user avatar", "avatarUrl", mutateCommunity(discordID, func(c *CommunityView) { c.Live.Users[0].AvatarURL = ptr("https://cdn/x") })},
		{"link key", "invalid id", mutateLink(blogID, func(l *LinkView) { l.ID = mastoID })},
		{"link kind", "kind", mutateLink(blogID, func(l *LinkView) { l.Kind = "video" })},
		{"link url", "allowed url", mutateLink(blogID, func(l *LinkView) { l.URL = "javascript:alert(1)" })},
		{"link href", "href", mutateLink(blogID, func(l *LinkView) { l.Href = l.URL })},
		{"rel me href", "relMe", mutateLink(mastoID, func(l *LinkView) { l.Href = "/go/mastodon" })},
		{"icon kind", "icon kind", mutateLink(blogID, func(l *LinkView) { l.Icon = &IconView{Kind: "emoji", Name: "x"} })},
		{"simple icon url", "null url", mutateLink(blogID, func(l *LinkView) { l.Icon.URL = ptr("/x") })},
		{"media icon", "media icon", mutateLink(mastoID, func(l *LinkView) { l.Icon.Name = "other.png" })},
		{"platform icon", "platforms", func(p *PublicPage) {
			pl := p.Platforms["kook"]
			pl.Name = nil
			p.Platforms["kook"] = pl
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, p := loadFixture(t)
			tc.mutate(p)
			err := p.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestSettingsM1Validation(t *testing.T) {
	long := strings.Repeat("字", MaxPlainTextRunes+1)
	cases := []struct{ name, in, want string }{
		{"display name long", `{"displayName":{"en":"` + long + `"}}`, "displayName.en"},
		{"bio locale", `{"bio":{"EN_us":"x"}}`, "bio: invalid locale"},
		{"bio too long", `{"bio":{"en":"` + strings.Repeat("a", MaxMarkdownTextBytes+1) + `"}}`, "bio.en"},
		{"avatar key", `{"avatarKey":"../x.png"}`, "avatarKey"},
		{"og image", `{"og":{"imageKey":"x"}}`, "og.imageKey"},
		{"indexing", `{"searchIndexing":"maybe"}`, "searchIndexing"},
		{"copy locale", `{"copy":{"EN":{"openInBrowser":"x"}}}`, "copy: invalid locale"},
		{"copy key", `{"copy":{"en":{"title":"x"}}}`, "unknown key"},
		{"copy long", `{"copy":{"en":{"openOnDesktop":"` + long + `"}}}`, "copy.en.openOnDesktop"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseSettings([]byte(tc.in))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
	s, err := ParseSettings([]byte(`{"avatarKey":"0123456789abcdef0123456789abcdef.webp","searchIndexing":"noindex",` +
		`"copy":{"zh-CN":{"openInBrowser":"打开"}},"og":{"title":{"en":"OG"}},"showPoweredBy":false}`))
	if err != nil || s.SearchIndexing != SearchNoIndex || s.ShowPoweredBy {
		t.Fatalf("settings = %+v err=%v", s, err)
	}
	c := s.Clone()
	c.Copy["zh-CN"]["openInBrowser"] = "mutated"
	c.OG.Title["en"] = "mutated"
	if s.Copy["zh-CN"]["openInBrowser"] != "打开" || s.OG.Title["en"] != "OG" {
		t.Error("Clone must deep-copy copy and og")
	}
	if s.Copy.Get("ja", "zh-CN", CopyOpenInBrowser) != "打开" || s.Copy.Get("en", "en", CopyOpenOnDesktop) != "" {
		t.Error("CopyOverrides.Get fallback wrong")
	}
	if len(CopyKeys()) != 4 {
		t.Error("copy keys changed; update docs/m1/contract.md and the frontend")
	}
}

type fakeAssets struct{}

func (fakeAssets) Generate(context.Context, media.GenerateInput) (media.Generated, error) {
	return media.Generated{}, nil
}

func TestBuilderSkeleton(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if _, err := NewBuilder(BuilderDeps{}); err == nil {
		t.Error("missing deps should fail")
	}
	cat := &provider.Catalog{}
	b, err := NewBuilder(BuilderDeps{Queries: nil, Live: provider.NewLiveStore(), Platforms: cat, Assets: fakeAssets{}, Logger: logger})
	if err == nil || b != nil {
		t.Error("nil Queries should fail")
	}
	if err := DecodeStrict([]byte(`{"name":{"en":"x"},"bogus":1}`), &CommunityDisplay{}); err == nil {
		t.Error("unknown display key accepted")
	}
	var d CommunityDisplay
	if err := DecodeStrict([]byte(`{"name":{"en":"x"},"qqGroupNumber":"12345"}`), &d); err != nil || d.QQGroupNumber != "12345" {
		t.Errorf("display = %+v err=%v", d, err)
	}
}
