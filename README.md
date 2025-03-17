# DSGo
[![Go Report Card](https://goreportcard.com/badge/github.com/scottdavis/dsgo)](https://goreportcard.com/report/github.com/scottdavis/dsgo)
[![Go Reference](https://pkg.go.dev/badge/github.com/scottdavis/dsgo)](https://pkg.go.dev/github.com/scottdavis/dsgo)

**This is mostly for my personal use, so support is not guaranteed.**

A Go implementation of the Declarative Self-improving programming (DSPy) pattern.

## Acknowledgments

I would like to express my sincere gratitude to Xiao Constantine ([@XiaoConstantine](https://github.com/XiaoConstantine)), the original author of dspy-go. This project builds upon his excellent foundation and pioneering work in bringing the DSPy pattern to the Go ecosystem.

### Recent Improvements

This fork includes several enhancements to the original project:

- **Advanced LLM Configuration**: Added more flexible ways to configure Ollama with custom hosts via multiple format options
- **Ability to inject llm instances**: into functions so you cna use different llms for different parts/steps of your program
- **OpenRouter Integration**: Full support for [OpenRouter](https://openrouter.ai) as an LLM provider, enabling access to hundreds of AI models through a single interface
- **Interface for Memory**: Added an interface for memory, allowing you to use different memory backends, including Redis



DSGo is a Go implementation of DSPy, bringing systematic prompt engineering and automated reasoning capabilities to Go applications. It provides a flexible framework for building reliable and effective Language Model (LLM) applications through composable modules and workflows.


### Installation
```go
go get github.com/scottdavis/dsgo
```

### Quick Start

Here's a simple example to get you started with DSGo:

```go
import (
    "context"
    "fmt"
    "log"

    "github.com/scottdavis/dsgo/pkg/core"
    "github.com/scottdavis/dsgo/pkg/llms"
    "github.com/scottdavis/dsgo/pkg/modules"
    "github.com/scottdavis/dsgo/pkg/config"
)

func main() {
    // Configure the default LLM
    llms.EnsureFactory()
    
    // Create an LLM instance (example with Anthropic)
    anthropicLLM, err := llms.NewLLM("your-api-key", llms.NewAnthropicConfig(core.ModelAnthropicSonnet))
    if err != nil {
        log.Fatalf("Failed to configure LLM: %v", err)
    }
    
    // Create a DSPYConfig with the LLM (proper dependency injection)
    dspyConfig := core.NewDSPYConfig().WithDefaultLLM(anthropicLLM)

    // Create a signature for question answering
    signature := core.NewSignature(
        []core.InputField{{Field: core.Field{Name: "question"}}},
        []core.OutputField{{Field: core.Field{Name: "answer"}}},
    )

    // Create a ChainOfThought module with the config containing the LLM
    cot := modules.NewChainOfThought(signature, dspyConfig)

    // Create a program
    program := core.NewProgram(
        map[string]core.Module{"cot": cot},
        func(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
            return cot.Process(ctx, inputs)
        },
        dspyConfig,
    )

    // Execute the program
    result, err := program.Execute(context.Background(), map[string]interface{}{
        "question": "What is the capital of France?",
    })
    if err != nil {
        log.Fatalf("Error executing program: %v", err)
    }

    fmt.Printf("Answer: %s\n", result["answer"])
}
```

### Core Concepts

#### Signatures
Signatures define the input and output fields for modules. They help in creating type-safe and well-defined interfaces for your AI components.

#### Modules
Modules are the building blocks of DSGo programs. They encapsulate specific functionalities and can be composed to create complex pipelines. Some key modules include:

* Predict: Basic prediction module
* ChainOfThought: Implements chain-of-thought reasoning
* ReAct: Implements the ReAct (Reasoning and Acting) paradigm


#### Optimizers
Optimizers help improve the performance of your DSGo programs by automatically tuning prompts and module parameters. Including:
* BootstrapFewShot: Automatic few-shot example selection
* MIPRO: Multi-step interactive prompt optimization
* Copro: Collaborative prompt optimization


#### Agents
Use dspy's core concepts as building blocks, impl [Building Effective Agents](https://github.com/anthropics/anthropic-cookbook/tree/main/patterns/agents)


* Chain Workflow: Sequential execution of steps
* Parallel Workflow: Concurrent execution with controlled parallelism
* Router Workflow: Dynamic routing based on classification
* Orchestrator: Flexible task decomposition and execution

See [agent examples](/examples/agents/main.go)


```go
// Chain
workflow := workflows.NewChainWorkflow(store)
workflow.AddStep(&workflows.Step{
    ID: "step1",
    Module: modules.NewPredict(signature1),
})
workflow.AddStep(&workflows.Step{
    ID: "step2", 
    Module: modules.NewPredict(signature2),
})
```
Each workflow step can be configured with:
* Retry logic with exponential backoff
* Conditional execution based on workflow state
* Custom error handling

```go

step := &workflows.Step{
    ID: "retry_example",
    Module: myModule,
    RetryConfig: &workflows.RetryConfig{
        MaxAttempts: 3,
        BackoffMultiplier: 2.0,
    },
    Condition: func(state map[string]interface{}) bool {
        return someCondition(state)
    },
}
```

### Advanced Features

#### Tracing and Logging
```go
// Enable detailed tracing
ctx = core.WithExecutionState(context.Background())

// Configure logging
logger := logging.NewLogger(logging.Config{
    Severity: logging.DEBUG,
    Outputs:  []logging.Output{logging.NewConsoleOutput(true)},
})
logging.SetLogger(logger)
```

#### Custom Tools
You can extend ReAct modules with custom tools:
```go

func (t *CustomTool) CanHandle(action string) bool {
    return strings.HasPrefix(action, "custom_")
}

func (t *CustomTool) Execute(ctx context.Context, action string) (string, error) {
    // Implement tool logic
    return "Tool result", nil
}
```

#### Working with Different LLM Providers
```go
// Using Anthropic Claude
anthropicConfig := llms.NewAnthropicConfig(llms.AnthropicModelSonnet)
llm, _ := llms.NewLLM("api-key", anthropicConfig)

// Using Ollama (various options)
// Option 1: Using NewLLM with OllamaConfig
ollamaConfig := llms.NewOllamaConfig("http://localhost:11434", "llama2")
llm, _ := llms.NewLLM("", ollamaConfig)

// Option 2: Using OpenRouter
openRouterConfig := llms.NewOpenRouterConfig(llms.OpenRouterModelNeevaAI)
llm, _ := llms.NewLLM("api-key", openRouterConfig)

// Option 3: Using LlamaCPP
llm, _ := llms.NewLlamacppLLM("http://localhost:8080")

// Option 4: Using string format with custom host
llm, _ := llms.NewLLM("", "ollama:example.com:11434:llama2")
// or with protocol
llm, _ := llms.NewLLM("", "ollama:http://example.com:11434:llama2")

// Create DSPYConfig with the LLM
dspyConfig := core.NewDSPYConfig().WithDefaultLLM(llm)
```


### Examples
Check the examples directory for complete implementations:

* examples/agents: Demonstrates different agent patterns
* examples/hotpotqa: Question-answering implementation
* examples/gsm8k: Math problem solving


### License
DSGo is released under the MIT License. See the LICENSE file for details.

# Redis Memory Store Example

This example demonstrates how to use Redis as a memory store for DSP-GO applications. The example shows a practical workflow where data is downloaded from an external source, stored in Redis, and then processed in subsequent steps.

## Features Demonstrated

1. **Redis Connection**: Setting up and connecting to a Redis server
2. **Data Storage**: Storing data in Redis between processing steps
3. **Data Retrieval**: Retrieving and processing the stored data
4. **TTL (Time-to-Live)**: Using TTL functionality for automatic expiration of data
5. **Cleanup**: Properly cleaning up Redis resources

## Running the Example

### Prerequisites

- Redis server running locally or accessible remotely
- Go 1.18 or later

### Start Redis

If you don't have Redis running, you can quickly start it with Docker:

```bash
docker run -d --name redis-example -p 6379:6379 redis:latest
```

### Run the Example

```bash
go run main.go
```

By default, it connects to Redis at `localhost:6379`. You can specify a different Redis server:

```bash
go run main.go --redis-addr=your-redis-server:6379 --redis-password=yourpassword
```

You can also specify a different URL to download:

```bash
go run main.go --url=https://raw.githubusercontent.com/your-repo/your-file.txt
```

## How It Works

The example performs these steps:

1. **Connect to Redis**: Establishes a connection to the Redis server
2. **Download Content**: Fetches content from a specified URL
3. **Store in Redis**: Stores the downloaded content in Redis
4. **Process Content**: Retrieves the content from Redis and processes it (counts lines and words)
5. **Demonstrate TTL**: Shows how to use TTL functionality to automatically expire data
6. **Clean Up**: Properly cleans up Redis resources

## Code Structure

- `main.go`: The main application code demonstrating Redis usage
- `main_test.go`: Tests for the Redis functionality

## Redis Memory Options in DSP-GO

DSP-GO provides several memory store implementations including:

- **Redis**: Used in this example, provides persistent storage with TTL support
- **In-Memory**: Simple in-memory storage
- **SQLite**: Local disk-based storage

Redis is particularly useful for:
- High-performance persistence between application restarts
- Distributed applications needing shared memory
- Scenarios requiring automatic data expiration
