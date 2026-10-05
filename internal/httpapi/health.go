package httpapi

import (
	"context"
	"errors"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// readyPingTimeout bounds the database ping done by /readyz.
	readyPingTimeout = 2 * time.Second
	// readyPingCacheTTL reuses the last ping result so that a flood of
	// /readyz requests cannot exhaust the database pool.
	readyPingCacheTTL = time.Second
)

// Readiness tracks whether the instance may receive traffic: the database
// is reachable, migrations are done and the instance is not draining.
type Readiness struct {
	ready    atomic.Bool
	draining atomic.Bool
	ping     func(context.Context) error
	now      func() time.Time

	// mu serializes pings: at most one is in flight, concurrent callers
	// wait for it and reuse its result.
	mu       sync.Mutex
	pingedAt time.Time
	pingErr  error
}

// NewReadiness creates a tracker. ping may be nil (no database check).
func NewReadiness(ping func(context.Context) error) *Readiness {
	return &Readiness{ping: ping, now: time.Now}
}

// SetReady marks startup (migrations, initial load) as complete.
func (r *Readiness) SetReady() { r.ready.Store(true) }

// StartDraining makes /readyz fail during graceful shutdown.
func (r *Readiness) StartDraining() { r.draining.Store(true) }

// IsReady reports startup completion and not draining (no I/O).
func (r *Readiness) IsReady() bool { return r.ready.Load() && !r.draining.Load() }

// Errors reported by Check.
var (
	ErrDraining = errors.New("draining")
	ErrStarting = errors.New("starting")

	errDatabaseUnavailable = errors.New("database unavailable")
)

// Check returns nil when the instance is ready, pinging the database.
func (r *Readiness) Check(ctx context.Context) error {
	if r.draining.Load() {
		return ErrDraining
	}
	if !r.ready.Load() {
		return ErrStarting
	}
	if r.ping == nil {
		return nil
	}
	return r.cachedPing(ctx)
}

func (r *Readiness) cachedPing(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.pingedAt.IsZero() && r.now().Sub(r.pingedAt) < readyPingCacheTTL {
		return r.pingErr
	}
	ctx, cancel := context.WithTimeout(ctx, readyPingTimeout)
	defer cancel()
	r.pingErr = nil
	if err := r.ping(ctx); err != nil {
		r.pingErr = errDatabaseUnavailable
	}
	r.pingedAt = r.now()
	return r.pingErr
}

func plainText(w http.ResponseWriter, status int, body string) {
	h := w.Header()
	h.Set("Content-Type", "text/plain; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body+"\n")
}

func healthz(w http.ResponseWriter, _ *http.Request) {
	plainText(w, http.StatusOK, "ok")
}

func readyz(ready *Readiness) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := ready.Check(r.Context()); err != nil {
			plainText(w, http.StatusServiceUnavailable, err.Error())
			return
		}
		plainText(w, http.StatusOK, "ok")
	}
}
