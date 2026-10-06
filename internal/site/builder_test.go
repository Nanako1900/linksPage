package site

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Nanako1900/linksPage/internal/provider"
)

var testNow = time.Date(2026, 10, 5, 13, 46, 30, 0, time.UTC)

func buildFull(t *testing.T) (*Snapshot, *buildFake, *bytes.Buffer) {
	t.Helper()
	f, live := fullFake(t, testNow)
	var logs bytes.Buffer
	b := newTestBuilder(t, f, live, &stubAssets{}, testNow, bufLogger(&logs))
	snap, err := b.Build(context.Background())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	return snap, f, &logs
}

func TestBuildFullPageValidates(t *testing.T) {
	snap, _, logs := buildFull(t)
	p := snap.Public
	if err := p.Validate(); err != nil {
		t.Fatalf("built page violates the contract:\n%v", err)
	}
	if _, err := json.Marshal(p); err != nil {
		t.Fatal(err)
	}
	if len(p.Communities) != 9 || len(p.Links) != 3 || len(p.Platforms) != 5 {
		t.Errorf("counts communities=%d links=%d platforms=%d", len(p.Communities), len(p.Links), len(p.Platforms))
	}
	kinds := []BlockKind{}
	for _, b := range p.Blocks {
		kinds = append(kinds, b.Kind)
	}
	if len(p.Blocks) != 14 || p.Blocks[0].Count == nil || *p.Blocks[0].Count != 9 || p.Blocks[10].Count != nil {
		t.Errorf("blocks = %v (count %v)", kinds, p.Blocks[0].Count)
	}
	if p.Version != 4 || !strings.HasPrefix(p.Revision, "r-") || p.Site.BaseURL != "https://links.example.com" {
		t.Errorf("page meta = %d %q %q", p.Version, p.Revision, p.Site.BaseURL)
	}
	if p.Site.Avatar == nil || p.Site.Avatar.Width != 256 || p.NextBoundary == nil || !p.GeneratedAt.Equal(testNow) {
		t.Errorf("avatar/boundary = %+v %v", p.Site.Avatar, p.NextBoundary)
	}
	for _, want := range []string{"skipping community", "skipping link", "skipping page block", "skipping invalid custom platform"} {
		if !strings.Contains(logs.String(), want) {
			t.Errorf("logs lack %q:\n%s", want, logs)
		}
	}
	if snap.Files == nil || snap.Head.Icons[32] == "" || snap.Head.OGImage == nil || snap.Head.Robots != SearchIndex {
		t.Errorf("head = %+v files=%v", snap.Head, snap.Files)
	}
	if snap.Head.OGTitle["en"] != "OG" || snap.Head.OGDescription["zh-CN"] == "" {
		t.Errorf("og text = %v %v", snap.Head.OGTitle, snap.Head.OGDescription)
	}
	if !p.NeedsDiscordFrame() {
		t.Error("discord embed expected")
	}
}

func TestBuildCommunityCards(t *testing.T) {
	snap, _, _ := buildFull(t)
	c := snap.Public.Communities
	tests := []struct {
		name  string
		id    string
		check func(v CommunityView) bool
	}{
		{"live uses memory", cid(1), func(v CommunityView) bool {
			l := v.Live
			return l.State == provider.StateLive && *l.Online == 13 && *l.OnlineSource == "invite" && len(l.Channels) == 1 &&
				l.UpdatedAt.Equal(testNow.Add(-30*time.Second)) && *l.JoinURL == "/go/discord" && v.Embed != nil &&
				v.Icon.URL == proxyIcon && v.Icon.Width == ProviderIconSize && *v.InviteURL == "https://discord.gg/KwdRuAkT"
		}},
		{"users filtered and limited", cid(1), func(v CommunityView) bool {
			u := v.Live.Users
			return len(u) == 3 && *u[0].Name == "user1" && *u[0].AvatarURL == proxyAva && u[1].AvatarURL == nil &&
				u[1].Status == "idle" && u[2].Status == "online"
		}},
		{"stale from db", cid(2), func(v CommunityView) bool {
			l := v.Live
			return l.State == provider.StateStale && *l.Online == 4 && len(l.Channels) == 0 && len(l.Users) == 0 &&
				l.UpdatedAt.Equal(testNow.Add(-10*time.Minute)) && v.MemberDisplay == MemberAvatars
		}},
		{"degraded hides invite and counts", cid(3), func(v CommunityView) bool {
			l := v.Live
			return l.State == provider.StateDegraded && v.InviteURL == nil && l.Online == nil && l.Members == nil &&
				l.OnlineSource == nil && l.JoinURL == nil
		}},
		{"uploaded icon wins", cid(4), func(v CommunityView) bool {
			return v.Icon.URL == PathUploads+iconKey && *v.Live.Members == 107345 && *v.Live.OnlineSource == "badge"
		}},
		{"qq group with link", cid(5), func(v CommunityView) bool {
			return v.Card == CardQQGroup && v.QQ.GroupNumber == "123456789" && v.QR.URL == PathQR+qid(1) &&
				v.Live.State == provider.StateStatic && *v.Live.JoinURL == "/go/qq-fans" && v.Live.UpdatedAt == nil
		}},
		{"qq group without link", cid(6), func(v CommunityView) bool {
			return v.Live.JoinURL == nil && v.QR == nil && v.InviteURL == nil
		}},
		{"wechat qr only", cid(7), func(v CommunityView) bool {
			return v.Live.State == provider.StateQROnly && v.QR.Note["en"] == "valid 7 days" && v.Contact.Value == "lodge_admin" && v.Live.JoinURL == nil
		}},
		{"unavailable", cid(8), func(v CommunityView) bool {
			return v.Live.State == provider.StateUnavailable && v.Live.Online == nil && v.Live.JoinURL == nil &&
				v.Live.UpdatedAt != nil && v.UnavailableText["en"] == "closed"
		}},
		{"custom platform", cid(9), func(v CommunityView) bool {
			return v.Card == CardStatic && *v.Live.JoinURL == "/go/heybox" && v.Icon == nil
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, ok := c[tt.id]
			if !ok {
				t.Fatalf("community %s missing", tt.id)
			}
			if !tt.check(v) {
				b, _ := json.MarshalIndent(v, "", " ")
				t.Errorf("unexpected view:\n%s", b)
			}
		})
	}
	hb := snap.Public.Platforms["heybox"]
	if hb.Icon == nil || hb.Icon.Kind != IconMedia || *hb.Icon.URL != PathUploads+heyboxKey || hb.Name["zh-CN"] != "黑盒语音" {
		t.Errorf("custom platform view = %+v", hb)
	}
}

func TestBuildLinks(t *testing.T) {
	snap, _, _ := buildFull(t)
	l := snap.Public.Links
	if v := l[lid(1)]; v.Href != "/go/blog" || v.Icon.Kind != IconBuiltin || v.Icon.Name != "link" {
		t.Errorf("blog = %+v", v)
	}
	if v := l[lid(3)]; v.Href != v.URL || !v.RelMe || v.Icon.Kind != IconMedia {
		t.Errorf("mastodon = %+v", v)
	}
	if _, ok := l[lid(4)]; ok {
		t.Error("unsafe link must be skipped")
	}
	var row BlockView
	for _, b := range snap.Public.Blocks {
		if b.Kind == BlockSocialRow {
			row = b
		}
	}
	if len(row.LinkIDs) != 2 {
		t.Errorf("social row = %v", row.LinkIDs)
	}
}

func TestBuildRevisionIgnoresLiveData(t *testing.T) {
	f, live := fullFake(t, testNow)
	b := newTestBuilder(t, f, live, &stubAssets{}, testNow, discardLogger())
	s1, err := b.Build(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	m := live[cid(1)]
	m.Online = intp(99)
	live[cid(1)] = m
	s2, err := b.Build(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if s1.Public.Revision != s2.Public.Revision || sameLive(s1.Public, s2.Public) {
		t.Error("live change must keep the revision but differ in live data")
	}
	f.links[0].Label = []byte(`{"en":"Blog!"}`)
	s3, err := b.Build(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if s3.Public.Revision == s1.Public.Revision {
		t.Error("content change must change the revision")
	}
}

func TestBuildErrors(t *testing.T) {
	for _, step := range []string{"settings", "page", "blocks", "boundary", "communities", "snapshots", "qr", "links", "platforms", "media"} {
		t.Run(step, func(t *testing.T) {
			f, live := fullFake(t, testNow)
			f.failOn = step
			b := newTestBuilder(t, f, live, &stubAssets{}, testNow, discardLogger())
			if _, err := b.Build(context.Background()); !errors.Is(err, errDB) {
				t.Fatalf("err = %v, want errDB", err)
			}
		})
	}
}

func TestBuildDegradedInputs(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(f *buildFake, live fakeLive)
		check  func(t *testing.T, s *Snapshot)
	}{
		{"invalid settings use defaults", func(f *buildFake, _ fakeLive) {
			f.settings.Data = []byte(`{"appearance":"neon"}`)
		}, func(t *testing.T, s *Snapshot) {
			if s.Settings.Appearance != AppearanceAuto || s.Public.Site.Avatar != nil {
				t.Errorf("settings = %+v", s.Settings)
			}
		}},
		{"asset failure", func(*buildFake, fakeLive) {}, nil},
		{"custom platform collides with preset", func(f *buildFake, _ fakeLive) {
			f.platforms = append(f.platforms, f.platforms[0])
		}, func(t *testing.T, s *Snapshot) {
			if _, ok := s.Public.Communities[cid(9)]; ok {
				t.Error("community on rejected custom platform must be skipped")
			}
		}},
		{"memory snapshot older than db", func(_ *buildFake, live fakeLive) {
			m := live[cid(1)]
			m.FetchedAt = testNow.Add(-time.Hour)
			live[cid(1)] = m
		}, func(t *testing.T, s *Snapshot) {
			if *s.Public.Communities[cid(1)].Live.Online != 1 {
				t.Error("newer db snapshot must win")
			}
		}},
		{"memory snapshot invalid state", func(_ *buildFake, live fakeLive) {
			m := live[cid(1)]
			m.State = ""
			live[cid(1)] = m
		}, func(t *testing.T, s *Snapshot) {
			if *s.Public.Communities[cid(1)].Live.Online != 1 {
				t.Error("invalid memory snapshot must be ignored")
			}
		}},
		{"pending without snapshot", func(f *buildFake, live fakeLive) {
			f.snapshots = nil
			delete(live, cid(1))
		}, func(t *testing.T, s *Snapshot) {
			l := s.Public.Communities[cid(1)].Live
			if l.State != provider.StatePending || l.Online != nil || l.UpdatedAt != nil || *l.JoinURL != "/go/discord" {
				t.Errorf("pending live = %+v", l)
			}
		}},
		{"corrupt snapshot data", func(f *buildFake, _ fakeLive) {
			f.snapshots[1].Data = []byte(`[`)
		}, func(t *testing.T, s *Snapshot) {
			if _, ok := s.Public.Communities[cid(2)]; ok {
				t.Error("community with corrupt snapshot must be skipped")
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, live := fullFake(t, testNow)
			tt.mutate(f, live)
			assets := &stubAssets{}
			if tt.check == nil {
				assets.err = errors.New("encode failed")
			}
			b := newTestBuilder(t, f, live, assets, testNow, discardLogger())
			s, err := b.Build(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if err := s.Public.Validate(); err != nil {
				t.Fatalf("page invalid: %v", err)
			}
			if tt.check == nil {
				if s.Files != nil || len(s.Head.Icons) != 0 {
					t.Error("asset failure must leave files nil")
				}
				return
			}
			tt.check(t, s)
		})
	}
}

func TestNewBuilderRequiresDeps(t *testing.T) {
	if _, err := NewBuilder(BuilderDeps{Logger: discardLogger()}); err == nil {
		t.Error("missing deps must fail")
	}
	b, err := NewBuilder(BuilderDeps{Queries: &buildFake{}, Live: fakeLive{}, Platforms: presets(t), Assets: &stubAssets{}, Logger: discardLogger()})
	if err != nil || b.deps.Now == nil {
		t.Errorf("default Now missing: %v", err)
	}
}
