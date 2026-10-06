package jobs

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// syncBuffer is a goroutine-safe log sink.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func testLogger() (*slog.Logger, *syncBuffer) {
	buf := &syncBuffer{}
	return slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})), buf
}

type fakeLock struct {
	leader   atomic.Bool
	err      error
	released atomic.Int32
	checks   atomic.Int32
}

func (l *fakeLock) TryAcquire(context.Context) (bool, error) {
	l.checks.Add(1)
	return l.leader.Load(), l.err
}

func (l *fakeLock) Release(context.Context) error {
	l.released.Add(1)
	return errors.New("release failed")
}

func TestNewSchedulerValidation(t *testing.T) {
	logger, _ := testLogger()
	run := func(context.Context) error { return nil }
	tests := []struct {
		name string
		jobs []Job
	}{
		{"no name", []Job{{Interval: time.Second, Run: run}}},
		{"duplicate", []Job{{Name: "a", Interval: time.Second, Run: run}, {Name: "a", Interval: time.Second, Run: run}}},
		{"zero interval", []Job{{Name: "a", Run: run}}},
		{"negative timeout", []Job{{Name: "a", Interval: time.Second, Timeout: -1, Run: run}}},
		{"no run", []Job{{Name: "a", Interval: time.Second}}},
	}
	for _, tt := range tests {
		if _, err := NewScheduler(nil, logger, tt.jobs...); err == nil {
			t.Errorf("%s: expected error", tt.name)
		}
	}
	if _, err := NewScheduler(nil, nil); err == nil {
		t.Error("nil logger must fail")
	}
}

func TestSchedulerRunsJobsAndHonoursLeadership(t *testing.T) {
	logger, logs := testLogger()
	lock := &fakeLock{}
	var plain, leaderOnly, panics atomic.Int32
	jobs := []Job{
		{Name: "plain", Interval: 10 * time.Millisecond, Run: func(context.Context) error { plain.Add(1); return nil }},
		{Name: "leader", Interval: 10 * time.Millisecond, LeaderOnly: true, Run: func(context.Context) error {
			leaderOnly.Add(1)
			return nil
		}},
		{Name: "panics", Interval: 10 * time.Millisecond, Run: func(context.Context) error {
			panics.Add(1)
			panic("boom")
		}},
	}
	s, err := NewScheduler(lock, logger, jobs...)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	waitFor(t, func() bool { return plain.Load() >= 3 && lock.checks.Load() >= 2 })
	if leaderOnly.Load() != 0 {
		t.Error("leader-only job ran without leadership")
	}
	lock.leader.Store(true)
	waitFor(t, func() bool { return leaderOnly.Load() >= 2 })
	cancel()
	if err := <-done; err != nil {
		t.Errorf("Run = %v", err)
	}
	if lock.released.Load() != 1 || panics.Load() == 0 {
		t.Errorf("released=%d panics=%d", lock.released.Load(), panics.Load())
	}
	out := logs.String()
	if !strings.Contains(out, "panicked: boom") || !strings.Contains(out, "release leader lock") {
		t.Errorf("logs = %s", out)
	}
	if err := s.Run(context.Background()); err == nil {
		t.Error("second Run must fail")
	}
}

func TestSchedulerLockErrorSkipsRun(t *testing.T) {
	logger, logs := testLogger()
	lock := &fakeLock{err: errors.New("db gone")}
	var runs atomic.Int32
	s, _ := NewScheduler(lock, logger, Job{
		Name: "leader", Interval: 5 * time.Millisecond, LeaderOnly: true,
		Run: func(context.Context) error { runs.Add(1); return nil },
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	waitFor(t, func() bool { return lock.checks.Load() >= 2 })
	cancel()
	<-done
	if runs.Load() != 0 || !strings.Contains(logs.String(), "leader lock check failed") {
		t.Errorf("runs=%d logs=%s", runs.Load(), logs.String())
	}
}

func TestSchedulerNilLockRunsLeaderJobs(t *testing.T) {
	logger, _ := testLogger()
	var runs atomic.Int32
	s, _ := NewScheduler(nil, logger, Job{
		Name: "leader", Interval: time.Hour, LeaderOnly: true,
		Run: func(context.Context) error { runs.Add(1); return nil },
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	waitFor(t, func() bool { return runs.Load() == 1 })
	cancel()
	if err := <-done; err != nil {
		t.Error(err)
	}
}

func TestRunSafe(t *testing.T) {
	logger, logs := testLogger()
	err := runSafe(context.Background(), logger, Job{Name: "slow", Interval: 10 * time.Millisecond, Run: func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	}})
	if !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(logs.String(), "job failed") {
		t.Errorf("err = %v logs=%s", err, logs.String())
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	logger2, logs2 := testLogger()
	err = runSafe(ctx, logger2, Job{Name: "interrupted", Interval: time.Second, Run: func(ctx context.Context) error { return ctx.Err() }})
	if !errors.Is(err, context.Canceled) || !strings.Contains(logs2.String(), "job interrupted") || strings.Contains(logs2.String(), "ERROR") {
		t.Errorf("err = %v logs=%s", err, logs2.String())
	}
}

func TestRunOnceSkipsCancelled(t *testing.T) {
	logger, _ := testLogger()
	s, _ := NewScheduler(nil, logger)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ran := false
	s.runOnce(ctx, Job{Name: "x", Interval: time.Second, Run: func(context.Context) error { ran = true; return nil }})
	if ran {
		t.Error("cancelled context must skip the run")
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met in time")
		}
		time.Sleep(2 * time.Millisecond)
	}
}
