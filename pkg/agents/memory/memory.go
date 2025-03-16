package memory

import (
	"context"
	"time"
)

// Memory defines the interface for key-value storage backends.
type Memory interface {
	// Store saves a value with the specified key.
	Store(key string, value any) error

	// Retrieve gets a value by its key.
	Retrieve(key string) (any, error)

	// List returns all keys in the store.
	List() ([]string, error)

	// Clear removes all values from the store.
	Clear() error

	// StoreWithTTL stores a value with a specified time-to-live duration.
	StoreWithTTL(ctx context.Context, key string, value any, ttl time.Duration) error

	// CleanExpired removes all expired entries from the store.
	CleanExpired(ctx context.Context) (int64, error)

	// Close releases resources used by the memory store.
	Close() error
} 