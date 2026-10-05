package seed

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/Nanako1900/linksPage/internal/media"
	"github.com/Nanako1900/linksPage/internal/site"
	"github.com/Nanako1900/linksPage/internal/store/dbq"
)

func (w *writer) platforms(ctx context.Context) error {
	for _, p := range w.plan.platforms {
		params := p.params
		params.IconKey = w.key(p.icon, media.KindIcon)
		if err := w.q.InsertCustomPlatform(ctx, params); err != nil {
			return fmt.Errorf("seed: insert platform %s: %w", params.ID, err)
		}
	}
	return nil
}

func (w *writer) insertCommunities(ctx context.Context) error {
	for _, c := range w.plan.communities {
		params := c.params
		params.IconKey = w.key(c.icon, media.KindIcon)
		id, err := w.q.InsertCommunity(ctx, params)
		if err != nil {
			return fmt.Errorf("seed: insert community %s: %w", params.Slug, err)
		}
		w.communities[params.Slug] = id
		if err := w.insertQR(ctx, id, c); err != nil {
			return err
		}
		if c.fetch {
			if err := w.q.EnsureProviderSnapshot(ctx, id); err != nil {
				return fmt.Errorf("seed: create snapshot for %s: %w", params.Slug, err)
			}
		}
	}
	w.result.Communities = len(w.plan.communities)
	return nil
}

func (w *writer) insertQR(ctx context.Context, id pgtype.UUID, c communityPlan) error {
	key := w.key(c.qrImage, media.KindQR)
	if key == nil {
		return nil
	}
	note, err := json.Marshal(c.qrNote)
	if err != nil {
		return fmt.Errorf("seed: encode qr note: %w", err)
	}
	if _, err := w.q.InsertQRCode(ctx, dbq.InsertQRCodeParams{CommunityID: id, MediaKey: *key, Note: note}); err != nil {
		return fmt.Errorf("seed: insert qr code for %s: %w", c.params.Slug, err)
	}
	return nil
}

func (w *writer) insertLinks(ctx context.Context) error {
	for _, l := range w.plan.links {
		params := l.params
		params.IconKey = w.key(l.icon, media.KindIcon)
		id, err := w.q.InsertLink(ctx, params)
		if err != nil {
			return fmt.Errorf("seed: insert link %s: %w", params.Slug, err)
		}
		w.links[params.Slug] = id
	}
	w.result.Links = len(w.plan.links)
	return nil
}

func (w *writer) insertBlocks(ctx context.Context) error {
	for i, b := range w.plan.blocks {
		params, err := w.blockParams(b)
		if err != nil {
			return err
		}
		params.SortOrder = int32((i + 1) * blockSortStep)
		if _, err := w.q.InsertBlock(ctx, params); err != nil {
			return fmt.Errorf("seed: insert block %d: %w", i, err)
		}
	}
	w.result.Blocks = len(w.plan.blocks)
	return nil
}

func (w *writer) blockParams(b blockPlan) (dbq.InsertBlockParams, error) {
	p := dbq.InsertBlockParams{PageID: 1, Kind: string(b.kind), Visible: b.visible, VisibleFrom: tstz(b.from), VisibleTo: tstz(b.to)}
	var data any = struct{}{}
	switch b.kind {
	case site.BlockCommunity:
		p.CommunityID = w.communities[b.community]
	case site.BlockLink:
		p.LinkID = w.links[b.link]
	case site.BlockHeading:
		data = b.heading
	case site.BlockText:
		data = b.text
	case site.BlockSocialRow:
		ids := make([]string, 0, len(b.social))
		for _, slug := range b.social {
			ids = append(ids, w.links[slug].String())
		}
		data = site.SocialRowBlockData{LinkIDs: ids}
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return p, fmt.Errorf("seed: encode block data: %w", err)
	}
	p.Data = raw
	return p, nil
}

func tstz(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: t.UTC(), Valid: true}
}
