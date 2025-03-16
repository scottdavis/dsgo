package memory

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MemoryTestSuite runs a suite of tests against any Memory implementation.
func MemoryTestSuite(t *testing.T, name string, store Memory) {
	t.Run(name+"/StoreAndRetrieve", func(t *testing.T) {
		// Clean up before test
		err := store.Clear()
		require.NoError(t, err)

		// Test string
		err = store.Store("key1", "value1")
		require.NoError(t, err)
		
		val, err := store.Retrieve("key1")
		require.NoError(t, err)
		assert.Equal(t, "value1", val)
		
		// Test int
		err = store.Store("key2", 42)
		require.NoError(t, err)
		
		val, err = store.Retrieve("key2")
		require.NoError(t, err)
		assert.Equal(t, 42, val)
		
		// Test map
		testMap := map[string]int{"one": 1, "two": 2}
		err = store.Store("key3", testMap)
		require.NoError(t, err)
		
		val, err = store.Retrieve("key3")
		require.NoError(t, err)
		assert.Equal(t, testMap, val)
		
		// Test string array
		testArray := []string{"a", "b", "c"}
		err = store.Store("key4", testArray)
		require.NoError(t, err)
		
		val, err = store.Retrieve("key4")
		require.NoError(t, err)
		assert.Equal(t, testArray, val)
	})

	t.Run(name+"/NotFound", func(t *testing.T) {
		_, err := store.Retrieve("nonexistent-key")
		assert.Error(t, err)
	})

	t.Run(name+"/List", func(t *testing.T) {
		// Clear and add test data
		err := store.Clear()
		require.NoError(t, err)
		
		err = store.Store("key1", "value1")
		require.NoError(t, err)
		
		err = store.Store("key2", "value2")
		require.NoError(t, err)
		
		// List keys
		keys, err := store.List()
		require.NoError(t, err)
		assert.Len(t, keys, 2)
		assert.Contains(t, keys, "key1")
		assert.Contains(t, keys, "key2")
	})

	t.Run(name+"/Clear", func(t *testing.T) {
		// Add test data
		err := store.Store("key1", "value1")
		require.NoError(t, err)
		
		// Clear
		err = store.Clear()
		require.NoError(t, err)
		
		// Verify everything is gone
		keys, err := store.List()
		require.NoError(t, err)
		assert.Len(t, keys, 0)
	})

	t.Run(name+"/TTL", func(t *testing.T) {
		// Clear first
		err := store.Clear()
		require.NoError(t, err)
		
		ctx := context.Background()
		
		// Store with short TTL
		err = store.StoreWithTTL(ctx, "ttl-key", "ttl-value", 100*time.Millisecond)
		require.NoError(t, err)
		
		// Verify it exists
		val, err := store.Retrieve("ttl-key")
		require.NoError(t, err)
		assert.Equal(t, "ttl-value", val)
		
		// Wait for expiration
		time.Sleep(200 * time.Millisecond)
		
		// Clean expired entries
		_, err = store.CleanExpired(ctx)
		require.NoError(t, err)
		
		// Verify it's gone
		_, err = store.Retrieve("ttl-key")
		assert.Error(t, err)
		
		// Store with longer TTL
		err = store.StoreWithTTL(ctx, "ttl-key2", "ttl-value2", 1*time.Hour)
		require.NoError(t, err)
		
		// Verify it exists
		val, err = store.Retrieve("ttl-key2")
		require.NoError(t, err)
		assert.Equal(t, "ttl-value2", val)
		
		// Clean up
		err = store.Clear()
		require.NoError(t, err)
	})
} 