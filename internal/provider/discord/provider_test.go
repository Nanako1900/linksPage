package discord

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Nanako1900/linksPage/internal/provider"
)

const (
	guildOK         = "1114391825336250432"
	guildMismatch   = "123456789012345678"
	guildUnknown    = "100000000000000000"
	guildDisabled   = "662267976984297473"
	guildRateLimit  = "200000000000000000"
	guildCloudflare = "300000000000000000"
	guildBroken     = "400000000000000000"
	inviteOK        = "KwdRuAkT"
	inviteUnknown   = "lpUnknownInvite0x"
	inviteLimited   = "rateLimited"
	inviteBroken    = "broken"
	inviteDisabled  = "disabledGuild"
	inviteBanner    = "withBanner"
)

// upstream serves the recorded fixtures by guild id / invite code.
type upstream struct {
	srv      *httptest.Server
	fixtures map[string]fixture
	widget   atomic.Int32
	invite   atomic.Int32
}

func newUpstream(t *testing.T) *upstream {
	t.Helper()
	u := &upstream{fixtures: map[string]fixture{
		"widget/" + guildOK:         loadFixture(t, "widget_ok", ".json"),
		"widget/" + guildMismatch:   loadFixture(t, "widget_ok", ".json"),
		"widget/" + guildUnknown:    loadFixture(t, "widget_unknown_guild", ".json"),
		"widget/" + guildDisabled:   loadFixture(t, "widget_disabled", ".json"),
		"widget/" + guildRateLimit:  loadFixture(t, "ratelimited_synthetic", ".json"),
		"widget/" + guildCloudflare: loadFixture(t, "ratelimited_cloudflare_synthetic", ".html"),
		"widget/" + guildBroken:     {status: http.StatusBadGateway, header: http.Header{}, body: []byte("bad gateway")},
		"invite/" + inviteOK:        loadFixture(t, "invite_ok", ".json"),
		"invite/" + inviteUnknown:   loadFixture(t, "invite_unknown", ".json"),
		"invite/" + inviteLimited:   loadFixture(t, "ratelimited_global_synthetic", ".json"),
		"invite/" + inviteBroken:    {status: http.StatusInternalServerError, header: http.Header{}, body: []byte("{}")},
	}}
	other := u.fixtures["invite/"+inviteOK]
	other.body = []byte(strings.ReplaceAll(string(other.body), guildOK, guildDisabled))
	u.fixtures["invite/"+inviteDisabled] = other
	banner := u.fixtures["invite/"+inviteOK]
	banner.body = []byte(strings.Replace(string(banner.body), `"banner": null`, `"banner": "a_0123456789abcdef0123456789abcdef"`, 1))
	u.fixtures["invite/"+inviteBanner] = banner
	u.srv = httptest.NewServer(http.HandlerFunc(u.serve))
	t.Cleanup(u.srv.Close)
	return u
}

func (u *upstream) serve(w http.ResponseWriter, r *http.Request) {
	var key string
	switch {
	case strings.HasPrefix(r.URL.Path, "/prefix/api/guilds/") && strings.HasSuffix(r.URL.Path, "/widget.json"):
		u.widget.Add(1)
		key = "widget/" + strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/prefix/api/guilds/"), "/widget.json")
	case strings.HasPrefix(r.URL.Path, "/prefix/api/v10/invites/") && r.URL.Query().Get("with_counts") == "true":
		u.invite.Add(1)
		key = "invite/" + strings.TrimPrefix(r.URL.Path, "/prefix/api/v10/invites/")
	}
	f, ok := u.fixtures[key]
	if !ok {
		http.NotFound(w, r)
		return
	}
	for k, vs := range f.header {
		if strings.EqualFold(k, "content-length") {
			continue
		}
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(f.status)
	_, _ = w.Write(f.body)
}

// fakeImages records registrations; URLs containing failOn fail.
type fakeImages struct {
	mu     sync.Mutex
	calls  []string
	failOn string
}

func (f *fakeImages) Register(_ context.Context, providerKind, imageKind, rawURL string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, providerKind+"|"+imageKind+"|"+rawURL)
	if f.failOn != "" && strings.Contains(rawURL, f.failOn) {
		return "", errors.New("registration failed")
	}
	return "/media/p/" + imageKind + "AAAAAAAAAAAAAAAAAA.png", nil
}

func newTestProvider(t *testing.T, u *upstream, images provider.ImageRegistrar) *Provider {
	t.Helper()
	client, err := provider.NewHTTPClient(provider.ClientOptions{UserAgent: provider.UserAgent("test")})
	if err != nil {
		t.Fatal(err)
	}
	p, err := NewProvider(Options{APIBase: u.srv.URL + "/prefix", Client: client, Images: images})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

var now = time.Date(2026, 10, 5, 14, 0, 0, 0, time.UTC)

func fetch(t *testing.T, p *Provider, guild, invite string, prev *provider.Snapshot) (*provider.Snapshot, error) {
	t.Helper()
	inviteURL := ""
	if invite != "" {
		inviteURL = "https://discord.gg/" + invite
	}
	cfg, err := p.ValidateConfig(provider.ConfigInput{ExternalID: guild, InviteURL: inviteURL, Raw: []byte(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	return p.Fetch(context.Background(), provider.FetchInput{Config: cfg, Previous: prev, Now: now})
}

func TestFetchWidgetAndInvite(t *testing.T) {
	u := newUpstream(t)
	images := &fakeImages{}
	s, err := fetch(t, newTestProvider(t, u, images), guildOK, inviteOK, nil)
	if err != nil {
		t.Fatal(err)
	}
	switch {
	case s.State != provider.StateLive || s.ErrCode != "":
		t.Errorf("state = %s %q", s.State, s.ErrCode)
	case s.Name != "Nanako‘s Community":
		t.Errorf("name = %q", s.Name)
	case *s.Online != 16 || s.OnlineSource != provider.SourceInvite || *s.Members != 125:
		t.Errorf("counts = %d %s %d", *s.Online, s.OnlineSource, *s.Members)
	case !strings.HasPrefix(s.IconPath, "/media/p/icon") || !strings.HasPrefix(s.BannerPath, "/media/p/splash"):
		t.Errorf("images = %q %q", s.IconPath, s.BannerPath)
	case len(s.Channels) != 4 || s.Channels[0].Name != "Room 1" || s.Channels[3].Name != "Nanako's Room":
		t.Errorf("channels = %+v", s.Channels)
	case len(s.Users) != 16 || !strings.HasPrefix(s.Users[0].AvatarPath, "/media/p/avatar") || s.Users[0].Status != "idle":
		t.Errorf("users = %+v", s.Users)
	case s.InstantInviteURL != "https://discord.com/invite/KwdRuAkT" || s.InviteInvalid:
		t.Errorf("instant = %q invalid=%v", s.InstantInviteURL, s.InviteInvalid)
	case s.InviteFetchedAt == nil || !s.InviteFetchedAt.Equal(now) || !s.FetchedAt.Equal(now):
		t.Errorf("times = %v %v", s.InviteFetchedAt, s.FetchedAt)
	}
	wantIcon := "discord|icon|https://cdn.discordapp.com/icons/" + guildOK + "/42e0892e057eccdf8f50765cef3fbfd8.png?size=256"
	if images.calls[0] != wantIcon {
		t.Errorf("first registration = %q", images.calls[0])
	}
	if !strings.Contains(images.calls[1], "|splash|https://cdn.discordapp.com/splashes/") {
		t.Errorf("splash registration = %q", images.calls[1])
	}
}

func TestFetchOutcomes(t *testing.T) {
	tests := []struct {
		name      string
		guild     string
		invite    string
		state     provider.State
		code      string
		source    string
		members   bool
		users     int
		invalid   bool
		inviteReq int32
	}{
		{"widget only", guildOK, "", provider.StateLive, "", provider.SourceWidget, false, 16, false, 0},
		{"unknown guild", guildUnknown, inviteOK, provider.StateUnavailable, "discord_404_10004", "", false, 0, false, 0},
		{"widget disabled", guildDisabled, inviteDisabled, provider.StateStatic, "discord_403_50004", provider.SourceInvite, true, 0, false, 1},
		{"invite unknown", guildOK, inviteUnknown, provider.StateDegraded, "discord_404_10006", provider.SourceWidget, false, 16, true, 1},
		{"invite of another guild", guildMismatch, inviteOK, provider.StateDegraded, CodeInviteGuildMismatch, provider.SourceWidget, false, 16, true, 1},
		{"widget disabled no invite", guildDisabled, "", provider.StateStatic, "discord_403_50004", "", false, 0, false, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := newUpstream(t)
			s, err := fetch(t, newTestProvider(t, u, &fakeImages{}), tt.guild, tt.invite, nil)
			if err != nil {
				t.Fatal(err)
			}
			if s.State != tt.state || s.ErrCode != tt.code || s.OnlineSource != tt.source ||
				(s.Members != nil) != tt.members || len(s.Users) != tt.users || s.InviteInvalid != tt.invalid {
				t.Errorf("snapshot = state %s code %q source %q members %v users %d invalid %v",
					s.State, s.ErrCode, s.OnlineSource, s.Members, len(s.Users), s.InviteInvalid)
			}
			if got := u.invite.Load(); got != tt.inviteReq {
				t.Errorf("invite requests = %d, want %d", got, tt.inviteReq)
			}
			if s.Channels == nil {
				t.Error("channels must not be nil")
			}
		})
	}
}

func TestFetchTransientErrors(t *testing.T) {
	tests := []struct {
		name       string
		guild      string
		code       string
		retryAfter time.Duration
		hasRetry   bool
	}{
		{"rate limited", guildRateLimit, "discord_429", 64570 * time.Millisecond, true},
		{"cloudflare ban", guildCloudflare, "discord_429", 0, true},
		{"bad gateway", guildBroken, "discord_502_0", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := newUpstream(t)
			s, err := fetch(t, newTestProvider(t, u, nil), tt.guild, inviteOK, nil)
			if err == nil || s != nil {
				t.Fatalf("want error, got %+v", s)
			}
			if got := provider.ErrCode(err); got != tt.code {
				t.Errorf("code = %q, want %q", got, tt.code)
			}
			ra, ok := provider.RetryAfter(err)
			if ok != tt.hasRetry || ra != tt.retryAfter {
				t.Errorf("retry after = %s %v", ra, ok)
			}
			if u.invite.Load() != 0 {
				t.Error("invite must not be fetched after a widget failure")
			}
		})
	}
	u := newUpstream(t)
	p := newTestProvider(t, u, nil)
	u.srv.Close()
	_, err := fetch(t, p, guildOK, "", nil)
	if provider.ErrCode(err) != "discord_error" {
		t.Errorf("network error code = %q (%v)", provider.ErrCode(err), err)
	}
}

func TestInstantInviteExpiry(t *testing.T) {
	const instant = "https://discord.com/invite/KwdRuAkT"
	earlier := now.Add(-2 * time.Hour).Add(InstantInviteTTL)
	tests := []struct {
		name string
		prev *provider.Snapshot
		want time.Time
	}{
		{"first sighting expires after the TTL", nil, now.Add(InstantInviteTTL)},
		{"same invite keeps its first expiry", &provider.Snapshot{InstantInviteURL: instant, InviteExpiresAt: &earlier, State: provider.StateLive}, earlier},
		{"new invite gets a new expiry", &provider.Snapshot{InstantInviteURL: "https://discord.com/invite/older", InviteExpiresAt: &earlier, State: provider.StateLive}, now.Add(InstantInviteTTL)},
		{"undated previous invite gets dated", &provider.Snapshot{InstantInviteURL: instant, State: provider.StateLive}, now.Add(InstantInviteTTL)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := fetch(t, newTestProvider(t, newUpstream(t), &fakeImages{}), guildOK, inviteOK, tt.prev)
			if err != nil {
				t.Fatal(err)
			}
			if s.InstantInviteURL != instant || s.InviteExpiresAt == nil || !s.InviteExpiresAt.Equal(tt.want) {
				t.Fatalf("instant = %q expires %v, want %v", s.InstantInviteURL, s.InviteExpiresAt, tt.want)
			}
			if tt.prev != nil && tt.prev.InviteExpiresAt != nil && s.InviteExpiresAt == tt.prev.InviteExpiresAt {
				t.Error("expiry must be copied, not shared with the previous snapshot")
			}
		})
	}
}

func TestInstantInviteExpiryWithoutInvite(t *testing.T) {
	if got := instantInviteExpiry("", nil, now); got != nil {
		t.Errorf("expiry without an instant invite = %v", got)
	}
}

func TestInviteCadenceAndCarryOver(t *testing.T) {
	recent := now.Add(-5 * time.Minute)
	old := now.Add(-20 * time.Minute)
	members, online := 99, 7
	prev := func(fetchedAt time.Time) *provider.Snapshot {
		return &provider.Snapshot{
			Members: &members, Online: &online, OnlineSource: provider.SourceInvite,
			IconPath: "/media/p/prevIcon.png", BannerPath: "/media/p/prevBanner.png",
			InviteFetchedAt: &fetchedAt, State: provider.StateLive,
		}
	}
	tests := []struct {
		name        string
		invite      string
		prev        *provider.Snapshot
		requests    int32
		online      int
		source      string
		fetchedAt   time.Time
		wantMembers int
	}{
		{"not due carries over", inviteOK, prev(recent), 0, 7, provider.SourceInvite, recent, 99},
		{"due refetches", inviteOK, prev(old), 1, 16, provider.SourceInvite, now, 125},
		{"429 carries over and waits", inviteLimited, prev(old), 1, 7, provider.SourceInvite, now, 99},
		{"5xx carries over and retries", inviteBroken, prev(old), 1, 7, provider.SourceInvite, old, 99},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := newUpstream(t)
			s, err := fetch(t, newTestProvider(t, u, &fakeImages{}), guildOK, tt.invite, tt.prev)
			if err != nil {
				t.Fatal(err)
			}
			if u.invite.Load() != tt.requests || *s.Online != tt.online || s.OnlineSource != tt.source ||
				!s.InviteFetchedAt.Equal(tt.fetchedAt) || *s.Members != tt.wantMembers {
				t.Errorf("requests=%d online=%d source=%s fetchedAt=%v members=%d",
					u.invite.Load(), *s.Online, s.OnlineSource, s.InviteFetchedAt, *s.Members)
			}
			if tt.requests == 0 && (s.IconPath != "/media/p/prevIcon.png" || s.BannerPath != "/media/p/prevBanner.png") {
				t.Errorf("images not carried: %q %q", s.IconPath, s.BannerPath)
			}
		})
	}
}

func TestCarriedInvalidInvite(t *testing.T) {
	recent := now.Add(-time.Minute)
	tests := []struct {
		name string
		prev provider.Snapshot
		want string
	}{
		{"degraded keeps its code", provider.Snapshot{
			InviteInvalid: true, InviteFetchedAt: &recent,
			State: provider.StateDegraded, ErrCode: CodeInviteGuildMismatch,
		}, CodeInviteGuildMismatch},
		{"static falls back to 10006", provider.Snapshot{
			InviteInvalid: true, InviteFetchedAt: &recent,
			State: provider.StateStatic, ErrCode: "discord_403_50004",
		}, "discord_404_10006"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := newUpstream(t)
			s, err := fetch(t, newTestProvider(t, u, nil), guildOK, inviteOK, &tt.prev)
			if err != nil {
				t.Fatal(err)
			}
			if s.State != provider.StateDegraded || s.ErrCode != tt.want || s.OnlineSource != provider.SourceWidget {
				t.Errorf("state=%s code=%q source=%s", s.State, s.ErrCode, s.OnlineSource)
			}
		})
	}
}

func TestImageRegistrationFailures(t *testing.T) {
	u := newUpstream(t)
	images := &fakeImages{failOn: "cdn.discordapp.com"}
	s, err := fetch(t, newTestProvider(t, u, images), guildOK, inviteOK, &provider.Snapshot{Name: "old"})
	if err != nil {
		t.Fatal(err)
	}
	if s.IconPath != "" || s.BannerPath != "" || s.Users[0].AvatarPath != "" || len(s.Users) != 16 {
		t.Errorf("failed registrations must yield empty paths: %+v", s)
	}
	u2 := newUpstream(t)
	s2, err := fetch(t, newTestProvider(t, u2, nil), guildOK, inviteOK, nil)
	if err != nil || s2.IconPath != "" || s2.Users[0].AvatarPath != "" {
		t.Errorf("nil registrar: %+v %v", s2, err)
	}
}

func TestProviderConstructionAndConfig(t *testing.T) {
	client := http.DefaultClient
	if _, err := NewProvider(Options{}); err == nil {
		t.Error("nil client must fail")
	}
	if _, err := NewProvider(Options{Client: client, APIBase: "ftp://x"}); err == nil {
		t.Error("bad api base must fail")
	}
	p, err := NewProvider(Options{Client: client})
	if err != nil {
		t.Fatal(err)
	}
	if p.Kind() != "discord" || p.MinInterval() != WidgetInterval || !p.Capabilities().Embed ||
		len(p.ImageHosts()[CDNHost]) != 4 || p.base.String() != provider.DefaultDiscordAPIBase {
		t.Error("provider metadata changed")
	}
	cfgTests := []struct {
		name string
		in   provider.ConfigInput
		want Config
		ok   bool
	}{
		{"guild only", provider.ConfigInput{ExternalID: guildOK}, Config{GuildID: guildOK}, true},
		{"with invite", provider.ConfigInput{ExternalID: guildOK, InviteURL: "https://discord.com/invite/abc-1"}, Config{GuildID: guildOK, InviteCode: "abc-1"}, true},
		{"short guild", provider.ConfigInput{ExternalID: "123"}, Config{}, false},
		{"bad invite", provider.ConfigInput{ExternalID: guildOK, InviteURL: "https://evil.example/x"}, Config{}, false},
		{"bad raw", provider.ConfigInput{ExternalID: guildOK, Raw: []byte(`{"x":1}`)}, Config{}, false},
	}
	for _, tt := range cfgTests {
		got, err := p.ValidateConfig(tt.in)
		if (err == nil) != tt.ok || (err != nil && !errors.Is(err, provider.ErrInvalidConfig)) {
			t.Errorf("%s: err = %v", tt.name, err)
			continue
		}
		if tt.ok && got != tt.want {
			t.Errorf("%s: cfg = %+v", tt.name, got)
		}
	}
	for _, v := range []any{nil, (*Config)(nil), "x"} {
		if _, err := p.Fetch(context.Background(), provider.FetchInput{Config: v}); !errors.Is(err, provider.ErrInvalidConfig) {
			t.Errorf("Fetch(%T) err = %v", v, err)
		}
	}
	if c, err := configFrom(&Config{GuildID: guildOK}); err != nil || c.GuildID != guildOK {
		t.Error("pointer config must work")
	}
	if _, err := WidgetURLAt(p.base, "x"); err == nil {
		t.Error("WidgetURLAt must validate")
	}
	if _, err := InviteURLAt(p.base, "!"); err == nil {
		t.Error("InviteURLAt must validate")
	}
}

func TestFetchDefaultsNow(t *testing.T) {
	u := newUpstream(t)
	p := newTestProvider(t, u, nil)
	before := time.Now()
	s, err := p.Fetch(context.Background(), provider.FetchInput{Config: Config{GuildID: guildOK}})
	if err != nil || s.FetchedAt.Before(before) {
		t.Errorf("fetchedAt = %v err = %v", s.FetchedAt, err)
	}
}

func TestInstantInvite(t *testing.T) {
	s := func(v string) *string { return &v }
	tests := []struct {
		in   *string
		want string
	}{
		{nil, ""},
		{s(""), ""},
		{s("https://discord.gg/abcd"), "https://discord.com/invite/abcd"},
		{s("https://evil.example/invite/abcd"), ""},
	}
	for _, tt := range tests {
		if got := instantInvite(tt.in); got != tt.want {
			t.Errorf("instantInvite = %q, want %q", got, tt.want)
		}
	}
}

func TestFetchBannerPreferredOverSplash(t *testing.T) {
	u := newUpstream(t)
	images := &fakeImages{}
	s, err := fetch(t, newTestProvider(t, u, images), guildOK, inviteBanner, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := "discord|banner|https://cdn.discordapp.com/banners/" + guildOK + "/a_0123456789abcdef0123456789abcdef.gif?size=1024"
	if !strings.HasPrefix(s.BannerPath, "/media/p/banner") || images.calls[1] != want {
		t.Errorf("banner = %q calls = %v", s.BannerPath, images.calls[:2])
	}
}

func TestWidgetDisabledUsesInviteName(t *testing.T) {
	u := newUpstream(t)
	s, err := fetch(t, newTestProvider(t, u, nil), guildDisabled, inviteDisabled, nil)
	if err != nil || s.Name != "Nanako‘s Community" {
		t.Errorf("name = %q err = %v", s.Name, err)
	}
	s, err = fetch(t, newTestProvider(t, u, nil), guildDisabled, "", &provider.Snapshot{Name: "Previous"})
	if err != nil || s.Name != "Previous" {
		t.Errorf("name = %q err = %v", s.Name, err)
	}
}
