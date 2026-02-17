package api

import (
	"testing"
	"time"
)

func TestNewCache(t *testing.T) {
	cache := NewCache(time.Minute)

	if cache.ttl != time.Minute {
		t.Errorf("expected ttl %v, got %v", time.Minute, cache.ttl)
	}
}

func TestCacheSetGet(t *testing.T) {
	cache := NewCache(time.Minute)

	// Set a value
	cache.Set("key1", "value1")

	// Get it back
	val, ok := cache.Get("key1")
	if !ok {
		t.Fatal("expected to find key1")
	}
	if val != "value1" {
		t.Errorf("expected value1, got %v", val)
	}
}

func TestCacheGetMissing(t *testing.T) {
	cache := NewCache(time.Minute)

	val, ok := cache.Get("nonexistent")
	if ok {
		t.Error("expected not to find nonexistent key")
	}
	if val != nil {
		t.Errorf("expected nil value, got %v", val)
	}
}

func TestCacheExpiry(t *testing.T) {
	// Very short TTL
	cache := NewCache(10 * time.Millisecond)

	cache.Set("key1", "value1")

	// Should find it immediately
	_, ok := cache.Get("key1")
	if !ok {
		t.Fatal("expected to find key1 immediately")
	}

	// Wait for expiry
	time.Sleep(20 * time.Millisecond)

	// Should not find it after expiry
	_, ok = cache.Get("key1")
	if ok {
		t.Error("expected key1 to be expired")
	}
}

func TestCacheDelete(t *testing.T) {
	cache := NewCache(time.Minute)

	cache.Set("key1", "value1")
	cache.Delete("key1")

	_, ok := cache.Get("key1")
	if ok {
		t.Error("expected key1 to be deleted")
	}
}

func TestCacheClear(t *testing.T) {
	cache := NewCache(time.Minute)

	cache.Set("key1", "value1")
	cache.Set("key2", "value2")
	cache.Clear()

	_, ok1 := cache.Get("key1")
	_, ok2 := cache.Get("key2")
	if ok1 || ok2 {
		t.Error("expected cache to be cleared")
	}
}

func TestCacheKeyBuilder(t *testing.T) {
	tests := []struct {
		name     string
		prefix   string
		params   map[string]interface{}
		expected string
	}{
		{
			name:     "no params",
			prefix:   "incidents",
			params:   nil,
			expected: "incidents",
		},
		{
			name:   "single param",
			prefix: "incidents",
			params: map[string]interface{}{
				"pageSize": 50,
			},
			expected: "incidents:pageSize=50",
		},
		{
			name:   "multiple params sorted",
			prefix: "incidents",
			params: map[string]interface{}{
				"status":   "open",
				"pageSize": 50,
				"page":     1,
			},
			expected: "incidents:page=1:pageSize=50:status=open",
		},
		{
			name:   "alerts prefix",
			prefix: "alerts",
			params: map[string]interface{}{
				"pageSize": 25,
			},
			expected: "alerts:pageSize=25",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := NewCacheKey(tt.prefix)
			for k, v := range tt.params {
				builder.With(k, v)
			}
			result := builder.Build()
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestCacheKeyBuilderChaining(t *testing.T) {
	key := NewCacheKey("incidents").
		With("pageSize", 50).
		With("status", "open").
		Build()

	expected := "incidents:pageSize=50:status=open"
	if key != expected {
		t.Errorf("expected %q, got %q", expected, key)
	}
}

func TestCacheWithDifferentKeys(t *testing.T) {
	cache := NewCache(time.Minute)

	// Store with different parameter-based keys
	key1 := NewCacheKey("incidents").With("pageSize", 50).Build()
	key2 := NewCacheKey("incidents").With("pageSize", 100).Build()

	cache.Set(key1, []string{"incident1"})
	cache.Set(key2, []string{"incident1", "incident2"})

	// Both should be retrievable
	val1, ok1 := cache.Get(key1)
	val2, ok2 := cache.Get(key2)

	if !ok1 || !ok2 {
		t.Fatal("expected both keys to be found")
	}

	slice1 := val1.([]string)
	slice2 := val2.([]string)

	if len(slice1) != 1 {
		t.Errorf("expected 1 item for key1, got %d", len(slice1))
	}
	if len(slice2) != 2 {
		t.Errorf("expected 2 items for key2, got %d", len(slice2))
	}
}

func TestCacheConcurrency(t *testing.T) {
	cache := NewCache(time.Minute)

	// Run concurrent reads and writes
	done := make(chan bool)

	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				key := NewCacheKey("test").With("id", id).With("j", j).Build()
				cache.Set(key, id*1000+j)
				cache.Get(key)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}
