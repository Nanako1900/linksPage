package site

import (
	"fmt"
	"testing"
	"time"

	"github.com/Nanako1900/linksPage/internal/provider"
	"github.com/Nanako1900/linksPage/internal/store/dbq"
)

// Test ids (c = community, l = link, b = block, q = QR code).
func cid(n int) string { return fmt.Sprintf("01920000-0000-7000-8000-%012d", n) }
func lid(n int) string { return fmt.Sprintf("01920000-0000-7000-8000-%012d", 200+n) }
func bid(n int) string { return fmt.Sprintf("01920000-0000-7000-8000-%012d", 100+n) }
func qid(n int) string { return fmt.Sprintf("01920000-0000-7000-8000-%012d", 300+n) }

const (
	iconKey   = "0123456789abcdef0123456789abcdef.webp"
	avatarKey = "fedcba9876543210fedcba9876543210.webp"
	mastoKey  = "00112233445566778899aabbccddeeff.png"
	heyboxKey = "ffeeddccbbaa99887766554433221100.webp"
	proxyIcon = "/media/p/Q2xvdWRmbGFyZUljb24xMg.png"
	proxyAva  = "/media/p/QXZhdGFyMDAwMDAwMDAwMQ.png"
)

func intp(v int) *int { return &v }

func community(t *testing.T, n int, slug, prov, platform string, ext *string, display string) dbq.Community {
	t.Helper()
	return dbq.Community{
		ID: uuidOf(t, cid(n)), PageID: 1, Slug: slug, Provider: prov, Platform: platform,
		ExternalID: ext, Config: []byte(`{}`), Display: []byte(display),
	}
}

func snapRow(t *testing.T, n int, state provider.State, data provider.Snapshot, fetched, lastOK time.Time) dbq.ProviderSnapshot {
	t.Helper()
	row := dbq.ProviderSnapshot{
		CommunityID: uuidOf(t, cid(n)), Data: mustJSON(t, data), State: string(state),
		FetchedAt: ts(fetched), NextFetchAt: ts(fetched.Add(5 * time.Minute)),
	}
	if !lastOK.IsZero() {
		row.LastOkAt = ts(lastOK)
	}
	return row
}

func block(t *testing.T, n int, kind BlockKind, ref string, data string) dbq.ListVisibleBlocksRow {
	t.Helper()
	b := dbq.ListVisibleBlocksRow{ID: uuidOf(t, bid(n)), PageID: 1, Kind: string(kind), Data: []byte(data), SortOrder: int32(n), Visible: true}
	switch kind {
	case BlockCommunity:
		b.CommunityID = uuidOf(t, ref)
	case BlockLink:
		b.LinkID = uuidOf(t, ref)
	}
	return b
}

func link(t *testing.T, n int, slug, kind, url string, icon, key *string, relMe bool) dbq.Link {
	t.Helper()
	return dbq.Link{
		ID: uuidOf(t, lid(n)), PageID: 1, Slug: slug, Kind: kind,
		Label: []byte(`{"en":"` + slug + `"}`), Url: url, Icon: icon, IconKey: key, RelMe: relMe,
	}
}

// fullFake builds a page exercising every card state, block kind and the
// skip paths (invalid community, unknown platform, unsafe link, empty text).
func fullFake(t *testing.T, now time.Time) (*buildFake, fakeLive) {
	t.Helper()
	guild := strp("1114391825336250432")
	kookGuild := strp("5417470909511807")
	earlier := now.Add(-10 * time.Minute)
	f := &buildFake{
		settings: dbq.SiteSetting{Version: 4, Data: []byte(`{"avatarKey":"` + avatarKey + `","og":{"title":{"en":"OG"},"description":{},"imageKey":""}}`)},
		next:     ts(now.Add(48 * time.Hour)),
		media: []dbq.Medium{
			{Key: iconKey, Kind: "icon", Width: 128, Height: 128},
			{Key: avatarKey, Kind: "avatar", Width: 256, Height: 256},
			{Key: mastoKey, Kind: "icon", Width: 64, Height: 64},
		},
		platforms: []dbq.CustomPlatform{
			{ID: "heybox", Name: []byte(`{"en":"Heybox","zh-CN":"黑盒语音"}`), IconKey: strp(heyboxKey)},
			{ID: "broken", Name: []byte(`not json`)},
		},
	}
	c1 := community(t, 1, "discord", "discord", "discord", guild, `{"name":{"en":"Main"},"embed":true,"nameBlocklist":["spam"],"memberLimit":3}`)
	c1.InviteUrl = strp("https://discord.gg/KwdRuAkT")
	c2 := community(t, 2, "discord-eu", "discord", "discord", strp("11143918253362504"), `{"name":{"en":"EU"},"memberDisplay":"avatars","showChannels":false}`)
	c2.InviteUrl = strp("https://discord.gg/euLodge")
	c3 := community(t, 3, "discord-old", "discord", "discord", strp("111439182533625043"), `{"name":{"en":"Old"},"memberDisplay":"hidden","showOnline":false}`)
	c3.InviteUrl = strp("https://discord.gg/oldOne")
	c4 := community(t, 4, "kook", "kook", "kook", kookGuild, `{"name":{"en":"KOOK"}}`)
	c4.IconKey = strp(iconKey)
	c4.InviteUrl = strp("https://kook.top/AbCdEf")
	c5 := community(t, 5, "qq-fans", "static", "qq-group", nil, `{"name":{"en":"QQ"},"qqGroupNumber":"123456789"}`)
	c5.InviteUrl = strp("https://qm.qq.com/q/AbCdEf")
	c6 := community(t, 6, "qq-chat", "static", "qq-group", nil, `{"name":{"en":"QQ chat"},"qqGroupNumber":"87654321"}`)
	c7 := community(t, 7, "wechat", "static", "wechat-group", nil, `{"name":{"en":"WeChat"},"contact":{"label":{"en":"Add me"},"value":"lodge_admin"}}`)
	c8 := community(t, 8, "kook-old", "kook", "kook", strp("1"), `{"name":{"en":"Old KOOK"},"unavailableText":{"en":"closed"}}`)
	c9 := community(t, 9, "heybox", "static", "heybox", nil, `{"name":{"en":"Heybox"}}`)
	c9.FallbackUrl = strp("https://heybox.example/join")
	c10 := community(t, 10, "bad-display", "static", "link", nil, `{"name":{"en":"x"},"bogus":1}`)
	c11 := community(t, 11, "gone-platform", "static", "broken", nil, `{"name":{"en":"x"}}`)
	f.communities = []dbq.Community{c1, c2, c3, c4, c5, c6, c7, c8, c9, c10, c11}

	f.snapshots = []dbq.ProviderSnapshot{
		snapRow(t, 1, provider.StateLive, provider.Snapshot{Online: intp(1), OnlineSource: "widget"}, earlier, earlier),
		snapRow(t, 2, provider.StateStale, provider.Snapshot{
			Online: intp(4), Members: intp(52), OnlineSource: "widget",
			Channels: []provider.Channel{{ID: "1", Name: "General"}},
		}, now.Add(-time.Minute), earlier),
		snapRow(t, 3, provider.StateDegraded, provider.Snapshot{Online: intp(2), Members: intp(40), OnlineSource: "widget", InviteInvalid: true}, earlier, earlier),
		snapRow(t, 4, provider.StateLive, provider.Snapshot{IconPath: proxyIcon, Online: intp(10350), Members: intp(107345), OnlineSource: "badge"}, earlier, earlier),
		snapRow(t, 8, provider.StateUnavailable, provider.Snapshot{Online: intp(9), OnlineSource: "badge"}, earlier, earlier.Add(-time.Hour)),
	}
	live := fakeLive{cid(1): {
		IconPath: proxyIcon, Online: intp(13), Members: intp(125), OnlineSource: "invite",
		Channels: []provider.Channel{{ID: "c1", Name: "General", Position: 1}},
		Users: []provider.Member{
			{Name: "user1", AvatarPath: proxyAva, Status: "online"},
			{Name: "SpamBot", AvatarPath: proxyAva, Status: "online"},
			{Name: "猎人二号", AvatarPath: "https://evil.example/a.png", Status: "idle"},
			{Name: "user3", Status: "weird"},
			{Name: "user4", Status: "dnd"},
		},
		State: provider.StateLive, FetchedAt: now.Add(-30 * time.Second),
	}}
	f.qrCodes = []dbq.ListQRCodesRow{
		{ID: uuidOf(t, qid(1)), CommunityID: uuidOf(t, cid(5)), Note: []byte(`{}`), Width: 480, Height: 480},
		{ID: uuidOf(t, qid(2)), CommunityID: uuidOf(t, cid(7)), Note: []byte(`{"en":"valid 7 days"}`), Width: 430, Height: 430},
	}
	f.links = []dbq.Link{
		link(t, 1, "blog", "link", "https://blog.example.com", strp("builtin:link"), nil, false),
		link(t, 2, "github", "social", "https://github.com/example", strp("si:github"), nil, false),
		link(t, 3, "mastodon", "social", "https://mastodon.social/@example", nil, strp(mastoKey), true),
		link(t, 4, "evil", "social", "javascript:alert(1)", nil, nil, false),
	}
	f.blocks = []dbq.ListVisibleBlocksRow{
		block(t, 1, BlockHeading, "", `{"text":{"en":"Communities"},"showCount":true}`),
	}
	for n := 1; n <= 11; n++ {
		f.blocks = append(f.blocks, block(t, 1+n, BlockCommunity, cid(n), `{}`))
	}
	f.blocks = append(f.blocks,
		block(t, 20, BlockHeading, "", `{"text":{"en":"Links"}}`),
		block(t, 21, BlockLink, lid(1), `{}`),
		block(t, 22, BlockText, "", `{"markdown":{"en":"See the [blog](https://blog.example.com)."}}`),
		block(t, 23, BlockSocialRow, "", `{"linkIds":["`+lid(2)+`","`+lid(3)+`","`+lid(4)+`"]}`),
		block(t, 24, BlockSocialRow, "", `{"linkIds":["`+lid(4)+`"]}`),
		block(t, 25, BlockText, "", `{"markdown":{}}`),
		block(t, 26, BlockLink, lid(4), `{}`),
		block(t, 27, "video", "", `{}`),
		block(t, 28, BlockHeading, "", ``),
	)
	return f, live
}
