package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	bolt "go.etcd.io/bbolt"
	berrors "go.etcd.io/bbolt/errors"

	"github.com/rootlyhq/rootly-tui/internal/debug"
)

var (
	cacheBucket      = []byte("cache")
	errCacheNotFound = errors.New("cache key not found")
)

// PersistentCache provides a TTL-based cache backed by BoltDB
type PersistentCache struct {
	db  *bolt.DB
	ttl time.Duration
}

type persistentCacheItem struct {
	Value     json.RawMessage `json:"value"`
	ExpiresAt time.Time       `json:"expires_at"`
}

// NewPersistentCache creates a new persistent cache at ~/.rootly-tui/cache.db
func NewPersistentCache(ttl time.Duration) (*PersistentCache, error) {
	// Get cache directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		debug.Logger.Error("Failed to get home directory", "error", err)
		return nil, fmt.Errorf("get home dir: %w", err)
	}

	cacheDir := filepath.Join(homeDir, ".rootly-tui")
	debug.Logger.Debug("Cache directory", "path", cacheDir)

	if err := os.MkdirAll(cacheDir, 0700); err != nil {
		debug.Logger.Error("Failed to create cache directory", "path", cacheDir, "error", err)
		return nil, fmt.Errorf("create cache dir: %w", err)
	}

	dbPath := filepath.Join(cacheDir, "cache.db")
	debug.Logger.Debug("Opening cache database", "path", dbPath)

	// Check if database file exists and its stats
	if info, err := os.Stat(dbPath); err == nil {
		debug.Logger.Debug("Cache database exists",
			"path", dbPath,
			"size", info.Size(),
			"modTime", info.ModTime(),
		)
	} else if os.IsNotExist(err) {
		debug.Logger.Debug("Cache database does not exist, will create", "path", dbPath)
	}

	db, err := bolt.Open(dbPath, 0600, &bolt.Options{
		Timeout: 2 * time.Second,
	})
	if err != nil {
		debug.Logger.Error("Failed to open cache database",
			"path", dbPath,
			"error", err,
			"hint", "Another instance may be running, or the database file may be corrupted",
		)
		// If timeout, suggest deleting the lock file
		if errors.Is(err, berrors.ErrTimeout) {
			debug.Logger.Warn("Database lock timeout - another process may have the file open",
				"path", dbPath,
				"suggestion", "Try closing other rootly-tui instances or delete the cache.db file",
			)
		}
		return nil, fmt.Errorf("open cache db: %w", err)
	}

	debug.Logger.Debug("Cache database opened successfully", "path", dbPath)

	// Create bucket if it doesn't exist
	err = db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(cacheBucket)
		return err
	})
	if err != nil {
		_ = db.Close()
		debug.Logger.Error("Failed to create cache bucket", "error", err)
		return nil, fmt.Errorf("create bucket: %w", err)
	}

	debug.Logger.Info("Persistent cache initialized", "path", dbPath, "ttl", ttl)

	return &PersistentCache{
		db:  db,
		ttl: ttl,
	}, nil
}

// Get retrieves an item from the cache
func (c *PersistentCache) Get(key string) (interface{}, bool) {
	var item persistentCacheItem

	err := c.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(cacheBucket)
		data := b.Get([]byte(key))
		if data == nil {
			return errCacheNotFound
		}
		return json.Unmarshal(data, &item)
	})

	if err != nil {
		return nil, false
	}

	// Check expiration
	if time.Now().After(item.ExpiresAt) {
		debug.Logger.Debug("Cache expired", "key", key)
		// Clean up expired item asynchronously
		go c.Delete(key)
		return nil, false
	}

	debug.Logger.Debug("Cache hit", "key", key)
	return item.Value, true
}

// GetTyped retrieves and unmarshals an item from the cache
func (c *PersistentCache) GetTyped(key string, dest interface{}) bool {
	value, ok := c.Get(key)
	if !ok {
		return false
	}

	rawJSON, ok := value.(json.RawMessage)
	if !ok {
		return false
	}

	if err := json.Unmarshal(rawJSON, dest); err != nil {
		debug.Logger.Debug("Cache unmarshal error", "key", key, "error", err)
		return false
	}

	return true
}

// Set stores an item in the cache
func (c *PersistentCache) Set(key string, value interface{}) {
	// Marshal the value to JSON
	valueJSON, err := json.Marshal(value)
	if err != nil {
		debug.Logger.Debug("Cache marshal error", "key", key, "error", err)
		return
	}

	item := persistentCacheItem{
		Value:     valueJSON,
		ExpiresAt: time.Now().Add(c.ttl),
	}

	data, err := json.Marshal(item)
	if err != nil {
		debug.Logger.Debug("Cache item marshal error", "key", key, "error", err)
		return
	}

	err = c.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(cacheBucket)
		return b.Put([]byte(key), data)
	})

	if err != nil {
		debug.Logger.Debug("Cache write error", "key", key, "error", err)
		return
	}

	debug.Logger.Debug("Cache set", "key", key, "ttl", c.ttl)
}

// Delete removes an item from the cache
func (c *PersistentCache) Delete(key string) {
	_ = c.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(cacheBucket)
		return b.Delete([]byte(key))
	})
}

// Clear removes all items from the cache
func (c *PersistentCache) Clear() {
	_ = c.db.Update(func(tx *bolt.Tx) error {
		// Delete and recreate the bucket
		if err := tx.DeleteBucket(cacheBucket); err != nil {
			return err
		}
		_, err := tx.CreateBucket(cacheBucket)
		return err
	})
	debug.Logger.Debug("Cache cleared")
}

// Close closes the database connection
func (c *PersistentCache) Close() error {
	if c.db != nil {
		return c.db.Close()
	}
	return nil
}

// Cleanup removes expired entries from the cache
func (c *PersistentCache) Cleanup() {
	var expiredKeys []string

	_ = c.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(cacheBucket)
		return b.ForEach(func(k, v []byte) error {
			var item persistentCacheItem
			if err := json.Unmarshal(v, &item); err == nil {
				if time.Now().After(item.ExpiresAt) {
					expiredKeys = append(expiredKeys, string(k))
				}
			}
			return nil
		})
	})

	if len(expiredKeys) > 0 {
		_ = c.db.Update(func(tx *bolt.Tx) error {
			b := tx.Bucket(cacheBucket)
			for _, key := range expiredKeys {
				_ = b.Delete([]byte(key))
			}
			return nil
		})
		debug.Logger.Debug("Cache cleanup", "removed", len(expiredKeys))
	}
}
