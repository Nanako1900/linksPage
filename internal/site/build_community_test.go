package site

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/Nanako1900/linksPage/internal/provider"
	"github.com/Nanako1900/linksPage/internal/store/dbq"
)

func TestPickSnapshot(t *testing.T) {
	const id = "c1"
	t0 := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	lastOK := t0.Add(-time.Hour)
	members := []provider.Member{{Name: "alice", Status: "online"}}
	row := func(state string, fetched time.Time) *dbq.ProviderSnapshot {
		return &dbq.ProviderSnapshot{
			Data: []byte(`{"name":"Row","channels":[]}`), State: state,
			FetchedAt: pgtype.Timestamptz{Time: fetched, Valid: true},
			LastOkAt:  pgtype.Timestamptz{Time: lastOK, Valid: true},
		}
	}
	mem := func(state provider.State, fetched time.Time) *provider.Snapshot {
		return &provider.Snapshot{Name: "Mem", State: state, FetchedAt: fetched, Channels: []provider.Channel{}, Users: members}
	}
	tests := []struct {
		name       string
		row        *dbq.ProviderSnapshot
		mem        *provider.Snapshot
		wantName   string
		wantUsers  int
		wantLastOK *time.Time
	}{
		{"stale after a failure keeps members (same fetched_at)", row("stale", t0), mem(provider.StateStale, t0), "Mem", 1, &lastOK},
		{"newer row from another instance wins", row("live", t0), mem(provider.StateLive, t0.Add(-time.Minute)), "Row", 0, &lastOK},
		{"newer live memory advances last success", row("live", t0), mem(provider.StateLive, t0.Add(time.Minute)), "Mem", 1, ptrTime(t0.Add(time.Minute))},
		{"memory only", nil, mem(provider.StateLive, t0), "Mem", 1, &t0},
		{"row only", row("live", t0), nil, "Row", 0, &lastOK},
		{"invalid memory state is ignored", row("live", t0), mem(provider.State("bogus"), t0), "Row", 0, &lastOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			live := provider.NewLiveStore()
			if tt.mem != nil {
				live.Put(id, *tt.mem)
			}
			got, err := pickSnapshot(id, tt.row, live)
			if err != nil {
				t.Fatal(err)
			}
			if got.snap.Name != tt.wantName || len(got.snap.Users) != tt.wantUsers {
				t.Errorf("picked %q with %d users, want %q with %d", got.snap.Name, len(got.snap.Users), tt.wantName, tt.wantUsers)
			}
			if got.lastOK == nil || !got.lastOK.Equal(*tt.wantLastOK) {
				t.Errorf("lastOK = %v, want %v", got.lastOK, *tt.wantLastOK)
			}
		})
	}
}

func ptrTime(t time.Time) *time.Time { return &t }

// A stale snapshot outlives its widget instant invite: once the invite is
// past its expiry the card has no join target (and /go agrees, since both
// use provider.JoinTarget).
func TestJoinTargetInstantInviteExpiry(t *testing.T) {
	t0 := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	const instant = "https://discord.com/invite/tmp"
	fallback := "https://example.com/join"
	permanent := "https://discord.gg/dead"
	past, future := t0.Add(-time.Minute), t0.Add(time.Hour)
	tests := []struct {
		name     string
		fallback *string
		expires  *time.Time
		want     string
	}{
		{"unexpired instant invite", nil, &future, instant},
		{"expired instant invite, no fallback", nil, &past, ""},
		{"expired instant invite uses fallback", &fallback, &past, fallback},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := communityInput{
				row:      dbq.Community{Provider: "discord", InviteUrl: &permanent, FallbackUrl: tt.fallback},
				platform: provider.Platform{ID: "discord", Card: provider.CardDiscord},
				now:      t0,
			}
			snap := provider.Snapshot{InviteInvalid: true, InstantInviteURL: instant, InviteExpiresAt: tt.expires}
			if got := joinTarget(in, provider.StateStale, snap); got != tt.want {
				t.Errorf("joinTarget = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEffectiveStateOnPage(t *testing.T) {
	t0 := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	hour := pgtype.Interval{Microseconds: time.Hour.Microseconds(), Valid: true}
	tests := []struct {
		name     string
		state    provider.State
		lastOK   *time.Time
		interval pgtype.Interval
		want     provider.State
	}{
		{"recent success stays live", provider.StateLive, ptrTime(t0.Add(-10 * time.Minute)), pgtype.Interval{}, provider.StateLive},
		{"no success for 15+ minutes is stale", provider.StateLive, ptrTime(t0.Add(-16 * time.Minute)), pgtype.Interval{}, provider.StateStale},
		{"degraded ages to stale too", provider.StateDegraded, ptrTime(t0.Add(-time.Hour)), pgtype.Interval{}, provider.StateStale},
		{"long refresh interval allows 3 intervals", provider.StateLive, ptrTime(t0.Add(-2 * time.Hour)), hour, provider.StateLive},
		{"beyond 3 long intervals is stale", provider.StateLive, ptrTime(t0.Add(-4 * time.Hour)), hour, provider.StateStale},
		{"live without any success is stale", provider.StateLive, nil, pgtype.Interval{}, provider.StateStale},
		{"static is untouched", provider.StateStatic, nil, pgtype.Interval{}, provider.StateStatic},
		{"unavailable is untouched", provider.StateUnavailable, ptrTime(t0.Add(-48 * time.Hour)), pgtype.Interval{}, provider.StateUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := communityInput{
				row:  dbq.Community{Provider: "discord", RefreshInterval: tt.interval},
				snap: &liveSnapshot{snap: provider.Snapshot{State: tt.state}, lastOK: tt.lastOK},
				now:  t0,
			}
			if got := effectiveState(in, tt.state); got != tt.want {
				t.Errorf("effectiveState = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestRefreshInterval(t *testing.T) {
	tests := []struct {
		in   pgtype.Interval
		want time.Duration
	}{
		{pgtype.Interval{}, 0},
		{pgtype.Interval{Microseconds: 90 * time.Second.Microseconds(), Valid: true}, 90 * time.Second},
		{pgtype.Interval{Days: 1, Months: 1, Valid: true}, 31 * 24 * time.Hour},
	}
	for _, tt := range tests {
		if got := refreshInterval(tt.in); got != tt.want {
			t.Errorf("refreshInterval(%+v) = %s, want %s", tt.in, got, tt.want)
		}
	}
}
