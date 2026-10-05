package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
)

// backgroundTask runs one long-lived function (the job scheduler) that is
// started once the instance is ready and stopped during shutdown.
type backgroundTask struct {
	name   string
	logger *slog.Logger

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

func newBackgroundTask(name string, logger *slog.Logger) *backgroundTask {
	return &backgroundTask{name: name, logger: logger}
}

// start runs fn in a goroutine with a context derived from ctx. Later
// calls are ignored.
func (b *backgroundTask) start(ctx context.Context, fn func(context.Context) error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.done != nil {
		return
	}
	runCtx, cancel := context.WithCancel(ctx)
	b.cancel, b.done = cancel, make(chan struct{})
	go func(done chan struct{}) {
		defer close(done)
		if err := fn(runCtx); err != nil && !errors.Is(err, context.Canceled) {
			b.logger.Error("background task stopped with error", slog.String("task", b.name), slog.Any("error", err))
		}
	}(b.done)
}

// stop cancels the task and waits for it to return (or ctx to end). A
// task that never started stops immediately.
func (b *backgroundTask) stop(ctx context.Context) error {
	b.mu.Lock()
	cancel, done := b.cancel, b.done
	b.mu.Unlock()
	if done == nil {
		return nil
	}
	cancel()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("%s did not stop: %w", b.name, ctx.Err())
	}
}
