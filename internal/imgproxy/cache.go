package imgproxy

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/hashicorp/golang-lru/v2/expirable"
)

// cacheEntries bounds the number of cached entries (positive and
// negative); CacheBytes bounds their total body size.
const cacheEntries = 2048

// imageCache is an expirable LRU bounded by entry count and body bytes.
// Entries live at most NegativeCacheTTL; an entry may expire earlier via
// cachedImage.expires.
type imageCache struct {
	mu    sync.Mutex // serializes Add + trimming
	lru   *expirable.LRU[string, cachedImage]
	bytes atomic.Int64
	limit int64
	now   func() time.Time
}

func newImageCache(limit int64, now func() time.Time) *imageCache {
	c := &imageCache{limit: limit, now: now}
	c.lru = expirable.NewLRU[string, cachedImage](cacheEntries, func(_ string, v cachedImage) {
		c.bytes.Add(-int64(len(v.body)))
	}, NegativeCacheTTL)
	return c
}

func (c *imageCache) get(key string) (cachedImage, bool) {
	v, ok := c.lru.Get(key)
	if !ok {
		return cachedImage{}, false
	}
	if !v.expires.IsZero() && !c.now().Before(v.expires) {
		c.lru.Remove(key)
		return cachedImage{}, false
	}
	return v, true
}

// add stores v unless its body alone exceeds the byte budget, then evicts
// the oldest entries until the budget holds.
func (c *imageCache) add(key string, v cachedImage) {
	size := int64(len(v.body))
	if size > c.limit {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lru.Remove(key)
	c.bytes.Add(size)
	c.lru.Add(key, v)
	for c.bytes.Load() > c.limit {
		if _, _, ok := c.lru.RemoveOldest(); !ok {
			return
		}
	}
}
