package site

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Nanako1900/linksPage/internal/provider"
)

// fixturePath is the canonical contract sample shared with the frontend.
var fixturePath = filepath.Join("..", "..", "web", "src", "test", "fixtures", "public-page.json")

func loadFixture(t *testing.T) ([]byte, *PublicPage) {
	t.Helper()
	raw, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var p PublicPage
	if err := DecodeStrict(raw, &p); err != nil {
		t.Fatalf("strict decode: %v", err)
	}
	return raw, &p
}

func TestFixtureDecodesStrictlyAndValidates(t *testing.T) {
	_, p := loadFixture(t)
	if err := p.Validate(); err != nil {
		t.Fatalf("fixture violates the contract:\n%v", err)
	}
	states := map[CardState]bool{}
	cards := map[CardKind]bool{}
	for _, c := range p.Communities {
		states[c.Live.State] = true
		cards[c.Card] = true
	}
	for _, s := range []CardState{
		provider.StateLive, provider.StateStale, provider.StateDegraded,
		provider.StateStatic, provider.StateQROnly, provider.StateUnavailable,
	} {
		if !states[s] {
			t.Errorf("fixture lacks a %s community", s)
		}
	}
	for k := range cardKinds {
		if k != CardStatic && !cards[k] {
			t.Errorf("fixture lacks a %s card", k)
		}
	}
	kinds := map[BlockKind]bool{}
	for _, b := range p.Blocks {
		kinds[b.Kind] = true
	}
	if len(kinds) != 5 {
		t.Errorf("fixture block kinds = %v, want all 5", kinds)
	}
}

// TestFixtureRoundTrip pins field presence: re-encoding the decoded page
// must reproduce the fixture exactly (no omitted nulls, no extra fields).
func TestFixtureRoundTrip(t *testing.T) {
	raw, p := loadFixture(t)
	out, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var want, got any
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(want, got) {
		t.Errorf("round trip differs:\nwant %s\ngot  %s", raw, out)
	}
}

func TestFixtureRejectsUnknownFields(t *testing.T) {
	raw, _ := loadFixture(t)
	bad := bytes.Replace(raw, []byte(`"revision"`), []byte(`"unknownField": 1, "revision"`), 1)
	var p PublicPage
	if err := DecodeStrict(bad, &p); err == nil {
		t.Fatal("unknown field accepted")
	}
	if err := DecodeStrict(append(raw, []byte(" {}")...), &p); err == nil {
		t.Fatal("trailing data accepted")
	}
}

func TestLiveAndFrame(t *testing.T) {
	_, p := loadFixture(t)
	live := p.Live()
	if live.Revision != p.Revision || len(live.Communities) != len(p.Communities) || !live.GeneratedAt.Equal(p.GeneratedAt) {
		t.Fatalf("live = %+v", live)
	}
	if !p.NeedsDiscordFrame() {
		t.Error("fixture has a discord embed")
	}
	empty := EmptyPublicPage(0, Page{ID: 1, Slug: "default"}, Default(), "https://x.example", time.Now())
	if empty.NeedsDiscordFrame() || len(empty.Live().Communities) != 0 {
		t.Error("empty page wrong")
	}
	if err := empty.Validate(); err != nil {
		t.Errorf("empty page invalid: %v", err)
	}
	b, err := json.Marshal(empty)
	if err != nil || !strings.Contains(string(b), `"blocks":[]`) || !strings.Contains(string(b), `"nextBoundary":null`) {
		t.Errorf("empty page JSON = %s err=%v", b, err)
	}
}

func TestSnapshotDefaults(t *testing.T) {
	s := DefaultSnapshot()
	if s.Public == nil || s.Head.Robots != SearchIndex || s.Head.Icons == nil {
		t.Fatalf("default snapshot = %+v", s)
	}
	if _, ok := s.NextBoundary(); ok {
		t.Error("default snapshot has no boundary")
	}
	at := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	s.Public.NextBoundary = &at
	if got, ok := s.NextBoundary(); !ok || !got.Equal(at) {
		t.Errorf("NextBoundary = %v %v", got, ok)
	}
	if _, ok := (&Snapshot{}).NextBoundary(); ok {
		t.Error("nil public page has no boundary")
	}
}
