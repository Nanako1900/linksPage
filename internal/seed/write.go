package seed

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/Nanako1900/linksPage/internal/media"
	"github.com/Nanako1900/linksPage/internal/site"
	"github.com/Nanako1900/linksPage/internal/store/dbq"
)

// actor is recorded in created_by / updated_by.
const actor = "seed"

// blockSortStep leaves room for inserting blocks between seeded ones.
const blockSortStep = 10

// writer performs the inserts of one import inside a transaction.
type writer struct {
	q           *dbq.Queries
	plan        *plan
	images      map[string]media.Result
	communities map[string]pgtype.UUID // slug → id
	links       map[string]pgtype.UUID // slug → id
	result      Result
}

// write runs the import transaction; it is a no-op when content exists.
func (im *Importer) write(ctx context.Context, pl *plan, images map[string]media.Result) (Result, error) {
	tx, err := im.deps.DB.Begin(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("seed: begin: %w", err)
	}
	defer im.rollback(ctx, tx)
	q := dbq.New(tx)
	empty, err := q.IsContentEmpty(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("seed: check content: %w", err)
	}
	if !empty {
		return Result{Skipped: true}, nil
	}
	w := &writer{q: q, plan: pl, images: images, communities: map[string]pgtype.UUID{}, links: map[string]pgtype.UUID{}}
	for _, step := range []func(context.Context) error{w.media, w.settings, w.platforms, w.insertCommunities, w.insertLinks, w.insertBlocks} {
		if err := step(ctx); err != nil {
			return Result{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Result{}, fmt.Errorf("seed: commit: %w", err)
	}
	return w.result, nil
}

// rollback ends the transaction when it was not committed (after Commit
// pgx reports ErrTxClosed, which is expected).
func (im *Importer) rollback(ctx context.Context, tx pgx.Tx) {
	if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		im.deps.Logger.Warn("seed: rollback failed", slog.Any("error", err))
	}
}

func (w *writer) key(path string, kind media.Kind) *string {
	if path == "" {
		return nil
	}
	res, ok := w.images[imageRef{path: path, kind: kind}.id()]
	if !ok {
		return nil
	}
	k := res.Primary.Key
	return &k
}

func (w *writer) media(ctx context.Context) error {
	by := actor
	for _, ref := range w.plan.images() {
		res := w.images[ref.id()]
		rows := []dbq.InsertMediaParams{mediaParams(res.Primary, ref.kind, res.VariantsJSON(), &by)}
		for _, v := range res.Variants {
			rows = append(rows, mediaParams(v, ref.kind, []byte("{}"), &by))
		}
		for _, r := range rows {
			if err := w.q.InsertMedia(ctx, r); err != nil {
				return fmt.Errorf("seed: insert media %s: %w", r.Key, err)
			}
		}
	}
	return nil
}

func mediaParams(s media.Stored, kind media.Kind, variants []byte, by *string) dbq.InsertMediaParams {
	return dbq.InsertMediaParams{
		Key: s.Key, Kind: string(kind), ContentType: s.ContentType,
		Bytes: int32(s.Bytes), Width: int32(s.Width), Height: int32(s.Height), //nolint:gosec // bounded by media limits
		Variants: variants, CreatedBy: by,
	}
}

// settings overlays the seeded keys on the stored settings and bumps the
// version (also when the seed has no site section).
func (w *writer) settings(ctx context.Context) error {
	row, err := w.q.GetSiteSettings(ctx)
	if err != nil {
		return fmt.Errorf("seed: read settings: %w", err)
	}
	seeded := withMediaKeys(w.plan.settings, deref(w.key(w.plan.avatar, media.KindAvatar)), deref(w.key(w.plan.ogImage, media.KindOG)))
	data, err := mergeSettings(row.Data, seeded)
	if err != nil {
		return err
	}
	by := actor
	if _, err := w.q.ReplaceSiteSettingsData(ctx, dbq.ReplaceSiteSettingsDataParams{Data: data, UpdatedBy: &by, PageID: row.PageID}); err != nil {
		return fmt.Errorf("seed: write settings: %w", err)
	}
	return nil
}

// mergeSettings overlays seeded top-level keys on the stored JSON object.
// When the stored data cannot be merged into valid settings, only the
// (already validated) seeded keys are kept.
func mergeSettings(stored []byte, seeded map[string]any) ([]byte, error) {
	only, err := json.Marshal(seeded)
	if err != nil {
		return nil, fmt.Errorf("seed: encode settings: %w", err)
	}
	merged := map[string]json.RawMessage{}
	if len(stored) > 0 && json.Unmarshal(stored, &merged) != nil {
		return only, nil
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(only, &top); err != nil {
		return nil, fmt.Errorf("seed: encode settings: %w", err)
	}
	for k, v := range top {
		merged[k] = v
	}
	out, err := json.Marshal(merged)
	if err != nil {
		return nil, fmt.Errorf("seed: encode settings: %w", err)
	}
	if _, err := site.ParseSettings(out); err != nil {
		return only, nil
	}
	return out, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
