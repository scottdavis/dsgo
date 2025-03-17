package memory

import (
	"context"

	"github.com/scottdavis/dsgo/pkg/agents"
)

// ListMemory extends the base Memory interface with list operations
type ListMemory interface {
	agents.Memory

	// List operations
	PushList(key string, value interface{}, opts ...agents.StoreOption) error
	PopList(key string) (interface{}, error)
	RemoveFromList(key string, value interface{}) error
	ListLength(key string) (int, error)
	ListItems(key string, start, end int) ([]interface{}, error)
}

// ExtendedMemoryStore wraps another Memory store and adds list operations
type ExtendedMemoryStore struct {
	agents.Memory

	// Store implementation specific to the extended operations
	// This is implemented by the concrete store (e.g., RedisStore)
	listOperations ListMemory
}

// NewExtendedMemoryStore creates a new extended memory store
func NewExtendedMemoryStore(store ListMemory) *ExtendedMemoryStore {
	return &ExtendedMemoryStore{
		Memory:         store,
		listOperations: store,
	}
}

// PushList adds an item to a list
func (s *ExtendedMemoryStore) PushList(key string, value interface{}, opts ...agents.StoreOption) error {
	return s.listOperations.PushList(key, value, opts...)
}

// PopList removes and returns the first item from a list
func (s *ExtendedMemoryStore) PopList(key string) (interface{}, error) {
	return s.listOperations.PopList(key)
}

// RemoveFromList removes an item from a list
func (s *ExtendedMemoryStore) RemoveFromList(key string, value interface{}) error {
	return s.listOperations.RemoveFromList(key, value)
}

// ListLength returns the length of a list
func (s *ExtendedMemoryStore) ListLength(key string) (int, error) {
	return s.listOperations.ListLength(key)
}

// ListItems returns items from a list in the given range
func (s *ExtendedMemoryStore) ListItems(key string, start, end int) ([]interface{}, error) {
	return s.listOperations.ListItems(key, start, end)
}

// Store is an alias for Memory.Store to simplify usage
func (s *ExtendedMemoryStore) Store(key string, value interface{}, opts ...agents.StoreOption) error {
	return s.Memory.Store(key, value, opts...)
}

// Retrieve is an alias for Memory.Retrieve to simplify usage
func (s *ExtendedMemoryStore) Retrieve(key string) (interface{}, error) {
	return s.Memory.Retrieve(key)
}

// List is an alias for Memory.List to simplify usage
func (s *ExtendedMemoryStore) List() ([]string, error) {
	return s.Memory.List()
}

// Clear is an alias for Memory.Clear to simplify usage
func (s *ExtendedMemoryStore) Clear() error {
	return s.Memory.Clear()
}

// CleanExpired is an alias for Memory.CleanExpired to simplify usage
func (s *ExtendedMemoryStore) CleanExpired(ctx context.Context) (int64, error) {
	return s.Memory.CleanExpired(ctx)
}

// Close is an alias for Memory.Close to simplify usage
func (s *ExtendedMemoryStore) Close() error {
	return s.Memory.Close()
} 