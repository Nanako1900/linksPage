package site

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestRebuildPublishesAndKeepsStableSnapshot(t *testing.T) {
	f, live := fullFake(t, testNow)
	now := testNow
	assets := &stubAssets{}
	b, err := NewBuilder(BuilderDeps{
		Queries: f, Live: live, Platforms: presets(t), Assets: assets,
		BaseURL: "https://links.example.com", Logger: discardLogger(), Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	h := NewHolder(DefaultSnapshot())
	ctx := context.Background()
	if err := b.Rebuild(ctx, h); err != nil {
		t.Fatal(err)
	}
	first := h.Current()
	if first.Public.Revision == "" || len(first.Public.Communities) != 9 {
		t.Fatalf("first build not published: %+v", first.Public)
	}

	now = now.Add(time.Minute)
	if err := b.Rebuild(ctx, h); err != nil {
		t.Fatal(err)
	}
	if h.Current() != first {
		t.Error("unchanged content must keep the current snapshot")
	}

	m := live[cid(1)]
	m.Online = intp(14)
	live[cid(1)] = m
	if err := b.Rebuild(ctx, h); err != nil {
		t.Fatal(err)
	}
	second := h.Current()
	if second == first || *second.Public.Communities[cid(1)].Live.Online != 14 {
		t.Error("live change must publish a new snapshot")
	}

	assets.err = errors.New("encoder busy")
	f.links[0].Label = []byte(`{"en":"Blog 2"}`)
	if err := b.Rebuild(ctx, h); err != nil {
		t.Fatal(err)
	}
	third := h.Current()
	if third == second || third.Files != second.Files || third.Head.Icons[32] != second.Head.Icons[32] || third.Head.OGImage == nil {
		t.Error("asset failure must carry the previous assets into the new snapshot")
	}

	f.failOn = "links"
	if err := b.Rebuild(ctx, h); !errors.Is(err, errDB) || h.Current() != third {
		t.Errorf("db error must keep the snapshot: %v", err)
	}
}

func TestRebuildReplacesWhenFilesChange(t *testing.T) {
	f, live := fullFake(t, testNow)
	assets := &stubAssets{}
	b := newTestBuilder(t, f, live, assets, testNow, discardLogger())
	h := NewHolder(DefaultSnapshot())
	ctx := context.Background()
	if err := b.Rebuild(ctx, h); err != nil {
		t.Fatal(err)
	}
	first := h.Current()
	assets.etag = `"e2"`
	if err := b.Rebuild(ctx, h); err != nil {
		t.Fatal(err)
	}
	if h.Current() == first || h.Current().Files.ETag != `"e2"` {
		t.Error("new site files must be published")
	}
}

func TestRebuildRecordsGeneratedMediaOnce(t *testing.T) {
	f, live := fullFake(t, testNow)
	b := newTestBuilder(t, f, live, &stubAssets{}, testNow, discardLogger())
	h := NewHolder(DefaultSnapshot())
	for range 3 {
		if err := b.Rebuild(context.Background(), h); err != nil {
			t.Fatal(err)
		}
	}
	if len(f.inserted) != 5 {
		t.Fatalf("inserted %d media rows, want 5 (og + 4 favicons)", len(f.inserted))
	}
	kinds := map[string]int{}
	for _, m := range f.inserted {
		kinds[m.Kind]++
		if string(m.Variants) != "{}" {
			t.Errorf("variants = %s", m.Variants)
		}
	}
	if kinds["og"] != 1 || kinds["favicon"] != 4 {
		t.Errorf("kinds = %v", kinds)
	}
}

func TestBuildMediaInsertFailureDropsAssets(t *testing.T) {
	f, live := fullFake(t, testNow)
	f.failOn = "insert"
	b := newTestBuilder(t, f, live, &stubAssets{}, testNow, discardLogger())
	s, err := b.Build(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if s.Files != nil {
		t.Error("unrecorded assets must not be published")
	}
}

func TestBuildUploadedOGImageNotRecorded(t *testing.T) {
	f, live := fullFake(t, testNow)
	ogKey := "0123456789abcdef0123456789abcdef.jpg"
	f.settings.Data = []byte(`{"og":{"title":{},"description":{},"imageKey":"` + ogKey + `"}}`)
	f.media = append(f.media, dbqMedium(ogKey, 1200, 630))
	b := newTestBuilder(t, f, live, &stubAssets{}, testNow, discardLogger())
	s, err := b.Build(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if s.Head.OGImage.URL != PathUploads+ogKey || len(f.inserted) != 4 {
		t.Errorf("og = %+v inserted = %d", s.Head.OGImage, len(f.inserted))
	}
	if s.Head.OGTitle["zh-CN"] != "我的社区" {
		t.Errorf("og title fallback = %v", s.Head.OGTitle)
	}
}

func TestRebuildIfDue(t *testing.T) {
	f, live := fullFake(t, testNow)
	now := testNow
	b, err := NewBuilder(BuilderDeps{
		Queries: f, Live: live, Platforms: presets(t), Assets: &stubAssets{},
		BaseURL: "https://links.example.com", Logger: discardLogger(), Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	h := NewHolder(DefaultSnapshot())
	ctx := context.Background()
	if err := b.RebuildIfDue(ctx, h); err != nil || f.builds != 0 {
		t.Fatalf("default snapshot has no boundary: builds=%d err=%v", f.builds, err)
	}
	if err := b.Rebuild(ctx, h); err != nil {
		t.Fatal(err)
	}
	if err := b.RebuildIfDue(ctx, h); err != nil || f.builds != 1 {
		t.Fatalf("boundary not reached: builds=%d err=%v", f.builds, err)
	}
	now = testNow.Add(49 * time.Hour)
	f.next.Valid = false
	if err := b.RebuildIfDue(ctx, h); err != nil || f.builds != 2 {
		t.Fatalf("boundary passed: builds=%d err=%v", f.builds, err)
	}
	if _, ok := h.Current().NextBoundary(); ok {
		t.Error("new snapshot has no boundary")
	}
}

func TestRebuildConcurrent(t *testing.T) {
	f, live := fullFake(t, testNow)
	b := newTestBuilder(t, f, live, &stubAssets{}, testNow, discardLogger())
	h := NewHolder(DefaultSnapshot())
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if err := b.Rebuild(context.Background(), h); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if h.Current().Public.Revision == "" {
		t.Error("not published")
	}
}
