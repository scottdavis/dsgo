package agents

import (
	"context"
	"sync"
	"time"

	"github.com/scottdavis/dsgo/pkg/errors"
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

// Memory provides storage capabilities for agents.
type Memory interface {
	// Store saves a value with a given key and optional TTL settings
	Store(key string, value any, opts ...StoreOption) error

	// Retrieve gets a value by key
	Retrieve(key string) (any, error)

	// List returns all stored keys
	List() ([]string, error)

	// Clear removes all stored values
	Clear() error

	// CleanExpired removes all expired entries
	CleanExpired(ctx context.Context) (int64, error)

	// Close releases resources
	Close() error
}

// Simple in-memory implementation.
type InMemoryStore struct {
	data     map[string]any
	expiry   map[string]time.Time
	mu       sync.RWMutex
}

func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		data:   make(map[string]any),
		expiry: make(map[string]time.Time),
	}
}

func (s *InMemoryStore) Store(key string, value any, opts ...StoreOption) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	// Process options
	options := &StoreOptions{}
	for _, opt := range opts {
		opt(options)
	}
	
	s.data[key] = value
	
	// Set expiry if TTL is specified
	if options.TTL > 0 {
		s.expiry[key] = time.Now().Add(options.TTL)
	} else {
		// Remove any existing expiry for this key
		delete(s.expiry, key)
	}
	
	return nil
}

func (s *InMemoryStore) Retrieve(key string) (any, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Check if key is expired
	if expiry, hasExpiry := s.expiry[key]; hasExpiry && time.Now().After(expiry) {
		// Key is expired, remove it
		s.mu.RUnlock()
		s.mu.Lock()
		delete(s.data, key)
		delete(s.expiry, key)
		s.mu.Unlock()
		s.mu.RLock()
		
		return nil, errors.WithFields(
			errors.New(errors.ResourceNotFound, "key expired in memory store"),
			errors.Fields{
				"key":         key,
				"access_time": time.Now().UTC(),
				"expiry_time": expiry,
			})
	}

	value, exists := s.data[key]
	if !exists {
		return nil, errors.WithFields(
			errors.New(errors.ResourceNotFound, "key not found in memory store"),
			errors.Fields{
				"key":         key,
				"access_time": time.Now().UTC(),
			})
	}
	return value, nil
}

func (s *InMemoryStore) List() ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Clean expired keys first
	s.cleanExpiredNoLock()

	keys := make([]string, 0, len(s.data))
	for k := range s.data {
		keys = append(keys, k)
	}

	return keys, nil
}

func (s *InMemoryStore) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Create new maps rather than ranging and deleting
	s.data = make(map[string]any)
	s.expiry = make(map[string]time.Time)
	return nil
}

// CleanExpired removes expired entries and returns count of removed items
func (s *InMemoryStore) CleanExpired(ctx context.Context) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	return s.cleanExpiredNoLock(), nil
}

// cleanExpiredNoLock removes expired entries without locking
// Caller must hold the lock
func (s *InMemoryStore) cleanExpiredNoLock() int64 {
	var count int64
	now := time.Now()
	
	for key, expiry := range s.expiry {
		if now.After(expiry) {
			delete(s.data, key)
			delete(s.expiry, key)
			count++
		}
	}
	
	return count
}

// Close is a no-op for InMemoryStore
func (s *InMemoryStore) Close() error {
	return nil
}
