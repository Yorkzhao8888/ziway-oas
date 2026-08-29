// Package cache provides a generic 3-level cache: L1 (in-process) → L2 (Redis) → L3 (origin/DB).
// On read: L1 hit → return; L1 miss → L2 hit → backfill L1 → return; L2 miss → L3 → backfill L2+L1.
// On invalidate: clear L1 + L2; L3 is the authority and is not cleared here.
package cache

import (
	"context"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// Origin is the L3 data source (e.g., database query).
type Origin[T any] func(ctx context.Context) (T, error)

// ThreeLevelCache implements L1 (sync.Map) → L2 (Redis) → L3 (origin func).
type ThreeLevelCache[T any] struct {
	mu       sync.RWMutex
	l1       map[string]l1Entry[T]
	l1TTL    time.Duration
	rdb      *redis.Client
	l2TTL    time.Duration
	keyPrefix string
	origin   Origin[T]
}

type l1Entry[T any] struct {
	value     T
	expiresAt time.Time
}

// NewThreeLevelCache creates a 3-level cache.
// rdb may be nil — L2 is skipped when Redis is unavailable.
func NewThreeLevelCache[T any](rdb *redis.Client, l1TTL, l2TTL time.Duration, keyPrefix string, origin Origin[T]) *ThreeLevelCache[T] {
	if l1TTL == 0 {
		l1TTL = 30 * time.Second
	}
	if l2TTL == 0 {
		l2TTL = 5 * time.Minute
	}
	return &ThreeLevelCache[T]{
		l1:        make(map[string]l1Entry[T]),
		l1TTL:     l1TTL,
		rdb:       rdb,
		l2TTL:     l2TTL,
		keyPrefix: keyPrefix,
		origin:    origin,
	}
}

// Get retrieves a value through the 3-level cascade.
func (c *ThreeLevelCache[T]) Get(ctx context.Context, key string) (T, error) {
	// L1: in-process
	c.mu.RLock()
	if entry, ok := c.l1[key]; ok && time.Now().Before(entry.expiresAt) {
		c.mu.RUnlock()
		return entry.value, nil
	}
	c.mu.RUnlock()

	// L2: Redis
	if c.rdb != nil {
		l2Key := c.keyPrefix + ":" + key
		var val T
		if err := c.rdb.Get(ctx, l2Key).Scan(&val); err == nil {
			c.backfillL1(key, val)
			return val, nil
		}
	}

	// L3: origin (DB)
	val, err := c.origin(ctx)
	if err != nil {
		var zero T
		return zero, err
	}

	c.backfillL1(key, val)
	c.backfillL2(ctx, key, val)
	return val, nil
}

// Invalidate clears L1 and L2 for a given key. L3 (DB) is the authority and is not touched.
func (c *ThreeLevelCache[T]) Invalidate(ctx context.Context, key string) {
	c.mu.Lock()
	delete(c.l1, key)
	c.mu.Unlock()

	if c.rdb != nil {
		l2Key := c.keyPrefix + ":" + key
		c.rdb.Del(ctx, l2Key)
	}
}

// InvalidateAll clears all L1 entries and L2 keys with the configured prefix.
func (c *ThreeLevelCache[T]) InvalidateAll(ctx context.Context) {
	c.mu.Lock()
	c.l1 = make(map[string]l1Entry[T])
	c.mu.Unlock()

	if c.rdb != nil {
		iter := c.rdb.Scan(ctx, 0, c.keyPrefix+":*", 100).Iterator()
		for iter.Next(ctx) {
			c.rdb.Del(ctx, iter.Val())
		}
	}
}

func (c *ThreeLevelCache[T]) backfillL1(key string, val T) {
	c.mu.Lock()
	c.l1[key] = l1Entry[T]{value: val, expiresAt: time.Now().Add(c.l1TTL)}
	c.mu.Unlock()
}

func (c *ThreeLevelCache[T]) backfillL2(ctx context.Context, key string, val T) {
	if c.rdb == nil {
		return
	}
	l2Key := c.keyPrefix + ":" + key
	c.rdb.Set(ctx, l2Key, val, c.l2TTL)
}
