package media

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestImageWeight(t *testing.T) {
	t.Parallel()
	if got := ImageWeight(MaxDimension, MaxDimension); got != 128<<20 {
		t.Fatalf("ImageWeight(4096,4096) = %d", got)
	}
	if ImageWeight(MaxDimension, MaxDimension)+ImageWeight(OGWidth, OGHeight) > MemoryBudget {
		t.Fatal("the largest accepted image must fit the budget")
	}
}

func TestSemaphoreAcquire(t *testing.T) {
	t.Parallel()
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name   string
		ctx    context.Context
		hold   int64
		weight int64
		want   error
	}{
		{"fits", context.Background(), 0, 50, nil},
		{"over budget", context.Background(), 0, 101, ErrDimensions},
		{"zero weight", context.Background(), 0, 0, errAny},
		{"busy", context.Background(), 60, 50, ErrBusy},
		{"cancelled", cancelled, 0, 10, context.Canceled},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := NewSemaphore(100)
			if s.Budget() != 100 {
				t.Fatalf("Budget = %d", s.Budget())
			}
			if tt.hold > 0 {
				rel, err := s.Acquire(context.Background(), tt.hold, time.Second)
				if err != nil {
					t.Fatal(err)
				}
				defer rel()
			}
			rel, err := s.Acquire(tt.ctx, tt.weight, 20*time.Millisecond)
			switch {
			case tt.want == nil && err != nil:
				t.Fatalf("err = %v", err)
			case errors.Is(tt.want, errAny) && err == nil:
				t.Fatal("want error")
			case tt.want != nil && !errors.Is(tt.want, errAny) && !errors.Is(err, tt.want):
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
			if rel != nil {
				rel()
				rel() // idempotent
			}
		})
	}
}

func TestSemaphoreParentCancelWhileWaiting(t *testing.T) {
	t.Parallel()
	s := NewSemaphore(10)
	rel, err := s.Acquire(context.Background(), 10, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer rel()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, err := s.Acquire(ctx, 5, time.Second); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want parent deadline", err)
	}
}

func TestSemaphoreReleaseFreesCapacity(t *testing.T) {
	t.Parallel()
	s := NewSemaphore(10)
	rel, err := s.Acquire(context.Background(), 10, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	rel()
	rel2, err := s.Acquire(context.Background(), 10, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("after release: %v", err)
	}
	rel2()
}

var errAny = errors.New("any error")
