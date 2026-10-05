package site

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/sync/singleflight"

	"github.com/Nanako1900/linksPage/internal/media"
	"github.com/Nanako1900/linksPage/internal/provider"
	"github.com/Nanako1900/linksPage/internal/store/dbq"
)

// BuildQuerier is the subset of dbq.Queries the page builder reads.
type BuildQuerier interface {
	Querier
	ListVisibleBlocks(ctx context.Context, arg dbq.ListVisibleBlocksParams) ([]dbq.ListVisibleBlocksRow, error)
	NextBlockBoundary(ctx context.Context, arg dbq.NextBlockBoundaryParams) (pgtype.Timestamptz, error)
	ListCommunities(ctx context.Context, pageID int16) ([]dbq.Community, error)
	ListProviderSnapshots(ctx context.Context, pageID int16) ([]dbq.ProviderSnapshot, error)
	ListQRCodes(ctx context.Context, pageID int16) ([]dbq.ListQRCodesRow, error)
	ListLinks(ctx context.Context, pageID int16) ([]dbq.Link, error)
	ListCustomPlatforms(ctx context.Context) ([]dbq.CustomPlatform, error)
	ListMediaByKeys(ctx context.Context, keys []string) ([]dbq.Medium, error)
	InsertMedia(ctx context.Context, arg dbq.InsertMediaParams) error
}

var _ BuildQuerier = (*dbq.Queries)(nil)

// AssetGenerator produces OG image, favicons and site files
// (media.Generator satisfies it).
type AssetGenerator interface {
	Generate(ctx context.Context, in media.GenerateInput) (media.Generated, error)
}

// BuilderDeps are the page builder's dependencies.
type BuilderDeps struct {
	Queries   BuildQuerier
	Live      provider.LiveSource
	Platforms *provider.Catalog
	Assets    AssetGenerator
	// BaseURL is config base_url.
	BaseURL string
	Logger  *slog.Logger
	Now     func() time.Time
}

// Builder assembles snapshots (settings + PublicPage + head assets) from
// the database and the in-memory provider snapshots. It is safe for
// concurrent use; the scheduler and (M2) admin writes call Rebuild.
type Builder struct {
	deps    BuilderDeps
	baseURL string
	group   singleflight.Group
	// warned holds keys of content problems already logged.
	warned sync.Map
	// recorded holds keys of generated media rows already inserted.
	recorded sync.Map
}

// NewBuilder returns a builder.
func NewBuilder(deps BuilderDeps) (*Builder, error) {
	if deps.Queries == nil || deps.Live == nil || deps.Platforms == nil || deps.Assets == nil || deps.Logger == nil {
		return nil, errors.New("site: Queries, Live, Platforms, Assets and Logger are required")
	}
	if deps.Now == nil {
		deps.Now = time.Now
	}
	return &Builder{deps: deps, baseURL: strings.TrimRight(deps.BaseURL, "/")}, nil
}

// Build reads everything and returns a new immutable snapshot. Invalid
// stored rows are logged and skipped (the page stays up); database errors
// are returned. When asset generation fails the snapshot has nil Files.
func (b *Builder) Build(ctx context.Context) (*Snapshot, error) {
	now := b.deps.Now().UTC()
	in, err := b.load(ctx, now)
	if err != nil {
		return nil, err
	}
	snap, err := NewSnapshot(in.version, in.page, in.settings, now)
	if err != nil {
		return nil, fmt.Errorf("site: snapshot: %w", err)
	}
	page, err := assemble(in, b.deps.Live, b.warnOnce)
	if err != nil {
		return nil, err
	}
	snap.Public = page
	snap.Head = headMeta(in.settings)
	files, err := b.generateAssets(ctx, in, &snap.Head)
	if err != nil {
		b.deps.Logger.Warn("site assets unavailable; keeping previous ones", slog.Any("error", err))
	}
	snap.Files = files
	return snap, nil
}

// Rebuild builds and publishes into h. Concurrent calls are coalesced
// (singleflight). When nothing but GeneratedAt changed (same Revision and
// live data) the current snapshot is kept so HTML/live ETags stay stable.
func (b *Builder) Rebuild(ctx context.Context, h *Holder) error {
	_, err, _ := b.group.Do("rebuild", func() (any, error) {
		next, err := b.Build(ctx)
		if err != nil {
			return nil, err
		}
		cur := h.Current()
		carryAssets(next, cur)
		if !unchanged(cur, next) {
			h.Set(next)
		}
		return nil, nil
	})
	if err != nil {
		return fmt.Errorf("site: rebuild: %w", err)
	}
	return nil
}

// unchanged reports whether next would serve exactly what cur serves.
func unchanged(cur, next *Snapshot) bool {
	if cur == nil || cur.Public == nil || cur.Public.Revision == "" {
		return false
	}
	if cur.Public.Revision != next.Public.Revision || !sameLive(cur.Public, next.Public) {
		return false
	}
	if cur.Version != next.Version || (cur.Files == nil) != (next.Files == nil) {
		return false
	}
	return cur.Files == nil || cur.Files.ETag == next.Files.ETag
}

// RebuildIfDue rebuilds when h's snapshot passed its NextBoundary.
func (b *Builder) RebuildIfDue(ctx context.Context, h *Holder) error {
	at, ok := h.Current().NextBoundary()
	if !ok || b.deps.Now().Before(at) {
		return nil
	}
	return b.Rebuild(ctx, h)
}

// warnOnce logs a content problem the first time it is seen.
func (b *Builder) warnOnce(key, msg string, attrs ...any) {
	if _, seen := b.warned.LoadOrStore(key+"\x00"+msg, true); seen {
		return
	}
	b.deps.Logger.Warn(msg, attrs...)
}
