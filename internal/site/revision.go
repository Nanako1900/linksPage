package site

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"time"
)

// computeRevision hashes everything in p except live data, GeneratedAt
// and Revision itself (encoding/json sorts map keys, so the hash is
// deterministic).
func computeRevision(p *PublicPage) (string, error) {
	c := *p
	c.Revision = ""
	c.GeneratedAt = time.Time{}
	c.Communities = make(map[string]CommunityView, len(p.Communities))
	for id, v := range p.Communities {
		v.Live = LiveView{}
		c.Communities[id] = v
	}
	raw, err := json.Marshal(c)
	if err != nil {
		return "", fmt.Errorf("site: hash page: %w", err)
	}
	sum := sha256.Sum256(raw)
	return "r-" + hex.EncodeToString(sum[:8]), nil
}

// sameLive reports whether two pages carry identical live data.
func sameLive(a, b *PublicPage) bool {
	if len(a.Communities) != len(b.Communities) {
		return false
	}
	for id, ca := range a.Communities {
		cb, ok := b.Communities[id]
		if !ok || !reflect.DeepEqual(ca.Live, cb.Live) {
			return false
		}
	}
	return true
}
