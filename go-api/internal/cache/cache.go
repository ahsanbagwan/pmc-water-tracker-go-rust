// Package cache is the @Cacheable/@CacheEvict(allEntries=true) equivalent on top of the shared
// KV. Eviction is O(1): every key embeds a generation number and "evict all" just bumps it, so
// stale generations simply expire by TTL. Redis down => read straight through to the loader.
package cache

import (
	"context"
	"encoding/json"
	"log/slog"
	"strconv"
	"time"

	"pmc/internal/kv"
)

type Cache struct {
	kv  kv.KV
	ttl time.Duration
	ns  string
}

func New(k kv.KV, namespace string, ttl time.Duration) *Cache {
	return &Cache{kv: k, ttl: ttl, ns: namespace}
}

func (c *Cache) gen(ctx context.Context) string {
	v, ok, err := c.kv.Get(ctx, c.ns+":gen")
	if err != nil || !ok {
		return "0"
	}
	return v
}

func (c *Cache) key(ctx context.Context, k string) string { return c.ns + ":" + c.gen(ctx) + ":" + k }

// Load returns the cached value for key or runs loader and caches its result. Loader errors are
// never cached.
func Load[T any](ctx context.Context, c *Cache, key string, loader func() (T, error)) (T, error) {
	full := c.key(ctx, key)
	if s, ok, err := c.kv.Get(ctx, full); err == nil && ok {
		var v T
		if json.Unmarshal([]byte(s), &v) == nil {
			return v, nil
		}
	} else if err != nil {
		slog.WarnContext(ctx, "cache read failed, going to the database", "error", err)
	}
	v, err := loader()
	if err != nil {
		return v, err
	}
	if b, merr := json.Marshal(v); merr == nil {
		if serr := c.kv.Set(ctx, full, string(b), c.ttl); serr != nil {
			slog.WarnContext(ctx, "cache write failed", "error", serr)
		}
	}
	return v, nil
}

// EvictAll invalidates every entry in this namespace on every replica.
func (c *Cache) EvictAll(ctx context.Context) {
	if _, err := c.kv.Incr(ctx, c.ns+":gen"); err != nil {
		slog.WarnContext(ctx, "cache evict failed - entries will expire by TTL", "error", err)
	}
}

var _ = strconv.Itoa
