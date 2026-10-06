package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Nanako1900/linksPage/internal/config"
	"github.com/Nanako1900/linksPage/internal/httpapi"
	"github.com/Nanako1900/linksPage/internal/site"
	"github.com/Nanako1900/linksPage/internal/store"
	"github.com/Nanako1900/linksPage/internal/store/dbq"
)

// Startup retry policy while waiting for the database.
const (
	startupMinBackoff = 2 * time.Second
	startupMaxBackoff = 30 * time.Second
	// startupProbeTimeout bounds the ping, version check, compatibility
	// check and snapshot load. Migrations are bounded only by the server
	// context: a long migration or a wait on the advisory lock held by
	// another instance must be allowed to finish.
	startupProbeTimeout = 30 * time.Second
	// startupSeedTimeout bounds the seed import (image processing included).
	startupSeedTimeout = 2 * time.Minute
	// startupErrorAfter is the number of consecutive failures after which
	// retries are logged at ERROR instead of WARN.
	startupErrorAfter = 5
)

// errFatalStartup marks startup errors that must stop the process.
var errFatalStartup = errors.New("fatal startup error")

// startupSteps are the I/O operations performed during startup. They are
// functions so tests can drive startup without a database.
type startupSteps struct {
	ping        func(context.Context) error
	checkServer func(context.Context) error
	migrate     func(context.Context) error
	checkCompat func(context.Context) error
	// importSeed imports the configured seed file; nil when none is set.
	importSeed   func(context.Context) error
	loadSnapshot func(context.Context) (*site.Snapshot, error)
}

// startupRunner waits for the database, runs or checks migrations, loads
// the public snapshot and marks the instance ready. Transient failures are
// retried with backoff; permanent ones (unsupported server, incompatible
// schema, rejected migration) stop the server.
type startupRunner struct {
	steps        startupSteps
	holder       *site.Holder
	ready        *httpapi.Readiness
	logger       *slog.Logger
	minBackoff   time.Duration
	maxBackoff   time.Duration
	probeTimeout time.Duration
	seedTimeout  time.Duration
	// onReady runs once after the instance became ready (starts jobs).
	onReady func()
}

func newStartupRunner(steps startupSteps, holder *site.Holder, ready *httpapi.Readiness, logger *slog.Logger) startupRunner {
	return startupRunner{
		steps: steps, holder: holder, ready: ready, logger: logger,
		minBackoff: startupMinBackoff, maxBackoff: startupMaxBackoff, probeTimeout: startupProbeTimeout,
		seedTimeout: startupSeedTimeout,
	}
}

// run retries once until it succeeds, fails permanently (fail is called
// with an error wrapping errFatalStartup) or ctx is cancelled.
func (s startupRunner) run(ctx context.Context, fail context.CancelCauseFunc) {
	backoff := s.minBackoff
	for failures := 1; ; failures++ {
		snap, err := s.once(ctx)
		if ctx.Err() != nil {
			return
		}
		if err == nil {
			s.holder.Set(snap)
			s.ready.SetReady()
			s.logger.Info("ready")
			if s.onReady != nil {
				s.onReady()
			}
			return
		}
		if errors.Is(err, errFatalStartup) {
			fail(err)
			return
		}
		level := slog.LevelWarn
		if failures >= startupErrorAfter {
			level = slog.LevelError
		}
		s.logger.Log(ctx, level, "startup not complete; retrying", slog.Any("error", err),
			slog.Int("attempt", failures), slog.Duration("retry_in", backoff))
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, s.maxBackoff)
	}
}

func (s startupRunner) once(ctx context.Context) (*site.Snapshot, error) {
	if err := s.probe(ctx, s.steps.ping); err != nil {
		return nil, fmt.Errorf("database ping: %w", err)
	}
	if err := s.probe(ctx, s.steps.checkServer); err != nil {
		return nil, fatalIf(err, store.ErrUnsupportedServer)
	}
	if err := s.steps.migrate(ctx); err != nil {
		return nil, fatalIf(err, store.ErrMigrationRejected)
	}
	if err := s.probe(ctx, s.steps.checkCompat); err != nil {
		return nil, fatalIf(err, store.ErrIncompatibleApp)
	}
	if s.steps.importSeed != nil {
		seedCtx, cancel := context.WithTimeout(ctx, s.seedTimeout)
		err := s.steps.importSeed(seedCtx)
		cancel()
		if err != nil {
			return nil, err
		}
	}
	var snap *site.Snapshot
	err := s.probe(ctx, func(ctx context.Context) error {
		var err error
		snap, err = s.steps.loadSnapshot(ctx)
		return err
	})
	return snap, err
}

func (s startupRunner) probe(ctx context.Context, step func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(ctx, s.probeTimeout)
	defer cancel()
	return step(ctx)
}

// fatalIf marks err as fatal when it matches one of the permanent errors.
func fatalIf(err error, permanent ...error) error {
	for _, p := range permanent {
		if errors.Is(err, p) {
			return errors.Join(errFatalStartup, err)
		}
	}
	return err
}

// migrator is the part of store.Migrator used at startup.
type migrator interface {
	Up(context.Context) ([]store.MigrationState, error)
	Pending(context.Context) ([]store.MigrationState, error)
	Close() error
}

// dbStartupSteps wires the startup steps to PostgreSQL: migrations, the
// optional seed import and the first page build.
func dbStartupSteps(cfg *config.Config, pool *pgxpool.Pool, logger *slog.Logger, a *app, holder *site.Holder) startupSteps {
	q := dbq.New(pool)
	open := func() (migrator, error) { return store.NewMigrator(pool, logger) }
	return startupSteps{
		ping:         pool.Ping,
		checkServer:  func(ctx context.Context) error { return store.CheckServerVersion(ctx, pool) },
		migrate:      func(ctx context.Context) error { return migrateOrCheck(ctx, cfg.DB.AutoMigrate, open) },
		checkCompat:  func(ctx context.Context) error { return store.CheckAppCompatibility(ctx, q, version) },
		importSeed:   a.importSeed(cfg),
		loadSnapshot: a.loadSnapshot(holder),
	}
}

// migrateOrCheck applies pending migrations when auto is set; otherwise it
// fails (retryably) while migrations are pending, so an operator can run
// `linkspage migrate up` and the server picks the schema up afterwards.
func migrateOrCheck(ctx context.Context, auto bool, open func() (migrator, error)) (err error) {
	m, err := open()
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, m.Close()) }()
	if auto {
		_, err := m.Up(ctx)
		return err
	}
	pending, err := m.Pending(ctx)
	if err != nil {
		return err
	}
	if len(pending) > 0 {
		return fmt.Errorf("%d pending migration(s) and db.auto_migrate is false; run `linkspage migrate up`", len(pending))
	}
	return nil
}
