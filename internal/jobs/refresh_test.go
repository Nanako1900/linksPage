package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/Nanako1900/linksPage/internal/provider"
	"github.com/Nanako1900/linksPage/internal/store/dbq"
)

var t0 = time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)

func uuid(b byte) pgtype.UUID {
	return pgtype.UUID{Bytes: [16]byte{b}, Valid: true}
}

// memStore is an in-memory communities + provider_snapshots store.
type memStore struct {
	mu        sync.Mutex
	due       []dbq.ListDueCommunitiesRow
	snaps     map[pgtype.UUID]dbq.ProviderSnapshot
	listErr   error
	getErr    error
	upsertErr error
	upserts   []dbq.UpsertProviderSnapshotParams
}

func newMemStore(rows ...dbq.ListDueCommunitiesRow) *memStore {
	return &memStore{due: rows, snaps: map[pgtype.UUID]dbq.ProviderSnapshot{}}
}

func (m *memStore) ListDueCommunities(_ context.Context, arg dbq.ListDueCommunitiesParams) ([]dbq.ListDueCommunitiesRow, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	if !arg.Now.Valid || arg.MaxRows != RefreshBatch {
		return nil, errors.New("bad params")
	}
	return m.due, nil
}

func (m *memStore) GetProviderSnapshot(_ context.Context, id pgtype.UUID) (dbq.ProviderSnapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.getErr != nil {
		return dbq.ProviderSnapshot{}, m.getErr
	}
	s, ok := m.snaps[id]
	if !ok {
		return dbq.ProviderSnapshot{}, pgx.ErrNoRows
	}
	return s, nil
}

func (m *memStore) UpsertProviderSnapshot(_ context.Context, arg dbq.UpsertProviderSnapshotParams) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.upsertErr != nil {
		return m.upsertErr
	}
	m.upserts = append(m.upserts, arg)
	m.snaps[arg.CommunityID] = dbq.ProviderSnapshot(arg)
	return nil
}

func (m *memStore) last(t *testing.T) dbq.UpsertProviderSnapshotParams {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.upserts) == 0 {
		t.Fatal("no upsert")
	}
	return m.upserts[len(m.upserts)-1]
}

// stubProvider returns queued results.
type stubProvider struct {
	kind     string
	mu       sync.Mutex
	results  []stubResult
	prevSeen []*provider.Snapshot
	cfgErr   error
}

type stubResult struct {
	snap *provider.Snapshot
	err  error
}

func (p *stubProvider) Kind() string                        { return p.kind }
func (p *stubProvider) Capabilities() provider.Capabilities { return provider.Capabilities{} }
func (p *stubProvider) MinInterval() time.Duration          { return 5 * time.Minute }
func (p *stubProvider) ImageHosts() map[string][]string     { return nil }
func (p *stubProvider) ValidateConfig(in provider.ConfigInput) (any, error) {
	if p.cfgErr != nil {
		return nil, p.cfgErr
	}
	return in.ExternalID, nil
}

func (p *stubProvider) Fetch(_ context.Context, in provider.FetchInput) (*provider.Snapshot, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.prevSeen = append(p.prevSeen, in.Previous)
	r := p.results[0]
	if len(p.results) > 1 {
		p.results = p.results[1:]
	}
	return r.snap, r.err
}

func liveSnap(online int) *provider.Snapshot {
	return &provider.Snapshot{
		Name: "Guild", Online: &online, OnlineSource: provider.SourceWidget,
		Users: []provider.Member{{Name: "u", Status: "online"}}, State: provider.StateLive,
	}
}

func dueRow(id byte, prov string, fail *int32) dbq.ListDueCommunitiesRow {
	ext := "1114391825336250432"
	return dbq.ListDueCommunitiesRow{
		ID: uuid(id), Slug: "c" + string(rune('a'+id)), Provider: prov, ExternalID: &ext, Config: []byte(`{}`),
		RefreshInterval: pgtype.Interval{Microseconds: int64(time.Minute / time.Microsecond), Valid: true},
		FailCount:       fail, LastOkAt: pgtype.Timestamptz{Time: t0.Add(-time.Hour), Valid: true},
	}
}

type refreshEnv struct {
	store   *memStore
	stub    *stubProvider
	live    *provider.LiveStore
	job     Job
	changes int
	logs    *syncBuffer
	now     time.Time
}

func newRefreshEnv(t *testing.T, store *memStore, results ...stubResult) *refreshEnv {
	t.Helper()
	stub := &stubProvider{kind: "discord", results: results}
	reg, err := provider.NewRegistry(stub)
	if err != nil {
		t.Fatal(err)
	}
	logger, logs := testLogger()
	env := &refreshEnv{store: store, stub: stub, live: provider.NewLiveStore(), logs: logs, now: t0}
	job, err := NewRefreshJob(RefreshDeps{
		Store: store, Registry: reg, Live: env.live, Logger: logger,
		OnChange: func(context.Context) { env.changes++ },
		Now:      func() time.Time { return env.now },
	})
	if err != nil {
		t.Fatal(err)
	}
	env.job = job
	return env
}

func TestRefreshJobShape(t *testing.T) {
	env := newRefreshEnv(t, newMemStore(), stubResult{})
	if env.job.Name != RefreshJobName || !env.job.LeaderOnly || env.job.Interval != RefreshTick || env.job.Timeout <= 0 {
		t.Errorf("job = %+v", env.job)
	}
	if _, err := NewRefreshJob(RefreshDeps{}); err == nil {
		t.Error("missing deps must fail")
	}
	reg, _ := provider.NewRegistry()
	if _, err := NewRefreshJob(RefreshDeps{Store: newMemStore(), Registry: reg, Live: provider.NewLiveStore()}); err != nil {
		t.Errorf("defaults: %v", err)
	}
}

func TestRefreshSuccessPersistsWithoutUsers(t *testing.T) {
	store := newMemStore(dueRow(1, "discord", nil))
	env := newRefreshEnv(t, store, stubResult{snap: liveSnap(5)})
	if err := env.job.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	up := store.last(t)
	if up.State != "live" || up.FailCount != 0 || up.ErrCode != nil || !up.LastOkAt.Time.Equal(t0) ||
		!up.NextFetchAt.Time.Equal(t0.Add(5*time.Minute)) {
		t.Errorf("upsert = %+v", up)
	}
	if strings.Contains(string(up.Data), "users") || strings.Contains(string(up.Data), `"u"`) {
		t.Errorf("users persisted: %s", up.Data)
	}
	var stored map[string]any
	if err := json.Unmarshal(up.Data, &stored); err != nil || stored["name"] != "Guild" {
		t.Errorf("data = %s", up.Data)
	}
	got, ok := env.live.Live(uuid(1).String())
	if !ok || len(got.Users) != 1 || got.State != provider.StateLive || !got.FetchedAt.Equal(t0) {
		t.Errorf("live = %+v", got)
	}
	if env.changes != 1 || env.stub.prevSeen[0] != nil {
		t.Errorf("changes=%d prev=%v", env.changes, env.stub.prevSeen[0])
	}
	// Same data again: nothing changed, previous comes from memory.
	env.now = t0.Add(5 * time.Minute)
	if err := env.job.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if env.changes != 1 || env.stub.prevSeen[1] == nil || len(env.stub.prevSeen[1].Users) != 1 {
		t.Errorf("unchanged run: changes=%d prev=%+v", env.changes, env.stub.prevSeen[1])
	}
}

func TestRefreshFailures(t *testing.T) {
	two := int32(2)
	boom := &provider.FetchError{Code: "discord_502_0", Err: errors.New("bad gateway")}
	tests := []struct {
		name      string
		prevState string
		fail      *int32
		err       error
		wantState string
		wantDelay time.Duration
		wantWarn  bool
	}{
		{"never fetched stays pending", "", nil, boom, "pending", 5 * time.Minute, false},
		{"pending row stays pending", "pending", nil, boom, "pending", 5 * time.Minute, false},
		{"live becomes stale", "live", nil, boom, "stale", 5 * time.Minute, false},
		{"degraded becomes stale", "degraded", &two, boom, "stale", 20 * time.Minute, true},
		{"static keeps state", "static", nil, boom, "static", 5 * time.Minute, false},
		{"retry-after wins", "live", nil, &provider.RetryAfterError{After: 30 * time.Minute, Err: boom}, "stale", 30 * time.Minute, false},
		{"nil snapshot", "live", nil, nil, "stale", 5 * time.Minute, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newMemStore(dueRow(1, "discord", tt.fail))
			if tt.prevState != "" {
				store.snaps[uuid(1)] = dbq.ProviderSnapshot{
					CommunityID: uuid(1), Data: []byte(`{"name":"Old","channels":[]}`), State: tt.prevState,
				}
			}
			env := newRefreshEnv(t, store, stubResult{err: tt.err})
			if err := env.job.Run(context.Background()); err != nil {
				t.Fatal(err)
			}
			up := store.last(t)
			wantFail := int32(1)
			if tt.fail != nil {
				wantFail = *tt.fail + 1
			}
			if up.State != tt.wantState || up.FailCount != wantFail || !up.NextFetchAt.Time.Equal(t0.Add(tt.wantDelay)) ||
				!up.LastOkAt.Time.Equal(t0.Add(-time.Hour)) || up.ErrCode == nil {
				t.Errorf("upsert = state %s fail %d next %s err %v", up.State, up.FailCount, up.NextFetchAt.Time.Sub(t0), up.ErrCode)
			}
			if tt.prevState != "" && tt.prevState != "pending" && !strings.Contains(string(up.Data), `"Old"`) {
				t.Errorf("previous data must be kept: %s", up.Data)
			}
			if tt.prevState == "pending" && env.stub.prevSeen[0] != nil {
				t.Error("pending previous must be hidden from the provider")
			}
			if strings.Contains(env.logs.String(), "level=WARN") != tt.wantWarn {
				t.Errorf("warn = %v, logs: %s", !tt.wantWarn, env.logs.String())
			}
			live, ok := env.live.Live(uuid(1).String())
			if !ok || string(live.State) != tt.wantState {
				t.Errorf("live state = %s", live.State)
			}
			if !up.FetchedAt.Time.Equal(t0) || !live.FetchedAt.Equal(up.FetchedAt.Time) {
				t.Errorf("fetched_at: row %s, memory %s, want both %s", up.FetchedAt.Time, live.FetchedAt, t0)
			}
		})
	}
}

// A failed refresh must keep the in-memory member list: the stale copy in
// memory and the row share fetched_at, so the page builder prefers memory.
func TestRefreshFailureKeepsMembers(t *testing.T) {
	store := newMemStore(dueRow(1, "discord", nil))
	store.snaps[uuid(1)] = dbq.ProviderSnapshot{
		CommunityID: uuid(1), Data: []byte(`{"name":"Old","channels":[]}`), State: "live",
	}
	env := newRefreshEnv(t, store, stubResult{err: &provider.FetchError{Code: "discord_502_0", Err: errors.New("bad gateway")}})
	before := t0.Add(-5 * time.Minute)
	env.live.Put(uuid(1).String(), provider.Snapshot{
		Name: "Old", State: provider.StateLive, FetchedAt: before, Channels: []provider.Channel{},
		Users: []provider.Member{{Name: "alice", Status: "online"}},
	})
	if err := env.job.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	live, ok := env.live.Live(uuid(1).String())
	up := store.last(t)
	switch {
	case !ok || live.State != provider.StateStale:
		t.Fatalf("live = %+v", live)
	case len(live.Users) != 1 || live.Users[0].Name != "alice":
		t.Errorf("members lost: %+v", live.Users)
	case live.FetchedAt.Before(up.FetchedAt.Time):
		t.Errorf("memory fetched_at %s older than row %s: the builder would drop the members", live.FetchedAt, up.FetchedAt.Time)
	case !up.LastOkAt.Time.Equal(t0.Add(-time.Hour)):
		t.Errorf("last_ok_at = %s, must stay at the last success", up.LastOkAt.Time)
	}
}

func TestRefreshLogicalOutcomeAndConfig(t *testing.T) {
	store := newMemStore(dueRow(1, "discord", nil))
	unavailable := &provider.Snapshot{State: provider.StateUnavailable, ErrCode: "discord_404_10004"}
	env := newRefreshEnv(t, store, stubResult{snap: unavailable})
	if err := env.job.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	up := store.last(t)
	if up.State != "unavailable" || *up.ErrCode != "discord_404_10004" || up.FailCount != 0 || !strings.Contains(env.logs.String(), "reported a problem") {
		t.Errorf("upsert = %+v", up)
	}

	store2 := newMemStore(dueRow(2, "discord", nil))
	env2 := newRefreshEnv(t, store2, stubResult{})
	env2.stub.cfgErr = provider.ErrInvalidConfig
	if err := env2.job.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	up2 := store2.last(t)
	if up2.State != "static" || *up2.ErrCode != provider.CodeConfigInvalid || !up2.NextFetchAt.Time.Equal(t0.Add(MaxBackoff)) {
		t.Errorf("config upsert = %+v", up2)
	}

	store3 := newMemStore(dueRow(3, "telegram", nil))
	env3 := newRefreshEnv(t, store3, stubResult{})
	if err := env3.job.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	up3 := store3.last(t)
	if up3.State != "pending" || *up3.ErrCode != provider.CodeUnknownKind || !up3.NextFetchAt.Time.Equal(t0.Add(MaxBackoff)) {
		t.Errorf("unknown provider upsert = %+v", up3)
	}
}

func TestRefreshStoreErrors(t *testing.T) {
	store := newMemStore(dueRow(1, "discord", nil))
	store.listErr = errors.New("db down")
	env := newRefreshEnv(t, store, stubResult{snap: liveSnap(1)})
	if err := env.job.Run(context.Background()); err == nil {
		t.Error("list error must fail the run")
	}
	store.listErr = nil
	store.getErr = errors.New("get failed")
	if err := env.job.Run(context.Background()); err == nil || !strings.Contains(err.Error(), "community cb") {
		t.Errorf("get error = %v", err)
	}
	store.getErr = nil
	store.upsertErr = errors.New("upsert failed")
	if err := env.job.Run(context.Background()); err == nil {
		t.Error("upsert error must be reported")
	}
	env.stub.results = []stubResult{{err: errors.New("x")}}
	if err := env.job.Run(context.Background()); err == nil {
		t.Error("failed-upsert error must be reported")
	}
	if env.changes != 0 {
		t.Errorf("changes = %d", env.changes)
	}
	store.upsertErr = nil
	store.snaps[uuid(1)] = dbq.ProviderSnapshot{CommunityID: uuid(1), Data: []byte(`{`), State: "live"}
	if err := env.job.Run(context.Background()); err == nil {
		t.Error("corrupt snapshot must be reported")
	}
}

func TestRefreshLoadsPreviousFromDatabase(t *testing.T) {
	store := newMemStore(dueRow(1, "discord", nil))
	code := "discord_403_50004"
	store.snaps[uuid(1)] = dbq.ProviderSnapshot{
		CommunityID: uuid(1), Data: []byte(`{"name":"Db","channels":[]}`),
		State: "static", ErrCode: &code, FetchedAt: pgtype.Timestamptz{Time: t0.Add(-time.Hour), Valid: true},
	}
	env := newRefreshEnv(t, store, stubResult{snap: liveSnap(2)})
	if err := env.job.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	prev := env.stub.prevSeen[0]
	if prev == nil || prev.Name != "Db" || prev.State != provider.StateStatic || prev.ErrCode != code || !prev.FetchedAt.Equal(t0.Add(-time.Hour)) {
		t.Errorf("previous = %+v", prev)
	}
	if env.changes != 1 {
		t.Errorf("changes = %d", env.changes)
	}
}

func TestRetryDelay(t *testing.T) {
	five := 5 * time.Minute
	ra := &provider.RetryAfterError{After: 2 * time.Hour}
	tests := []struct {
		interval time.Duration
		fails    int32
		err      error
		want     time.Duration
	}{
		{five, 1, nil, five},
		{five, 2, nil, 10 * time.Minute},
		{five, 4, nil, 40 * time.Minute},
		{five, 5, nil, time.Hour},
		{five, 50, nil, time.Hour},
		{five, 1, ra, time.Hour},
		{2 * time.Hour, 3, nil, 2 * time.Hour},
	}
	for _, tt := range tests {
		if got := retryDelay(tt.interval, tt.fails, tt.err); got != tt.want {
			t.Errorf("retryDelay(%s, %d) = %s, want %s", tt.interval, tt.fails, got, tt.want)
		}
	}
}

func TestIntervalAndHelpers(t *testing.T) {
	iv := pgtype.Interval{Microseconds: 1_000_000, Days: 1, Months: 1, Valid: true}
	if got := intervalDuration(iv); got != time.Second+24*time.Hour+30*24*time.Hour {
		t.Errorf("interval = %s", got)
	}
	if intervalDuration(pgtype.Interval{}) != 0 || timePtr(pgtype.Timestamptz{}) != nil || optional("") != nil {
		t.Error("zero helpers")
	}
	exp := t0
	a := provider.Snapshot{Name: "x", InviteExpiresAt: &exp, FetchedAt: t0}
	b := provider.Snapshot{Name: "x", InviteExpiresAt: &exp, FetchedAt: t0.Add(time.Hour), Users: []provider.Member{}, Channels: []provider.Channel{}}
	if !sameLive(a, b) {
		t.Error("fetch time and empty slices must not count as changes")
	}
	b.Name = "y"
	if sameLive(a, b) {
		t.Error("name change must count")
	}
}
