package httpapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"
)

// http.Server limits (doc 4.11).
const (
	ReadHeaderTimeout = 5 * time.Second
	ReadTimeout       = 15 * time.Second
	WriteTimeout      = 30 * time.Second
	IdleTimeout       = 120 * time.Second
	MaxHeaderBytes    = 64 << 10
	ShutdownTimeout   = 15 * time.Second
)

// NewServer returns an http.Server with the documented timeouts.
func NewServer(addr string, h http.Handler, logger *slog.Logger) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: ReadHeaderTimeout,
		ReadTimeout:       ReadTimeout,
		WriteTimeout:      WriteTimeout,
		IdleTimeout:       IdleTimeout,
		MaxHeaderBytes:    MaxHeaderBytes,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelWarn),
	}
}

// ShutdownStep is run after the HTTP server has stopped, in order
// (stop scheduler, flush analytics buffers, close the pool, ...).
type ShutdownStep struct {
	Name string
	Run  func(context.Context) error
}

// Serve runs srv on ln until ctx is cancelled, then shuts down in the
// documented order: readiness → 503, http Shutdown(15s), then each step.
func Serve(ctx context.Context, srv *http.Server, ln net.Listener, ready *Readiness, logger *slog.Logger, steps ...ShutdownStep) error {
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()

	var serveErr error
	select {
	case <-ctx.Done():
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			serveErr = fmt.Errorf("http server: %w", err)
		}
	}

	logger.Info("shutting down")
	ready.StartDraining()
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), ShutdownTimeout)
	defer cancel()
	errs := []error{serveErr}
	if err := srv.Shutdown(shutdownCtx); err != nil {
		errs = append(errs, fmt.Errorf("http shutdown: %w", err))
	}
	for _, s := range steps {
		if err := s.Run(shutdownCtx); err != nil {
			errs = append(errs, fmt.Errorf("shutdown %s: %w", s.Name, err))
		}
	}
	return errors.Join(errs...)
}
