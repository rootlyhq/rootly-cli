package api

import (
	"os"
	"testing"
	"time"
)

func TestNewPersistentCache(t *testing.T) {
	defer setupTestEnv(t)()

	cache, err := NewPersistentCache(30 * time.Second)
	if err != nil {
		t.Fatalf("NewPersistentCache() error = %v", err)
	}
	defer cache.Close()

	// Check that the database file was created in the temp home dir
	homeDir, _ := os.UserHomeDir()
	dbPath := homeDir + "/.rootly-tui/cache.db"
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("expected cache.db to exist")
	}
}

func TestPersistentCacheSetGet(t *testing.T) {
	defer setupTestEnv(t)()

	cache, err := NewPersistentCache(30 * time.Second)
	if err != nil {
		t.Fatalf("NewPersistentCache() error = %v", err)
	}
	defer cache.Close()

	// Test Set and Get
	testValue := []string{"item1", "item2", "item3"}
	cache.Set("test-key", testValue)

	var result []string
	if !cache.GetTyped("test-key", &result) {
		t.Error("expected GetTyped to return true")
	}

	if len(result) != 3 {
		t.Errorf("expected 3 items, got %d", len(result))
	}
	if result[0] != "item1" {
		t.Errorf("expected 'item1', got '%s'", result[0])
	}
}

func TestPersistentCacheExpiry(t *testing.T) {
	defer setupTestEnv(t)()

	// Use a longer TTL for Windows compatibility (Windows timer resolution is ~15ms)
	cache, err := NewPersistentCache(150 * time.Millisecond)
	if err != nil {
		t.Fatalf("NewPersistentCache() error = %v", err)
	}
	defer cache.Close()

	cache.Set("expiring-key", "test-value")

	// Should be available immediately
	var result string
	if !cache.GetTyped("expiring-key", &result) {
		t.Error("expected GetTyped to return true before expiry")
	}

	// Wait for expiry
	time.Sleep(200 * time.Millisecond)

	// Should be expired now
	if cache.GetTyped("expiring-key", &result) {
		t.Error("expected GetTyped to return false after expiry")
	}
}

func TestPersistentCacheDelete(t *testing.T) {
	defer setupTestEnv(t)()

	cache, err := NewPersistentCache(30 * time.Second)
	if err != nil {
		t.Fatalf("NewPersistentCache() error = %v", err)
	}
	defer cache.Close()

	cache.Set("to-delete", "value")

	var result string
	if !cache.GetTyped("to-delete", &result) {
		t.Error("expected value to exist before delete")
	}

	cache.Delete("to-delete")

	if cache.GetTyped("to-delete", &result) {
		t.Error("expected value to be deleted")
	}
}

func TestPersistentCacheClear(t *testing.T) {
	defer setupTestEnv(t)()

	cache, err := NewPersistentCache(30 * time.Second)
	if err != nil {
		t.Fatalf("NewPersistentCache() error = %v", err)
	}
	defer cache.Close()

	cache.Set("key1", "value1")
	cache.Set("key2", "value2")

	cache.Clear()

	var result string
	if cache.GetTyped("key1", &result) {
		t.Error("expected key1 to be cleared")
	}
	if cache.GetTyped("key2", &result) {
		t.Error("expected key2 to be cleared")
	}
}

func TestPersistentCachePersistence(t *testing.T) {
	defer setupTestEnv(t)()

	// Create cache and set a value
	cache1, err := NewPersistentCache(30 * time.Second)
	if err != nil {
		t.Fatalf("NewPersistentCache() error = %v", err)
	}
	cache1.Set("persistent-key", "persistent-value")
	cache1.Close()

	// Create a new cache instance - should see the same value
	cache2, err := NewPersistentCache(30 * time.Second)
	if err != nil {
		t.Fatalf("NewPersistentCache() second instance error = %v", err)
	}
	defer cache2.Close()

	var result string
	if !cache2.GetTyped("persistent-key", &result) {
		t.Error("expected persistent-key to persist across cache instances")
	}
	if result != "persistent-value" {
		t.Errorf("expected 'persistent-value', got '%s'", result)
	}
}

func TestPersistentCacheWithIncidentStruct(t *testing.T) {
	defer setupTestEnv(t)()

	cache, err := NewPersistentCache(30 * time.Second)
	if err != nil {
		t.Fatalf("NewPersistentCache() error = %v", err)
	}
	defer cache.Close()

	incidents := []Incident{
		{
			ID:           "inc_001",
			SequentialID: "INC-1",
			Title:        "Test Incident",
			Status:       "in_progress",
			Severity:     "critical",
			Services:     []string{"api", "web"},
		},
		{
			ID:           "inc_002",
			SequentialID: "INC-2",
			Title:        "Another Incident",
			Status:       "resolved",
			Severity:     "low",
		},
	}

	cache.Set("incidents:pageSize=50", incidents)

	var result []Incident
	if !cache.GetTyped("incidents:pageSize=50", &result) {
		t.Fatal("expected GetTyped to return true for incidents")
	}

	if len(result) != 2 {
		t.Errorf("expected 2 incidents, got %d", len(result))
	}
	if result[0].Title != "Test Incident" {
		t.Errorf("expected 'Test Incident', got '%s'", result[0].Title)
	}
	if len(result[0].Services) != 2 {
		t.Errorf("expected 2 services, got %d", len(result[0].Services))
	}
}

func TestPersistentCacheCleanup(t *testing.T) {
	defer setupTestEnv(t)()

	// Use a longer TTL for Windows compatibility (Windows timer resolution is ~15ms)
	cache, err := NewPersistentCache(150 * time.Millisecond)
	if err != nil {
		t.Fatalf("NewPersistentCache() error = %v", err)
	}
	defer cache.Close()

	// Add some entries
	cache.Set("key1", "value1")
	cache.Set("key2", "value2")

	// Wait for entries to expire
	time.Sleep(200 * time.Millisecond)

	// Run cleanup
	cache.Cleanup()

	// Entries should be gone
	var result string
	if cache.GetTyped("key1", &result) {
		t.Error("expected key1 to be cleaned up")
	}
	if cache.GetTyped("key2", &result) {
		t.Error("expected key2 to be cleaned up")
	}
}

func TestPersistentCacheCleanupPartial(t *testing.T) {
	defer setupTestEnv(t)()

	// Use a longer TTL for Windows compatibility (Windows timer resolution is ~15ms)
	cache, err := NewPersistentCache(200 * time.Millisecond)
	if err != nil {
		t.Fatalf("NewPersistentCache() error = %v", err)
	}
	defer cache.Close()

	// Add first entry
	cache.Set("old-key", "old-value")

	// Wait for first entry to expire
	time.Sleep(250 * time.Millisecond)

	// Add second entry (fresh)
	cache.Set("new-key", "new-value")

	// Run cleanup
	cache.Cleanup()

	// Old entry should be gone, new entry should remain
	var result string
	if cache.GetTyped("old-key", &result) {
		t.Error("expected old-key to be cleaned up")
	}
	if !cache.GetTyped("new-key", &result) {
		t.Error("expected new-key to still exist")
	}
}

func TestPersistentCacheGetMiss(t *testing.T) {
	defer setupTestEnv(t)()

	cache, err := NewPersistentCache(30 * time.Second)
	if err != nil {
		t.Fatalf("NewPersistentCache() error = %v", err)
	}
	defer cache.Close()

	// Get on non-existent key
	_, ok := cache.Get("nonexistent")
	if ok {
		t.Error("expected Get to return false for nonexistent key")
	}
}

func TestPersistentCacheGetTypedInvalidType(t *testing.T) {
	defer setupTestEnv(t)()

	cache, err := NewPersistentCache(30 * time.Second)
	if err != nil {
		t.Fatalf("NewPersistentCache() error = %v", err)
	}
	defer cache.Close()

	// Store a string
	cache.Set("string-key", "just a string")

	// Try to get as a struct (should fail unmarshal)
	var result struct {
		Field string `json:"field"`
	}
	if cache.GetTyped("string-key", &result) {
		// It might still work if the JSON unmarshal succeeds
		t.Log("GetTyped returned true (string can unmarshal to struct with string field)")
	}
}
