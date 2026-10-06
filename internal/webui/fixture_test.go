package webui

import (
	"errors"
	"html"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Nanako1900/linksPage/internal/media"
	"github.com/Nanako1900/linksPage/internal/site"
)

// fixturePath is the canonical contract sample shared with the frontend.
var fixturePath = filepath.Join("..", "..", "web", "src", "test", "fixtures", "public-page.json")

// fakeMarkdown marks rendered Markdown so tests can see it was used.
type fakeMarkdown struct{ err error }

func (f fakeMarkdown) Render(src string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return `<p class="md">` + html.EscapeString(src) + "</p>", nil
}

func loadFixturePage(t *testing.T) *site.PublicPage {
	t.Helper()
	raw, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var p site.PublicPage
	if err := site.DecodeStrict(raw, &p); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	if err := p.Validate(); err != nil {
		t.Fatalf("fixture violates the contract: %v", err)
	}
	return &p
}

// fixtureSettings mirrors the fixture's site block.
func fixtureSettings(p *site.PublicPage) site.Settings {
	s := site.Default()
	s.Title, s.Description = p.Site.Title, p.Site.Description
	s.DisplayName, s.Bio, s.Footer = p.Site.DisplayName, p.Site.Bio, p.Site.Footer
	s.NotFound, s.Copy, s.Theme = p.Site.NotFound, p.Site.Copy, p.Site.Theme
	return s
}

// fixtureEnv is a renderer whose snapshot carries the canonical fixture.
// mutate may adjust the snapshot (settings changes need a new snapshot).
func fixtureEnv(t *testing.T, md MarkdownRenderer, mutate func(*site.Snapshot)) *testEnv {
	t.Helper()
	pub := loadFixturePage(t)
	snap, err := site.NewSnapshot(pub.Version, pub.Page, fixtureSettings(pub), time.Unix(1, 0))
	if err != nil {
		t.Fatalf("NewSnapshot: %v", err)
	}
	snap.Public = pub
	if mutate != nil {
		mutate(snap)
	}
	assets, err := LoadAssets(fakeDist())
	if err != nil {
		t.Fatal(err)
	}
	env := &testEnv{snap: snap, logs: &strings.Builder{}}
	rd, err := NewRenderer(Options{
		Assets: assets, BaseURL: "https://links.example.com/", AppVersion: "test",
		Logger:   slog.New(slog.NewTextHandler(env.logs, &slog.HandlerOptions{Level: slog.LevelDebug})),
		Snapshot: func() *site.Snapshot { return env.snap }, Markdown: md,
	})
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	env.rd = rd
	return env
}

func withHeadAssets(s *site.Snapshot) {
	s.Head.OGImage = &site.ImageView{URL: "/media/u/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.png", Width: 1200, Height: 630}
	s.Head.OGTitle = site.LocalizedText{"zh-CN": "猎人小屋 · 社区"}
	s.Head.Icons = map[int]string{
		32: "/media/u/bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb.png", 180: "/media/u/cccccccccccccccccccccccccccccccc.png",
		192: "/media/u/dddddddddddddddddddddddddddddddd.png",
	}
	s.Files = &media.SiteFiles{ETag: `"x"`}
}

var errMarkdown = errors.New("markdown broke")
