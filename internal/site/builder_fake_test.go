package site

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/Nanako1900/linksPage/internal/media"
	"github.com/Nanako1900/linksPage/internal/provider"
	"github.com/Nanako1900/linksPage/internal/store/dbq"
)

var errDB = errors.New("db down")

// buildFake is an in-memory BuildQuerier. failOn names a method that
// returns errDB.
type buildFake struct {
	mu          sync.Mutex
	settings    dbq.SiteSetting
	blocks      []dbq.ListVisibleBlocksRow
	next        pgtype.Timestamptz
	communities []dbq.Community
	snapshots   []dbq.ProviderSnapshot
	qrCodes     []dbq.ListQRCodesRow
	links       []dbq.Link
	platforms   []dbq.CustomPlatform
	media       []dbq.Medium
	inserted    []dbq.InsertMediaParams
	failOn      string
	builds      int
}

func (f *buildFake) fail(name string) error {
	if f.failOn == name {
		return errDB
	}
	return nil
}

func (f *buildFake) GetSiteSettings(context.Context) (dbq.SiteSetting, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.builds++
	return f.settings, f.fail("settings")
}

func (f *buildFake) GetDefaultPage(context.Context) (dbq.Page, error) {
	return dbq.Page{ID: 1, Slug: "default"}, f.fail("page")
}

func (f *buildFake) ListVisibleBlocks(context.Context, dbq.ListVisibleBlocksParams) ([]dbq.ListVisibleBlocksRow, error) {
	return f.blocks, f.fail("blocks")
}

func (f *buildFake) NextBlockBoundary(context.Context, dbq.NextBlockBoundaryParams) (pgtype.Timestamptz, error) {
	return f.next, f.fail("boundary")
}

func (f *buildFake) ListCommunities(context.Context, int16) ([]dbq.Community, error) {
	return f.communities, f.fail("communities")
}

func (f *buildFake) ListProviderSnapshots(context.Context, int16) ([]dbq.ProviderSnapshot, error) {
	return f.snapshots, f.fail("snapshots")
}

func (f *buildFake) ListQRCodes(context.Context, int16) ([]dbq.ListQRCodesRow, error) {
	return f.qrCodes, f.fail("qr")
}

func (f *buildFake) ListLinks(context.Context, int16) ([]dbq.Link, error) {
	return f.links, f.fail("links")
}

func (f *buildFake) ListCustomPlatforms(context.Context) ([]dbq.CustomPlatform, error) {
	return f.platforms, f.fail("platforms")
}

func (f *buildFake) ListMediaByKeys(_ context.Context, keys []string) ([]dbq.Medium, error) {
	var out []dbq.Medium
	for _, m := range f.media {
		for _, k := range keys {
			if m.Key == k {
				out = append(out, m)
			}
		}
	}
	return out, f.fail("media")
}

func (f *buildFake) InsertMedia(_ context.Context, arg dbq.InsertMediaParams) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.fail("insert"); err != nil {
		return err
	}
	f.inserted = append(f.inserted, arg)
	return nil
}

// fakeLive is a provider.LiveSource.
type fakeLive map[string]provider.Snapshot

func (l fakeLive) Live(id string) (provider.Snapshot, bool) {
	s, ok := l[id]
	return s, ok
}

// stubAssets is an AssetGenerator.
type stubAssets struct {
	err   error
	etag  string
	calls int
}

func (a *stubAssets) Generate(_ context.Context, in media.GenerateInput) (media.Generated, error) {
	a.calls++
	if a.err != nil {
		return media.Generated{}, a.err
	}
	og := media.Stored{Key: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.jpg", ContentType: "image/jpeg", Bytes: 10, Width: 1200, Height: 630}
	if in.OGImageKey != "" {
		og.Key = in.OGImageKey
	}
	fav := map[int]media.Stored{}
	for i, size := range media.FaviconSizes {
		key := string(rune('b'+i)) + "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb.png"
		fav[size] = media.Stored{Key: key, ContentType: "image/png", Bytes: 5, Width: size, Height: size}
	}
	etag := a.etag
	if etag == "" {
		etag = `"e1"`
	}
	return media.Generated{OGImage: og, Favicons: fav, Files: media.SiteFiles{FaviconICO: []byte{1}, Manifest: []byte("{}"), RobotsTxt: []byte("User-agent: *"), ETag: etag}}, nil
}

func uuidOf(t *testing.T, s string) pgtype.UUID {
	t.Helper()
	var u pgtype.UUID
	if err := u.Scan(s); err != nil {
		t.Fatalf("uuid %q: %v", s, err)
	}
	return u
}

func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func strp(s string) *string { return &s }

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// bufLogger returns a logger writing to buf.
func bufLogger(buf *bytes.Buffer) *slog.Logger { return slog.New(slog.NewTextHandler(buf, nil)) }

func presets(t *testing.T) *provider.Catalog {
	t.Helper()
	c, err := provider.LoadPresets()
	if err != nil {
		t.Fatalf("presets: %v", err)
	}
	return c
}

func newTestBuilder(t *testing.T, f *buildFake, live provider.LiveSource, assets AssetGenerator, now time.Time, logger *slog.Logger) *Builder {
	t.Helper()
	b, err := NewBuilder(BuilderDeps{
		Queries: f, Live: live, Platforms: presets(t), Assets: assets,
		BaseURL: "https://links.example.com/", Logger: logger, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func dbqMedium(key string, w, h int32) dbq.Medium {
	return dbq.Medium{Key: key, Kind: "og", Width: w, Height: h}
}
