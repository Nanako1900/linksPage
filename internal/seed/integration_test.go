package seed

import (
	"context"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/Nanako1900/linksPage/internal/config"
	"github.com/Nanako1900/linksPage/internal/site"
	"github.com/Nanako1900/linksPage/internal/store"
	"github.com/Nanako1900/linksPage/internal/store/dbq"
)

// migratedPool starts postgres:18-alpine, applies all migrations and
// returns a pool (skips without Docker or in -short mode).
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
	m, err := store.NewMigrator(pool, discard())
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

// copyExample copies seed.example.yaml plus a fake QR image into a temp dir.
func copyExample(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(examplePath)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "seed.yaml")
	writeFile(t, path, string(raw))
	writeFile(t, filepath.Join(dir, "images", "wechat-qr.png"), "qr")
	return path
}

func TestIntegrationImport(t *testing.T) {
	pool := migratedPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	ing := &fakeIngester{}
	im := newTestImporter(t, pool, ing)

	// An invalid seed is rejected before anything is written.
	bad := filepath.Join(t.TempDir(), "bad.yaml")
	writeFile(t, bad, "version: 1\ncommunities: [{ slug: a, platform: nope, name: { en: x } }]\n")
	if _, err := im.Import(ctx, bad); !errors.Is(err, ErrInvalidSeed) || !strings.Contains(err.Error(), "communities[0].platform") {
		t.Fatalf("invalid seed: %v", err)
	}

	path := copyExample(t)
	res, err := im.Import(ctx, path)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if res.Skipped || res.Communities != 4 || res.Links != 3 || res.Blocks != 9 || len(ing.calls) != 1 {
		t.Fatalf("result = %+v ingest calls = %d", res, len(ing.calls))
	}
	assertImported(ctx, t, dbq.New(pool))

	again, err := im.Import(ctx, path)
	if err != nil || !again.Skipped || len(ing.calls) != 1 {
		t.Fatalf("second import = %+v err=%v (must be skipped without processing images)", again, err)
	}
	// The transaction re-checks emptiness (a concurrent instance won).
	if res, err := im.write(ctx, &plan{}, nil); err != nil || !res.Skipped {
		t.Fatalf("write on non-empty content = %+v err=%v", res, err)
	}
}

func assertImported(ctx context.Context, t *testing.T, q *dbq.Queries) {
	t.Helper()
	settings, err := q.GetSiteSettings(ctx)
	if err != nil || settings.Version != 2 || settings.UpdatedBy == nil || *settings.UpdatedBy != actor {
		t.Fatalf("settings = %+v err=%v", settings, err)
	}
	s, err := site.ParseSettings(settings.Data)
	if err != nil || s.Title["zh-CN"] != "猎人小屋" || s.Copy["zh-CN"]["openInBrowser"] == "" {
		t.Errorf("parsed settings = %+v err=%v", s, err)
	}
	communities, err := q.ListCommunities(ctx, 1)
	if err != nil || len(communities) != 4 {
		t.Fatalf("communities = %d err=%v", len(communities), err)
	}
	qr, err := q.ListQRCodes(ctx, 1)
	if err != nil || len(qr) != 1 || qr[0].ContentType != "image/png" || string(qr[0].Note) == "{}" {
		t.Fatalf("qr = %+v err=%v", qr, err)
	}
	due, err := q.ListDueCommunities(ctx, dbq.ListDueCommunitiesParams{Now: pgtype.Timestamptz{Time: time.Now().Add(time.Minute), Valid: true}, MaxRows: 10})
	if err != nil || len(due) != 2 {
		t.Fatalf("due = %d err=%v (discord and kook)", len(due), err)
	}
	blocks, err := q.ListVisibleBlocks(ctx, dbq.ListVisibleBlocksParams{PageID: 1, Now: pgtype.Timestamptz{Time: time.Now(), Valid: true}})
	if err != nil || len(blocks) != 9 || blocks[0].Kind != "heading" || blocks[8].Kind != "social_row" {
		t.Fatalf("blocks = %d err=%v", len(blocks), err)
	}
	var row site.SocialRowBlockData
	if err := site.DecodeStrict(blocks[8].Data, &row); err != nil || len(row.LinkIDs) != 2 {
		t.Errorf("social row = %+v err=%v", row, err)
	}
	links, err := q.ListLinks(ctx, 1)
	if err != nil || len(links) != 3 {
		t.Fatalf("links = %d err=%v", len(links), err)
	}
}

func TestIntegrationImportImagesAndCustomPlatforms(t *testing.T) {
	pool := migratedPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	dir := t.TempDir()
	for _, name := range []string{"avatar.png", "og.jpg", "hb.webp", "icon.png"} {
		writeFile(t, filepath.Join(dir, "img", name), name)
	}
	path := filepath.Join(dir, "seed.yaml")
	writeFile(t, path, `version: 1
site:
  avatar: img/avatar.png
  og: { image: img/og.jpg }
platforms:
  - { id: heybox, name: { zh-CN: 黑盒语音 }, icon: img/hb.webp }
communities:
  - { slug: hb, platform: heybox, name: { en: HB }, icon: img/icon.png, fallback_url: "https://heybox.example/" }
links:
  - { slug: me, kind: social, label: { en: Me }, url: "https://mastodon.social/@me", icon: img/icon.png, rel_me: true }
blocks:
  - { community: hb, visible_from: 2026-01-01T00:00:00Z, visible_to: 2036-01-01T00:00:00Z }
  - { social_row: [me] }
`)
	im := newTestImporter(t, pool, &fakeIngester{})
	if _, err := im.Import(ctx, path); err != nil {
		t.Fatalf("import: %v", err)
	}
	q := dbq.New(pool)
	s, err := q.GetSiteSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := site.ParseSettings(s.Data)
	if err != nil || !site.MediaKeyRe.MatchString(parsed.AvatarKey) || !strings.HasSuffix(parsed.OG.ImageKey, ".jpg") {
		t.Fatalf("settings = %+v err=%v", parsed, err)
	}
	avatar, err := q.GetMedia(ctx, parsed.AvatarKey)
	if err != nil || avatar.Kind != "avatar" || !strings.Contains(string(avatar.Variants), ".png") {
		t.Errorf("avatar media = %+v err=%v", avatar, err)
	}
	platforms, err := q.ListCustomPlatforms(ctx)
	if err != nil || len(platforms) != 1 || platforms[0].IconKey == nil {
		t.Fatalf("platforms = %+v err=%v", platforms, err)
	}
	links, err := q.ListLinks(ctx, 1)
	if err != nil || len(links) != 1 || links[0].IconKey == nil || !links[0].RelMe {
		t.Fatalf("links = %+v err=%v", links, err)
	}
}
