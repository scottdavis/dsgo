//go:build redis
// +build redis

package memory

import (
	"context"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRedisStore(t *testing.T) {
	// Skip this test if not in CI or if REDIS_TEST_ADDR environment variable is not set
	if os.Getenv("REDIS_TEST_ADDR") == "" {
		// On CI, only require Redis for Linux platforms
		if os.Getenv("CI") != "" && runtime.GOOS == "linux" {
			t.Fatal("REDIS_TEST_ADDR environment variable must be set in Linux CI environment")
		} else {
			t.Skip("Skipping Redis tests. Set REDIS_TEST_ADDR to run.")
		}
	}

	// Get Redis connection details from environment
	redisAddr := os.Getenv("REDIS_TEST_ADDR")
	redisPassword := os.Getenv("REDIS_TEST_PASSWORD") // Can be empty
	
	// Create a new store
	store, err := NewRedisStore(redisAddr, redisPassword, 0)
	require.NoError(t, err)
	defer store.Close()

	// Clear existing data before running tests
	err = store.Clear()
	require.NoError(t, err)

	// Run all the common tests
	MemoryTestSuite(t, "Redis", store)
}

// TestRedisStoreReconnection tests that Redis can handle reconnection
func TestRedisStoreReconnection(t *testing.T) {
	// Skip this test if REDIS_TEST_ADDR environment variable is not set
	if os.Getenv("REDIS_TEST_ADDR") == "" {
		// On CI, only require Redis for Linux platforms
		if os.Getenv("CI") != "" && runtime.GOOS == "linux" {
			t.Skip("Skipping Redis reconnection test in non-Linux CI environment")
		} else {
			t.Skip("Skipping Redis reconnection test. Set REDIS_TEST_ADDR to run.")
		}
	}

	// Get Redis connection details from environment
	redisAddr := os.Getenv("REDIS_TEST_ADDR")
	redisPassword := os.Getenv("REDIS_TEST_PASSWORD") // Can be empty
	
	// Create a new store
	store, err := NewRedisStore(redisAddr, redisPassword, 0)
	require.NoError(t, err)
	defer store.Close()

	// Test basic operations
	err = store.Store("reconnect-test", "value")
	require.NoError(t, err)

	val, err := store.Retrieve("reconnect-test")
	require.NoError(t, err)
	require.Equal(t, "value", val)

	// Close and reopen connection
	err = store.Close()
	require.NoError(t, err)

	// Create a new client
	store, err = NewRedisStore(redisAddr, redisPassword, 0)
	require.NoError(t, err)
	defer store.Close()

	// Key should still be available
	val, err = store.Retrieve("reconnect-test")
	require.NoError(t, err)
	require.Equal(t, "value", val)

	// Clean up
	err = store.Clear()
	require.NoError(t, err)
}

// TestRedisAutoExpiration tests that Redis automatically expires keys
func TestRedisAutoExpiration(t *testing.T) {
	// Skip this test if REDIS_TEST_ADDR environment variable is not set
	if os.Getenv("REDIS_TEST_ADDR") == "" {
		// On CI, only require Redis for Linux platforms
		if os.Getenv("CI") != "" && runtime.GOOS == "linux" {
			t.Skip("Skipping Redis auto-expiration test in non-Linux CI environment")
		} else {
			t.Skip("Skipping Redis expiration test. Set REDIS_TEST_ADDR to run.")
		}
	}

	// Get Redis connection details from environment
	redisAddr := os.Getenv("REDIS_TEST_ADDR")
	redisPassword := os.Getenv("REDIS_TEST_PASSWORD") // Can be empty
	
	// Create a new store
	store, err := NewRedisStore(redisAddr, redisPassword, 0)
	require.NoError(t, err)
	defer store.Close()

	// Clear existing data before running tests
	err = store.Clear()
	require.NoError(t, err)

	ctx := context.Background()
	
	// Store with very short TTL (100ms)
	err = store.StoreWithTTL(ctx, "expire-key", "expire-value", 100*time.Millisecond)
	require.NoError(t, err)
	
	// Verify value exists
	val, err := store.Retrieve("expire-key")
	require.NoError(t, err)
	require.Equal(t, "expire-value", val)
	
	// Wait for expiration
	time.Sleep(200 * time.Millisecond)
	
	// Value should be gone (auto-expired by Redis)
	_, err = store.Retrieve("expire-key")
	require.Error(t, err, "Key should be automatically expired")
	
	// CleanExpired should be a no-op with no error
	count, err := store.CleanExpired(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(0), count)
} 