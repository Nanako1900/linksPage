package site

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/Nanako1900/linksPage/internal/media"
	"github.com/Nanako1900/linksPage/internal/store/dbq"
)

// Page identifies a public page.
type Page struct {
	ID   int16  `json:"id"`
	Slug string `json:"slug"`
}

// Snapshot is an immutable view of everything the public page needs. The
// theme CSS and its CSP hash are computed once when the snapshot is built.
type Snapshot struct {
	Version   int64
	Page      Page
	Settings  Settings
	ThemeCSS  string
	ThemeHash string
	BuiltAt   time.Time

	// Public is the public DTO (never nil; empty content before the first
	// build).
	Public *PublicPage
	// Head holds server-only <head> metadata.
	Head HeadMeta
	// Files are the in-memory site files (nil before the first build).
	Files *media.SiteFiles
}

// HeadMeta is <head> metadata that is not part of the public DTO.
type HeadMeta struct {
	// OGTitle / OGDescription fall back to Title / Description.
	OGTitle       LocalizedText
	OGDescription LocalizedText
	// OGImage is root-relative (prefix base_url when rendering), nil →
	// twitter:card "summary" without og:image.
	OGImage *ImageView
	// Robots is "index" or "noindex" (settings.searchIndexing).
	Robots string
	// Icons are root-relative favicon URLs by size (32, 180, 192, 512);
	// empty before the first build.
	Icons map[int]string
}

// NextBoundary returns the public page's next visibility boundary, if any.
func (s *Snapshot) NextBoundary() (time.Time, bool) {
	if s.Public == nil || s.Public.NextBoundary == nil {
		return time.Time{}, false
	}
	return *s.Public.NextBoundary, true
}

// NewSnapshot validates settings and precomputes the theme CSS and hash.
func NewSnapshot(version int64, page Page, settings Settings, now time.Time) (*Snapshot, error) {
	if err := settings.Validate(); err != nil {
		return nil, err
	}
	css, err := settings.Theme.CSS()
	if err != nil {
		return nil, err
	}
	return &Snapshot{
		Version:   version,
		Page:      page,
		Settings:  settings.Clone(),
		ThemeCSS:  css,
		ThemeHash: CSPHash(css),
		BuiltAt:   now,
		Public:    EmptyPublicPage(version, page, settings, "", now),
		Head:      HeadMeta{OGTitle: LocalizedText{}, OGDescription: LocalizedText{}, Robots: settings.SearchIndexing, Icons: map[int]string{}},
	}, nil
}

// DefaultSnapshot is served before the database has been read.
func DefaultSnapshot() *Snapshot {
	s, err := NewSnapshot(0, Page{ID: 1, Slug: "default"}, Default(), time.Now())
	if err != nil {
		panic(fmt.Sprintf("built-in site defaults are invalid: %v", err)) // programming error
	}
	return s
}

// Bootstrap is the public bootstrap DTO (GET /api/v1/public/bootstrap and
// the #lp-data payload).
type Bootstrap struct {
	Version int64    `json:"version" doc:"Settings version; changes whenever settings change"`
	Page    Page     `json:"page"`
	Site    Settings `json:"site"`
}

// Bootstrap returns the DTO for this snapshot.
func (s *Snapshot) Bootstrap() Bootstrap {
	return Bootstrap{Version: s.Version, Page: s.Page, Site: s.Settings.Clone()}
}

// Holder publishes the current snapshot atomically.
type Holder struct {
	p atomic.Pointer[Snapshot]
}

// NewHolder returns a holder initialised with initial.
func NewHolder(initial *Snapshot) *Holder {
	h := &Holder{}
	h.p.Store(initial)
	return h
}

// Current returns the current snapshot (never nil).
func (h *Holder) Current() *Snapshot { return h.p.Load() }

// Set replaces the current snapshot.
func (h *Holder) Set(s *Snapshot) { h.p.Store(s) }

// Querier is the subset of dbq.Queries used to build snapshots.
type Querier interface {
	GetSiteSettings(ctx context.Context) (dbq.SiteSetting, error)
	GetDefaultPage(ctx context.Context) (dbq.Page, error)
}

// LoadSnapshot reads settings from the database. Invalid stored settings
// are logged and replaced by the defaults so the public page stays up.
func LoadSnapshot(ctx context.Context, q Querier, logger *slog.Logger) (*Snapshot, error) {
	row, err := q.GetSiteSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("load site settings: %w", err)
	}
	page, err := q.GetDefaultPage(ctx)
	if err != nil {
		return nil, fmt.Errorf("load default page: %w", err)
	}
	settings, err := ParseSettings(row.Data)
	if err != nil {
		logger.Error("stored site settings are invalid; using defaults", slog.Any("error", err))
		settings = Default()
	}
	return NewSnapshot(row.Version, Page{ID: page.ID, Slug: page.Slug}, settings, time.Now())
}
