package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Nanako1900/linksPage/internal/store/dbq"
)

// assertM1GoVisibility checks that /go/{slug} only resolves content that is
// published on the page right now (seedM1Blocks: discord visible, kook
// hidden, qq scheduled for later, blog visible for two more hours).
func assertM1GoVisibility(ctx context.Context, t *testing.T, q *dbq.Queries, pool *pgxpool.Pool, f m1Fixture) {
	t.Helper()
	for _, slug := range []string{"kook", "qq"} {
		if g, err := q.GetGoCommunity(ctx, slug); !errors.Is(err, pgx.ErrNoRows) {
			t.Errorf("go community %s (not published) = %+v err=%v", slug, g, err)
		}
	}
	if l, err := q.GetGoLink(ctx, "blog"); err != nil || l.ID != f.link || l.Url != "https://example.com" {
		t.Errorf("go link blog = %+v err=%v", l, err)
	}

	// Once the scheduled block starts, qq resolves with its QR code.
	if _, err := pool.Exec(ctx, `UPDATE page_blocks SET visible_from = $1 WHERE community_id = $2`, f.now.Add(-time.Minute), f.qq); err != nil {
		t.Fatal(err)
	}
	if g, err := q.GetGoCommunity(ctx, "qq"); err != nil || g.ID != f.qq || g.SnapshotState != nil || g.QrID != f.qr {
		t.Errorf("go community qq (published) = %+v err=%v", g, err)
	}

	// A link outside any block is unknown; listing it in a visible social
	// row publishes it, an expired social row does not.
	id, err := q.InsertLink(ctx, dbq.InsertLinkParams{
		PageID: 1, Slug: "social-x", Kind: "social", Label: []byte(`{"en":"X"}`), Url: "https://x.example/me",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.GetGoLink(ctx, "social-x"); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("unplaced link: err = %v, want no rows", err)
	}
	row := []byte(`{"linkIds":["` + id.String() + `"]}`)
	if _, err := q.InsertBlock(ctx, dbq.InsertBlockParams{
		PageID: 1, Kind: "social_row", Data: row, SortOrder: 90, Visible: true, VisibleTo: ts(f.now.Add(-time.Minute)),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := q.GetGoLink(ctx, "social-x"); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("link in an expired social row: err = %v, want no rows", err)
	}
	if _, err := q.InsertBlock(ctx, dbq.InsertBlockParams{PageID: 1, Kind: "social_row", Data: row, SortOrder: 91, Visible: true}); err != nil {
		t.Fatal(err)
	}
	if l, err := q.GetGoLink(ctx, "social-x"); err != nil || l.ID != id {
		t.Errorf("link in a visible social row = %+v err=%v", l, err)
	}
}
