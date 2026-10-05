package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"slices"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

//go:embed migrations/*.sql
var embeddedMigrations embed.FS

// MigrationsFS returns the embedded migrations directory.
func MigrationsFS() fs.FS {
	sub, err := fs.Sub(embeddedMigrations, "migrations")
	if err != nil {
		panic(fmt.Sprintf("embedded migrations: %v", err)) // unreachable: path is static
	}
	return sub
}

// ErrMigrationRejected marks migration failures that retrying cannot fix
// (SQL errors, missing privileges, broken migration sources).
var ErrMigrationRejected = errors.New("migration rejected")

// permanentSQLStateClasses are SQLSTATE classes that will fail the same
// way on every attempt: 0A feature not supported, 22 data exception,
// 23 integrity constraint violation, 42 syntax error or access rule
// violation (including 42501 insufficient_privilege).
var permanentSQLStateClasses = []string{"0A", "22", "23", "42"}

// classifyMigrationError wraps err with ErrMigrationRejected when it is
// permanent. Connection problems, timeouts, lock contention and server
// restarts stay retryable.
func classifyMigrationError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, goose.ErrNoMigrations) || errors.Is(err, goose.ErrVersionNotFound) {
		return fmt.Errorf("%w: %w", ErrMigrationRejected, err)
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && len(pgErr.Code) == 5 && slices.Contains(permanentSQLStateClasses, pgErr.Code[:2]) {
		return fmt.Errorf("%w: %w", ErrMigrationRejected, err)
	}
	return err
}

// MigrationState describes one migration.
type MigrationState struct {
	Version int64  `json:"version"`
	Name    string `json:"name"`
	Applied bool   `json:"applied"`
}

// Migrator runs the embedded goose migrations under a PostgreSQL session
// advisory lock so that concurrent instances cannot race.
type Migrator struct {
	db       *sql.DB
	provider *goose.Provider
	logger   *slog.Logger
}

// NewMigrator builds a Migrator on top of pool. Close releases the
// database/sql wrapper (not the pool).
func NewMigrator(pool *pgxpool.Pool, logger *slog.Logger) (*Migrator, error) {
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return nil, fmt.Errorf("create migration locker: %w", err)
	}
	db := stdlib.OpenDBFromPool(pool)
	p, err := goose.NewProvider(goose.DialectPostgres, db, MigrationsFS(),
		goose.WithSessionLocker(locker),
		goose.WithDisableGlobalRegistry(true),
	)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("create migration provider: %w", err), db.Close())
	}
	return &Migrator{db: db, provider: p, logger: logger}, nil
}

// Close releases resources held by the migrator.
func (m *Migrator) Close() error {
	return m.db.Close()
}

// Status lists every known migration and whether it is applied.
func (m *Migrator) Status(ctx context.Context) ([]MigrationState, error) {
	st, err := m.provider.Status(ctx)
	if err != nil {
		return nil, fmt.Errorf("migration status: %w", err)
	}
	out := make([]MigrationState, 0, len(st))
	for _, s := range st {
		out = append(out, MigrationState{
			Version: s.Source.Version,
			Name:    s.Source.Path,
			Applied: s.State == goose.StateApplied,
		})
	}
	return out, nil
}

// Pending returns the migrations that are not applied yet.
func (m *Migrator) Pending(ctx context.Context) ([]MigrationState, error) {
	all, err := m.Status(ctx)
	if err != nil {
		return nil, err
	}
	var pending []MigrationState
	for _, s := range all {
		if !s.Applied {
			pending = append(pending, s)
		}
	}
	return pending, nil
}

// Up applies all pending migrations, logging the versions it will run
// first.
func (m *Migrator) Up(ctx context.Context) ([]MigrationState, error) {
	pending, err := m.Pending(ctx)
	if err != nil {
		return nil, err
	}
	for _, p := range pending {
		m.logger.Info("migration pending", slog.Int64("version", p.Version), slog.String("name", p.Name))
	}
	results, err := m.provider.Up(ctx)
	if err != nil {
		return nil, fmt.Errorf("apply migrations: %w", classifyMigrationError(err))
	}
	applied := make([]MigrationState, 0, len(results))
	for _, r := range results {
		m.logger.Info("migration applied", slog.Int64("version", r.Source.Version),
			slog.String("name", r.Source.Path), slog.Duration("took", r.Duration))
		applied = append(applied, MigrationState{Version: r.Source.Version, Name: r.Source.Path, Applied: true})
	}
	return applied, nil
}
