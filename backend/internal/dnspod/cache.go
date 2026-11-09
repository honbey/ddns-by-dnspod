package dnspod

import (
	"sync"
	"time"
)

// maxCacheEntries caps how many entries a cache holds.
//
// Without a bound the map only ever grows: expired entries are skipped on read
// but never removed, and callers may produce an unbounded number of distinct
// keys. A record list page can hold up to maxLimit records, so a few dozen
// entries is already a meaningful amount of memory.
const maxCacheEntries = 64

// entry is a cached value together with its expiry time
type entry[T any] struct {
	value   T
	expires time.Time
	used    time.Time
}

// cache is a minimal in-memory TTL cache.
//
// DNSPod rate limits the read APIs, and the frontend refreshes domain and record
// lists on every page visit, so repeated reads are served from here instead.
type cache[T any] struct {
	mu    sync.Mutex
	ttl   time.Duration
	items map[string]entry[T]
}

func newCache[T any](ttl time.Duration) *cache[T] {
	// A non-positive TTL means caching is off for this call
	if ttl <= 0 {
		return &cache[T]{items: make(map[string]entry[T])}
	}
	return &cache[T]{ttl: ttl, items: make(map[string]entry[T])}
}

// get returns a value that has not expired yet.
//
// Expired entries are dropped on the way, so a key that is never looked up
// again still gets released by the next sweep.
func (c *cache[T]) get(key string) (T, bool) {
	var zero T
	if c == nil || c.ttl <= 0 {
		return zero, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	e, ok := c.items[key]
	if !ok {
		return zero, false
	}
	if time.Now().After(e.expires) {
		delete(c.items, key)
		return zero, false
	}
	e.used = time.Now()
	c.items[key] = e
	return e.value, true
}

// set stores a value for the configured TTL, evicting when the cache is full
func (c *cache[T]) set(key string, value T) {
	if c == nil || c.ttl <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	c.items[key] = entry[T]{value: value, expires: now.Add(c.ttl), used: now}

	if len(c.items) <= maxCacheEntries {
		return
	}
	c.evict(now)
}

// evict removes expired entries first, then falls back to the least recently
// used ones until the cache is back within its bound. Caller must hold the lock.
func (c *cache[T]) evict(now time.Time) {
	for k, e := range c.items {
		if now.After(e.expires) {
			delete(c.items, k)
		}
	}

	// Still over the limit: drop the oldest by last use until it fits.
	// This is linear in the number of entries, which is fine at this size.
	for len(c.items) > maxCacheEntries {
		var oldestKey string
		var oldest time.Time
		first := true
		for k, e := range c.items {
			if first || e.used.Before(oldest) {
				oldestKey, oldest, first = k, e.used, false
			}
		}
		if first {
			return
		}
		delete(c.items, oldestKey)
	}
}

// invalidate drops every cached entry whose key starts with prefix
func (c *cache[T]) invalidate(prefix string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if prefix == "" {
		c.items = make(map[string]entry[T])
		return
	}
	for k := range c.items {
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			delete(c.items, k)
		}
	}
}
