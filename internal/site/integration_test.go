package site_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/Nanako1900/linksPage/internal/config"
	"github.com/Nanako1900/linksPage/internal/media"
	"github.com/Nanako1900/linksPage/internal/provider"
	"github.com/Nanako1900/linksPage/internal/seed"
	"github.com/Nanako1900/linksPage/internal/site"
	"github.com/Nanako1900/linksPage/internal/store"
	"github.com/Nanako1900/linksPage/internal/store/dbq"
)

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func migratedPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test skipped in -short mode")
	}
	testcontainers.SkipIfProviderIsNotHealthy(t)
	ctx := context.Background()
	ctr, err := postgres.Run(ctx, "postgres:18-alpine",
		postgres.WithDatabase("linkspage"), postgres.WithUsername("linkspage"),
		postgres.WithPassword("test-password"), postgres.BasicWaitStrategies())
	testcontainers.CleanupContainer(t, ctr)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	dsn, err := ctr.ConnectionString(ctx)
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatal(err)
	}
	pool, err := store.Connect(ctx, config.DB{
		Host: u.Hostname(), Port: port, User: "linkspage", Name: "linkspage",
		Password: config.Secret("test-password"), SSLMode: "disable",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	m, err := store.NewMigrator(pool, quiet())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := m.Close(); err != nil {
			t.Error(err)
		}
	}()
	if _, err := m.Up(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return pool
}

// hashIngester stores nothing; it returns well-formed keys (PNG for QR
// codes, WebP otherwise).
type hashIngester struct{}

func (hashIngester) Ingest(_ context.Context, r io.Reader, opts media.ProcessOptions) (media.Result, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return media.Result{}, err
	}
	sum := sha256.Sum256(data)
	ext, ct := "webp", "image/webp"
	if opts.Kind == media.KindQR {
		ext, ct = "png", "image/png"
	}
	return media.Result{Primary: media.Stored{Key: hex.EncodeToString(sum[:16]) + "." + ext, ContentType: ct, Bytes: len(data), Width: 300, Height: 300}}, nil
}

// pngAssets returns fixed, well-formed generated assets.
type pngAssets struct{}

func (pngAssets) Generate(context.Context, media.GenerateInput) (media.Generated, error) {
	fav := map[int]media.Stored{}
	for i, size := range media.FaviconSizes {
		key := hex.EncodeToString([]byte{byte(i), 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}) + ".png"
		fav[size] = media.Stored{Key: key, ContentType: "image/png", Bytes: 10, Width: size, Height: size}
	}
	og := media.Stored{Key: "abcdefabcdefabcdefabcdefabcdefab.jpg", ContentType: "image/jpeg", Bytes: 10, Width: 1200, Height: 630}
	return media.Generated{OGImage: og, Favicons: fav, Files: media.SiteFiles{ETag: `"x"`}}, nil
}

func seedExample(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "config", "seed.example.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "images"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{"seed.yaml": raw, filepath.Join("images", "wechat-qr.png"): []byte("qr")} {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	im, err := seed.NewImporter(seed.Deps{DB: pool, Media: hashIngester{}, Logger: quiet()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := im.Import(ctx, filepath.Join(dir, "seed.yaml")); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

func communityBySlug(t *testing.T, p *site.PublicPage, slug string) site.CommunityView {
	t.Helper()
	for _, c := range p.Communities {
		if c.Slug == slug {
			return c
		}
	}
	t.Fatalf("community %q missing", slug)
	return site.CommunityView{}
}

func TestIntegrationBuildFromDatabase(t *testing.T) {
	pool := migratedPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	seedExample(ctx, t, pool)
	q := dbq.New(pool)

	discordRow, err := q.GetCommunityBySlug(ctx, "discord")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	data, err := json.Marshal(provider.Snapshot{Online: ptr(13), Members: ptr(125), OnlineSource: provider.SourceInvite, Channels: []provider.Channel{}})
	if err != nil {
		t.Fatal(err)
	}
	ok := pgtype.Timestamptz{Time: now.Add(-time.Minute), Valid: true}
	if err := q.UpsertProviderSnapshot(ctx, dbq.UpsertProviderSnapshotParams{
		CommunityID: discordRow.ID, Data: data, State: string(provider.StateLive),
		FetchedAt: ok, LastOkAt: ok, NextFetchAt: pgtype.Timestamptz{Time: now.Add(5 * time.Minute), Valid: true},
	}); err != nil {
		t.Fatal(err)
	}
	live := provider.NewLiveStore()
	live.Put(discordRow.ID.String(), provider.Snapshot{
		Online: ptr(14), OnlineSource: provider.SourceInvite, State: provider.StateLive, FetchedAt: now,
		Users: []provider.Member{{Name: "user1", Status: "online"}, {Name: "spammer", Status: "idle"}},
	})

	clock := now
	catalog, err := provider.LoadPresets()
	if err != nil {
		t.Fatal(err)
	}
	b, err := site.NewBuilder(site.BuilderDeps{
		Queries: q, Live: live, Platforms: catalog, Assets: pngAssets{},
		BaseURL: "https://links.example.com", Logger: quiet(), Now: func() time.Time { return clock },
	})
	if err != nil {
		t.Fatal(err)
	}
	h := site.NewHolder(site.DefaultSnapshot())
	if err := b.Rebuild(ctx, h); err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	p := h.Current().Public
	if err := p.Validate(); err != nil {
		t.Fatalf("page from the database violates the contract:\n%v", err)
	}
	if len(p.Blocks) != 9 || *p.Blocks[0].Count != 4 || len(p.Communities) != 4 || len(p.Links) != 3 || p.Version != 2 {
		t.Fatalf("page shape: blocks=%d communities=%d links=%d version=%d", len(p.Blocks), len(p.Communities), len(p.Links), p.Version)
	}
	d := communityBySlug(t, p, "discord")
	if *d.Live.Online != 14 || len(d.Live.Users) != 1 || *d.Live.JoinURL != "/go/discord" || d.Live.UpdatedAt == nil {
		t.Errorf("discord live = %+v", d.Live)
	}
	if k := communityBySlug(t, p, "kook"); k.Live.State != provider.StatePending || *k.Live.JoinURL != "/go/kook" {
		t.Errorf("kook = %+v", k.Live)
	}
	if w := communityBySlug(t, p, "wechat"); w.Live.State != provider.StateQROnly || w.QR == nil || w.QR.Width != 300 {
		t.Errorf("wechat = %+v", w)
	}
	if qq := communityBySlug(t, p, "qq"); qq.QQ.GroupNumber != "123456789" || qq.Live.JoinURL != nil {
		t.Errorf("qq = %+v", qq)
	}
	if h.Current().Files == nil || h.Current().Head.Icons[32] == "" {
		t.Error("generated assets missing")
	}
	assertVisibilityWindow(ctx, t, q, b, h, &clock)
}

// assertVisibilityWindow schedules a block and checks NextBoundary and
// RebuildIfDue.
func assertVisibilityWindow(ctx context.Context, t *testing.T, q *dbq.Queries, b *site.Builder, h *site.Holder, clock *time.Time) {
	t.Helper()
	from := clock.Add(time.Hour)
	to := clock.Add(2 * time.Hour)
	data, err := json.Marshal(site.HeadingBlockData{Text: site.LocalizedText{"en": "Event"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.InsertBlock(ctx, dbq.InsertBlockParams{
		PageID: 1, Kind: "heading", Data: data, SortOrder: 1, Visible: true,
		VisibleFrom: pgtype.Timestamptz{Time: from, Valid: true}, VisibleTo: pgtype.Timestamptz{Time: to, Valid: true},
	}); err != nil {
		t.Fatal(err)
	}
	if err := b.Rebuild(ctx, h); err != nil {
		t.Fatal(err)
	}
	next, ok := h.Current().NextBoundary()
	if !ok || !next.Equal(from) || len(h.Current().Public.Blocks) != 9 {
		t.Fatalf("next boundary = %v %v blocks=%d", next, ok, len(h.Current().Public.Blocks))
	}
	*clock = from
	if err := b.RebuildIfDue(ctx, h); err != nil {
		t.Fatal(err)
	}
	cur := h.Current()
	if next, ok := cur.NextBoundary(); !ok || !next.Equal(to) || len(cur.Public.Blocks) != 10 || cur.Public.Blocks[0].Text["en"] != "Event" {
		t.Fatalf("after boundary: next=%v blocks=%d", next, len(cur.Public.Blocks))
	}
	*clock = to
	if err := b.RebuildIfDue(ctx, h); err != nil {
		t.Fatal(err)
	}
	if _, ok := h.Current().NextBoundary(); ok || len(h.Current().Public.Blocks) != 9 {
		t.Errorf("after window: blocks=%d", len(h.Current().Public.Blocks))
	}
}

func ptr[T any](v T) *T { return &v }
