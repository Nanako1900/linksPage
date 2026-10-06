package jobs

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// lockConn is the dedicated session that holds the advisory lock.
type lockConn interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	// Close closes the underlying connection (dropping any session lock).
	Close(ctx context.Context) error
	// Release returns the connection to the pool (destroyed if closed).
	Release()
}

// poolConn adapts *pgxpool.Conn to lockConn.
type poolConn struct{ c *pgxpool.Conn }

func (p poolConn) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return p.c.QueryRow(ctx, sql, args...)
}

func (p poolConn) Close(ctx context.Context) error { return p.c.Conn().Close(ctx) }

func (p poolConn) Release() { p.c.Release() }

// PGLeaderLock implements LeaderLock with pg_try_advisory_lock on one
// dedicated pooled connection (session-level lock: losing the connection
// loses leadership, which TryAcquire then reports).
type PGLeaderLock struct {
	mu      sync.Mutex
	key     int64
	acquire func(ctx context.Context) (lockConn, error)
	conn    lockConn
}

// NewPGLeaderLock returns a lock on key (LeaderLockKey).
func NewPGLeaderLock(pool *pgxpool.Pool, key int64) *PGLeaderLock {
	return &PGLeaderLock{key: key, acquire: func(ctx context.Context) (lockConn, error) {
		c, err := pool.Acquire(ctx)
		if err != nil {
			return nil, err
		}
		return poolConn{c: c}, nil
	}}
}

// TryAcquire implements LeaderLock. While leadership is held, it only
// checks that the session is still alive.
func (l *PGLeaderLock) TryAcquire(ctx context.Context) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.conn != nil {
		var one int
		err := l.conn.QueryRow(ctx, "SELECT 1").Scan(&one)
		if err == nil {
			return true, nil
		}
		l.dropConn(ctx)
		return false, fmt.Errorf("jobs: leader session lost: %w", err)
	}
	conn, err := l.acquire(ctx)
	if err != nil {
		return false, fmt.Errorf("jobs: acquire connection: %w", err)
	}
	var locked bool
	if err := conn.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", l.key).Scan(&locked); err != nil {
		closeConn(ctx, conn)
		conn.Release()
		return false, fmt.Errorf("jobs: try advisory lock: %w", err)
	}
	if !locked {
		conn.Release()
		return false, nil
	}
	l.conn = conn
	return true, nil
}

// Release implements LeaderLock.
func (l *PGLeaderLock) Release(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.conn == nil {
		return nil
	}
	var unlocked bool
	err := l.conn.QueryRow(ctx, "SELECT pg_advisory_unlock($1)", l.key).Scan(&unlocked)
	if err != nil {
		l.dropConn(ctx)
		return fmt.Errorf("jobs: advisory unlock: %w", err)
	}
	l.conn.Release()
	l.conn = nil
	return nil
}

// dropConn closes the session (so a possibly still held lock is freed)
// and forgets it. The caller holds l.mu.
func (l *PGLeaderLock) dropConn(ctx context.Context) {
	closeConn(ctx, l.conn)
	l.conn.Release()
	l.conn = nil
}

// closeTimeout bounds closing a broken session.
const closeTimeout = 2 * time.Second

// closeConn closes c even when ctx is already cancelled; the session is
// unusable either way, so the close error is irrelevant.
func closeConn(ctx context.Context, c lockConn) {
	closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), closeTimeout)
	defer cancel()
	_ = c.Close(closeCtx)
}
