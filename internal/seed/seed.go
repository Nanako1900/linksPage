// Package seed imports initial content from seed.yaml (config seed_file)
// on first start: site settings, custom platforms, communities (including
// QR codes and icons), links and blocks. It runs only when the content
// tables are empty and is idempotent: a second run is a no-op.
package seed

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/jackc/pgx/v5"
	"go.yaml.in/yaml/v3"

	"github.com/Nanako1900/linksPage/internal/content"
	"github.com/Nanako1900/linksPage/internal/media"
	"github.com/Nanako1900/linksPage/internal/provider"
	"github.com/Nanako1900/linksPage/internal/store/dbq"
)

// ErrInvalidSeed wraps every error caused by the seed file itself
// (unreadable, syntax, validation, bad image). It is permanent: startup
// treats it as fatal. Other errors (database) are retryable.
var ErrInvalidSeed = errors.New("seed: invalid seed file")

// MaxSeedBytes bounds the seed file size.
const MaxSeedBytes = 1 << 20

// TxBeginner starts the single transaction the import runs in
// (*pgxpool.Pool satisfies it).
type TxBeginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Ingester stores images (media.Processor satisfies it).
type Ingester interface {
	Ingest(ctx context.Context, r io.Reader, opts media.ProcessOptions) (media.Result, error)
}

// Deps are the importer's dependencies.
type Deps struct {
	DB     TxBeginner
	Media  Ingester
	Logger *slog.Logger
	// MaxUploadBytes is uploads.max_bytes.
	MaxUploadBytes int64
}

// Result summarizes an import.
type Result struct {
	// Skipped is true when content already existed (nothing written).
	Skipped     bool
	Communities int
	Links       int
	Blocks      int
}

// Importer imports seed files.
type Importer struct {
	deps    Deps
	presets *provider.Catalog
	md      *content.Markdown
}

// NewImporter returns an importer.
func NewImporter(deps Deps) (*Importer, error) {
	if deps.DB == nil || deps.Media == nil || deps.Logger == nil {
		return nil, errors.New("seed: DB, Media and Logger are required")
	}
	presets, err := provider.LoadPresets()
	if err != nil {
		return nil, fmt.Errorf("seed: %w", err)
	}
	return &Importer{deps: deps, presets: presets, md: content.NewMarkdown()}, nil
}

// Import reads, validates and imports path. Validation errors name the
// YAML path (e.g. communities[2].guild_id) and are returned before
// anything is written; images are processed before the transaction
// (orphaned content-addressed files are harmless); all rows are written in
// one transaction that re-checks IsContentEmpty and bumps the settings
// version.
//
// The emptiness check also runs first, so later starts skip the file
// entirely (it is not even read or validated once content exists).
func (im *Importer) Import(ctx context.Context, path string) (Result, error) {
	empty, err := im.contentEmpty(ctx)
	if err != nil {
		return Result{}, err
	}
	if !empty {
		im.deps.Logger.Info("seed: content exists; skipping seed file", slog.String("path", path))
		return Result{Skipped: true}, nil
	}
	f, err := readFile(path)
	if err != nil {
		return Result{}, err
	}
	pl, err := planFile(f, im.presets, im.md)
	if err != nil {
		return Result{}, err
	}
	images, err := im.ingestAll(ctx, filepath.Dir(path), pl.images())
	if err != nil {
		return Result{}, err
	}
	res, err := im.write(ctx, pl, images)
	if err != nil {
		return Result{}, err
	}
	im.deps.Logger.Info("seed: imported", slog.String("path", path), slog.Bool("skipped", res.Skipped),
		slog.Int("communities", res.Communities), slog.Int("links", res.Links), slog.Int("blocks", res.Blocks))
	return res, nil
}

// contentEmpty runs IsContentEmpty in a short read-only transaction.
func (im *Importer) contentEmpty(ctx context.Context) (bool, error) {
	tx, err := im.deps.DB.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("seed: begin: %w", err)
	}
	defer im.rollback(ctx, tx)
	empty, err := dbq.New(tx).IsContentEmpty(ctx)
	if err != nil {
		return false, fmt.Errorf("seed: check content: %w", err)
	}
	return empty, nil
}

// readFile reads and strictly parses the seed file.
func readFile(path string) (*File, error) {
	if path == "" {
		return nil, fmt.Errorf("%w: empty path", ErrInvalidSeed)
	}
	fh, err := os.Open(path) //nolint:gosec // operator-configured path
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidSeed, err)
	}
	defer closeQuietly(fh)
	data, err := io.ReadAll(io.LimitReader(fh, MaxSeedBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%w: read %s: %w", ErrInvalidSeed, path, err)
	}
	return Parse(data)
}

// planFile validates the whole document; every problem is reported at
// once with its YAML path.
func planFile(f *File, presets *provider.Catalog, md *content.Markdown) (*plan, error) {
	p := &problems{}
	out := &plan{}
	planSite(f.Site, md, p, out)
	catalog := planPlatforms(f.Platforms, presets, p, out)
	slugs := slugSet{}
	planCommunities(f.Communities, catalog, slugs, p, out)
	planLinks(f.Links, slugs, p, out)
	planBlocks(f, md, p, out)
	if err := p.err(); err != nil {
		return nil, err
	}
	return out, nil
}

// Parse strictly decodes a seed document (unknown keys are errors).
// Semantic validation happens in Import.
func Parse(data []byte) (*File, error) {
	if len(data) > MaxSeedBytes {
		return nil, fmt.Errorf("%w: file larger than %d bytes", ErrInvalidSeed, MaxSeedBytes)
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var f File
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("%w: decode: %w", ErrInvalidSeed, err)
	}
	if f.Version != 1 {
		return nil, fmt.Errorf("%w: version must be 1", ErrInvalidSeed)
	}
	return &f, nil
}
