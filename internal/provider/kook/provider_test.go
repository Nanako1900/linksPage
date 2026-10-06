package kook

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Nanako1900/linksPage/internal/provider"
)

const (
	guildPublic    = "5417470909511807"
	guildPrivate   = "8474959284287105"
	guildBadCounts = "1"
	guildLimited   = "2"
	guildDown      = "3"
	guildOK200     = "4"
	guildForbidden = "5"
)

type badgeServer struct {
	srv      *httptest.Server
	requests atomic.Int32
	byKey    map[string]providerResponse
}

type providerResponse struct {
	status int
	header http.Header
}

func newBadgeServer(t *testing.T) *badgeServer {
	t.Helper()
	rec := func(name string) providerResponse {
		r := loadRecorded(t, name)
		return providerResponse{status: r.Status, header: r.Header}
	}
	b := &badgeServer{byKey: map[string]providerResponse{
		guildPublic + "/0":    rec("badge_public_style0"),
		guildPublic + "/2":    rec("badge_public_style2"),
		guildPrivate + "/0":   rec("badge_not_public_style0"),
		guildPrivate + "/2":   rec("badge_not_public_style2"),
		guildBadCounts + "/0": rec("badge_public_style0"),
		guildBadCounts + "/2": rec("badge_public_style0"),
		guildLimited + "/0":   {status: http.StatusTooManyRequests, header: http.Header{"Retry-After": {"120"}}},
		guildDown + "/0":      {status: http.StatusServiceUnavailable, header: http.Header{}},
		guildOK200 + "/0":     {status: http.StatusOK, header: http.Header{}},
		guildForbidden + "/0": {status: http.StatusForbidden, header: http.Header{}},
	}}
	b.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b.requests.Add(1)
		if r.URL.Path != "/kook/api/v3/badge/guild" {
			http.NotFound(w, r)
			return
		}
		resp, ok := b.byKey[r.URL.Query().Get("guild_id")+"/"+r.URL.Query().Get("style")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		for k, vs := range resp.header {
			for _, v := range vs {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(resp.status)
		_, _ = w.Write([]byte("null"))
	}))
	t.Cleanup(b.srv.Close)
	return b
}

func newTestProvider(t *testing.T, b *badgeServer) *Provider {
	t.Helper()
	client, err := provider.NewHTTPClient(provider.ClientOptions{})
	if err != nil {
		t.Fatal(err)
	}
	p, err := NewProvider(Options{APIBase: b.srv.URL + "/kook", Client: client})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

var now = time.Date(2026, 10, 5, 14, 0, 0, 0, time.UTC)

func TestFetchBadges(t *testing.T) {
	tests := []struct {
		name      string
		guild     string
		state     provider.State
		code      string
		requests  int32
		wantName  string
		online    int
		members   int
		transient string
		retry     time.Duration
	}{
		{
			name: "public", guild: guildPublic, state: provider.StateLive, requests: 2,
			wantName: "「猎杀对决」中文玩家社区", online: 10350, members: 107345,
		},
		{name: "not public", guild: guildPrivate, state: provider.StateUnavailable, code: CodeNotPublic, requests: 1},
		{name: "count label malformed", guild: guildBadCounts, state: provider.StateStatic, code: CodeMalformed, requests: 2},
		{name: "unexpected 200", guild: guildOK200, state: provider.StateStatic, code: CodeMalformed, requests: 1},
		{name: "rate limited", guild: guildLimited, transient: CodeRateLimited, retry: 2 * time.Minute, requests: 1},
		{name: "server down", guild: guildDown, transient: "kook_503", requests: 1},
		{name: "forbidden", guild: guildForbidden, transient: "kook_403", requests: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newBadgeServer(t)
			p := newTestProvider(t, b)
			cfg, err := p.ValidateConfig(provider.ConfigInput{ExternalID: tt.guild})
			if err != nil {
				t.Fatal(err)
			}
			s, err := p.Fetch(context.Background(), provider.FetchInput{Config: cfg, Now: now})
			if got := b.requests.Load(); got != tt.requests {
				t.Errorf("requests = %d, want %d", got, tt.requests)
			}
			if tt.transient != "" {
				if err == nil || provider.ErrCode(err) != tt.transient {
					t.Fatalf("err = %v (code %q)", err, provider.ErrCode(err))
				}
				if ra, _ := provider.RetryAfter(err); ra != tt.retry {
					t.Errorf("retry after = %s", ra)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if s.State != tt.state || s.ErrCode != tt.code || !s.FetchedAt.Equal(now) || s.Channels == nil {
				t.Errorf("snapshot = %+v", s)
			}
			if tt.state == provider.StateLive && (s.Name != tt.wantName || *s.Online != tt.online ||
				*s.Members != tt.members || s.OnlineSource != provider.SourceBadge) {
				t.Errorf("live snapshot = %+v", s)
			}
		})
	}
}

func TestFetchNeverFollowsRedirects(t *testing.T) {
	b := newBadgeServer(t)
	p, err := NewProvider(Options{APIBase: b.srv.URL + "/kook", Client: &http.Client{}})
	if err != nil {
		t.Fatal(err)
	}
	s, err := p.Fetch(context.Background(), provider.FetchInput{Config: Config{GuildID: guildPublic}, Now: now})
	if err != nil || s.State != provider.StateLive || b.requests.Load() != 2 {
		t.Errorf("snapshot = %+v err = %v requests = %d", s, err, b.requests.Load())
	}
}

func TestFetchNetworkError(t *testing.T) {
	b := newBadgeServer(t)
	p := newTestProvider(t, b)
	b.srv.Close()
	_, err := p.Fetch(context.Background(), provider.FetchInput{Config: Config{GuildID: guildPublic}})
	if err == nil || provider.ErrCode(err) != CodeError {
		t.Errorf("err = %v", err)
	}
	if _, err := p.Fetch(context.Background(), provider.FetchInput{Config: Config{GuildID: "x"}}); !errors.Is(err, ErrInvalidGuildID) {
		t.Errorf("invalid guild err = %v", err)
	}
}

func TestKOOKProviderConfig(t *testing.T) {
	if _, err := NewProvider(Options{}); err == nil {
		t.Error("nil client must fail")
	}
	if _, err := NewProvider(Options{Client: http.DefaultClient, APIBase: "https://x/"}); err == nil {
		t.Error("bad api base must fail")
	}
	p, err := NewProvider(Options{Client: http.DefaultClient})
	if err != nil {
		t.Fatal(err)
	}
	if p.Kind() != "kook" || p.MinInterval() != BadgeInterval || len(p.ImageHosts()) != 0 ||
		!p.Capabilities().Online || p.Capabilities().Users || p.base.String() != provider.DefaultKOOKAPIBase {
		t.Error("metadata changed")
	}
	tests := []struct {
		in provider.ConfigInput
		ok bool
	}{
		{provider.ConfigInput{ExternalID: guildPublic}, true},
		{provider.ConfigInput{ExternalID: "abc"}, false},
		{provider.ConfigInput{ExternalID: strings.Repeat("1", 21)}, false},
		{provider.ConfigInput{ExternalID: "1", Raw: []byte(`{"token":"x"}`)}, false},
	}
	for _, tt := range tests {
		_, err := p.ValidateConfig(tt.in)
		if (err == nil) != tt.ok || (err != nil && !errors.Is(err, provider.ErrInvalidConfig)) {
			t.Errorf("ValidateConfig(%+v) = %v", tt.in, err)
		}
	}
	for _, v := range []any{nil, (*Config)(nil), 3} {
		if _, err := p.Fetch(context.Background(), provider.FetchInput{Config: v}); !errors.Is(err, provider.ErrInvalidConfig) {
			t.Errorf("Fetch(%T) = %v", v, err)
		}
	}
	if c, err := configFrom(&Config{GuildID: "1"}); err != nil || c.GuildID != "1" {
		t.Error("pointer config")
	}
	if _, err := BadgeURLAt(p.base, "1", Style(9)); !errors.Is(err, ErrInvalidStyle) {
		t.Errorf("style err = %v", err)
	}
	if got, _ := BadgeURLAt(p.base, "1", StyleOnlineTotal); got != "https://www.kookapp.cn/api/v3/badge/guild?guild_id=1&style=2" {
		t.Errorf("BadgeURLAt = %q", got)
	}
}

func TestParseRetryAfter(t *testing.T) {
	tests := map[string]time.Duration{"": 0, "abc": 0, "-1": 0, "5": 5 * time.Second, "99999999999": time.Hour, "3600": time.Hour}
	for in, want := range tests {
		if got := parseRetryAfter(in); got != want {
			t.Errorf("parseRetryAfter(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestFetchDefaultsNow(t *testing.T) {
	b := newBadgeServer(t)
	before := time.Now()
	s, err := newTestProvider(t, b).Fetch(context.Background(), provider.FetchInput{Config: Config{GuildID: guildPrivate}})
	if err != nil || s.FetchedAt.Before(before) {
		t.Errorf("snapshot = %+v err = %v", s, err)
	}
}
