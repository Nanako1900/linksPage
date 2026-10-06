package seed

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Nanako1900/linksPage/internal/content"
	"github.com/Nanako1900/linksPage/internal/provider"
	"github.com/Nanako1900/linksPage/internal/site"
)

func loadPresets(t *testing.T) *provider.Catalog {
	t.Helper()
	c, err := provider.LoadPresets()
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func planYAML(t *testing.T, doc string) (*plan, error) {
	t.Helper()
	f, err := Parse([]byte(doc))
	if err != nil {
		return nil, err
	}
	return planFile(f, loadPresets(t), content.NewMarkdown())
}

const minimalDoc = `version: 1
communities:
  - slug: discord
    platform: discord
    name: { en: Discord }
    guild_id: "1114391825336250432"
`

func TestPlanValidationErrors(t *testing.T) {
	cases := []struct{ name, doc, want string }{
		{"site appearance", "version: 1\nsite: { appearance: neon }\n", "site: appearance"},
		{"site locale", "version: 1\nsite: { default_locale: fr, locales: [zh-CN] }\n", "defaultLocale"},
		{"site title locale", "version: 1\nsite: { title: { 'x y': a } }\n", `site.title: invalid locale "x y"`},
		{"site title long", "version: 1\nsite: { title: { en: " + strings.Repeat("a", 301) + " } }\n", "site.title.en: longer than 300"},
		{"site bio long", "version: 1\nsite: { bio: { en: " + strings.Repeat("a", 4097) + " } }\n", "site.bio.en"},
		{"site copy key", "version: 1\nsite: { copy: { en: { bogus: x } } }\n", `unknown key "bogus"`},
		{"site avatar escape", "version: 1\nsite: { avatar: ../x.png }\n", "site.avatar: image path"},
		{"site avatar absolute", "version: 1\nsite: { avatar: /etc/x.png }\n", "site.avatar: image path"},
		{"site og svg", "version: 1\nsite: { og: { image: og.svg } }\n", "site.og.image: image"},
		{"platform preset id", "version: 1\nplatforms: [{ id: discord, name: { en: D } }]\n", `platforms[0].id: "discord" is a built-in`},
		{"platform bad id", "version: 1\nplatforms: [{ id: X, name: { en: D } }]\n", "platforms[0].id: must match"},
		{"platform dup", "version: 1\nplatforms: [{ id: hb, name: { en: A } }, { id: hb, name: { en: B } }]\n", "platforms[1].id: duplicate"},
		{"platform name", "version: 1\nplatforms: [{ id: hb }]\n", "platforms[0].name: is required"},
		{"platform pattern", "version: 1\nplatforms: [{ id: hb, name: { en: A }, url_pattern: '(' }]\n", "platforms[0].url_pattern"},
		{"slug", "version: 1\ncommunities: [{ slug: Bad, platform: link, name: { en: x } }]\n", "communities[0].slug"},
		{"slug shared", "version: 1\ncommunities: [{ slug: a, platform: link, name: { en: x } }]\nlinks: [{ slug: a, label: { en: x }, url: 'https://a.example' }]\n", "links[0].slug: slug \"a\" is already used by communities[0].slug"},
		{"unknown platform", "version: 1\ncommunities: [{ slug: a, platform: nope, name: { en: x } }]\n", `communities[0].platform: unknown platform "nope"`},
		{"name required", "version: 1\ncommunities: [{ slug: a, platform: link }]\n", "communities[0].name: is required"},
		{"discord guild", "version: 1\ncommunities: [{ slug: a, platform: discord, name: { en: x }, guild_id: '12' }]\n", "communities[0].guild_id: must be a Discord"},
		{"kook guild", "version: 1\ncommunities: [{ slug: a, platform: kook, name: { en: x } }]\n", "communities[0].guild_id: must be a KOOK"},
		{"static guild", "version: 1\ncommunities: [{ slug: a, platform: link, name: { en: x }, guild_id: '1' }]\n", "communities[0].guild_id: only discord"},
		{"invite pattern", minimalDoc + "    invite: https://evil.example/x\n", "communities[0].invite: \"https://evil.example/x\" does not match"},
		{"invite http", minimalDoc + "    invite: http://discord.gg/abc\n", "communities[0].invite: must be an https URL"},
		{"fallback scheme", minimalDoc + "    fallback_url: javascript:x\n", "communities[0].fallback_url: must be an http(s) URL"},
		{"wechat invite", "version: 1\ncommunities: [{ slug: w, platform: wechat-group, name: { en: x }, invite: 'https://a.example', qr: { image: q.png } }]\n", "wechat-group communities have no join link"},
		{"wechat qr", "version: 1\ncommunities: [{ slug: w, platform: wechat-group, name: { en: x } }]\n", "communities[0].qr: is required"},
		{"qr image", "version: 1\ncommunities: [{ slug: w, platform: wechat-group, name: { en: x }, qr: { note: { en: x } } }]\n", "communities[0].qr.image: is required"},
		{"qr on discord", minimalDoc + "    qr: { image: q.png }\n", "communities[0].qr: is only available"},
		{"qq group", "version: 1\ncommunities: [{ slug: q, platform: qq-group, name: { en: x }, qq_group: '12' }]\n", "communities[0].qq_group: is required"},
		{"qq on discord", minimalDoc + "    qq_group: '123456'\n", "communities[0].qq_group: is only valid"},
		{"members", minimalDoc + "    members: everyone\n", "communities[0].members"},
		{"member limit", minimalDoc + "    member_limit: 101\n", "communities[0].member_limit"},
		{"embed", "version: 1\ncommunities: [{ slug: k, platform: kook, name: { en: x }, guild_id: '1', embed: true }]\n", "communities[0].embed"},
		{"blocklist entry", minimalDoc + "    name_blocklist: ['']\n", "communities[0].name_blocklist[0]"},
		{"blocklist size", minimalDoc + "    name_blocklist: [" + strings.Repeat("a, ", 51) + "]\n", "communities[0].name_blocklist: at most"},
		{"contact", "version: 1\ncommunities: [{ slug: w, platform: link, name: { en: x }, contact: { label: { en: x } } }]\n", "communities[0].contact.value"},
		{"contact label", "version: 1\ncommunities: [{ slug: w, platform: link, name: { en: x }, contact: { value: me } }]\n", "communities[0].contact.label: is required"},
		{"interval static", "version: 1\ncommunities: [{ slug: w, platform: link, name: { en: x }, refresh_interval: 10m }]\n", "only discord and kook communities are refreshed"},
		{"interval short", minimalDoc + "    refresh_interval: 1m\n", "must be at least 5m0s"},
		{"link url", "version: 1\nlinks: [{ slug: a, label: { en: x }, url: 'javascript:x' }]\n", "links[0].url"},
		{"link kind", "version: 1\nlinks: [{ slug: a, kind: video, label: { en: x }, url: 'https://a.example' }]\n", "links[0].kind"},
		{"link label", "version: 1\nlinks: [{ slug: a, url: 'https://a.example' }]\n", "links[0].label: is required"},
		{"rel me mailto", "version: 1\nlinks: [{ slug: a, label: { en: x }, url: 'mailto:a@b.c', rel_me: true }]\n", "links[0].rel_me"},
		{"block two keys", minimalDoc + "blocks: [{ community: discord, link: x }]\n", "blocks[0]: needs exactly one"},
		{"block none", minimalDoc + "blocks: [{ visible: true }]\n", "blocks[0]: needs exactly one"},
		{"block community", minimalDoc + "blocks: [{ community: nope }]\n", `blocks[0].community: unknown community "nope"`},
		{"block link", minimalDoc + "blocks: [{ link: nope }]\n", `blocks[0].link: unknown link "nope"`},
		{"block social", minimalDoc + "blocks: [{ social_row: [nope] }]\n", `blocks[0].social_row[0]: unknown link "nope"`},
		{"block social empty", minimalDoc + "blocks: [{ social_row: [] }]\n", "blocks[0].social_row: needs at least one"},
		{"block show_count", minimalDoc + "blocks: [{ community: discord, show_count: true }]\n", "blocks[0].show_count"},
		{"block heading", minimalDoc + "blocks: [{ heading: {} }]\n", "blocks[0].heading: is required"},
		{"block text", minimalDoc + "blocks: [{ text: { en: '' } }]\n", "blocks[0].text: is required"},
		{"block window", minimalDoc + "blocks: [{ community: discord, visible_from: 2026-10-10T00:00:00Z, visible_to: 2026-10-09T00:00:00Z }]\n", "blocks[0].visible_to"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := planYAML(t, tc.doc)
			if err == nil || !errors.Is(err, ErrInvalidSeed) || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v\nwant %q wrapping ErrInvalidSeed", err, tc.want)
			}
		})
	}
}

func TestPlanReportsAllProblems(t *testing.T) {
	_, err := planYAML(t, "version: 1\ncommunities: [{ slug: Bad, platform: link }, { slug: b, platform: nope }]\n")
	if err == nil || strings.Count(err.Error(), "\n") < 2 {
		t.Fatalf("want several problems, got %v", err)
	}
}

func TestPlanExample(t *testing.T) {
	f := parseExample(t)
	pl, err := planFile(f, loadPresets(t), content.NewMarkdown())
	if err != nil {
		t.Fatalf("seed.example.yaml: %v", err)
	}
	if len(pl.communities) != 4 || len(pl.links) != 3 || len(pl.blocks) != 9 {
		t.Fatalf("plan counts %d/%d/%d", len(pl.communities), len(pl.links), len(pl.blocks))
	}
	imgs := pl.images()
	if len(imgs) != 1 || imgs[0].field != "communities[3].qr.image" {
		t.Errorf("images = %+v", imgs)
	}
	kook := pl.communities[1].params
	if kook.Provider != "kook" || *kook.ExternalID != "5417470909511807" || kook.RefreshInterval.Microseconds != 600e6 {
		t.Errorf("kook = %+v", kook)
	}
	var d site.CommunityDisplay
	if err := site.DecodeStrict(pl.communities[0].params.Display, &d); err != nil || d.Name["zh-CN"] != "Discord 主服务器" || d.NameBlocklist[0] != "spam" {
		t.Errorf("discord display = %+v err=%v", d, err)
	}
	if pl.communities[2].params.Provider != "static" || pl.communities[2].params.ExternalID != nil || pl.communities[2].fetch {
		t.Error("qq group must be static without a guild id")
	}
	if pl.settings["copy"] == nil || pl.settings["appearance"] != "auto" {
		t.Errorf("settings = %v", pl.settings)
	}
}

func TestPlanCleansText(t *testing.T) {
	pl, err := planYAML(t, "version: 1\ncommunities: [{ slug: a, platform: link, name: { en: \"A\u202eB\u200b \" }, contact: { label: { en: L }, value: \" id\u2066 \" } }]\n")
	if err != nil {
		t.Fatal(err)
	}
	var d site.CommunityDisplay
	if err := json.Unmarshal(pl.communities[0].params.Display, &d); err != nil {
		t.Fatal(err)
	}
	if d.Name["en"] != "AB" || d.Contact.Value != "id" || d.Description != nil {
		t.Errorf("display = %+v", d)
	}
}

func TestDefaultBlocks(t *testing.T) {
	pl, err := planYAML(t, `version: 1
communities:
  - { slug: a, platform: link, name: { en: A } }
  - { slug: b, platform: telegram, name: { en: B }, invite: "https://t.me/example" }
links:
  - { slug: blog, label: { en: Blog }, url: "https://a.example" }
  - { slug: gh, kind: social, label: { en: GH }, url: "https://github.com/x", icon: "si:github" }
  - { slug: x, kind: social, label: { en: X }, url: "https://x.com/x", icon: "images/x.png" }
`)
	if err != nil {
		t.Fatal(err)
	}
	var kinds []string
	for _, b := range pl.blocks {
		kinds = append(kinds, string(b.kind))
	}
	want := "heading,community,community,link,social_row"
	if strings.Join(kinds, ",") != want || !pl.blocks[0].heading.ShowCount || len(pl.blocks[4].social) != 2 {
		t.Errorf("default blocks = %v, want %s", kinds, want)
	}
	if pl.links[2].icon != "images/x.png" || pl.links[1].params.Icon == nil || *pl.links[1].params.Icon != "si:github" {
		t.Errorf("link icons = %+v", pl.links)
	}
	noSocial, err := planYAML(t, "version: 1\n")
	if err != nil || len(noSocial.blocks) != 1 {
		t.Errorf("empty seed blocks = %+v err=%v", noSocial, err)
	}
}

func TestPlanCustomPlatform(t *testing.T) {
	pl, err := planYAML(t, `version: 1
platforms:
  - { id: heybox, name: { zh-CN: 黑盒语音 }, icon: "images/hb.webp", url_pattern: '^https://heybox\.example/' }
  - { id: yy, name: { en: YY }, icon: "si:yy" }
communities:
  - { slug: hb, platform: heybox, name: { en: HB }, invite: "https://heybox.example/join", qr: { image: images/hb-qr.jpg } }
`)
	if err != nil {
		t.Fatal(err)
	}
	if len(pl.platforms) != 2 || pl.platforms[0].icon != "images/hb.webp" || *pl.platforms[1].params.Icon != "si:yy" {
		t.Errorf("platforms = %+v", pl.platforms)
	}
	if *pl.communities[0].params.InviteUrl != "https://heybox.example/join" || pl.communities[0].qrImage != "images/hb-qr.jpg" {
		t.Errorf("community = %+v", pl.communities[0])
	}
	if len(pl.images()) != 2 {
		t.Errorf("images = %+v", pl.images())
	}
}
