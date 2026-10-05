package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/sync/errgroup"

	"github.com/Nanako1900/linksPage/internal/provider"
	"github.com/Nanako1900/linksPage/internal/store/dbq"
)

// Refresh cadence (doc 4.5 + spikes/providers.md).
const (
	// RefreshTick is how often due communities are looked up.
	RefreshTick = 30 * time.Second
	// RefreshBatch caps communities fetched per tick.
	RefreshBatch = 10
	// MaxBackoff caps the failure backoff.
	MaxBackoff = time.Hour
	// NotifyAfterFailures is the consecutive failure count that triggers a
	// warning log (webhook notifications arrive in M2a).
	NotifyAfterFailures = 3
)

// RefreshJobName identifies the refresh job in logs.
const RefreshJobName = "provider-refresh"

// Internal refresh limits.
const (
	// refreshConcurrency bounds parallel provider fetches per tick.
	refreshConcurrency = 4
	// fetchTimeout bounds one community (widget + invite at 10 s each).
	fetchTimeout = 45 * time.Second
	// refreshTimeout bounds one tick (batch / concurrency × fetchTimeout).
	refreshTimeout = 3 * time.Minute
)

// RefreshStore is the subset of dbq.Queries used by the refresh job.
type RefreshStore interface {
	ListDueCommunities(ctx context.Context, arg dbq.ListDueCommunitiesParams) ([]dbq.ListDueCommunitiesRow, error)
	GetProviderSnapshot(ctx context.Context, communityID pgtype.UUID) (dbq.ProviderSnapshot, error)
	UpsertProviderSnapshot(ctx context.Context, arg dbq.UpsertProviderSnapshotParams) error
}

// RefreshDeps are the refresh job's dependencies.
type RefreshDeps struct {
	Store    RefreshStore
	Registry *provider.Registry
	Live     *provider.LiveStore
	// OnChange is called (once per tick) after any snapshot changed so the
	// public page is rebuilt.
	OnChange func(ctx context.Context)
	Logger   *slog.Logger
	Now      func() time.Time
}

// NewRefreshJob returns the leader-only provider refresh job. Per due
// community: ValidateConfig → Fetch(previous) → SanitizeSnapshot →
// LiveStore.Put → UpsertProviderSnapshot (data without users,
// next_fetch_at = now + max(refresh_interval, MinInterval)). On error the
// previous data is kept with state stale (pending stays pending),
// fail_count+1 and next_fetch_at = now + max(RetryAfter, exponential
// backoff capped at MaxBackoff).
func NewRefreshJob(deps RefreshDeps) (Job, error) {
	if deps.Store == nil || deps.Registry == nil || deps.Live == nil {
		return Job{}, errors.New("jobs: refresh needs store, registry and live store")
	}
	if deps.Logger == nil {
		deps.Logger = slog.New(slog.DiscardHandler)
	}
	if deps.Now == nil {
		deps.Now = time.Now
	}
	r := &refresher{deps: deps}
	return Job{
		Name:       RefreshJobName,
		Interval:   RefreshTick,
		Timeout:    refreshTimeout,
		LeaderOnly: true,
		Run:        r.run,
	}, nil
}

type refresher struct {
	deps RefreshDeps
	// warmed is set after the first listing (see dueRows).
	warmed atomic.Bool
}

// run refreshes up to RefreshBatch due communities.
func (r *refresher) run(ctx context.Context) error {
	now := r.deps.Now().UTC()
	rows, err := r.dueRows(ctx, now)
	if err != nil {
		return err
	}
	var (
		changed atomic.Bool
		mu      sync.Mutex
		errs    []error
		g       errgroup.Group
	)
	g.SetLimit(refreshConcurrency)
	for _, row := range rows {
		g.Go(func() error {
			ch, err := r.refreshOne(ctx, row, now)
			if ch {
				changed.Store(true)
			}
			if err != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("community %s: %w", row.Slug, err))
				mu.Unlock()
			}
			return nil
		})
	}
	_ = g.Wait() // goroutines always return nil; errors are collected
	if changed.Load() && r.deps.OnChange != nil {
		r.deps.OnChange(ctx)
	}
	return errors.Join(errs...)
}

// refreshOne fetches one community and persists the outcome. It reports
// whether the published data changed.
func (r *refresher) refreshOne(ctx context.Context, row dbq.ListDueCommunitiesRow, now time.Time) (bool, error) {
	prev, err := r.previous(ctx, row.ID)
	if err != nil {
		return false, err
	}
	p, ok := r.deps.Registry.Get(row.Provider)
	if !ok {
		fe := &provider.FetchError{Code: provider.CodeUnknownKind, Err: fmt.Errorf("no provider %q", row.Provider)}
		return r.persistFailure(ctx, row, prev, now, MaxBackoff, fe)
	}
	interval := max(intervalDuration(row.RefreshInterval), p.MinInterval())
	cfg, err := p.ValidateConfig(provider.ConfigInput{
		ExternalID: deref(row.ExternalID), InviteURL: deref(row.InviteUrl), Raw: row.Config,
	})
	if err != nil {
		r.deps.Logger.ErrorContext(ctx, "invalid community provider config",
			slog.String("community", row.Slug), slog.Any("error", err))
		bad := provider.Snapshot{State: provider.StateStatic, ErrCode: provider.CodeConfigInvalid}
		return r.persistSuccess(ctx, row, prev, bad, now, max(interval, MaxBackoff))
	}
	fetchCtx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	snap, err := p.Fetch(fetchCtx, provider.FetchInput{Config: cfg, Previous: fetchPrevious(prev), Now: now})
	if err == nil && snap == nil {
		err = errors.New("provider returned no snapshot")
	}
	if err != nil {
		return r.persistFailure(ctx, row, prev, now, interval, err)
	}
	return r.persistSuccess(ctx, row, prev, *snap, now, interval)
}

// previous returns the last snapshot: the in-memory one (with users) when
// this instance has it, otherwise the persisted row; nil when none.
func (r *refresher) previous(ctx context.Context, id pgtype.UUID) (*provider.Snapshot, error) {
	if s, ok := r.deps.Live.Live(id.String()); ok {
		return &s, nil
	}
	row, err := r.deps.Store.GetProviderSnapshot(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load snapshot: %w", err)
	}
	s, err := provider.SnapshotFromRow(provider.SnapshotRow{
		Data: row.Data, State: row.State, ErrCode: row.ErrCode, FetchedAt: timePtr(row.FetchedAt),
	})
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// fetchPrevious hides pending rows (no data yet) from providers.
func fetchPrevious(prev *provider.Snapshot) *provider.Snapshot {
	if prev == nil || prev.State == provider.StatePending {
		return nil
	}
	return prev
}
