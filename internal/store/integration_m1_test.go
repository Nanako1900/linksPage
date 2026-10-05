package store

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Nanako1900/linksPage/internal/store/dbq"
)

const (
	testAvatarKey = "0123456789abcdef0123456789abcdef.webp"
	testQRKey     = "fedcba9876543210fedcba9876543210.png"
	sqlStateCheck = "23514"
	sqlStateUniq  = "23505"
	sqlStateFK    = "23503"
	sqlStateRestr = "23001"
)

func strPtr(s string) *string { return &s }

func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }

func wantSQLState(t *testing.T, name string, err error, code string) {
	t.Helper()
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != code {
		t.Errorf("%s: err = %v, want SQLSTATE %s", name, err, code)
	}
}

// m1Fixture holds ids created by seedM1.
type m1Fixture struct {
	discord, kook, qq pgtype.UUID
	link              pgtype.UUID
	qr                pgtype.UUID
	now               time.Time
}

// assertM1Schema exercises the 00002 migration and the M1 queries.
func assertM1Schema(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	q := dbq.New(pool)
	empty, err := q.IsContentEmpty(ctx)
	if err != nil || !empty {
		t.Fatalf("IsContentEmpty before seed = %v err=%v", empty, err)
	}
	f := seedM1(ctx, t, q)
	if empty, err = q.IsContentEmpty(ctx); err != nil || empty {
		t.Fatalf("IsContentEmpty after seed = %v err=%v", empty, err)
	}
	assertM1Constraints(ctx, t, q, pool)
	assertM1Blocks(ctx, t, q, f)
	assertM1Snapshots(ctx, t, q, f)
	assertM1Media(ctx, t, q, f)
	assertM1Reads(ctx, t, q, f)
	assertM1GoVisibility(ctx, t, q, pool, f)
	assertM1Cascade(ctx, t, q, pool, f)
}

func seedM1(ctx context.Context, t *testing.T, q *dbq.Queries) m1Fixture {
	t.Helper()
	f := m1Fixture{now: time.Now()}
	must := func(name string, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	must("media avatar", q.InsertMedia(ctx, dbq.InsertMediaParams{
		Key: testAvatarKey, Kind: "avatar",
		ContentType: "image/webp", Bytes: 10, Width: 256, Height: 256, Variants: []byte(`{}`),
	}))
	must("media qr", q.InsertMedia(ctx, dbq.InsertMediaParams{
		Key: testQRKey, Kind: "qr",
		ContentType: "image/png", Bytes: 20, Width: 480, Height: 480, Variants: []byte(`{}`),
	}))
	must("media dup", q.InsertMedia(ctx, dbq.InsertMediaParams{
		Key: testQRKey, Kind: "qr",
		ContentType: "image/png", Bytes: 20, Width: 480, Height: 480, Variants: []byte(`{}`),
	}))
	var err error
	f.discord, err = q.InsertCommunity(ctx, dbq.InsertCommunityParams{
		PageID: 1, Slug: "discord", Provider: "discord",
		Platform: "discord", ExternalID: strPtr("1114391825336250432"), Config: []byte(`{}`),
		Display: []byte(`{"name":{"en":"Discord"}}`), IconKey: strPtr(testAvatarKey),
		InviteUrl: strPtr("https://discord.gg/KwdRuAkT"),
	})
	must("discord", err)
	f.kook, err = q.InsertCommunity(ctx, dbq.InsertCommunityParams{
		PageID: 1, Slug: "kook", Provider: "kook",
		Platform: "kook", ExternalID: strPtr("5417470909511807"), Config: []byte(`{}`),
		Display: []byte(`{"name":{"en":"KOOK"}}`),
	})
	must("kook", err)
	f.qq, err = q.InsertCommunity(ctx, dbq.InsertCommunityParams{
		PageID: 1, Slug: "qq", Provider: "static",
		Platform: "qq-group", Config: []byte(`{}`), Display: []byte(`{"name":{"en":"QQ"},"qqGroupNumber":"12345678"}`),
	})
	must("qq", err)
	f.qr, err = q.InsertQRCode(ctx, dbq.InsertQRCodeParams{CommunityID: f.qq, MediaKey: testQRKey, Note: []byte(`{"en":"7 days"}`)})
	must("qr", err)
	f.link, err = q.InsertLink(ctx, dbq.InsertLinkParams{
		PageID: 1, Slug: "blog", Kind: "link",
		Label: []byte(`{"en":"Blog"}`), Url: "https://example.com", Icon: strPtr("si:github"),
	})
	must("link", err)
	must("platform", q.InsertCustomPlatform(ctx, dbq.InsertCustomPlatformParams{
		ID:   "heybox",
		Name: []byte(`{"zh-CN":"黑盒语音"}`), Icon: strPtr("builtin:heybox"), NeedsExternalBrowser: true,
	}))
	seedM1Blocks(ctx, t, q, f)
	return f
}

func seedM1Blocks(ctx context.Context, t *testing.T, q *dbq.Queries, f m1Fixture) {
	t.Helper()
	blocks := []dbq.InsertBlockParams{
		{Kind: "heading", Data: []byte(`{"text":{"en":"Communities"},"showCount":true}`), SortOrder: 10, Visible: true},
		{Kind: "community", CommunityID: f.discord, Data: []byte(`{}`), SortOrder: 20, Visible: true},
		{Kind: "community", CommunityID: f.kook, Data: []byte(`{}`), SortOrder: 30, Visible: false},
		{
			Kind: "community", CommunityID: f.qq, Data: []byte(`{}`), SortOrder: 40, Visible: true,
			VisibleFrom: ts(f.now.Add(time.Hour)),
		},
		{
			Kind: "link", LinkID: f.link, Data: []byte(`{}`), SortOrder: 50, Visible: true,
			VisibleTo: ts(f.now.Add(2 * time.Hour)),
		},
		{
			Kind: "text", Data: []byte(`{"markdown":{"en":"old"}}`), SortOrder: 60, Visible: true,
			VisibleTo: ts(f.now.Add(-time.Hour)),
		},
	}
	for i, b := range blocks {
		b.PageID = 1
		if _, err := q.InsertBlock(ctx, b); err != nil {
			t.Fatalf("block %d: %v", i, err)
		}
	}
}

func assertM1Constraints(ctx context.Context, t *testing.T, q *dbq.Queries, pool *pgxpool.Pool) {
	t.Helper()
	base := dbq.InsertCommunityParams{
		PageID: 1, Provider: "static", Platform: "telegram", Config: []byte(`{}`),
		Display: []byte(`{"name":{"en":"x"}}`),
	}
	cases := []struct {
		name string
		mod  func(p dbq.InsertCommunityParams) dbq.InsertCommunityParams
		code string
	}{
		{"bad slug", func(p dbq.InsertCommunityParams) dbq.InsertCommunityParams { p.Slug = "Bad_Slug"; return p }, sqlStateCheck},
		{"short discord id", func(p dbq.InsertCommunityParams) dbq.InsertCommunityParams {
			p.Slug, p.Provider, p.ExternalID = "d2", "discord", strPtr("123")
			return p
		}, sqlStateCheck},
		{"static with external id", func(p dbq.InsertCommunityParams) dbq.InsertCommunityParams {
			p.Slug, p.ExternalID = "s2", strPtr("1")
			return p
		}, sqlStateCheck},
		{"missing name", func(p dbq.InsertCommunityParams) dbq.InsertCommunityParams {
			p.Slug, p.Display = "s3", []byte(`{"name":{}}`)
			return p
		}, sqlStateCheck},
		{"http invite", func(p dbq.InsertCommunityParams) dbq.InsertCommunityParams {
			p.Slug, p.InviteUrl = "s4", strPtr("http://qm.qq.com/x")
			return p
		}, sqlStateCheck},
		{"slug used by link", func(p dbq.InsertCommunityParams) dbq.InsertCommunityParams { p.Slug = "blog"; return p }, sqlStateUniq},
		{"unknown icon", func(p dbq.InsertCommunityParams) dbq.InsertCommunityParams {
			p.Slug, p.IconKey = "s5", strPtr("00000000000000000000000000000000.png")
			return p
		}, sqlStateFK},
	}
	for _, tc := range cases {
		_, err := q.InsertCommunity(ctx, tc.mod(base))
		wantSQLState(t, tc.name, err, tc.code)
	}
	_, err := q.InsertLink(ctx, dbq.InsertLinkParams{PageID: 1, Slug: "qq", Kind: "link", Label: []byte(`{"en":"x"}`), Url: "https://x.example"})
	wantSQLState(t, "link slug used by community", err, sqlStateUniq)
	_, err = q.InsertLink(ctx, dbq.InsertLinkParams{PageID: 1, Slug: "js", Kind: "link", Label: []byte(`{"en":"x"}`), Url: "javascript:alert(1)"})
	wantSQLState(t, "javascript url", err, sqlStateCheck)
	_, err = q.InsertBlock(ctx, dbq.InsertBlockParams{PageID: 1, Kind: "heading", LinkID: pgtype.UUID{}, Data: []byte(`[]`)})
	wantSQLState(t, "block data array", err, sqlStateCheck)
	_, err = q.InsertBlock(ctx, dbq.InsertBlockParams{PageID: 1, Kind: "community", Data: []byte(`{}`)})
	wantSQLState(t, "community block without ref", err, sqlStateCheck)
	_, err = pool.Exec(ctx, `UPDATE media SET kind = 'svg' WHERE key = $1`, testQRKey)
	wantSQLState(t, "media kind", err, sqlStateCheck)
}

func assertM1Blocks(ctx context.Context, t *testing.T, q *dbq.Queries, f m1Fixture) {
	t.Helper()
	rows, err := q.ListVisibleBlocks(ctx, dbq.ListVisibleBlocksParams{PageID: 1, Now: ts(f.now)})
	if err != nil {
		t.Fatal(err)
	}
	var kinds []string
	for _, r := range rows {
		kinds = append(kinds, r.Kind)
	}
	if want := []string{"heading", "community", "link"}; !slices.Equal(kinds, want) {
		t.Errorf("visible blocks = %v, want %v", kinds, want)
	}
	next, err := q.NextBlockBoundary(ctx, dbq.NextBlockBoundaryParams{PageID: 1, Now: ts(f.now)})
	if err != nil || !next.Valid || !next.Time.Equal(f.now.Add(time.Hour).Truncate(time.Microsecond)) {
		t.Errorf("next boundary = %+v err=%v", next, err)
	}
	none, err := q.NextBlockBoundary(ctx, dbq.NextBlockBoundaryParams{PageID: 1, Now: ts(f.now.Add(3 * time.Hour))})
	if err != nil || none.Valid {
		t.Errorf("no boundary expected: %+v err=%v", none, err)
	}
}

func assertM1Snapshots(ctx context.Context, t *testing.T, q *dbq.Queries, f m1Fixture) {
	t.Helper()
	for _, id := range []pgtype.UUID{f.discord, f.kook} {
		if err := q.EnsureProviderSnapshot(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	due, err := q.ListDueCommunities(ctx, dbq.ListDueCommunitiesParams{Now: ts(time.Now().Add(time.Second)), MaxRows: 10})
	if err != nil || len(due) != 2 {
		t.Fatalf("due = %+v err=%v", due, err)
	}
	err = q.UpsertProviderSnapshot(ctx, dbq.UpsertProviderSnapshotParams{
		CommunityID: f.discord,
		Data:        []byte(`{"online":13}`), State: "live", FetchedAt: ts(f.now), LastOkAt: ts(f.now),
		NextFetchAt: ts(f.now.Add(5 * time.Minute)),
	})
	if err != nil {
		t.Fatal(err)
	}
	due, err = q.ListDueCommunities(ctx, dbq.ListDueCommunitiesParams{Now: ts(time.Now().Add(time.Second)), MaxRows: 10})
	if err != nil || len(due) != 1 || due[0].ID != f.kook || due[0].FailCount == nil || *due[0].FailCount != 0 {
		t.Fatalf("due after upsert = %+v err=%v", due, err)
	}
	err = q.UpsertProviderSnapshot(ctx, dbq.UpsertProviderSnapshotParams{
		CommunityID: f.kook,
		Data:        []byte(`{"users":[]}`), State: "live", NextFetchAt: ts(f.now),
	})
	wantSQLState(t, "snapshot with users", err, sqlStateCheck)
	err = q.UpsertProviderSnapshot(ctx, dbq.UpsertProviderSnapshotParams{
		CommunityID: f.kook,
		Data:        []byte(`{}`), State: "broken", NextFetchAt: ts(f.now),
	})
	wantSQLState(t, "snapshot state", err, sqlStateCheck)
	s, err := q.GetProviderSnapshot(ctx, f.discord)
	if err != nil || s.State != "live" || string(s.Data) != `{"online": 13}` {
		t.Errorf("snapshot = %+v (%s) err=%v", s, s.Data, err)
	}
	all, err := q.ListProviderSnapshots(ctx, 1)
	if err != nil || len(all) != 2 {
		t.Errorf("snapshots = %d err=%v", len(all), err)
	}
}

func assertM1Media(ctx context.Context, t *testing.T, q *dbq.Queries, f m1Fixture) {
	t.Helper()
	const key = "AAAAAAAAAAAAAAAAAAAAAA"
	p := dbq.UpsertMediaProxyParams{
		Key: key, Provider: "discord",
		Url: "https://cdn.discordapp.com/icons/1/a.png?size=256", Ext: "png", Kind: "icon",
	}
	for range 2 {
		if err := q.UpsertMediaProxy(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	got, err := q.GetMediaProxy(ctx, key)
	if err != nil || got.Url != p.Url || got.Ext != "png" {
		t.Fatalf("media proxy = %+v err=%v", got, err)
	}
	if err := q.TouchMediaProxy(ctx, []string{key}); err != nil {
		t.Fatal(err)
	}
	p.Key = "short"
	wantSQLState(t, "proxy key", q.UpsertMediaProxy(ctx, p), sqlStateCheck)
	if _, err := q.GetMediaProxy(ctx, "BBBBBBBBBBBBBBBBBBBBBB"); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("unknown proxy key err = %v", err)
	}
	m, err := q.ListMediaByKeys(ctx, []string{testAvatarKey, testQRKey, "missing"})
	if err != nil || len(m) != 2 {
		t.Errorf("media by keys = %d err=%v", len(m), err)
	}
	qr, err := q.GetQRCode(ctx, f.qr)
	if err != nil || qr.MediaKey != testQRKey || qr.Width != 480 || qr.ContentType != "image/png" {
		t.Errorf("qr = %+v err=%v", qr, err)
	}
	qrs, err := q.ListQRCodes(ctx, 1)
	if err != nil || len(qrs) != 1 || qrs[0].CommunityID != f.qq {
		t.Errorf("qrs = %+v err=%v", qrs, err)
	}
	mm, err := q.GetMedia(ctx, testAvatarKey)
	if err != nil || mm.Kind != "avatar" || mm.Width != 256 {
		t.Errorf("media = %+v err=%v", mm, err)
	}
}

func assertM1Reads(ctx context.Context, t *testing.T, q *dbq.Queries, f m1Fixture) {
	t.Helper()
	cs, err := q.ListCommunities(ctx, 1)
	if err != nil || len(cs) != 3 {
		t.Errorf("communities = %d err=%v", len(cs), err)
	}
	c, err := q.GetCommunityBySlug(ctx, "discord")
	if err != nil || c.ID != f.discord || c.InviteUrl == nil {
		t.Errorf("community = %+v err=%v", c, err)
	}
	g, err := q.GetGoCommunity(ctx, "discord")
	if err != nil || g.SnapshotState == nil || *g.SnapshotState != "live" || g.QrID.Valid {
		t.Errorf("go community discord = %+v err=%v", g, err)
	}
	ls, err := q.ListLinks(ctx, 1)
	if err != nil || len(ls) != 1 {
		t.Errorf("links = %d err=%v", len(ls), err)
	}
	l, err := q.GetLinkBySlug(ctx, "blog")
	if err != nil || l.ID != f.link || l.RelMe {
		t.Errorf("link = %+v err=%v", l, err)
	}
	ps, err := q.ListCustomPlatforms(ctx)
	if err != nil || len(ps) != 1 || !ps[0].NeedsExternalBrowser {
		t.Errorf("platforms = %+v err=%v", ps, err)
	}
	v, err := q.ReplaceSiteSettingsData(ctx, dbq.ReplaceSiteSettingsDataParams{
		Data:      []byte(`{"appearance":"dark"}`),
		UpdatedBy: strPtr("seed"), PageID: 1,
	})
	if err != nil || v != 2 {
		t.Errorf("settings version = %d err=%v", v, err)
	}
}

func assertM1Cascade(ctx context.Context, t *testing.T, q *dbq.Queries, pool *pgxpool.Pool, f m1Fixture) {
	t.Helper()
	_, err := pool.Exec(ctx, `DELETE FROM media WHERE key = $1`, testQRKey)
	wantSQLState(t, "delete referenced qr media", err, sqlStateRestr)
	if _, err := pool.Exec(ctx, `DELETE FROM communities WHERE id = $1`, f.qq); err != nil {
		t.Fatal(err)
	}
	if _, err := q.GetQRCode(ctx, f.qr); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("qr should cascade: %v", err)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM page_blocks WHERE community_id = $1`, f.qq).Scan(&n); err != nil || n != 0 {
		t.Errorf("blocks should cascade: %d %v", n, err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM media WHERE key = $1`, testAvatarKey); err != nil {
		t.Fatal(err)
	}
	c, err := q.GetCommunityBySlug(ctx, "discord")
	if err != nil || c.IconKey != nil {
		t.Errorf("icon_key should be set null: %+v %v", c.IconKey, err)
	}
}
