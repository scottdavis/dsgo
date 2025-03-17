# DSP-GO Redis Integration Example

This example demonstrates how to integrate Redis storage with DSP-GO workflows and modules, implementing a practical NLP workflow with proper dependency injection.

## Prerequisites

- Go 1.19 or higher
- Docker (optional, for running Redis locally)
- Ollama (for running the LLM locally)

## Running the Example

### 1. Start a Redis Server

If you don't have a Redis server already running, you can start one with Docker:

```bash
docker run -d --name redis-example -p 6379:6379 redis:latest
```

### 2. Start Ollama with llama3

This example uses Ollama with the llama3 model. Make sure Ollama is running and the llama3 model is available:

```bash
# Install Ollama if you haven't already
# Pull the llama3 model
ollama pull llama3
# Start the Ollama server
ollama serve
```

### 3. Run the Example

```bash
go run main.go
```

You can also specify custom Redis connection parameters and a different URL to process:

```bash
go run main.go --redis-addr=my-redis:6379 --redis-password=mypassword --redis-db=1 --url=https://example.com/myfile.txt
```

## What This Example Demonstrates

This example showcases a practical NLP workflow using Redis as a memory store:

1. **Dependency Injection**: Proper LLM injection into modules without global state
2. **Custom DSP-GO Modules**: Building custom modules for summarization and translation
3. **Redis as Workflow Memory**: Using Redis to store intermediate results
4. **TTL Functionality**: Demonstrating time-to-live for temporary data
5. **Practical Workflow**: A real-world workflow that downloads content, summarizes it, and translates it
6. **Proper Error Handling**: Comprehensive error handling throughout the workflow

## Key DSP-GO Concepts Used

- **Workflows**: ChainWorkflow for sequential processing
- **Custom Modules**: SummaryModule and TranslationModule implementing the core.Module interface
- **Memory Store**: Redis implementation of the Memory interface
- **Context Helpers**: Using `memory.WithMemoryStore` and `memory.GetMemoryStore` for dependency injection
- **LLM Integration**: Proper injection of LLM into modules
- **TTL**: Time-to-live functionality for temporary data

## Workflow Steps

1. **Download**: Downloads content from a specified URL
2. **Summarize**: Summarizes the downloaded content using an LLM
3. **Translate**: Translates the summary to Spanish using an LLM
4. **TTL Demo**: Demonstrates Redis TTL functionality with temporary data

## Testing

The example includes comprehensive tests:

```bash
# Run the tests
go test -v
```

The tests include:
- Unit tests for the custom modules
- Integration tests for the workflow
- Tests for TTL functionality
- Mocking of the LLM interface for reliable testing

## Cleanup

When you're done, you can stop and remove the Redis container:

```bash
docker stop redis-example
docker rm redis-example
``` 