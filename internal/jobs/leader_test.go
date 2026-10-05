package jobs

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
)

// fakeRow scans a fixed value or returns err.
type fakeRow struct {
	val any
	err error
}

func (r fakeRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	switch d := dest[0].(type) {
	case *bool:
		*d = r.val.(bool)
	case *int:
		*d = r.val.(int)
	}
	return nil
}

type fakeConn struct {
	rows     map[string]fakeRow
	closed   int
	released int
}

func (c *fakeConn) QueryRow(_ context.Context, sql string, _ ...any) pgx.Row { return c.rows[sql] }
func (c *fakeConn) Close(context.Context) error                              { c.closed++; return nil }
func (c *fakeConn) Release()                                                 { c.released++ }

const (
	sqlPing   = "SELECT 1"
	sqlLock   = "SELECT pg_try_advisory_lock($1)"
	sqlUnlock = "SELECT pg_advisory_unlock($1)"
)

func newFakeLock(conns ...*fakeConn) *PGLeaderLock {
	i := 0
	return &PGLeaderLock{key: LeaderLockKey, acquire: func(context.Context) (lockConn, error) {
		if i >= len(conns) {
			return nil, errors.New("pool exhausted")
		}
		c := conns[i]
		i++
		return c, nil
	}}
}

func TestPGLeaderLockLifecycle(t *testing.T) {
	ctx := context.Background()
	held := &fakeConn{rows: map[string]fakeRow{sqlLock: {val: true}, sqlPing: {val: 1}, sqlUnlock: {val: true}}}
	l := newFakeLock(held)
	for range 2 {
		if ok, err := l.TryAcquire(ctx); !ok || err != nil {
			t.Fatalf("TryAcquire = %v %v", ok, err)
		}
	}
	if err := l.Release(ctx); err != nil || held.released != 1 || held.closed != 0 {
		t.Errorf("release err=%v released=%d closed=%d", err, held.released, held.closed)
	}
	if err := l.Release(ctx); err != nil {
		t.Errorf("second release = %v", err)
	}
}

func TestPGLeaderLockFailures(t *testing.T) {
	ctx := context.Background()
	boom := errors.New("boom")
	tests := []struct {
		name         string
		conns        []*fakeConn
		wantOK       bool
		wantErr      bool
		wantClosed   int
		wantReleased int
	}{
		{"not leader", []*fakeConn{{rows: map[string]fakeRow{sqlLock: {val: false}}}}, false, false, 0, 1},
		{"lock query fails", []*fakeConn{{rows: map[string]fakeRow{sqlLock: {err: boom}}}}, false, true, 1, 1},
		{"pool exhausted", nil, false, true, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := newFakeLock(tt.conns...)
			ok, err := l.TryAcquire(ctx)
			if ok != tt.wantOK || (err != nil) != tt.wantErr {
				t.Fatalf("TryAcquire = %v %v", ok, err)
			}
			if len(tt.conns) > 0 && (tt.conns[0].closed != tt.wantClosed || tt.conns[0].released != tt.wantReleased) {
				t.Errorf("closed=%d released=%d", tt.conns[0].closed, tt.conns[0].released)
			}
		})
	}
}

func TestPGLeaderLockSessionLost(t *testing.T) {
	ctx := context.Background()
	first := &fakeConn{rows: map[string]fakeRow{sqlLock: {val: true}, sqlPing: {err: errors.New("conn reset")}}}
	second := &fakeConn{rows: map[string]fakeRow{sqlLock: {val: true}, sqlPing: {val: 1}, sqlUnlock: {err: errors.New("gone")}}}
	l := newFakeLock(first, second)
	if ok, _ := l.TryAcquire(ctx); !ok {
		t.Fatal("first acquire")
	}
	if ok, err := l.TryAcquire(ctx); ok || err == nil || first.closed != 1 || first.released != 1 {
		t.Errorf("lost session = %v %v closed=%d", ok, err, first.closed)
	}
	if ok, _ := l.TryAcquire(ctx); !ok {
		t.Error("re-acquire on a new session")
	}
	if err := l.Release(ctx); err == nil || second.closed != 1 || second.released != 1 {
		t.Errorf("failed unlock must close the session: %v", err)
	}
}
