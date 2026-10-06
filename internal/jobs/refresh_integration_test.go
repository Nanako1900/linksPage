package jobs

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Nanako1900/linksPage/internal/imgproxy"
	"github.com/Nanako1900/linksPage/internal/provider"
	"github.com/Nanako1900/linksPage/internal/provider/discord"
	"github.com/Nanako1900/linksPage/internal/provider/kook"
	"github.com/Nanako1900/linksPage/internal/provider/providertest"
	"github.com/Nanako1900/linksPage/internal/store"
	"github.com/Nanako1900/linksPage/internal/store/dbq"
)

// fixtureUpstream serves recorded Discord/KOOK fixtures; failing makes
// every request answer 502.
type fixtureUpstream struct {
	srv     *httptest.Server
	failing bool
}

func newFixtureUpstream(t *testing.T) *fixtureUpstream {
	t.Helper()
	load := func(dir, name, bodyExt string) (int, http.Header, []byte) {
		rec, err := providertest.LoadHeaders(filepath.Join("..", "provider", dir, "testdata", name+".headers"))
		if err != nil {
			t.Fatal(err)
		}
		var body []byte
		if bodyExt != "" {
			if body, err = os.ReadFile(filepath.Join("..", "provider", dir, "testdata", name+bodyExt)); err != nil {
				t.Fatal(err)
			}
		}
		rec.Header.Del("Content-Length")
		return rec.Status, rec.Header, body
	}
	routes := map[string]string{
		"/api/guilds/1114391825336250432/widget.json":           "discord/widget_ok/.json",
		"/api/v10/invites/KwdRuAkT":                             "discord/invite_ok/.json",
		"/api/v3/badge/guild?guild_id=5417470909511807&style=0": "kook/badge_public_style0/",
		"/api/v3/badge/guild?guild_id=5417470909511807&style=2": "kook/badge_public_style2/",
	}
	u := &fixtureUpstream{}
	u.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Path
		if r.URL.Path == "/api/v3/badge/guild" {
			key += "?" + r.URL.RawQuery
		}
		route, ok := routes[key]
		if u.failing || !ok {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		parts := strings.Split(route, "/")
		status, hdr, body := load(parts[0], parts[1], parts[2])
		for k, vs := range hdr {
			w.Header()[k] = vs
		}
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}))
	t.Cleanup(u.srv.Close)
	return u
}

func TestIntegrationRefreshJob(t *testing.T) {
	dsn := startPostgres(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	logger := slog.New(slog.DiscardHandler)
	mig, err := store.NewMigrator(pool, logger)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mig.Up(ctx); err != nil {
		t.Fatal(err)
	}
	_ = mig.Close()
	q := dbq.New(pool)
	discordID := insertCommunity(ctx, t, q, "discord", "discord", "1114391825336250432", "https://discord.gg/KwdRuAkT")
	kookID := insertCommunity(ctx, t, q, "kook", "kook", "5417470909511807", "")

	up := newFixtureUpstream(t)
	client, err := provider.NewHTTPClient(provider.ClientOptions{UserAgent: provider.UserAgent("test")})
	if err != nil {
		t.Fatal(err)
	}
	registrar, err := imgproxy.NewRegistrar(q, []byte(strings.Repeat("m", 32)),
		map[string]map[string][]string{"discord": discord.ImageHosts(), "kook": {}})
	if err != nil {
		t.Fatal(err)
	}
	dp, err := discord.NewProvider(discord.Options{APIBase: up.srv.URL, Client: client, Images: registrar})
	if err != nil {
		t.Fatal(err)
	}
	kp, err := kook.NewProvider(kook.Options{APIBase: up.srv.URL, Client: client})
	if err != nil {
		t.Fatal(err)
	}
	reg, err := provider.NewRegistry(dp, kp)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	changes := 0
	live := provider.NewLiveStore()
	job, err := NewRefreshJob(RefreshDeps{
		Store: q, Registry: reg, Live: live, Logger: logger,
		OnChange: func(context.Context) { changes++ }, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := job.Run(ctx); err != nil {
		t.Fatal(err)
	}
	ds := mustSnapshot(ctx, t, q, discordID)
	ks := mustSnapshot(ctx, t, q, kookID)
	if ds.State != "live" || ks.State != "live" || changes != 1 {
		t.Fatalf("states = %s %s changes=%d", ds.State, ks.State, changes)
	}
	if !ds.NextFetchAt.Time.Equal(now.Add(10*time.Minute)) || !ks.NextFetchAt.Time.Equal(now.Add(10*time.Minute)) {
		t.Errorf("next fetch = %s / %s", ds.NextFetchAt.Time.Sub(now), ks.NextFetchAt.Time.Sub(now))
	}
	snap, err := provider.DecodeSnapshotData(ds.Data)
	if err != nil || snap.Name == "" || *snap.Members != 125 || !strings.HasPrefix(snap.IconPath, "/media/p/") {
		t.Errorf("discord data = %+v %v", snap, err)
	}
	key := strings.TrimSuffix(strings.TrimPrefix(snap.IconPath, imgproxy.PathPrefix), ".png")
	if row, err := q.GetMediaProxy(ctx, key); err != nil || row.Kind != "icon" || row.Provider != "discord" {
		t.Errorf("media proxy row = %+v %v", row, err)
	}
	if mem, _ := live.Live(discordID.String()); len(mem.Users) != 16 || !strings.HasPrefix(mem.Users[0].AvatarPath, "/media/p/") {
		t.Errorf("in-memory users = %d", len(mem.Users))
	}
	due, err := q.ListDueCommunities(ctx, dbq.ListDueCommunitiesParams{Now: timestamptz(now), MaxRows: 10})
	if err != nil || len(due) != 0 {
		t.Errorf("due after refresh = %d %v", len(due), err)
	}

	// Upstream outage after the interval: data kept, state stale, backoff.
	up.failing = true
	now = now.Add(11 * time.Minute)
	if err := job.Run(ctx); err != nil {
		t.Fatal(err)
	}
	ds = mustSnapshot(ctx, t, q, discordID)
	kept, err := provider.DecodeSnapshotData(ds.Data)
	if err != nil || ds.State != "stale" || ds.FailCount != 1 || deref(ds.ErrCode) != "discord_502_0" ||
		!ds.LastOkAt.Time.Equal(now.Add(-11*time.Minute)) || kept.Members == nil || *kept.Members != 125 {
		t.Errorf("stale row = state %s fail %d code %q data %s", ds.State, ds.FailCount, deref(ds.ErrCode), ds.Data)
	}
	if !ds.NextFetchAt.Time.Equal(now.Add(10 * time.Minute)) {
		t.Errorf("backoff = %s", ds.NextFetchAt.Time.Sub(now))
	}
}

func insertCommunity(ctx context.Context, t *testing.T, q *dbq.Queries, slug, prov, external, invite string) pgtype.UUID {
	t.Helper()
	var inviteURL *string
	if invite != "" {
		inviteURL = &invite
	}
	id, err := q.InsertCommunity(ctx, dbq.InsertCommunityParams{
		PageID: 1, Slug: slug, Provider: prov, Platform: prov, ExternalID: &external, Config: []byte(`{}`),
		Display: []byte(`{"name":{"en":"` + slug + `"}}`), InviteUrl: inviteURL,
		RefreshInterval: pgtype.Interval{Microseconds: int64(10 * time.Minute / time.Microsecond), Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func mustSnapshot(ctx context.Context, t *testing.T, q *dbq.Queries, id pgtype.UUID) dbq.ProviderSnapshot {
	t.Helper()
	s, err := q.GetProviderSnapshot(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
