package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"reflect"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/Nanako1900/linksPage/internal/provider"
	"github.com/Nanako1900/linksPage/internal/store/dbq"
)

// persistSuccess stores a provider result (including logical outcomes
// such as widget disabled or unavailable): fail_count resets, last_ok_at
// and fetched_at become now.
func (r *refresher) persistSuccess(ctx context.Context, row dbq.ListDueCommunitiesRow, prev *provider.Snapshot,
	snap provider.Snapshot, now time.Time, interval time.Duration,
) (bool, error) {
	s := provider.SanitizeSnapshot(snap)
	s.FetchedAt = now
	if !s.State.Valid() || s.State == provider.StatePending {
		s.State = provider.StateLive
	}
	data, err := provider.EncodeSnapshotData(s)
	if err != nil {
		return false, err
	}
	err = r.deps.Store.UpsertProviderSnapshot(ctx, dbq.UpsertProviderSnapshotParams{
		CommunityID: row.ID,
		Data:        data,
		State:       string(s.State),
		ErrCode:     optional(s.ErrCode),
		FetchedAt:   timestamptz(now),
		LastOkAt:    timestamptz(now),
		NextFetchAt: timestamptz(now.Add(interval)),
		FailCount:   0,
	})
	if err != nil {
		return false, fmt.Errorf("save snapshot: %w", err)
	}
	r.deps.Live.Put(row.ID.String(), s)
	if s.ErrCode != "" && (prev == nil || prev.ErrCode != s.ErrCode) {
		r.deps.Logger.WarnContext(ctx, "community provider reported a problem",
			slog.String("community", row.Slug), slog.String("state", string(s.State)), slog.String("code", s.ErrCode))
	}
	return prev == nil || !sameLive(*prev, s), nil
}

// persistFailure keeps the previous data (members included), marks it
// stale (pending stays pending; static and unavailable keep their state
// because they carry no live data) and postpones the next fetch with
// backoff.
func (r *refresher) persistFailure(ctx context.Context, row dbq.ListDueCommunitiesRow, prev *provider.Snapshot,
	now time.Time, interval time.Duration, fetchErr error,
) (bool, error) {
	failCount := deref(row.FailCount) + 1
	delay := retryDelay(interval, failCount, fetchErr)
	s := provider.Snapshot{State: provider.StatePending, Channels: []provider.Channel{}}
	if prev != nil {
		s = *prev
		s.State = failedState(prev.State)
	}
	s.ErrCode = provider.ErrCode(fetchErr)
	// fetched_at is the time of this attempt in both copies: the page
	// builder keeps the in-memory snapshot (which alone carries Users) only
	// when it is not older than the row. last_ok_at keeps the last success.
	s.FetchedAt = now
	data, err := provider.EncodeSnapshotData(s)
	if err != nil {
		return false, err
	}
	err = r.deps.Store.UpsertProviderSnapshot(ctx, dbq.UpsertProviderSnapshotParams{
		CommunityID: row.ID,
		Data:        data,
		State:       string(s.State),
		ErrCode:     optional(s.ErrCode),
		FetchedAt:   timestamptz(now),
		LastOkAt:    row.LastOkAt,
		NextFetchAt: timestamptz(now.Add(delay)),
		FailCount:   failCount,
	})
	if err != nil {
		return false, fmt.Errorf("save failed snapshot: %w", err)
	}
	r.deps.Live.Put(row.ID.String(), s)
	level := slog.LevelInfo
	if failCount >= NotifyAfterFailures {
		level = slog.LevelWarn
	}
	r.deps.Logger.Log(ctx, level, "community provider fetch failed",
		slog.String("community", row.Slug), slog.String("code", s.ErrCode), slog.Int("failures", int(failCount)),
		slog.Duration("retry_in", delay), slog.Any("error", fetchErr))
	return prev == nil || prev.State != s.State, nil
}

func failedState(prev provider.State) provider.State {
	switch prev {
	case provider.StatePending, provider.StateStatic, provider.StateUnavailable:
		return prev
	default:
		return provider.StateStale
	}
}

// retryDelay is max(RetryAfter, interval × 2^(failures-1)) capped at
// MaxBackoff, but never shorter than interval (MinInterval stays honoured).
func retryDelay(interval time.Duration, failures int32, err error) time.Duration {
	delay := interval
	for i := int32(1); i < failures && delay < MaxBackoff; i++ {
		delay *= 2
	}
	if ra, ok := provider.RetryAfter(err); ok && ra > delay {
		delay = ra
	}
	return max(min(delay, MaxBackoff), interval)
}

// sameLive reports whether two snapshots publish the same data (fetch
// timestamps ignored).
func sameLive(a, b provider.Snapshot) bool {
	return reflect.DeepEqual(forCompare(a), forCompare(b))
}

func forCompare(s provider.Snapshot) provider.Snapshot {
	s.FetchedAt = time.Time{}
	s.InviteFetchedAt = nil
	if len(s.Users) == 0 {
		s.Users = nil
	}
	if len(s.Channels) == 0 {
		s.Channels = nil
	}
	if s.InviteExpiresAt != nil {
		t := s.InviteExpiresAt.UTC()
		s.InviteExpiresAt = &t
	}
	return s
}

func timestamptz(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

func timePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

// intervalDuration converts communities.refresh_interval (months count as
// 30 days).
func intervalDuration(iv pgtype.Interval) time.Duration {
	if !iv.Valid {
		return 0
	}
	const day = 24 * time.Hour
	return time.Duration(iv.Microseconds)*time.Microsecond +
		time.Duration(iv.Days)*day + time.Duration(iv.Months)*30*day
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func deref[T any](p *T) T {
	var zero T
	if p == nil {
		return zero
	}
	return *p
}
