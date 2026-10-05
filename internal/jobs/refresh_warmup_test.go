package jobs

import (
	"context"
	"errors"
	"testing"

	"github.com/Nanako1900/linksPage/internal/store/dbq"
)

// horizonStore returns due rows for "now" and early rows for later times.
type horizonStore struct {
	*memStore
	early    []dbq.ListDueCommunitiesRow
	earlyErr error
	calls    int
}

func (h *horizonStore) ListDueCommunities(ctx context.Context, arg dbq.ListDueCommunitiesParams) ([]dbq.ListDueCommunitiesRow, error) {
	h.calls++
	if arg.Now.Time.After(t0) {
		if h.earlyErr != nil {
			return nil, h.earlyErr
		}
		return h.early, nil
	}
	return h.memStore.ListDueCommunities(ctx, arg)
}

func int32p(v int32) *int32 { return &v }

func TestRefreshWarmUp(t *testing.T) {
	failing := int32p(2)
	tests := []struct {
		name    string
		due     []dbq.ListDueCommunitiesRow
		early   []dbq.ListDueCommunitiesRow
		inLive  []byte
		wantIDs []byte
	}{
		{
			"healthy early rows are added",
			[]dbq.ListDueCommunitiesRow{dueRow(1, "discord", nil)},
			[]dbq.ListDueCommunitiesRow{dueRow(1, "discord", nil), dueRow(2, "discord", int32p(0))},
			nil,
			[]byte{1, 2},
		},
		{
			"failing rows keep their backoff", nil,
			[]dbq.ListDueCommunitiesRow{dueRow(3, "discord", failing)},
			nil,
			[]byte{},
		},
		{
			"rows already live are skipped", nil,
			[]dbq.ListDueCommunitiesRow{dueRow(4, "discord", nil)},
			[]byte{4},
			[]byte{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &horizonStore{memStore: newMemStore(tt.due...), early: tt.early}
			env := newRefreshEnv(t, store.memStore, stubResult{snap: liveSnap(3)})
			for _, id := range tt.inLive {
				env.live.Put(uuid(id).String(), *liveSnap(1))
			}
			r := &refresher{deps: RefreshDeps{Store: store, Live: env.live}}
			rows, err := r.dueRows(context.Background(), t0)
			if err != nil {
				t.Fatal(err)
			}
			got := make([]byte, 0, len(rows))
			for _, row := range rows {
				got = append(got, row.ID.Bytes[0])
			}
			if string(got) != string(tt.wantIDs) {
				t.Errorf("ids = %v, want %v", got, tt.wantIDs)
			}
			before := store.calls
			if _, err := r.dueRows(context.Background(), t0); err != nil || store.calls != before+1 {
				t.Errorf("warm-up must happen once: calls %d → %d", before, store.calls)
			}
		})
	}
}

func TestRefreshWarmUpCap(t *testing.T) {
	var early []dbq.ListDueCommunitiesRow
	for i := range byte(RefreshBatch + 3) {
		early = append(early, dueRow(i+1, "discord", nil))
	}
	store := &horizonStore{memStore: newMemStore(), early: early}
	env := newRefreshEnv(t, store.memStore)
	r := &refresher{deps: RefreshDeps{Store: store, Live: env.live}}
	rows, err := r.dueRows(context.Background(), t0)
	if err != nil || len(rows) != RefreshBatch {
		t.Errorf("rows = %d, err %v", len(rows), err)
	}
}

func TestRefreshWarmUpErrors(t *testing.T) {
	boom := errors.New("db down")
	store := &horizonStore{memStore: newMemStore(), earlyErr: boom}
	env := newRefreshEnv(t, store.memStore)
	r := &refresher{deps: RefreshDeps{Store: store, Live: env.live}}
	if _, err := r.dueRows(context.Background(), t0); !errors.Is(err, boom) || r.warmed.Load() {
		t.Errorf("early listing error = %v, warmed %v", err, r.warmed.Load())
	}
	store.earlyErr = nil
	store.listErr = boom
	if _, err := r.dueRows(context.Background(), t0); !errors.Is(err, boom) {
		t.Errorf("due listing error = %v", err)
	}
}
