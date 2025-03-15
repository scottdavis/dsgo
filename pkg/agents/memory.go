package agents

import (
	"sync"
	"time"

	"github.com/XiaoConstantine/dspy-go/pkg/errors"
)

// Memory provides storage capabilities for agents.
type Memory interface {
	// Store saves a value with a given key
	Store(key string, value any) error

	// Retrieve gets a value by key
	Retrieve(key string) (any, error)

	// List returns all stored keys
	List() ([]string, error)

	// Clear removes all stored values
	Clear() error
}

// Simple in-memory implementation.
type InMemoryStore struct {
	data map[string]any
	mu   sync.RWMutex
}

func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		data: make(map[string]any),
	}
}

func (s *InMemoryStore) Store(key string, value any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = value
	return nil
}

func (s *InMemoryStore) Retrieve(key string) (any, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

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

	keys := make([]string, 0, len(s.data))
	for k := range s.data {
		keys = append(keys, k)
	}

	return keys, nil
}

func (s *InMemoryStore) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Create a new map rather than ranging and deleting
	// This is more efficient for clearing everything
	s.data = make(map[string]any)
	return nil
}
