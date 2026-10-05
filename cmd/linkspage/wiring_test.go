package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Nanako1900/linksPage/internal/config"
	"github.com/Nanako1900/linksPage/internal/httpapi"
	"github.com/Nanako1900/linksPage/internal/seed"
)

func TestStartupSeedStep(t *testing.T) {
	tests := []struct {
		name      string
		seedErrs  []error
		wantFatal bool
		wantCalls int32
	}{
		{"imports once", []error{nil}, false, 1},
		{"database error is retried", []error{errors.New("conn reset"), nil}, false, 2},
		{"invalid seed is fatal", []error{fmt.Errorf("%w: communities[0].slug", seed.ErrInvalidSeed)}, true, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			steps := okSteps()
			var calls atomic.Int32
			steps.importSeed = func(ctx context.Context) error {
				if _, ok := ctx.Deadline(); !ok {
					t.Error("seed import must be bounded")
				}
				err := tt.seedErrs[calls.Add(1)-1]
				return fatalIf(err, seed.ErrInvalidSeed)
			}
			var readyCalls atomic.Int32
			r, ready, _ := testRunner(steps, io.Discard)
			r.onReady = func() { readyCalls.Add(1) }
			cause := runToEnd(t, r)
			if errors.Is(cause, errFatalStartup) != tt.wantFatal || calls.Load() != tt.wantCalls {
				t.Fatalf("cause = %v calls = %d", cause, calls.Load())
			}
			if wantReady := !tt.wantFatal; ready.IsReady() != wantReady || (readyCalls.Load() == 1) != wantReady {
				t.Errorf("ready = %v onReady calls = %d", ready.IsReady(), readyCalls.Load())
			}
		})
	}
}

func TestBackgroundTask(t *testing.T) {
	logs := &syncBuffer{}
	logger := slog.New(slog.NewTextHandler(logs, nil))
	idle := newBackgroundTask("idle", logger)
	if err := idle.stop(context.Background()); err != nil {
		t.Errorf("stopping a task that never started: %v", err)
	}

	task := newBackgroundTask("sched", logger)
	var runs atomic.Int32
	run := func(ctx context.Context) error {
		runs.Add(1)
		<-ctx.Done()
		return ctx.Err()
	}
	task.start(context.Background(), run)
	task.start(context.Background(), run)
	if err := task.stop(context.Background()); err != nil || runs.Load() != 1 {
		t.Errorf("stop = %v runs = %d", err, runs.Load())
	}
	if strings.Contains(logs.String(), "level=ERROR") {
		t.Errorf("cancellation is not an error:\n%s", logs.String())
	}

	failing := newBackgroundTask("failing", logger)
	failing.start(context.Background(), func(context.Context) error { return errors.New("boom") })
	if err := failing.stop(context.Background()); err != nil || !strings.Contains(logs.String(), "boom") {
		t.Errorf("failure must be logged: %v\n%s", err, logs.String())
	}

	stuck := newBackgroundTask("stuck", logger)
	release := make(chan struct{})
	defer close(release)
	stuck.start(context.Background(), func(context.Context) error { <-release; return nil })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := stuck.stop(ctx); err == nil || !strings.Contains(err.Error(), "stuck did not stop") {
		t.Errorf("stuck stop = %v", err)
	}
}

func TestImportSeedDisabled(t *testing.T) {
	a := &app{}
	if f := a.importSeed(&config.Config{}); f != nil {
		t.Error("no seed_file means no seed step")
	}
}

func TestPublicHandlersApply(t *testing.T) {
	h := publicHandlers{
		favicon: nopHandler{}, robots: nopHandler{}, manifest: nopHandler{}, goLink: nopHandler{},
		uploads: nopHandler{}, imgProxy: nopHandler{}, qr: nopHandler{},
	}
	var d httpapi.Deps
	h.apply(&d)
	if d.Favicon == nil || d.Robots == nil || d.Manifest == nil || d.GoLink == nil || d.Uploads == nil || d.ImgProxy == nil || d.QR == nil {
		t.Errorf("deps = %+v", d)
	}
}

type nopHandler struct{}

func (nopHandler) ServeHTTP(http.ResponseWriter, *http.Request) {}

type failCloser struct{ err error }

func (f failCloser) Close() error { return f.err }

func TestJoinClose(t *testing.T) {
	base := errors.New("base")
	if err := joinClose(base, failCloser{}); err != base { //nolint:errorlint // identity check
		t.Errorf("joinClose without close error = %v", err)
	}
	if err := joinClose(base, failCloser{errors.New("close")}); !errors.Is(err, base) || !strings.Contains(err.Error(), "close") {
		t.Errorf("joinClose = %v", err)
	}
}
