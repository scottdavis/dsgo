# Distributed Worker Example

This example demonstrates how to set up a distributed worker system using Docker containers. It shows how to run multiple workers with a supervisor that monitors for stalled workflows.

## Architecture

The distributed worker system consists of:

1. **Redis** - Used as a queue and state store
2. **Workers** - Process tasks from the queue
3. **Supervisor** - Monitors workers and recovers stalled workflows

## Prerequisites

- Docker and Docker Compose
- Go 1.21 or later (for local development)

## Running the Example

### Using Docker Compose

The easiest way to run the example is using Docker Compose:

```bash
# Build and start all containers
docker-compose up --build

# Run in detached mode
docker-compose up -d --build

# View logs
docker-compose logs -f

# Stop all containers
docker-compose down
```

### Configuration

The system is configured via environment variables in the docker-compose.yml file:

| Variable | Description | Default |
|----------|-------------|---------|
| REDIS_ADDR | Redis server address | redis:6379 |
| ROLE | Role (worker or supervisor) | worker |
| WORKER_ID | Unique worker ID | worker1 |
| QUEUE_NAME | Name of the queue to process | default |
| POLL_INTERVAL | How often to poll for new jobs | 1s |
| CONCURRENCY | Number of jobs to process concurrently | 5 |
| WORKERS_TO_MONITOR | Comma-separated list of worker IDs to monitor | worker1,worker2 |
| STALLED_WORKFLOW_THRESHOLD | How long a workflow can be inactive before considered stalled | 30s |

## Running Locally

You can also run the worker and supervisor locally:

```bash
# Start Redis
docker run -d --name redis-test -p 6379:6379 redis:latest

# Run a worker
go run main.go --role=worker --worker-id=worker1 --redis-addr=localhost:6379

# Run a supervisor
go run main.go --role=supervisor --redis-addr=localhost:6379 --workers-to-monitor=worker1
```

## Example Workflow

1. The supervisor starts and monitors workers
2. Workers connect to Redis and start polling for jobs
3. If a job stalls, the supervisor detects it and can:
   - Automatically recover the job (enabled by default)
   - Send it to a dead letter queue if recovery fails
   - Alert administrators
4. Workers process recovered jobs like normal jobs

## Pushing Jobs

To push a job to the queue, you can use the distributed worker API:

```go
worker, _ := workflows.NewDistributedWorker(&workflows.DistributedWorkerConfig{
    QueueStore:     redisStore,
    StateStore:     redisStore,
    QueueName:      "default",
    ModuleRegistry: registry,
})

// Push a job for the AdditionModule
job, err := worker.PushJob(ctx, &AdditionModule{}, map[string]any{
    "a": 5,
    "b": 10,
})
``` 