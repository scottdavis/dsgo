package memory

import (
	"context"
	"fmt"

	"github.com/scottdavis/dsgo/pkg/agents"
)

// memoryContextKey is a context key for memory access (private)
type memoryContextKey struct{}

// WithMemoryStore adds a memory store to the context
func WithMemoryStore(ctx context.Context, store agents.Memory) context.Context {
	return context.WithValue(ctx, memoryContextKey{}, store)
}

// GetMemoryStore retrieves the memory store from the context
func GetMemoryStore(ctx context.Context) (agents.Memory, error) {
	memory, ok := ctx.Value(memoryContextKey{}).(agents.Memory)
	if !ok {
		return nil, fmt.Errorf("memory store not found in context")
	}
	return memory, nil
}

// WithRedisStore adds a Redis memory store to the context
// This is an alias for WithMemoryStore for clarity when using Redis specifically
func WithRedisStore(ctx context.Context, store *RedisStore) context.Context {
	return WithMemoryStore(ctx, store)
}

// GetRedisStore retrieves the Redis memory store from the context
func GetRedisStore(ctx context.Context) (*RedisStore, error) {
	memory, err := GetMemoryStore(ctx)
	if err != nil {
		return nil, err
	}
	
	redisStore, ok := memory.(*RedisStore)
	if !ok {
		return nil, fmt.Errorf("memory store in context is not a Redis store")
	}
	
	return redisStore, nil
} 