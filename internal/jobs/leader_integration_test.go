package jobs

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

// TestIntegrationPGLeaderLock checks advisory-lock leadership against a
// real PostgreSQL (skipped with -short or without Docker).
func TestIntegrationPGLeaderLock(t *testing.T) {
	dsn := startPostgres(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	poolA, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer poolA.Close()
	poolB, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer poolB.Close()

	a, b := NewPGLeaderLock(poolA, LeaderLockKey), NewPGLeaderLock(poolB, LeaderLockKey)
	steps := []struct {
		name string
		lock *PGLeaderLock
		want bool
	}{
		{"a becomes leader", a, true},
		{"b is follower", b, false},
		{"a stays leader", a, true},
	}
	for _, s := range steps {
		got, err := s.lock.TryAcquire(ctx)
		if err != nil || got != s.want {
			t.Fatalf("%s: TryAcquire = %v, %v", s.name, got, err)
		}
	}
	if err := a.Release(ctx); err != nil {
		t.Fatal(err)
	}
	if got, err := b.TryAcquire(ctx); err != nil || !got {
		t.Fatalf("b after release = %v, %v", got, err)
	}
	// Killing b's session must hand leadership back to a.
	if _, err := poolA.Exec(ctx, "SELECT pg_terminate_backend(pid) FROM pg_locks WHERE locktype = 'advisory' AND granted"); err != nil {
		t.Fatal(err)
	}
	if got, err := b.TryAcquire(ctx); got || err == nil {
		t.Errorf("b after termination = %v, %v", got, err)
	}
	if got, err := a.TryAcquire(ctx); err != nil || !got {
		t.Errorf("a after b lost its session = %v, %v", got, err)
	}
	if err := a.Release(ctx); err != nil {
		t.Error(err)
	}
}

// startPostgres starts postgres:18-alpine and returns its DSN, or skips
// when Docker is unavailable or -short is set.
func startPostgres(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test skipped in -short mode")
	}
	testcontainers.SkipIfProviderIsNotHealthy(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	ctr, err := postgres.Run(ctx, "postgres:18-alpine",
		postgres.WithDatabase("linkspage"), postgres.WithUsername("linkspage"),
		postgres.WithPassword("test-password"), postgres.BasicWaitStrategies())
	testcontainers.CleanupContainer(t, ctr)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	dsn, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	return dsn
}
