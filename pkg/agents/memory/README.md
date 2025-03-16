# Memory Interface

This package provides a common interface for key-value storage backends, with implementations for SQLite and Redis.

## Usage

```go
import "github.com/scottdavis/dsgo/pkg/agents/memory"

// Create a SQLite-backed memory store (in-memory or file-based)
sqliteStore, err := memory.NewSQLiteStore(":memory:")
if err != nil {
    return err
}
defer sqliteStore.Close()

// Create a Redis-backed memory store
redisStore, err := memory.NewRedisStore("localhost:6379", "", 0)
if err != nil {
    return err
}
defer redisStore.Close()

// Store values
err = sqliteStore.Store("key", "value")
if err != nil {
    return err
}

// Retrieve values
value, err := sqliteStore.Retrieve("key")
if err != nil {
    return err
}

// Store with TTL (time-to-live)
ctx := context.Background()
err = redisStore.StoreWithTTL(ctx, "key", "value", 5*time.Minute)
if err != nil {
    return err
}
```

## Testing

### Local Testing

SQLite tests run in all environments. Redis tests are skipped by default in local environments.

To run Redis tests locally, you need to:

1. Start a Redis server (e.g., using Docker)
2. Set the `REDIS_TEST_ADDR` environment variable to point to your Redis server
3. Optionally set `REDIS_TEST_PASSWORD` if your Redis server requires authentication

```bash
# Start Redis with Docker
docker run -d --name redis-test -p 6379:6379 redis:latest

# Run tests with Redis
REDIS_TEST_ADDR=localhost:6379 go test -v ./pkg/agents/memory/...
```

### CI Setup

For running tests in CI, you need to add Redis to your CI pipeline. Here's an example setup for GitHub Actions:

```yaml
# .github/workflows/test.yml
name: Tests

on:
  push:
    branches: [ main ]
  pull_request:
    branches: [ main ]

jobs:
  test:
    runs-on: ubuntu-latest
    
    services:
      redis:
        image: redis:latest
        ports:
          - 6379:6379
        options: >-
          --health-cmd "redis-cli ping"
          --health-interval 10s
          --health-timeout 5s
          --health-retries 5
    
    steps:
    - uses: actions/checkout@v3
    
    - name: Set up Go
      uses: actions/setup-go@v3
      with:
        go-version: '1.21'
    
    - name: Install dependencies
      run: go mod download
    
    - name: Run tests
      run: go test -v ./...
      env:
        REDIS_TEST_ADDR: localhost:6379
```

For other CI systems (like GitLab CI, Circle CI, etc.), the approach is similar - you need to:

1. Add a Redis service to your CI configuration
2. Set the `REDIS_TEST_ADDR` environment variable to point to the Redis service
3. Optionally set `REDIS_TEST_PASSWORD` if needed 