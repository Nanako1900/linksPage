package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Nanako1900/linksPage/internal/httpapi"
	"github.com/Nanako1900/linksPage/internal/site"
	"github.com/Nanako1900/linksPage/internal/store"
)

// syncBuffer is a goroutine-safe log sink.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func okSteps() startupSteps {
	ok := func(context.Context) error { return nil }
	return startupSteps{
		ping: ok, checkServer: ok, migrate: ok, checkCompat: ok,
		loadSnapshot: func(context.Context) (*site.Snapshot, error) { return site.DefaultSnapshot(), nil },
	}
}

func testRunner(steps startupSteps, logs io.Writer) (startupRunner, *httpapi.Readiness, *site.Holder) {
	ready := httpapi.NewReadiness(nil)
	holder := site.NewHolder(site.DefaultSnapshot())
	r := newStartupRunner(steps, holder, ready, slog.New(slog.NewTextHandler(logs, nil)))
	r.minBackoff, r.maxBackoff = time.Millisecond, 2*time.Millisecond
	return r, ready, holder
}

// runToEnd runs the startup loop and returns the cancel cause.
func runToEnd(t *testing.T, r startupRunner) error {
	t.Helper()
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	done := make(chan struct{})
	go func() { defer close(done); r.run(ctx, cancel) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("startup did not finish")
	}
	return context.Cause(ctx)
}

func TestStartupRetriesTransientErrors(t *testing.T) {
	steps := okSteps()
	var pings, checks atomic.Int32
	steps.ping = func(context.Context) error {
		if pings.Add(1) < 3 {
			return errors.New("connection refused")
		}
		return nil
	}
	// A query error right after a successful ping (e.g. 57P03) must not be fatal.
	steps.checkServer = func(context.Context) error {
		if checks.Add(1) < 2 {
			return errors.New("query server version: conn reset")
		}
		return nil
	}
	want, err := site.NewSnapshot(7, site.Page{ID: 1, Slug: "default"}, site.Default(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	steps.loadSnapshot = func(context.Context) (*site.Snapshot, error) { return want, nil }
	var logs syncBuffer
	r, ready, holder := testRunner(steps, &logs)
	if cause := runToEnd(t, r); cause != nil {
		t.Fatalf("cause = %v", cause)
	}
	if !ready.IsReady() || holder.Current() != want {
		t.Error("instance must be ready with the loaded snapshot")
	}
	if strings.Count(logs.String(), "retrying") != 3 {
		t.Errorf("logs = %s", logs.String())
	}
}

func TestStartupEscalatesLogLevel(t *testing.T) {
	steps := okSteps()
	var n atomic.Int32
	steps.migrate = func(context.Context) error {
		if n.Add(1) <= startupErrorAfter {
			return errors.New("advisory lock busy")
		}
		return nil
	}
	var logs syncBuffer
	r, _, _ := testRunner(steps, &logs)
	if cause := runToEnd(t, r); cause != nil {
		t.Fatal(cause)
	}
	if strings.Count(logs.String(), "level=ERROR") != 1 || strings.Count(logs.String(), "level=WARN") != startupErrorAfter-1 {
		t.Errorf("logs = %s", logs.String())
	}
}

func TestStartupFatalErrors(t *testing.T) {
	tests := []struct {
		name string
		edit func(*startupSteps)
		want error
	}{
		{"unsupported server", func(s *startupSteps) {
			s.checkServer = func(context.Context) error { return store.ErrUnsupportedServer }
		}, store.ErrUnsupportedServer},
		{"rejected migration", func(s *startupSteps) {
			s.migrate = func(context.Context) error { return store.ErrMigrationRejected }
		}, store.ErrMigrationRejected},
		{"incompatible app", func(s *startupSteps) {
			s.checkCompat = func(context.Context) error { return store.ErrIncompatibleApp }
		}, store.ErrIncompatibleApp},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			steps := okSteps()
			tt.edit(&steps)
			r, ready, _ := testRunner(steps, io.Discard)
			cause := runToEnd(t, r)
			if !errors.Is(cause, errFatalStartup) || !errors.Is(cause, tt.want) {
				t.Fatalf("cause = %v", cause)
			}
			if ready.IsReady() {
				t.Error("must not become ready")
			}
		})
	}
}

func TestStartupStopsOnCancel(t *testing.T) {
	steps := okSteps()
	steps.ping = func(context.Context) error { return errors.New("down") }
	var logs syncBuffer
	r, ready, _ := testRunner(steps, &logs)
	r.minBackoff, r.maxBackoff = time.Hour, time.Hour
	ctx, cancel := context.WithCancelCause(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); r.run(ctx, cancel) }()
	for !strings.Contains(logs.String(), "retrying") {
		time.Sleep(time.Millisecond)
	}
	cancel(nil)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("startup did not stop")
	}
	if ready.IsReady() || !errors.Is(context.Cause(ctx), context.Canceled) {
		t.Error("cancelled startup must not be ready or fatal")
	}
}

func TestStartupProbeTimeout(t *testing.T) {
	steps := okSteps()
	var n atomic.Int32
	steps.ping = func(ctx context.Context) error {
		if n.Add(1) == 1 {
			<-ctx.Done()
			return ctx.Err()
		}
		return nil
	}
	r, ready, _ := testRunner(steps, io.Discard)
	r.probeTimeout = 10 * time.Millisecond
	if cause := runToEnd(t, r); cause != nil || !ready.IsReady() {
		t.Fatalf("cause = %v ready = %v", cause, ready.IsReady())
	}
}

type fakeMigrator struct {
	pending  int
	upErr    error
	closeErr error
	ups      int
	closed   bool
}

func (f *fakeMigrator) Up(context.Context) ([]store.MigrationState, error) {
	f.ups++
	return nil, f.upErr
}

func (f *fakeMigrator) Pending(context.Context) ([]store.MigrationState, error) {
	return make([]store.MigrationState, f.pending), nil
}

func (f *fakeMigrator) Close() error {
	f.closed = true
	return f.closeErr
}

func TestMigrateOrCheck(t *testing.T) {
	ctx := context.Background()
	open := func(m *fakeMigrator) func() (migrator, error) { return func() (migrator, error) { return m, nil } }

	m := &fakeMigrator{pending: 1}
	if err := migrateOrCheck(ctx, true, open(m)); err != nil || m.ups != 1 || !m.closed {
		t.Errorf("auto: err=%v ups=%d closed=%v", err, m.ups, m.closed)
	}
	m = &fakeMigrator{pending: 2}
	err := migrateOrCheck(ctx, false, open(m))
	if err == nil || !strings.Contains(err.Error(), "2 pending migration(s)") || m.ups != 0 {
		t.Errorf("manual with pending: %v", err)
	}
	if errors.Is(fatalIf(err, store.ErrMigrationRejected), errFatalStartup) {
		t.Error("pending migrations must stay retryable")
	}
	if err := migrateOrCheck(ctx, false, open(&fakeMigrator{})); err != nil {
		t.Errorf("manual up to date: %v", err)
	}
	m = &fakeMigrator{upErr: store.ErrMigrationRejected, closeErr: errors.New("close")}
	if err := migrateOrCheck(ctx, true, open(m)); !errors.Is(err, store.ErrMigrationRejected) || !strings.Contains(err.Error(), "close") {
		t.Errorf("errors must be joined: %v", err)
	}
	openErr := errors.New("open")
	if err := migrateOrCheck(ctx, true, func() (migrator, error) { return nil, openErr }); !errors.Is(err, openErr) {
		t.Errorf("open error: %v", err)
	}
}
