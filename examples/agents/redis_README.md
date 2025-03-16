# Redis Storage Example for DSP-GO Agents

This example demonstrates how to use Redis as a persistent storage backend for DSP-GO agents, enabling stateful workflows with durable data storage.

## Features

- Persistent storage across application restarts
- TTL support for expiring entries
- Basic CRUD operations (Create, Read, Update, Delete)
- Fast, scalable in-memory storage with optional persistence to disk

## Prerequisites

- Go 1.19 or later
- Redis server (local or remote)

## Running the Example

1. Start a Redis server if you don't have one running:

```bash
# Using Docker
docker run -d --name redis-test -p 6379:6379 redis:latest
```

2. Run the example:

```bash
# Run with default settings (localhost:6379)
go run examples/agents/redis_example.go

# Or specify Redis connection details
go run examples/agents/redis_example.go --redis-addr=my-redis:6379 --redis-password=mypassword --redis-db=1
```

## Example Workflow

This example demonstrates a knowledge base agent with the following operations:

1. **Store**: Saves information to the Redis store with a specific key
2. **Retrieve**: Gets information from the store by key
3. **List**: Lists all keys in the store
4. **Store with TTL**: Stores information that will automatically expire after a specified time

## Key Components

- `RedisStore`: Implements the `Memory` interface for Redis storage
- `KnowledgeBaseModule`: A custom module that processes operations on the knowledge base
- `ChainWorkflow`: A workflow that executes operations in sequence

## Implementation Details

The example demonstrates how to:

1. Initialize and connect to a Redis store
2. Create a workflow with Redis-backed persistence
3. Store and retrieve data from the Redis store
4. Use Redis's built-in TTL feature for expiring data
5. Handle Redis-specific error conditions

## Usage in Your Own Code

To use Redis storage in your own code:

```go
import (
    "github.com/scottdavis/dsgo/pkg/agents/memory"
    "github.com/scottdavis/dsgo/pkg/agents/workflows"
)

// Create a Redis store
redisStore, err := memory.NewRedisStore("localhost:6379", "", 0)
if err != nil {
    return err
}
defer redisStore.Close()

// Use it with any workflow
workflow := workflows.NewChainWorkflow(redisStore)

// Add steps and execute as normal
// ...
```

## Advanced Usage

For production usage, consider:

1. Setting up Redis with authentication
2. Configuring Redis persistence settings (RDB or AOF)
3. Using Redis replication for high availability
4. Setting appropriate TTL values for different types of data
5. Implementing retries for Redis operations in mission-critical systems 