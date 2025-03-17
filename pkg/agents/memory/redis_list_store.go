package memory

import (
	"context"
	"encoding/json"

	"github.com/redis/go-redis/v9"
	"github.com/scottdavis/dsgo/pkg/agents"
	"github.com/scottdavis/dsgo/pkg/errors"
)

// RedisListStore extends RedisStore with list operations
type RedisListStore struct {
	RedisStore
}

// NewRedisListStore creates a new Redis-backed memory store with list operations
func NewRedisListStore(addr, password string, db int) (*RedisListStore, error) {
	redisStore, err := NewRedisStore(addr, password, db)
	if err != nil {
		return nil, err
	}
	
	return &RedisListStore{
		RedisStore: *redisStore,
	}, nil
}

// Ensure RedisListStore implements ListMemory interface
var _ ListMemory = (*RedisListStore)(nil)

// PushList adds an item to a list
func (s *RedisListStore) PushList(key string, value interface{}, opts ...agents.StoreOption) error {
	// Apply options
	options := &agents.StoreOptions{}
	for _, opt := range opts {
		opt(options)
	}

	ctx := context.Background()
	
	// Convert value to bytes
	valueBytes, err := json.Marshal(value)
	if err != nil {
		return errors.WithFields(
			errors.Wrap(err, errors.InvalidInput, "failed to marshal value to JSON"),
			errors.Fields{"key": key},
		)
	}
	
	// Push to list
	err = s.client.RPush(ctx, key, valueBytes).Err()
	if err != nil {
		return errors.WithFields(
			errors.Wrap(err, errors.Unknown, "failed to push to list in Redis"),
			errors.Fields{"key": key},
		)
	}
	
	// Set expiry if provided
	if options.TTL > 0 {
		err = s.client.Expire(ctx, key, options.TTL).Err()
		if err != nil {
			return errors.WithFields(
				errors.Wrap(err, errors.Unknown, "failed to set expiry on list in Redis"),
				errors.Fields{"key": key, "ttl": options.TTL},
			)
		}
	}
	
	return nil
}

// PopList removes and returns the first item from a list
func (s *RedisListStore) PopList(key string) (interface{}, error) {
	ctx := context.Background()
	
	// Pop from list
	valueBytes, err := s.client.LPop(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, errors.WithFields(
			errors.Wrap(err, errors.Unknown, "failed to pop from list in Redis"),
			errors.Fields{"key": key},
		)
	}
	
	// Unmarshal value
	var value interface{}
	err = json.Unmarshal(valueBytes, &value)
	if err != nil {
		return nil, errors.WithFields(
			errors.Wrap(err, errors.InvalidResponse, "failed to unmarshal value from JSON"),
			errors.Fields{"key": key},
		)
	}
	
	return value, nil
}

// RemoveFromList removes an item from a list
func (s *RedisListStore) RemoveFromList(key string, value interface{}) error {
	ctx := context.Background()
	
	// Convert value to bytes
	valueBytes, err := json.Marshal(value)
	if err != nil {
		return errors.WithFields(
			errors.Wrap(err, errors.InvalidInput, "failed to marshal value to JSON"),
			errors.Fields{"key": key},
		)
	}
	
	// Remove from list
	err = s.client.LRem(ctx, key, 0, valueBytes).Err()
	if err == redis.Nil {
		return nil
	}
	if err != nil {
		return errors.WithFields(
			errors.Wrap(err, errors.Unknown, "failed to remove from list in Redis"),
			errors.Fields{"key": key},
		)
	}
	
	return nil
}

// ListLength returns the length of a list
func (s *RedisListStore) ListLength(key string) (int, error) {
	ctx := context.Background()
	
	// Get list length
	length, err := s.client.LLen(ctx, key).Result()
	if err == redis.Nil {
		return 0, nil
	}
	if err != nil {
		return 0, errors.WithFields(
			errors.Wrap(err, errors.Unknown, "failed to get list length from Redis"),
			errors.Fields{"key": key},
		)
	}
	
	return int(length), nil
}

// ListItems returns items from a list in the given range
func (s *RedisListStore) ListItems(key string, start, end int) ([]interface{}, error) {
	ctx := context.Background()
	
	// Get list items
	values, err := s.client.LRange(ctx, key, int64(start), int64(end)).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, errors.WithFields(
			errors.Wrap(err, errors.Unknown, "failed to get list items from Redis"),
			errors.Fields{"key": key, "start": start, "end": end},
		)
	}
	
	// Unmarshal each value
	result := make([]interface{}, len(values))
	for i, valueStr := range values {
		var value interface{}
		err = json.Unmarshal([]byte(valueStr), &value)
		if err != nil {
			return nil, errors.WithFields(
				errors.Wrap(err, errors.InvalidResponse, "failed to unmarshal value from JSON"),
				errors.Fields{"key": key, "index": i},
			)
		}
		result[i] = value
	}
	
	return result, nil
} 