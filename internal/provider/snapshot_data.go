package provider

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"
)

// EncodeSnapshotData returns the provider_snapshots.data JSON for s.
// Users, State, ErrCode and FetchedAt are never included (json:"-"), and
// nil slices are written as [] so the stored object has a stable shape.
func EncodeSnapshotData(s Snapshot) ([]byte, error) {
	out := s
	if out.Channels == nil {
		out.Channels = []Channel{}
	}
	data, err := json.Marshal(out)
	if err != nil {
		return nil, fmt.Errorf("encode snapshot data: %w", err)
	}
	return data, nil
}

// DecodeSnapshotData parses provider_snapshots.data. An empty value or {}
// (a pending row) yields a zero Snapshot. Unknown keys are ignored so old
// rows stay readable after a field is removed.
func DecodeSnapshotData(data []byte) (Snapshot, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("{}")) || bytes.Equal(trimmed, []byte("null")) {
		return Snapshot{Channels: []Channel{}}, nil
	}
	var s Snapshot
	if err := json.Unmarshal(trimmed, &s); err != nil {
		return Snapshot{}, fmt.Errorf("decode snapshot data: %w", err)
	}
	if s.Channels == nil {
		s.Channels = []Channel{}
	}
	return s, nil
}

// SnapshotRow is the column set of provider_snapshots in Go types.
type SnapshotRow struct {
	Data      []byte
	State     string
	ErrCode   *string
	FetchedAt *time.Time
}

// SnapshotFromRow decodes a provider_snapshots row (data + columns) into
// a Snapshot without users. Unknown states become StatePending.
func SnapshotFromRow(row SnapshotRow) (Snapshot, error) {
	s, err := DecodeSnapshotData(row.Data)
	if err != nil {
		return Snapshot{}, err
	}
	s.State = State(row.State)
	if !s.State.Valid() {
		s.State = StatePending
	}
	if row.ErrCode != nil {
		s.ErrCode = *row.ErrCode
	}
	if row.FetchedAt != nil {
		s.FetchedAt = *row.FetchedAt
	}
	return s, nil
}
