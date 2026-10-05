package media

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"golang.org/x/sync/semaphore"
)

// Semaphore is the process-wide memory-weighted semaphore shared by image
// processing (w×h×4×2 bytes per image) and argon2 (PHC m, M2a).
type Semaphore struct {
	w      *semaphore.Weighted
	budget int64
}

// NewSemaphore returns a semaphore with budget bytes (MemoryBudget).
func NewSemaphore(budget int64) *Semaphore {
	return &Semaphore{w: semaphore.NewWeighted(budget), budget: budget}
}

// Budget returns the total weight the semaphore admits.
func (s *Semaphore) Budget() int64 { return s.budget }

// Acquire waits up to wait for weight bytes and returns a release func;
// it fails with ErrBusy on timeout (callers answer 503 + Retry-After) and
// rejects weights above the budget with ErrDimensions. A cancelled ctx
// returns ctx's error. The release func is safe to call more than once.
func (s *Semaphore) Acquire(ctx context.Context, weight int64, wait time.Duration) (func(), error) {
	if weight <= 0 {
		return nil, fmt.Errorf("media: invalid semaphore weight %d", weight)
	}
	if weight > s.budget {
		return nil, fmt.Errorf("media: needs %d bytes of %d: %w", weight, s.budget, ErrDimensions)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	wctx, cancel := context.WithTimeoutCause(ctx, wait, ErrBusy)
	defer cancel()
	if err := s.w.Acquire(wctx, weight); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if errors.Is(context.Cause(wctx), ErrBusy) {
			return nil, ErrBusy
		}
		return nil, err
	}
	var once sync.Once
	return func() { once.Do(func() { s.w.Release(weight) }) }, nil
}

// ImageWeight is the semaphore weight for decoding a w×h image.
func ImageWeight(w, h int) int64 { return int64(w) * int64(h) * 4 * 2 }
