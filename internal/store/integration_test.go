package store

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net/url"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/Nanako1900/linksPage/internal/config"
	"github.com/Nanako1900/linksPage/internal/store/dbq"
)

// MigrationsFSReadFile is a small test helper over the embedded FS.
func MigrationsFSReadFile(name string) ([]byte, error) {
	return fs.ReadFile(MigrationsFS(), name)
}

// startPostgres starts postgres:18-alpine or skips when Docker is not
// available.
func startPostgres(t *testing.T) config.DB {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test skipped in -short mode")
	}
	testcontainers.SkipIfProviderIsNotHealthy(t)
	ctx := context.Background()
	ctr, err := postgres.Run(ctx, "postgres:18-alpine",
		postgres.WithDatabase("linkspage"),
		postgres.WithUsername("linkspage"),
		postgres.WithPassword("test-password"),
		postgres.BasicWaitStrategies(),
	)
	testcontainers.CleanupContainer(t, ctr)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	dsn, err := ctr.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatal(err)
	}
	return config.DB{
		Host: u.Hostname(), Port: port, User: "linkspage", Name: "linkspage",
		Password: config.Secret("test-password"), SSLMode: "disable",
	}
}

func TestIntegrationMigrationsAndQueries(t *testing.T) {
	dbCfg := startPostgres(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	pool, err := Connect(ctx, dbCfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	if err := CheckServerVersion(ctx, pool); err != nil {
		t.Fatal(err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	m, err := NewMigrator(pool, logger)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := m.Close(); err != nil {
			t.Error(err)
		}
	}()

	pending, err := m.Pending(ctx)
	if err != nil || len(pending) != 1 || pending[0].Version != 1 {
		t.Fatalf("pending = %+v err=%v", pending, err)
	}

	// Two concurrent runs must serialize on the session lock.
	runConcurrentUp(ctx, t, pool, logger)

	status, err := m.Status(ctx)
	if err != nil || len(status) != 1 || !status[0].Applied {
		t.Fatalf("status = %+v err=%v", status, err)
	}
	applied, err := m.Up(ctx)
	if err != nil || len(applied) != 0 {
		t.Fatalf("second Up applied %v err=%v", applied, err)
	}

	assertQueries(ctx, t, dbq.New(pool))
	assertUUIDv7(ctx, t, pool)
}

func runConcurrentUp(ctx context.Context, t *testing.T, pool *pgxpool.Pool, logger *slog.Logger) {
	t.Helper()
	var wg sync.WaitGroup
	errs := make([]error, 2)
	total := make([]int, 2)
	for i := range 2 {
		wg.Go(func() {
			mi, err := NewMigrator(pool, logger)
			if err != nil {
				errs[i] = err
				return
			}
			res, err := mi.Up(ctx)
			total[i] = len(res)
			errs[i] = errors.Join(err, mi.Close())
		})
	}
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		t.Fatalf("concurrent Up: %v", err)
	}
	if total[0]+total[1] != 1 {
		t.Fatalf("migration applied %d times", total[0]+total[1])
	}
}

func assertQueries(ctx context.Context, t *testing.T, q *dbq.Queries) {
	t.Helper()
	s, err := q.GetSiteSettings(ctx)
	if err != nil || s.PageID != 1 || s.Version != 1 || string(s.Data) != "{}" {
		t.Fatalf("settings = %+v err=%v", s, err)
	}
	if s.UpdatedBy == nil || *s.UpdatedBy != "system" || !s.UpdatedAt.Valid {
		t.Errorf("settings metadata = %+v", s)
	}
	p, err := q.GetDefaultPage(ctx)
	if err != nil || p.ID != 1 || p.Slug != "default" {
		t.Fatalf("default page = %+v err=%v", p, err)
	}
	p2, err := q.GetPageBySlug(ctx, "default")
	if err != nil || p2 != p {
		t.Fatalf("page by slug = %+v err=%v", p2, err)
	}
	if _, err := q.GetPageBySlug(ctx, "missing"); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("missing slug err = %v", err)
	}
	if err := CheckAppCompatibility(ctx, q, "v0.1.0"); err != nil {
		t.Fatal(err)
	}
	if err := CheckAppCompatibility(ctx, q, "dev"); err != nil {
		t.Fatal(err)
	}
}

func assertUUIDv7(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	var v string
	if err := pool.QueryRow(ctx, "SELECT uuidv7()::text").Scan(&v); err != nil || len(v) != 36 || v[14] != '7' {
		t.Fatalf("uuidv7() = %q err=%v", v, err)
	}
}
