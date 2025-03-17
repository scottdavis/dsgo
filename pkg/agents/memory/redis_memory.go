package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/scottdavis/dsgo/pkg/agents"
	"github.com/scottdavis/dsgo/pkg/errors"
)

// RedisStore implements the Memory interface using Redis as the backend.
type RedisStore struct {
	client *redis.Client
}

// Ensure RedisStore implements Memory interface
var _ Memory = (*RedisStore)(nil)

// NewRedisStore creates a new Redis-backed memory store.
func NewRedisStore(addr, password string, db int) (*RedisStore, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	// Verify connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, errors.WithFields(
			errors.Wrap(err, errors.Unknown, "failed to connect to Redis"),
			errors.Fields{"addr": addr},
		)
	}

	return &RedisStore{
		client: client,
	}, nil
}

// Store implements the Memory interface Store method.
func (r *RedisStore) Store(key string, value any, opts ...agents.StoreOption) error {
	// Process options
	options := &agents.StoreOptions{}
	for _, opt := range opts {
		opt(options)
	}

	jsonValue, err := json.Marshal(value)
	if err != nil {
		return errors.WithFields(
			errors.Wrap(err, errors.InvalidInput, "failed to marshal value to JSON"),
			errors.Fields{
				"key":        key,
				"value_type": fmt.Sprintf("%T", value),
			},
		)
	}

	ctx := context.Background()
	// Use TTL if provided in options
	if options.TTL > 0 {
		if err := r.client.Set(ctx, key, jsonValue, options.TTL).Err(); err != nil {
			return errors.WithFields(
				errors.Wrap(err, errors.Unknown, "failed to store value with TTL in Redis"),
				errors.Fields{"key": key, "ttl": options.TTL},
			)
		}
	} else {
		if err := r.client.Set(ctx, key, jsonValue, 0).Err(); err != nil {
			return errors.WithFields(
				errors.Wrap(err, errors.Unknown, "failed to store value in Redis"),
				errors.Fields{"key": key},
			)
		}
	}

	return nil
}

// Retrieve implements the Memory interface Retrieve method.
func (r *RedisStore) Retrieve(key string) (any, error) {
	ctx := context.Background()
	jsonValue, err := r.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return nil, errors.WithFields(
			errors.New(errors.ResourceNotFound, "key not found"),
			errors.Fields{"key": key},
		)
	}
	if err != nil {
		return nil, errors.WithFields(
			errors.Wrap(err, errors.Unknown, "failed to retrieve value from Redis"),
			errors.Fields{"key": key},
		)
	}

	var value any
	if err := json.Unmarshal([]byte(jsonValue), &value); err != nil {
		return nil, errors.WithFields(
			errors.Wrap(err, errors.InvalidResponse, "failed to unmarshal value from JSON"),
			errors.Fields{"key": key},
		)
	}

	// Handle conversion of types similar to SQLiteStore
	switch v := value.(type) {
	case map[string]any:
		// Check if all values are numbers for map[string]int
		allInts := true
		for _, val := range v {
			if _, ok := val.(float64); !ok {
				allInts = false
				break
			}
		}
		if allInts {
			intMap := make(map[string]int)
			for key, val := range v {
				intMap[key] = int(val.(float64))
			}
			return intMap, nil
		}
	case []any:
		// Check if it's a string array
		allStrings := true
		for _, item := range v {
			if _, ok := item.(string); !ok {
				allStrings = false
				break
			}
		}
		if allStrings {
			strArr := make([]string, len(v))
			for i, item := range v {
				strArr[i] = item.(string)
			}
			return strArr, nil
		}
	case float64:
		// Convert to int if it's a whole number
		if v == float64(int(v)) {
			return int(v), nil
		}
	}

	return value, nil
}

// List implements the Memory interface List method.
func (r *RedisStore) List() ([]string, error) {
	ctx := context.Background()
	keys, err := r.client.Keys(ctx, "*").Result()
	if err != nil {
		return nil, errors.Wrap(err, errors.Unknown, "failed to list keys from Redis")
	}
	return keys, nil
}

// Clear implements the Memory interface Clear method.
func (r *RedisStore) Clear() error {
	ctx := context.Background()
	if err := r.client.FlushAll(ctx).Err(); err != nil {
		return errors.Wrap(err, errors.Unknown, "failed to clear Redis store")
	}
	return nil
}

// StoreWithTTL implements the Memory interface StoreWithTTL method.
func (r *RedisStore) StoreWithTTL(ctx context.Context, key string, value any, ttl time.Duration) error {
	jsonValue, err := json.Marshal(value)
	if err != nil {
		return errors.WithFields(
			errors.Wrap(err, errors.InvalidInput, "failed to marshal value to JSON"),
			errors.Fields{"key": key},
		)
	}

	if err := r.client.Set(ctx, key, jsonValue, ttl).Err(); err != nil {
		return errors.WithFields(
			errors.Wrap(err, errors.Unknown, "failed to store value with TTL in Redis"),
			errors.Fields{"key": key, "ttl": ttl},
		)
	}

	return nil
}

// CleanExpired implements the Memory interface CleanExpired method.
// Note: Redis automatically removes expired keys, so this is a no-op.
func (r *RedisStore) CleanExpired(ctx context.Context) (int64, error) {
	// Redis automatically expires keys, so this is a no-op
	return 0, nil
}

// Close implements the Memory interface Close method.
func (r *RedisStore) Close() error {
	if err := r.client.Close(); err != nil {
		return errors.Wrap(err, errors.Unknown, "failed to close Redis connection")
	}
	return nil
} 