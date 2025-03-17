package memory

import (
	"context"
	"time"

	"github.com/scottdavis/dsgo/pkg/agents"
)

// StoreOption defines options for Store operations
type StoreOption func(*StoreOptions)

// StoreOptions contains configuration for Store operations
type StoreOptions struct {
	TTL time.Duration
}

// WithTTL creates an option to set a TTL for a stored value
func WithTTL(ttl time.Duration) StoreOption {
	return func(options *StoreOptions) {
		options.TTL = ttl
	}
}

// Memory defines the interface for key-value storage backends.
type Memory interface {
	// Store saves a value with the specified key.
	// Optional StoreOption parameters can be provided to configure the storage behavior,
	// such as TTL (time-to-live).
	Store(key string, value any, opts ...agents.StoreOption) error

	// Retrieve gets a value by its key.
	Retrieve(key string) (any, error)

	// List returns all keys in the store.
	List() ([]string, error)

	// Clear removes all values from the store.
	Clear() error

	// CleanExpired removes all expired entries from the store.
	CleanExpired(ctx context.Context) (int64, error)

	// Close releases resources used by the memory store.
	Close() error
} 