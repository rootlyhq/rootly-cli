package api

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// Cache provides a simple TTL-based in-memory cache
type Cache struct {
	mu    sync.RWMutex
	items map[string]cacheItem
	ttl   time.Duration
}

type cacheItem struct {
	value     interface{}
	expiresAt time.Time
}

// NewCache creates a new cache with the given TTL
func NewCache(ttl time.Duration) *Cache {
	return &Cache{
		items: make(map[string]cacheItem),
		ttl:   ttl,
	}
}

// Get retrieves an item from the cache
// Returns the value and true if found and not expired, nil and false otherwise
func (c *Cache) Get(key string) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	item, found := c.items[key]
	if !found {
		return nil, false
	}

	if time.Now().After(item.expiresAt) {
		return nil, false
	}

	return item.value, true
}

// Set stores an item in the cache
func (c *Cache) Set(key string, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items[key] = cacheItem{
		value:     value,
		expiresAt: time.Now().Add(c.ttl),
	}
}

// Delete removes an item from the cache
func (c *Cache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.items, key)
}

// Clear removes all items from the cache
func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items = make(map[string]cacheItem)
}

// CacheKeyBuilder builds cache keys from parameters
type CacheKeyBuilder struct {
	prefix string
	params map[string]string
}

// NewCacheKey creates a new cache key builder
func NewCacheKey(prefix string) *CacheKeyBuilder {
	return &CacheKeyBuilder{
		prefix: prefix,
		params: make(map[string]string),
	}
}

// With adds a parameter to the cache key
func (b *CacheKeyBuilder) With(key string, value interface{}) *CacheKeyBuilder {
	b.params[key] = fmt.Sprintf("%v", value)
	return b
}

// Build generates the cache key string
// Format: "prefix:key1=value1:key2=value2" (sorted by key)
func (b *CacheKeyBuilder) Build() string {
	if len(b.params) == 0 {
		return b.prefix
	}

	// Sort keys for consistent cache key generation
	keys := make([]string, 0, len(b.params))
	for k := range b.params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+b.params[k])
	}

	return b.prefix + ":" + strings.Join(parts, ":")
}

// Cache key prefixes
const (
	CacheKeyPrefixIncidents      = "incidents"
	CacheKeyPrefixAlerts         = "alerts"
	CacheKeyPrefixIncidentDetail = "incident_detail"
	CacheKeyPrefixAlertDetail    = "alert_detail"
)
