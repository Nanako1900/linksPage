package provider

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestJoinTarget(t *testing.T) {
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	past, future := now.Add(-time.Minute), now.Add(time.Hour)
	const perm, inst, fb, qq = "https://discord.gg/perm", "https://discord.com/invite/inst", "https://fallback.example", "https://qm.qq.com/q/x"
	discord := JoinInput{Provider: "discord", Card: CardDiscord, State: StateLive, InviteURL: perm, InstantInviteURL: inst, FallbackURL: fb}
	tests := []struct {
		name string
		mod  func(in JoinInput) JoinInput
		want string
	}{
		{"permanent first", func(in JoinInput) JoinInput { return in }, perm},
		{"pending uses permanent", func(in JoinInput) JoinInput { in.State = StatePending; return in }, perm},
		{"widget disabled keeps permanent", func(in JoinInput) JoinInput { in.State = StateStatic; return in }, perm},
		{"10006 skips permanent", func(in JoinInput) JoinInput { in.InviteInvalid = true; return in }, inst},
		{"expired instant skipped", func(in JoinInput) JoinInput {
			in.InviteInvalid, in.InstantInviteExpiresAt = true, &past
			return in
		}, fb},
		{"unexpired instant", func(in JoinInput) JoinInput {
			in.InviteURL, in.InstantInviteExpiresAt = "", &future
			return in
		}, inst},
		{"nothing", func(in JoinInput) JoinInput {
			in.InviteURL, in.InstantInviteURL, in.FallbackURL = "", "", ""
			return in
		}, ""},
		{"unavailable has no target", func(in JoinInput) JoinInput { in.State = StateUnavailable; return in }, ""},
		{"static invite", func(JoinInput) JoinInput {
			return JoinInput{Provider: "static", Card: CardQQGroup, State: StateStatic, InviteURL: qq, FallbackURL: fb}
		}, qq},
		{"static fallback", func(JoinInput) JoinInput { return JoinInput{Provider: "static", Card: CardStatic, FallbackURL: fb} }, fb},
		{"static none", func(JoinInput) JoinInput { return JoinInput{Provider: "static", Card: CardQQGroup} }, ""},
		{"wechat never", func(JoinInput) JoinInput {
			return JoinInput{Provider: "static", Card: CardWeChatGroup, State: StateQROnly, FallbackURL: fb}
		}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := JoinTarget(tt.mod(discord), now); got != tt.want {
				t.Errorf("JoinTarget = %q, want %q", got, tt.want)
			}
		})
	}
}

type fakeProvider struct{ kind string }

func (f fakeProvider) Kind() string                                       { return f.kind }
func (fakeProvider) Capabilities() Capabilities                           { return Capabilities{} }
func (fakeProvider) ValidateConfig(ConfigInput) (any, error)              { return nil, nil }
func (fakeProvider) Fetch(context.Context, FetchInput) (*Snapshot, error) { return nil, nil }
func (fakeProvider) MinInterval() time.Duration                           { return time.Minute }
func (fakeProvider) ImageHosts() map[string][]string                      { return map[string][]string{"h": {"/p/"}} }

func TestRegistry(t *testing.T) {
	r, err := NewRegistry(fakeProvider{"b"}, fakeProvider{"a"})
	if err != nil {
		t.Fatal(err)
	}
	if got := r.Kinds(); strings.Join(got, ",") != "a,b" {
		t.Errorf("kinds = %v", got)
	}
	if _, ok := r.Get("a"); !ok {
		t.Error("Get(a) failed")
	}
	if _, ok := r.Get("zzz"); ok {
		t.Error("Get(zzz) should fail")
	}
	if hosts := r.ImageHosts(); len(hosts) != 2 || hosts["a"]["h"][0] != "/p/" {
		t.Errorf("hosts = %v", hosts)
	}
	for _, bad := range [][]Provider{{fakeProvider{"a"}, fakeProvider{"a"}}, {fakeProvider{""}}, {nil}} {
		if _, err := NewRegistry(bad...); err == nil {
			t.Errorf("NewRegistry(%v) should fail", bad)
		}
	}
}

func TestLiveStoreCopies(t *testing.T) {
	s := NewLiveStore()
	n := 3
	exp := time.Now()
	in := Snapshot{Name: "x", Online: &n, Users: []Member{{Name: "u"}}, Channels: []Channel{{ID: "1"}}, InviteExpiresAt: &exp}
	s.Put("c1", in)
	n = 99
	in.Users[0].Name = "mutated"
	got, ok := s.Live("c1")
	if !ok || *got.Online != 3 || got.Users[0].Name != "u" {
		t.Fatalf("live = %+v", got)
	}
	got.Channels[0].ID = "mutated"
	again, _ := s.Live("c1")
	if again.Channels[0].ID != "1" {
		t.Error("Live must return a copy")
	}
	s.Delete("c1")
	if _, ok := s.Live("c1"); ok {
		t.Error("Delete failed")
	}
}

func TestStatesAndErrors(t *testing.T) {
	for _, s := range States() {
		if !s.Valid() {
			t.Errorf("%s should be valid", s)
		}
	}
	if State("ok").Valid() || len(States()) != 7 {
		t.Error("state set changed; update the contract")
	}
	base := errors.New("429")
	err := error(&RetryAfterError{After: 3 * time.Second, Err: base})
	if d, ok := RetryAfter(err); !ok || d != 3*time.Second || !errors.Is(err, base) || !strings.Contains(err.Error(), "3s") {
		t.Errorf("RetryAfter = %v %v", d, ok)
	}
	if _, ok := RetryAfter(base); ok {
		t.Error("plain error has no retry-after")
	}
	if UserAgent("v1.2.3") != "LinksPage/v1.2.3 (+https://github.com/Nanako1900/linksPage)" {
		t.Error("user agent changed")
	}
}

func TestEmbeddedPresetsParse(t *testing.T) {
	ps, err := ParsePlatforms(presetsYAML)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, p := range ps {
		if !PlatformIDRe.MatchString(p.ID) || seen[p.ID] || (p.Icon != "" && !IconRefRe.MatchString(p.Icon)) || p.Name["en"] == "" {
			t.Errorf("invalid preset %+v", p)
		}
		seen[p.ID] = true
	}
	for _, id := range []string{"discord", "kook", "telegram", "qq-group", "wechat-group", "link"} {
		if !seen[id] {
			t.Errorf("missing preset %s", id)
		}
	}
	for _, bad := range []string{"version: 2\nplatforms: []\n", "version: 1\nplatforms:\n  - id: x\n    colour: red\n", "version: ["} {
		if _, err := ParsePlatforms([]byte(bad)); err == nil {
			t.Errorf("ParsePlatforms(%q) should fail", bad)
		}
	}
}

func TestSanitizeSnapshotName(t *testing.T) {
	out := SanitizeSnapshot(Snapshot{Name: " a\u202eb " + strings.Repeat("x", 200)})
	if strings.ContainsRune(out.Name, '\u202e') || len([]rune(out.Name)) != MaxNameRunes {
		t.Errorf("name = %q", out.Name)
	}
}
