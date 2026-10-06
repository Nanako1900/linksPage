package provider

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"sync"
)

// Registry maps provider kinds to implementations. It is immutable after
// construction.
type Registry struct {
	byKind map[string]Provider
}

// NewRegistry registers providers; duplicate or empty kinds are errors.
func NewRegistry(providers ...Provider) (*Registry, error) {
	r := &Registry{byKind: make(map[string]Provider, len(providers))}
	for _, p := range providers {
		if p == nil || p.Kind() == "" {
			return nil, errors.New("provider: nil provider or empty kind")
		}
		if _, dup := r.byKind[p.Kind()]; dup {
			return nil, fmt.Errorf("provider: duplicate kind %q", p.Kind())
		}
		r.byKind[p.Kind()] = p
	}
	return r, nil
}

// Get returns the provider for kind.
func (r *Registry) Get(kind string) (Provider, bool) {
	p, ok := r.byKind[kind]
	return p, ok
}

// Kinds returns the registered kinds, sorted.
func (r *Registry) Kinds() []string {
	return slices.Sorted(maps.Keys(r.byKind))
}

// ImageHosts returns provider kind → host → allowed path prefixes.
func (r *Registry) ImageHosts() map[string]map[string][]string {
	out := make(map[string]map[string][]string, len(r.byKind))
	for k, p := range r.byKind {
		out[k] = p.ImageHosts()
	}
	return out
}

// LiveSource exposes the latest in-memory snapshots (with Users) to the
// page builder. Only the leader instance has users; others read the
// persisted snapshot without users.
type LiveSource interface {
	Live(communityID string) (Snapshot, bool)
}

// LiveStore is the in-memory LiveSource written by the refresh job.
type LiveStore struct {
	mu sync.RWMutex
	m  map[string]Snapshot
}

// NewLiveStore returns an empty store.
func NewLiveStore() *LiveStore {
	return &LiveStore{m: map[string]Snapshot{}}
}

// Live returns a copy of the snapshot for communityID.
func (s *LiveStore) Live(communityID string) (Snapshot, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	snap, ok := s.m[communityID]
	if !ok {
		return Snapshot{}, false
	}
	return cloneSnapshot(snap), true
}

// Put stores a copy of snap for communityID.
func (s *LiveStore) Put(communityID string, snap Snapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[communityID] = cloneSnapshot(snap)
}

// Delete forgets communityID.
func (s *LiveStore) Delete(communityID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, communityID)
}

func cloneSnapshot(in Snapshot) Snapshot {
	out := in
	out.Channels = slices.Clone(in.Channels)
	out.Users = slices.Clone(in.Users)
	out.Online = clonePtr(in.Online)
	out.Members = clonePtr(in.Members)
	out.InviteExpiresAt = clonePtr(in.InviteExpiresAt)
	out.InviteFetchedAt = clonePtr(in.InviteFetchedAt)
	return out
}

func clonePtr[T any](p *T) *T {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}
