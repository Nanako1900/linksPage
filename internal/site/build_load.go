package site

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/Nanako1900/linksPage/internal/provider"
	"github.com/Nanako1900/linksPage/internal/store/dbq"
)

// buildInput is everything one build reads from the database, keyed by
// canonical UUID strings. It is assembled into a PublicPage without further
// I/O (see assemble), which keeps the assembly testable without a database.
type buildInput struct {
	version  int64
	page     Page
	settings Settings
	now      time.Time
	baseURL  string

	blocks       []dbq.ListVisibleBlocksRow
	nextBoundary *time.Time
	communities  map[string]dbq.Community
	snapshots    map[string]dbq.ProviderSnapshot
	qrCodes      map[string]dbq.ListQRCodesRow // by community id
	links        map[string]dbq.Link
	catalog      *provider.Catalog
	// platformIcons maps custom platform id → uploaded icon media key.
	platformIcons map[string]string
	media         map[string]dbq.Medium
}

// load reads the settings and all page content visible at now.
func (b *Builder) load(ctx context.Context, now time.Time) (*buildInput, error) {
	in, err := b.loadSettings(ctx, now)
	if err != nil {
		return nil, err
	}
	if err := b.loadBlocks(ctx, in); err != nil {
		return nil, err
	}
	if err := b.loadContent(ctx, in); err != nil {
		return nil, err
	}
	if err := b.loadPlatforms(ctx, in); err != nil {
		return nil, err
	}
	if err := b.loadMedia(ctx, in); err != nil {
		return nil, err
	}
	return in, nil
}

func (b *Builder) loadSettings(ctx context.Context, now time.Time) (*buildInput, error) {
	row, err := b.deps.Queries.GetSiteSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("load site settings: %w", err)
	}
	page, err := b.deps.Queries.GetDefaultPage(ctx)
	if err != nil {
		return nil, fmt.Errorf("load default page: %w", err)
	}
	settings, err := ParseSettings(row.Data)
	if err != nil {
		b.warnOnce("settings", "stored site settings are invalid; using defaults", slog.Any("error", err))
		settings = Default()
	}
	return &buildInput{
		version:  row.Version,
		page:     Page{ID: page.ID, Slug: page.Slug},
		settings: settings,
		now:      now,
		baseURL:  b.baseURL,
	}, nil
}

func (b *Builder) loadBlocks(ctx context.Context, in *buildInput) error {
	at := pgtype.Timestamptz{Time: in.now, Valid: true}
	blocks, err := b.deps.Queries.ListVisibleBlocks(ctx, dbq.ListVisibleBlocksParams{PageID: in.page.ID, Now: at})
	if err != nil {
		return fmt.Errorf("list visible blocks: %w", err)
	}
	next, err := b.deps.Queries.NextBlockBoundary(ctx, dbq.NextBlockBoundaryParams{PageID: in.page.ID, Now: at})
	if err != nil {
		return fmt.Errorf("next block boundary: %w", err)
	}
	in.blocks = blocks
	if next.Valid {
		t := next.Time.UTC()
		in.nextBoundary = &t
	}
	return nil
}

func (b *Builder) loadContent(ctx context.Context, in *buildInput) error {
	communities, err := b.deps.Queries.ListCommunities(ctx, in.page.ID)
	if err != nil {
		return fmt.Errorf("list communities: %w", err)
	}
	snapshots, err := b.deps.Queries.ListProviderSnapshots(ctx, in.page.ID)
	if err != nil {
		return fmt.Errorf("list provider snapshots: %w", err)
	}
	qrCodes, err := b.deps.Queries.ListQRCodes(ctx, in.page.ID)
	if err != nil {
		return fmt.Errorf("list qr codes: %w", err)
	}
	links, err := b.deps.Queries.ListLinks(ctx, in.page.ID)
	if err != nil {
		return fmt.Errorf("list links: %w", err)
	}
	in.communities = indexBy(communities, func(c dbq.Community) pgtype.UUID { return c.ID })
	in.snapshots = indexBy(snapshots, func(s dbq.ProviderSnapshot) pgtype.UUID { return s.CommunityID })
	in.qrCodes = indexBy(qrCodes, func(q dbq.ListQRCodesRow) pgtype.UUID { return q.CommunityID })
	in.links = indexBy(links, func(l dbq.Link) pgtype.UUID { return l.ID })
	return nil
}

// loadPlatforms merges custom platforms into the preset catalog. Invalid
// custom rows are skipped (communities using them are then skipped too).
func (b *Builder) loadPlatforms(ctx context.Context, in *buildInput) error {
	rows, err := b.deps.Queries.ListCustomPlatforms(ctx)
	if err != nil {
		return fmt.Errorf("list custom platforms: %w", err)
	}
	in.catalog = b.deps.Platforms
	in.platformIcons = map[string]string{}
	custom := make([]provider.Platform, 0, len(rows))
	for _, r := range rows {
		p, err := customPlatform(r)
		if err != nil {
			b.warnOnce("platform:"+r.ID, "skipping invalid custom platform", slog.String("id", r.ID), slog.Any("error", err))
			continue
		}
		custom = append(custom, p)
		if r.IconKey != nil {
			in.platformIcons[r.ID] = *r.IconKey
		}
	}
	if len(custom) == 0 {
		return nil
	}
	merged, err := b.deps.Platforms.WithCustom(custom)
	if err != nil {
		b.warnOnce("platforms", "custom platforms rejected; using presets only", slog.Any("error", err))
		in.platformIcons = map[string]string{}
		return nil
	}
	in.catalog = merged
	return nil
}

func customPlatform(r dbq.CustomPlatform) (provider.Platform, error) {
	var name map[string]string
	if err := DecodeStrict(r.Name, &name); err != nil {
		return provider.Platform{}, fmt.Errorf("name: %w", err)
	}
	p := provider.Platform{
		ID:                   r.ID,
		Name:                 name,
		Card:                 provider.CardStatic,
		NeedsExternalBrowser: r.NeedsExternalBrowser,
		Custom:               true,
	}
	if r.Icon != nil {
		p.Icon = *r.Icon
	}
	if r.UrlPattern != nil {
		p.URLPattern = *r.UrlPattern
	}
	return p, nil
}

// loadMedia fetches the rows of every referenced uploaded image (sizes).
func (b *Builder) loadMedia(ctx context.Context, in *buildInput) error {
	keys := map[string]bool{}
	add := func(k *string) {
		if k != nil && *k != "" {
			keys[*k] = true
		}
	}
	add(&in.settings.AvatarKey)
	add(&in.settings.OG.ImageKey)
	for _, c := range in.communities {
		add(c.IconKey)
	}
	for _, l := range in.links {
		add(l.IconKey)
	}
	for _, k := range in.platformIcons {
		add(&k)
	}
	in.media = map[string]dbq.Medium{}
	if len(keys) == 0 {
		return nil
	}
	rows, err := b.deps.Queries.ListMediaByKeys(ctx, slices.Sorted(maps.Keys(keys)))
	if err != nil {
		return fmt.Errorf("list media: %w", err)
	}
	for _, m := range rows {
		in.media[m.Key] = m
	}
	return nil
}

func indexBy[T any](rows []T, id func(T) pgtype.UUID) map[string]T {
	out := make(map[string]T, len(rows))
	for _, r := range rows {
		out[id(r).String()] = r
	}
	return out
}

// decodeSnapshot decodes provider_snapshots.data (tolerant: the shape is
// owned by the providers) and copies the column values into it.
func decodeSnapshot(row dbq.ProviderSnapshot) (provider.Snapshot, error) {
	var s provider.Snapshot
	if err := json.Unmarshal(row.Data, &s); err != nil {
		return provider.Snapshot{}, fmt.Errorf("decode snapshot data: %w", err)
	}
	s.State = provider.State(row.State)
	if row.ErrCode != nil {
		s.ErrCode = *row.ErrCode
	}
	if row.FetchedAt.Valid {
		s.FetchedAt = row.FetchedAt.Time.UTC()
	}
	return s, nil
}
