// Package jobs runs the periodic background jobs (doc 4.5): a ticker-based
// scheduler where leader-only jobs run on the instance holding a
// PostgreSQL advisory lock, each run wrapped in runSafe (recover, per-run
// timeout, slog).
package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"
)

// LeaderLockKey is the pg_try_advisory_lock key for job leadership.
const LeaderLockKey int64 = 0x4C50_4A4F_4253 // "LPJOBS"

// Job is one periodic task.
type Job struct {
	// Name identifies the job in logs.
	Name string
	// Interval between run starts (first run right after Start).
	Interval time.Duration
	// Timeout bounds a single run (defaults to Interval).
	Timeout time.Duration
	// LeaderOnly jobs run only while this instance holds the leader lock.
	LeaderOnly bool
	// Run performs one iteration; it must be idempotent.
	Run func(ctx context.Context) error
}

// LeaderLock elects one leader among instances.
type LeaderLock interface {
	// TryAcquire returns true while this instance is (or just became) the
	// leader. It must be cheap to call before every leader-only run.
	TryAcquire(ctx context.Context) (bool, error)
	// Release gives up leadership (on shutdown).
	Release(ctx context.Context) error
}

// Scheduler runs jobs until its context is cancelled.
type Scheduler struct {
	jobs    []Job
	lock    LeaderLock
	logger  *slog.Logger
	group   *errgroup.Group
	started atomic.Bool
}

// releaseTimeout bounds the leader lock release on shutdown.
const releaseTimeout = 5 * time.Second

// NewScheduler returns a scheduler; lock may be nil for single-instance
// tests (every job then runs).
func NewScheduler(lock LeaderLock, logger *slog.Logger, jobs ...Job) (*Scheduler, error) {
	if logger == nil {
		return nil, errors.New("jobs: logger is required")
	}
	seen := make(map[string]bool, len(jobs))
	for i, j := range jobs {
		switch {
		case j.Name == "":
			return nil, fmt.Errorf("jobs[%d]: name is required", i)
		case seen[j.Name]:
			return nil, fmt.Errorf("jobs[%d]: duplicate name %q", i, j.Name)
		case j.Interval <= 0:
			return nil, fmt.Errorf("job %s: interval must be positive", j.Name)
		case j.Timeout < 0:
			return nil, fmt.Errorf("job %s: timeout must not be negative", j.Name)
		case j.Run == nil:
			return nil, fmt.Errorf("job %s: run is required", j.Name)
		}
		seen[j.Name] = true
	}
	return &Scheduler{jobs: slices.Clone(jobs), lock: lock, logger: logger}, nil
}

// Run starts every job and blocks until ctx is cancelled and all running
// iterations returned; the leader lock is released on exit. Run may only
// be called once.
func (s *Scheduler) Run(ctx context.Context) error {
	if !s.started.CompareAndSwap(false, true) {
		return errors.New("jobs: scheduler already started")
	}
	s.group = &errgroup.Group{}
	for _, j := range s.jobs {
		s.group.Go(func() error {
			s.loop(ctx, j)
			return nil
		})
	}
	err := s.group.Wait()
	if s.lock != nil {
		releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), releaseTimeout)
		defer cancel()
		if rerr := s.lock.Release(releaseCtx); rerr != nil {
			s.logger.Warn("release leader lock", slog.Any("error", rerr))
		}
	}
	return err
}

// loop runs job immediately and then every Interval until ctx is done.
// Iterations of one job never overlap; a slow run delays the next tick.
func (s *Scheduler) loop(ctx context.Context, job Job) {
	ticker := time.NewTicker(job.Interval)
	defer ticker.Stop()
	for {
		s.runOnce(ctx, job)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// runOnce checks leadership for leader-only jobs and runs one iteration.
func (s *Scheduler) runOnce(ctx context.Context, job Job) {
	if ctx.Err() != nil {
		return
	}
	if job.LeaderOnly && s.lock != nil {
		leader, err := s.lock.TryAcquire(ctx)
		if err != nil {
			if ctx.Err() == nil {
				s.logger.Warn("leader lock check failed", slog.String("job", job.Name), slog.Any("error", err))
			}
			return
		}
		if !leader {
			return
		}
	}
	_ = runSafe(ctx, s.logger, job) // runSafe logs failures
}

// runSafe runs one iteration with a timeout, converting panics to errors
// and logging failures (job name, duration, error) without crashing.
func runSafe(ctx context.Context, logger *slog.Logger, job Job) (err error) {
	timeout := job.Timeout
	if timeout <= 0 {
		timeout = job.Interval
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	start := time.Now()
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("job %s panicked: %v", job.Name, r)
		}
		switch {
		case err == nil:
		case ctx.Err() != nil:
			// Shutdown interrupted the run; not a failure worth an ERROR.
			logger.Debug("job interrupted", slog.String("job", job.Name), slog.Any("error", err))
		default:
			logger.Error("job failed", slog.String("job", job.Name),
				slog.Duration("took", time.Since(start)), slog.Any("error", err))
		}
	}()
	return job.Run(runCtx)
}
