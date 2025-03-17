package memory

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/scottdavis/dsgo/pkg/agents"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSQLiteStore(t *testing.T) {
	// Test with in-memory database
	t.Run("InMemory", func(t *testing.T) {
		store, err := NewSQLiteStore(":memory:")
		require.NoError(t, err)
		defer store.Close()

		MemoryTestSuite(t, "SQLite-InMemory", store)
	})

	// Test with file-based database
	t.Run("FileBasedDB", func(t *testing.T) {
		tempFile, err := os.CreateTemp("", "sqlite-test-*.db")
		require.NoError(t, err)
		
		dbPath := tempFile.Name()
		tempFile.Close()
		
		// Clean up the temp file when done
		defer os.Remove(dbPath)

		store, err := NewSQLiteStore(dbPath)
		require.NoError(t, err)
		defer store.Close()

		MemoryTestSuite(t, "SQLite-FileBased", store)
	})
}

// TestSQLiteStoreInitialization tests specific SQLite initialization scenarios
func TestSQLiteStoreInitialization(t *testing.T) {
	store, err := NewSQLiteStore(":memory:")
	require.NoError(t, err)
	defer store.Close()

	// Test that the table is created
	rows, err := store.db.Query("SELECT name FROM sqlite_master WHERE type='table' AND name='memory_store'")
	require.NoError(t, err)
	defer rows.Close()

	hasTable := rows.Next()
	require.True(t, hasTable, "memory_store table should exist")
}

// TestSQLiteStoreOperations tests various operations on the SQLite store
func TestSQLiteStoreOperations(t *testing.T) {
	// Create an in-memory database for testing
	store, err := NewSQLiteStore(":memory:")
	require.NoError(t, err)
	defer store.Close()

	t.Run("Basic Store and Retrieve", func(t *testing.T) {
		testData := map[string]any{
			"string": "test value",
			"number": 42,
			"bool":   true,
			"array":  []string{"a", "b", "c"},
			"map":    map[string]int{"one": 1, "two": 2},
		}

		for key, value := range testData {
			err := store.Store(key, value)
			assert.NoError(t, err)

			retrieved, err := store.Retrieve(key)
			assert.NoError(t, err)
			assert.Equal(t, value, retrieved)
		}
	})

	t.Run("List Keys", func(t *testing.T) {
		// Clear existing data
		err := store.Clear()
		require.NoError(t, err)

		// Store some test data
		testKeys := []string{"key1", "key2", "key3"}
		for _, key := range testKeys {
			err := store.Store(key, "value")
			require.NoError(t, err)
		}

		// List keys
		keys, err := store.List()
		assert.NoError(t, err)
		assert.ElementsMatch(t, testKeys, keys)
	})

	t.Run("Clear Store", func(t *testing.T) {
		// Store some data
		err := store.Store("test", "value")
		require.NoError(t, err)

		// Clear the store
		err = store.Clear()
		assert.NoError(t, err)

		// Verify store is empty
		keys, err := store.List()
		assert.NoError(t, err)
		assert.Empty(t, keys)
	})

	t.Run("Non-existent Key", func(t *testing.T) {
		_, err := store.Retrieve("nonexistent")
		assert.Error(t, err)
	})

	t.Run("Concurrent Access", func(t *testing.T) {
		dbPath := filepath.Join(os.TempDir(), "test_ttl.db")
		os.Remove(dbPath) // Clean up before test

		store, err := NewSQLiteStore(dbPath)
		require.NoError(t, err)
		defer func() {
			store.Close()
			os.Remove(dbPath)
		}()
		const numGoroutines = 10
		done := make(chan bool, numGoroutines)
		var wg sync.WaitGroup

		// Clear any existing data
		err = store.Clear()
		require.NoError(t, err)

		wg.Add(numGoroutines)
		for i := 0; i < numGoroutines; i++ {
			go func(n int) {
				defer wg.Done()

				key := fmt.Sprintf("concurrent_key_%d", n)
				err := store.Store(key, n)
				if !assert.NoError(t, err, "Store failed for key: %s", key) {
					done <- false
					return
				}

				// Small delay to increase chance of concurrent access
				time.Sleep(time.Millisecond * time.Duration(rand.Intn(10)))

				retrieved, err := store.Retrieve(key)
				if !assert.NoError(t, err, "Retrieve failed for key: %s", key) {
					done <- false
					return
				}

				if !assert.Equal(t, n, retrieved, "Value mismatch for key: %s", key) {
					done <- false
					return
				}

				done <- true
			}(i)
		}

		// Wait for all goroutines to finish
		wg.Wait()
		close(done)

		// Check if any goroutine failed
		for success := range done {
			assert.True(t, success, "One or more goroutines failed")
		}
	})

	t.Run("TTL Storage", func(t *testing.T) {
		ctx := context.Background()

		// Store with short TTL
		err := store.Store("ttl_key", "ttl_value", agents.WithTTL(1*time.Second))
		require.NoError(t, err)

		// Verify value exists
		value, err := store.Retrieve("ttl_key")
		assert.NoError(t, err)
		assert.Equal(t, "ttl_value", value)
		t.Logf("Current time (UTC): %s\n", time.Now().UTC().Format(time.RFC3339))
		// Wait for TTL to expire
		time.Sleep(3 * time.Second)
		t.Logf("Current time (UTC): %s\n", time.Now().UTC().Format(time.RFC3339))

		cleaned, err := store.CleanExpired(ctx)
		assert.NoError(t, err)
		assert.Equal(t, int64(1), cleaned, "Expected one entry to be cleaned")
		// Verify value is gone
		_, err = store.Retrieve("ttl_key")
		assert.Error(t, err)
	})

	t.Run("Invalid JSON", func(t *testing.T) {
		// Try to store a channel (cannot be marshaled to JSON)
		ch := make(chan bool)
		err := store.Store("invalid", ch)
		assert.Error(t, err)
	})

	t.Run("Database Connection", func(t *testing.T) {
		// Test invalid database path
		_, err := NewSQLiteStore("/root/forbidden/db.sqlite")
		assert.Error(t, err)

		// Test closing database
		tempStore, err := NewSQLiteStore(":memory:")
		require.NoError(t, err)
		assert.NoError(t, tempStore.Close())

		// Try operations after closing
		err = tempStore.Store("key", "value")
		assert.Error(t, err)
	})
}
