// Package cacher provides an in-memory, thread-safe cache with support for TTL,
// multiple eviction policies (LRU, MRU, LFU, RANDOM), and automatic cleanup.
package cacher

import (
	"container/list"
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"
)

// Eviction policies
const (
	LRU    = iota // Least Recently Used
	MRU           // Most Recently Used
	LFU           // Least Frequently Used
	RANDOM        // Random eviction
)

var (
	defaultClearingInterval = 100 * time.Second
)

// Config holds configuration for the cache.
type Config struct {
	// Capacity is the maximum number of items in the cache.
	// If 0, capacity is unlimited.
	Capacity int

	// ClearingInterval is how often expired items are removed.
	// If 0, defaults to 100 seconds.
	ClearingInterval time.Duration

	// EvictionPolicy defines which item to remove when capacity is reached.
	// Must be one of: LRU, MRU, LFU, RANDOM.
	EvictionPolicy int
}

// cache holds the actual cached value and metadata.
type cache struct {
	value      interface{}   // The stored value
	ttl        time.Duration // Time-to-live
	counter    int           // Access counter (for LFU)
	lastUsedAt time.Time     // Last access time (for LRU/MRU)
}

// Cacher is a thread-safe in-memory cache with TTL and eviction policies.
type Cacher struct {
	mu               sync.RWMutex
	cache            map[interface{}]cache // Main storage
	capacity         int                   // Max items
	keys             *list.List            // Order of access (for LRU/MRU)
	clearingInterval time.Duration
	evictionPolicy   int
	ctx              context.Context
	cancel           context.CancelFunc
}

// New creates a new cache with the given configuration.
// Starts a background goroutine to clean expired items.
func New(cfg Config) *Cacher {
	if cfg.ClearingInterval == 0 {
		cfg.ClearingInterval = defaultClearingInterval
	}

	ctx, cancel := context.WithCancel(context.Background())
	cacher := &Cacher{
		cache:            make(map[interface{}]cache),
		capacity:         cfg.Capacity,
		keys:             list.New(),
		clearingInterval: cfg.ClearingInterval,
		evictionPolicy:   cfg.EvictionPolicy,
		ctx:              ctx,
		cancel:           cancel,
	}

	go cacher.startClearing()
	return cacher
}

// Get retrieves a value from the cache by key.
// Returns an error if the key is not found or the TTL has expired.
func (c *Cacher) Get(key interface{}) (interface{}, error) {
	c.mu.RLock()
	value, ok := c.cache[key]
	c.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("cache not found for key: %v", key)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if err := checkExpiration(value); err != nil {
		c.removeKey(key)
		return nil, err
	}
	c.update(key, value)

	keyNote := c.getKeyNote(key)
	if keyNote != nil {
		c.keys.MoveToFront(keyNote)
	}

	return value.value, nil
}

// GetAll returns all values in the cache (order not guaranteed).
func (c *Cacher) GetAll() []interface{} {
	c.mu.Lock()
	defer c.mu.Unlock()

	values := make([]interface{}, 0, len(c.cache))
	for _, item := range c.cache {
		values = append(values, item.value)
	}
	return values
}

// Set adds a value to the cache with a TTL.
// If capacity is reached, an item is evicted based on the policy.
func (c *Cacher) Set(key, value interface{}, ttl time.Duration) {
	item := cache{
		value:      value,
		ttl:        ttl,
		counter:    1,
		lastUsedAt: time.Now(),
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.capacity > 0 && len(c.cache) >= c.capacity {
		c.evict()
	}

	c.cache[key] = item
	c.keys.PushFront(key)
}

// Clear removes all items from the cache.
func (c *Cacher) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cache = make(map[interface{}]cache)
	c.keys = list.New()
}

// Delete removes an item from the cache by key.
// Returns an error if the key is not found.
func (c *Cacher) Delete(key interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.cache[key]; !ok {
		return fmt.Errorf("cache not found for key: %v", key)
	}

	c.removeKey(key)
	return nil
}

// SetCapacity changes the maximum number of items in the cache.
// Can be called at runtime.
func (c *Cacher) SetCapacity(newCapacity int) error {
	if newCapacity < 0 {
		return fmt.Errorf("capacity cannot be negative: %d", newCapacity)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.capacity = newCapacity
	return nil
}

// GetCapacity returns the current capacity of the cache.
func (c *Cacher) GetCapacity() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.capacity
}

// SetEvictionPolicy changes the eviction policy at runtime.
// Must be one of: LRU, MRU, LFU, RANDOM.
func (c *Cacher) SetEvictionPolicy(policy int) error {
	if policy < LRU || policy > RANDOM {
		return fmt.Errorf("invalid eviction policy: %d (must be 0-3)", policy)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.evictionPolicy = policy
	return nil
}

// GetEvictionPolicy returns the current eviction policy as a string.
func (c *Cacher) GetEvictionPolicy() string {
	c.mu.Lock()
	defer c.mu.Unlock()

	switch c.evictionPolicy {
	case LRU:
		return "LRU"
	case MRU:
		return "MRU"
	case LFU:
		return "LFU"
	case RANDOM:
		return "RANDOM"
	}
	return "UNKNOWN"
}

// SetTTL updates the TTL of an existing item.
func (c *Cacher) SetTTL(key interface{}, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	item, ok := c.cache[key]
	if !ok {
		return fmt.Errorf("cache not found for key: %v", key)
	}

	item.ttl = ttl
	c.cache[key] = item
	return nil
}

// GetTTL returns the remaining TTL for a key.
// Returns an error if the key is not found.
func (c *Cacher) GetTTL(key interface{}) (time.Duration, error) {
	c.mu.RLock()
	item, ok := c.cache[key]
	c.mu.RUnlock()
	if !ok {
		return 0, fmt.Errorf("cache not found for key: %v", key)
	}
	return item.ttl, nil
}

// GetCounter returns the access counter for a key.
// Useful for LFU debugging.
func (c *Cacher) GetCounter(key interface{}) (int, error) {
	c.mu.RLock()
	item, ok := c.cache[key]
	c.mu.RUnlock()
	if !ok {
		return -1, fmt.Errorf("cache not found for key: %v", key)
	}
	return item.counter, nil
}

// Keys returns a slice of all keys in the cache.
// Returns an error if the cache is empty.
func (c *Cacher) Keys() ([]interface{}, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.cache) == 0 {
		return nil, errors.New("no keys found")
	}

	keys := make([]interface{}, 0, len(c.cache))
	for key := range c.cache {
		keys = append(keys, key)
	}
	return keys, nil
}

// Stats returns a formatted string with cache statistics.
// Useful for debugging and monitoring.
func (c *Cacher) Stats() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	policy := "UNKNOWN"
	switch c.evictionPolicy {
	case LRU:
		policy = "LRU"
	case MRU:
		policy = "MRU"
	case LFU:
		policy = "LFU"
	case RANDOM:
		policy = "RANDOM"
	}

	capacity := "unlimited"
	if c.capacity > 0 {
		capacity = strconv.Itoa(c.capacity)
	}

	occupancy := 0.0
	if c.capacity > 0 {
		occupancy = (float64(len(c.cache)) * 100) / float64(c.capacity)
	}

	stats := fmt.Sprintf("STATS\n"+
		"Eviction Policy: %s\n"+
		"Capacity: %s\n"+
		"Clearing Interval: %v\n"+
		"Items: %d\n"+
		"Occupancy: %.2f%%\n"+
		"Cache:\n",
		policy, capacity, c.clearingInterval, len(c.cache), occupancy)

	for key, value := range c.cache {
		stats += fmt.Sprintf("  Key: %v Value: %v TTL: %v Counter: %d Last Used: %v\n",
			key, value.value, value.ttl, value.counter, value.lastUsedAt)
	}

	return stats
}

// Close stops the background clearing goroutine.
// Should be called when the cache is no longer needed.
func (c *Cacher) Close() {
	c.cancel()
}

// update increments the access counter and updates lastUsedAt.
func (c *Cacher) update(key interface{}, value cache) {
	value.counter++
	value.lastUsedAt = time.Now()
	c.cache[key] = value
}

// startClearing runs a background loop to remove expired items.
func (c *Cacher) startClearing() {
	ticker := time.NewTicker(c.clearingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.mu.Lock()
			c.processClearing()
			c.mu.Unlock()
		case <-c.ctx.Done():
			return
		}
	}
}

// processClearing removes all expired items from the cache.
func (c *Cacher) processClearing() {
	now := time.Now()
	for key, value := range c.cache {
		if value.ttl != 0 && value.lastUsedAt.Add(value.ttl).Before(now) {
			c.removeKey(key)
		}
	}
}

// removeKey removes a key from both the map and the list.
func (c *Cacher) removeKey(key interface{}) {
	e := c.getKeyNote(key)
	if e != nil {
		c.keys.Remove(e)
	}
	delete(c.cache, key)
}

// evict removes one item based on the current policy.
func (c *Cacher) evict() {
	switch c.evictionPolicy {
	case LRU:
		c.evictLRU()
	case MRU:
		c.evictMRU()
	case LFU:
		c.evictLFU()
	case RANDOM:
		c.evictRANDOM()
	}
}

// evictLRU removes the least recently used item (from the back of the list).
func (c *Cacher) evictLRU() {
	if e := c.keys.Back(); e != nil {
		c.removeKey(e.Value)
	}
}

// evictMRU removes the most recently used item (from the front of the list).
func (c *Cacher) evictMRU() {
	if e := c.keys.Front(); e != nil {
		c.removeKey(e.Value)
	}
}

// evictLFU removes the least frequently used item.
func (c *Cacher) evictLFU() {
	var minKey interface{}
	var minCount = -1
	for key, value := range c.cache {
		if minCount == -1 || value.counter < minCount {
			minKey = key
			minCount = value.counter
		}
	}
	if minKey != nil {
		c.removeKey(minKey)
	}
}

// evictRANDOM removes a random item (the first one iterated).
func (c *Cacher) evictRANDOM() {
	for key := range c.cache {
		c.removeKey(key)
		break
	}
}

// checkExpiration returns an error if the item has expired.
func checkExpiration(value cache) error {
	if value.ttl != 0 && value.lastUsedAt.Add(value.ttl).Before(time.Now()) {
		return errors.New("TTL expired")
	}
	return nil
}

// getKeyNote finds the list element for a key.
func (c *Cacher) getKeyNote(key interface{}) *list.Element {
	for e := c.keys.Front(); e != nil; e = e.Next() {
		if e.Value == key {
			return e
		}
	}
	return nil
}
