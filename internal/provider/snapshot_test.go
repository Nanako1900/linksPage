package provider

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func intp(v int) *int { return &v }

func TestSanitizeSnapshotFull(t *testing.T) {
	exp := time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC)
	channels := make([]Channel, 0, MaxChannels+5)
	for i := range MaxChannels + 5 {
		channels = append(channels, Channel{ID: fmt.Sprint(i), Name: "room\u200b" + fmt.Sprint(i), Position: i})
	}
	channels = append([]Channel{{ID: "x", Name: "\u200b"}}, channels...)
	users := make([]Member, 0, MaxUsers+1)
	for range MaxUsers + 1 {
		users = append(users, Member{Name: strings.Repeat("n", 40), AvatarPath: "/media/p/abc.png", Status: "offline"})
	}
	in := Snapshot{
		Name:             "Guild",
		IconPath:         "https://evil.example/x.png",
		BannerPath:       "/media/p/k.gif",
		Online:           intp(-1),
		Members:          intp(10),
		OnlineSource:     SourceWidget,
		Channels:         channels,
		Users:            users,
		InstantInviteURL: "javascript:alert(1)",
		InviteExpiresAt:  &exp,
		ErrCode:          "Bad Code!",
	}
	out := SanitizeSnapshot(in)
	switch {
	case out.IconPath != "" || out.BannerPath != "/media/p/k.gif":
		t.Errorf("image paths = %q %q", out.IconPath, out.BannerPath)
	case out.Online != nil || out.OnlineSource != "" || *out.Members != 10:
		t.Errorf("counts = %v %q %v", out.Online, out.OnlineSource, out.Members)
	case len(out.Channels) != MaxChannels || out.Channels[0].Name != "room0":
		t.Errorf("channels = %d %+v", len(out.Channels), out.Channels[0])
	case len(out.Users) != MaxUsers || len([]rune(out.Users[0].Name)) != MaxUserNameRunes || out.Users[0].Status != StatusOnline:
		t.Errorf("users = %d %+v", len(out.Users), out.Users[0])
	case out.InstantInviteURL != "" || out.InviteExpiresAt != nil:
		t.Errorf("instant invite = %q %v", out.InstantInviteURL, out.InviteExpiresAt)
	case out.ErrCode != "":
		t.Errorf("err code = %q", out.ErrCode)
	}
	if in.Members == out.Members {
		t.Error("counts must be copied")
	}
	again := SanitizeSnapshot(out)
	if len(again.Users) != len(out.Users) || again.BannerPath != out.BannerPath {
		t.Error("sanitize must be idempotent")
	}
}

func TestSanitizeHelpers(t *testing.T) {
	for _, s := range []string{StatusOnline, StatusIdle, StatusDND} {
		if cleanStatus(s) != s {
			t.Errorf("status %s changed", s)
		}
	}
	for _, s := range []string{SourceInvite, SourceWidget, SourceBadge} {
		if cleanSource(s) != s {
			t.Errorf("source %s changed", s)
		}
	}
	if cleanSource("other") != "" {
		t.Error("unknown source kept")
	}
	for _, p := range []string{"/media/p/", "/media/p/a/b.png", "/media/p/a.png?x", "/media/u/a.png"} {
		if cleanImagePath(p) != "" {
			t.Errorf("path %q accepted", p)
		}
	}
}

func TestSnapshotDataRoundTrip(t *testing.T) {
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	s := Snapshot{
		Name: "g", Online: intp(3), OnlineSource: SourceInvite,
		Users:           []Member{{Name: "secret"}},
		InviteFetchedAt: &now, State: StateLive, ErrCode: "x", FetchedAt: now,
	}
	data, err := EncodeSnapshotData(s)
	if err != nil {
		t.Fatal(err)
	}
	str := string(data)
	if strings.Contains(str, "secret") || strings.Contains(str, "users") || !strings.Contains(str, `"channels":[]`) {
		t.Errorf("data = %s", str)
	}
	code := "discord_403_50004"
	row, err := SnapshotFromRow(SnapshotRow{Data: data, State: "static", ErrCode: &code, FetchedAt: &now})
	if err != nil {
		t.Fatal(err)
	}
	if row.Name != "g" || *row.Online != 3 || row.State != StateStatic || row.ErrCode != code || !row.FetchedAt.Equal(now) || row.Users != nil {
		t.Errorf("row = %+v", row)
	}
	for _, empty := range []string{"", "{}", "null", "  "} {
		got, err := DecodeSnapshotData([]byte(empty))
		if err != nil || got.Channels == nil || got.Name != "" {
			t.Errorf("DecodeSnapshotData(%q) = %+v, %v", empty, got, err)
		}
	}
	got, err := DecodeSnapshotData([]byte(`{"name":"n","channels":null,"extra":1}`))
	if err != nil || got.Channels == nil || got.Name != "n" {
		t.Errorf("decode = %+v %v", got, err)
	}
	if _, err := DecodeSnapshotData([]byte(`[1]`)); err == nil {
		t.Error("array must fail")
	}
	if _, err := SnapshotFromRow(SnapshotRow{Data: []byte(`{`)}); err == nil {
		t.Error("bad row must fail")
	}
	pending, _ := SnapshotFromRow(SnapshotRow{Data: []byte(`{}`), State: "weird"})
	if pending.State != StatePending {
		t.Errorf("unknown state = %s", pending.State)
	}
}

func TestErrCodeAndConfig(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{
		{&FetchError{Code: "discord_429"}, "discord_429"},
		{fmt.Errorf("wrap: %w", &RetryAfterError{Err: &FetchError{Code: "kook_500", Err: errors.New("x")}}), "kook_500"},
		{&FetchError{Code: "Bad-Code"}, CodeFetchFailed},
		{errors.New("plain"), CodeFetchFailed},
	}
	for _, tt := range tests {
		if got := ErrCode(tt.err); got != tt.want {
			t.Errorf("ErrCode(%v) = %q, want %q", tt.err, got, tt.want)
		}
	}
	if !strings.Contains((&FetchError{Code: "c", Err: errors.New("boom")}).Error(), "boom") ||
		(&FetchError{Code: "c"}).Error() != "provider: c" {
		t.Error("FetchError.Error changed")
	}
	cfgTests := []struct {
		raw string
		ok  bool
	}{
		{"", true},
		{"null", true},
		{" {} ", true},
		{`{"a":1}`, false},
		{`[]`, false},
		{`{} {}`, false},
		{`"x"`, false},
	}
	for _, tt := range cfgTests {
		err := ValidateEmptyConfig([]byte(tt.raw))
		if (err == nil) != tt.ok || (err != nil && !errors.Is(err, ErrInvalidConfig)) {
			t.Errorf("ValidateEmptyConfig(%q) = %v", tt.raw, err)
		}
	}
}

func TestEffectiveState(t *testing.T) {
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	recent, old := now.Add(-10*time.Minute), now.Add(-time.Hour)
	tests := []struct {
		state  State
		lastOK *time.Time
		want   State
	}{
		{StateLive, &recent, StateLive},
		{StateLive, &old, StateStale},
		{StateDegraded, &old, StateStale},
		{StateLive, nil, StateStale},
		{StateStatic, &old, StateStatic},
		{StatePending, nil, StatePending},
	}
	for _, tt := range tests {
		if got := EffectiveState(tt.state, tt.lastOK, 5*time.Minute, now); got != tt.want {
			t.Errorf("EffectiveState(%s) = %s, want %s", tt.state, got, tt.want)
		}
	}
	if StaleAfter(time.Minute) != 15*time.Minute || StaleAfter(time.Hour) != 3*time.Hour {
		t.Error("StaleAfter changed")
	}
}
