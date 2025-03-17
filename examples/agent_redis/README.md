# Redis Agent Example

This example demonstrates how to use Redis as a storage backend for DSPy-Go agent workflows, providing persistent memory with time-to-live (TTL) capabilities.

## Features

- Persistent memory storage across runs
- TTL (Time-To-Live) support for automatic expiration of stored data
- Knowledge base operations (store, retrieve, list)
- Clean API for integrating with agent workflows

## Prerequisites

- Go 1.21 or higher
- Redis server running locally or accessible remotely
- An LLM service (Ollama or OpenAI)

## Running the Example

### 1. Start Redis Server

You can start a Redis server using Docker:

```bash
docker run --name redis-dspy -p 6379:6379 -d redis
```

### 2. Run the Example

```bash
go run examples/agent_redis/main.go [options]
```

Available options:

- `--redis-addr`: Redis server address (default: "localhost:6379")
- `--redis-password`: Redis password (default: "")
- `--redis-db`: Redis database number (default: 0)
- `--model-host`: LLM model host (default: "http://localhost:11434")
- `--model-name`: LLM model name (default: "llama3")

## Example Workflow

The example demonstrates:

1. Storing multiple pieces of information in Redis
2. Storing data with a TTL (time-to-live)
3. Retrieving data from Redis
4. Listing all keys in the knowledge base
5. Waiting for TTL expiration and verifying data is removed

## Makefile Targets

For convenience, the following Makefile targets are available:

```bash
# Start a Redis container for the example
make redis-start

# Run the Redis agent example
make run-redis-example

# Clean up Redis container when done
make redis-stop
``` 