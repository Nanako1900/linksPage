package jobs

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/Nanako1900/linksPage/internal/store/dbq"
)

// warmUpHorizon: on the first run after start, healthy communities due
// within this horizon are fetched early. Member lists are only kept in
// memory, so without this a restart would hide them until each
// community's next scheduled fetch (up to its refresh interval).
const warmUpHorizon = 24 * time.Hour

// dueRows lists the communities to refresh in this tick. The first
// successful listing additionally includes healthy (fail_count 0)
// communities that are not yet in the live store, up to RefreshBatch.
func (r *refresher) dueRows(ctx context.Context, now time.Time) ([]dbq.ListDueCommunitiesRow, error) {
	rows, err := r.list(ctx, now)
	if err != nil {
		return nil, err
	}
	if r.warmed.Load() {
		return rows, nil
	}
	early, err := r.list(ctx, now.Add(warmUpHorizon))
	if err != nil {
		return nil, err
	}
	r.warmed.Store(true)
	return r.mergeWarmUp(rows, early), nil
}

func (r *refresher) list(ctx context.Context, at time.Time) ([]dbq.ListDueCommunitiesRow, error) {
	rows, err := r.deps.Store.ListDueCommunities(ctx, dbq.ListDueCommunitiesParams{
		Now: timestamptz(at), MaxRows: RefreshBatch,
	})
	if err != nil {
		return nil, fmt.Errorf("list due communities: %w", err)
	}
	return rows, nil
}

// mergeWarmUp appends early rows that are healthy and missing from the
// live store to the due rows, without duplicates, capped at RefreshBatch.
func (r *refresher) mergeWarmUp(due, early []dbq.ListDueCommunitiesRow) []dbq.ListDueCommunitiesRow {
	seen := make(map[pgtype.UUID]bool, len(due))
	out := make([]dbq.ListDueCommunitiesRow, 0, RefreshBatch)
	for _, row := range due {
		seen[row.ID] = true
		out = append(out, row)
	}
	for _, row := range early {
		if len(out) >= RefreshBatch {
			break
		}
		if seen[row.ID] || (row.FailCount != nil && *row.FailCount > 0) {
			continue
		}
		if _, ok := r.deps.Live.Live(row.ID.String()); ok {
			continue
		}
		seen[row.ID] = true
		out = append(out, row)
	}
	return out
}
