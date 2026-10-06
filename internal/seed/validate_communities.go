package seed

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/Nanako1900/linksPage/internal/content"
	"github.com/Nanako1900/linksPage/internal/provider"
	"github.com/Nanako1900/linksPage/internal/provider/discord"
	"github.com/Nanako1900/linksPage/internal/provider/kook"
	"github.com/Nanako1900/linksPage/internal/site"
	"github.com/Nanako1900/linksPage/internal/store/dbq"
)

var (
	slugRe    = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)
	qqGroupRe = regexp.MustCompile(`^\d{5,12}$`)
)

// minIntervals are the providers' MinInterval values.
var minIntervals = map[string]time.Duration{
	discord.ProviderKind: discord.WidgetInterval,
	kook.ProviderKind:    kook.BadgeInterval,
}

// slugSet tracks the shared /go/{slug} namespace of communities and links.
type slugSet map[string]string

func (s slugSet) claim(p *problems, field, slug string) bool {
	if !slugRe.MatchString(slug) {
		p.add(field, "slug %q must match %s", slug, slugRe)
		return false
	}
	if first, dup := s[slug]; dup {
		p.add(field, "slug %q is already used by %s", slug, first)
		return false
	}
	s[slug] = field
	return true
}

// communityCtx carries one community through validation.
type communityCtx struct {
	field    string
	seed     CommunitySeed
	platform provider.Platform
	provider string
	p        *problems
}

func planCommunities(cs []CommunitySeed, cat *provider.Catalog, slugs slugSet, p *problems, out *plan) {
	for i, c := range cs {
		field := fmt.Sprintf("communities[%d]", i)
		slugs.claim(p, field+".slug", c.Slug)
		platform, ok := cat.Get(c.Platform)
		if !ok {
			p.add(field+".platform", "unknown platform %q", c.Platform)
			continue
		}
		cc := communityCtx{field: field, seed: c, platform: platform, provider: providerOf(platform), p: p}
		out.communities = append(out.communities, cc.plan())
	}
}

func providerOf(p provider.Platform) string {
	if p.Provider == "" {
		return "static"
	}
	return p.Provider
}

func (cc communityCtx) plan() communityPlan {
	c := cc.seed
	params := dbq.InsertCommunityParams{
		PageID: 1, Slug: c.Slug, Provider: cc.provider, Platform: cc.platform.ID,
		ExternalID: cc.externalID(), Config: []byte(`{}`), RefreshInterval: cc.interval(),
		InviteUrl: cc.url("invite", c.Invite, true), FallbackUrl: cc.url("fallback_url", c.FallbackURL, false),
	}
	display, err := json.Marshal(cc.display())
	if err != nil {
		cc.p.add(cc.field, "%v", err)
	}
	params.Display = display
	cp := communityPlan{params: params, icon: cc.p.imagePath(cc.field+".icon", c.Icon), fetch: cc.provider != "static"}
	cc.qr(&cp)
	return cp
}

func (cc communityCtx) externalID() *string {
	id := cc.seed.GuildID
	field := cc.field + ".guild_id"
	switch cc.provider {
	case "discord":
		if !discord.ValidGuildID(id) {
			cc.p.add(field, "must be a Discord server id (17–20 digits, quoted)")
		}
	case "kook":
		if !kook.ValidGuildID(id) {
			cc.p.add(field, "must be a KOOK server id (1–20 digits, quoted)")
		}
	default:
		if id != "" {
			cc.p.add(field, "only discord and kook communities have a server id")
		}
		return nil
	}
	return &id
}

// url validates an invite (https only) or fallback URL (http/https)
// against content.SafeURL and the platform's url_pattern.
func (cc communityCtx) url(key, raw string, httpsOnly bool) *string {
	if raw == "" {
		return nil
	}
	field := cc.field + "." + key
	if cc.platform.Card == provider.CardWeChatGroup {
		cc.p.add(field, "wechat-group communities have no join link; use qr and contact")
		return nil
	}
	safe, err := content.SafeURL(raw)
	if err != nil || (httpsOnly && !strings.HasPrefix(safe, "https://")) || strings.HasPrefix(safe, "mailto:") {
		want := "an http(s) URL"
		if httpsOnly {
			want = "an https URL"
		}
		cc.p.add(field, "must be %s", want)
		return nil
	}
	if !cc.platform.MatchURL(safe) {
		cc.p.add(field, "%q does not match the %s link format", raw, cc.platform.ID)
		return nil
	}
	return &safe
}

func (cc communityCtx) interval() pgtype.Interval {
	d := cc.seed.RefreshInterval
	if d == 0 {
		return pgtype.Interval{}
	}
	minimum, ok := minIntervals[cc.provider]
	switch {
	case !ok:
		cc.p.add(cc.field+".refresh_interval", "only discord and kook communities are refreshed")
	case d < minimum:
		cc.p.add(cc.field+".refresh_interval", "must be at least %s", minimum)
	}
	return pgtype.Interval{Microseconds: d.Microseconds(), Valid: true}
}

// display builds communities.display.
func (cc communityCtx) display() site.CommunityDisplay {
	c, f, p := cc.seed, cc.field, cc.p
	d := site.CommunityDisplay{
		Name:            p.text(f+".name", c.Name, MaxNameRunes, true),
		Description:     p.text(f+".description", c.Description, MaxPlainTextRunes, false),
		MemberDisplay:   site.MemberDisplay(c.Members),
		NameBlocklist:   cc.blocklist(),
		ShowChannels:    c.ShowChannels,
		ShowOnline:      c.ShowOnline,
		MemberLimit:     c.MemberLimit,
		Embed:           c.Embed,
		UnavailableText: p.text(f+".unavailable_text", c.UnavailableText, MaxPlainTextRunes, false),
	}
	switch d.MemberDisplay {
	case "", site.MemberHidden, site.MemberAvatars, site.MemberAvatarsNames:
	default:
		p.add(f+".members", "must be hidden, avatars or avatars_names")
	}
	if c.MemberLimit < 0 || c.MemberLimit > site.MaxMemberLimit {
		p.add(f+".member_limit", "must be between 0 and %d", site.MaxMemberLimit)
	}
	if c.Embed && cc.platform.Card != provider.CardDiscord {
		p.add(f+".embed", "is only available for discord")
	}
	d.QQGroupNumber = cc.qqGroup()
	d.Contact = cc.contact()
	if len(d.Description) == 0 {
		d.Description = nil
	}
	if len(d.UnavailableText) == 0 {
		d.UnavailableText = nil
	}
	return d
}

func (cc communityCtx) blocklist() []string {
	list := cc.seed.NameBlocklist
	if len(list) > MaxBlocklistItems {
		cc.p.add(cc.field+".name_blocklist", "at most %d entries", MaxBlocklistItems)
	}
	var out []string
	for i, w := range list {
		w = content.CleanText(w, 0)
		if w == "" || utf8.RuneCountInString(w) > MaxBlocklistRunes {
			cc.p.add(fmt.Sprintf("%s.name_blocklist[%d]", cc.field, i), "must be 1–%d characters", MaxBlocklistRunes)
			continue
		}
		out = append(out, w)
	}
	return out
}

func (cc communityCtx) qqGroup() string {
	n := cc.seed.QQGroup
	isQQ := cc.platform.Card == provider.CardQQGroup
	switch {
	case isQQ && !qqGroupRe.MatchString(n):
		cc.p.add(cc.field+".qq_group", "is required for qq-group: 5–12 digits, quoted")
	case !isQQ && n != "":
		cc.p.add(cc.field+".qq_group", "is only valid for the qq-group platform")
	}
	return n
}

func (cc communityCtx) contact() *site.ContactView {
	c := cc.seed.Contact
	if c == nil {
		return nil
	}
	value := content.CleanText(c.Value, 0)
	if value == "" || utf8.RuneCountInString(value) > MaxContactRunes {
		cc.p.add(cc.field+".contact.value", "must be 1–%d characters", MaxContactRunes)
	}
	label := cc.p.text(cc.field+".contact.label", c.Label, MaxNameRunes, true)
	return &site.ContactView{Label: label, Value: value}
}

// qr validates the QR code: required for wechat-group, allowed for
// qq-group and static cards.
func (cc communityCtx) qr(cp *communityPlan) {
	card := cc.platform.Card
	q := cc.seed.QR
	field := cc.field + ".qr"
	switch {
	case q == nil && card == provider.CardWeChatGroup:
		cc.p.add(field, "is required for wechat-group (qr.image)")
		return
	case q == nil:
		return
	case card == provider.CardDiscord || card == provider.CardKOOK:
		cc.p.add(field, "is only available for qq-group, wechat-group and static platforms")
		return
	case q.Image == "":
		cc.p.add(field+".image", "is required")
		return
	}
	cp.qrImage = cc.p.imagePath(field+".image", q.Image)
	cp.qrNote = cc.p.text(field+".note", q.Note, MaxPlainTextRunes, false)
}
