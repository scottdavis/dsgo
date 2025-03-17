package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/scottdavis/dsgo/pkg/agents"
	"github.com/scottdavis/dsgo/pkg/agents/memory"
	"github.com/scottdavis/dsgo/pkg/agents/workflows"
	"github.com/scottdavis/dsgo/pkg/core"
	"github.com/scottdavis/dsgo/pkg/llms"
	"github.com/scottdavis/dsgo/pkg/logging"
	"github.com/scottdavis/dsgo/pkg/modules"
)

// Define memory context key if it's not available
var MemoryKey = &contextKey{"memory"}

// contextKey is a custom type for context keys to avoid collisions
type contextKey struct {
	name string
}

// TTLStore extends the agents.Memory interface with TTL support
type TTLStore interface {
	agents.Memory
	StoreWithTTL(ctx context.Context, key string, value any, ttl time.Duration) error
	CleanExpired(ctx context.Context) (int64, error)
}

// RunRedisAgentExample demonstrates using Redis as a storage backend for agent workflows
func RunRedisAgentExample(ctx context.Context, logger *logging.Logger, dspyConfig *core.DSPYConfig, redisConfig RedisConfig) {
	logger.Info(ctx, "============ Example: Knowledge Base with Redis Storage ==============")

	// Initialize Redis memory store
	redisStore, err := memory.NewRedisStore(redisConfig.Addr, redisConfig.Password, redisConfig.DB)
	if err != nil {
		logger.Error(ctx, "Failed to create Redis store: %v", err)
		return
	}
	defer redisStore.Close()

	// Clear existing data for clean demo
	if err := redisStore.Clear(); err != nil {
		logger.Error(ctx, "Failed to clear Redis store: %v", err)
		return
	}

	// Create a knowledge base workflow
	workflow, err := CreateKnowledgeBaseWorkflow(dspyConfig, redisStore)
	if err != nil {
		logger.Error(ctx, "Failed to create knowledge base workflow: %v", err)
		return
	}

	// First, store some information in the knowledge base
	storeResult, err := workflow.Execute(ctx, map[string]any{
		"operation": "store",
		"key":       "company_history",
		"content":   "Founded in 2020, our company specializes in AI solutions for healthcare and finance industries. We have grown from 5 to 100 employees in 3 years and have opened offices in San Francisco, New York, and London.",
	})
	if err != nil {
		logger.Error(ctx, "Failed to store information: %v", err)
		return
	}
	logger.Info(ctx, "Storage result: %s", storeResult["message"])

	// Store another piece of information
	_, err = workflow.Execute(ctx, map[string]any{
		"operation": "store",
		"key":       "product_roadmap",
		"content":   "Q3 2024: Launch medical imaging AI. Q4 2024: Release financial forecasting tool. Q1 2025: Expand to European markets with localized solutions.",
	})
	if err != nil {
		logger.Error(ctx, "Failed to store roadmap: %v", err)
		return
	}

	// Store information with TTL
	// Don't create a new context, use the existing one
	ttlCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	_, err = workflow.Execute(ttlCtx, map[string]any{
		"operation": "store_with_ttl",
		"key":       "temporary_notice",
		"content":   "System maintenance scheduled for tonight from 10 PM to 2 AM.",
		"ttl":       30, // 30 seconds TTL
	})
	if err != nil {
		logger.Error(ctx, "Failed to store temporary notice: %v", err)
		return
	}
	logger.Info(ctx, "Stored temporary notice with 30-second TTL")

	// Retrieve information from the knowledge base
	retrieveResult, err := workflow.Execute(ctx, map[string]any{
		"operation": "retrieve",
		"key":       "company_history",
	})
	if err != nil {
		logger.Error(ctx, "Failed to retrieve information: %v", err)
		return
	}
	logger.Info(ctx, "Retrieved company history: %s", retrieveResult["content"])

	// List all keys in the knowledge base
	listResult, err := workflow.Execute(ctx, map[string]any{
		"operation": "list",
	})
	if err != nil {
		logger.Error(ctx, "Failed to list keys: %v", err)
		return
	}
	if keys, ok := listResult["keys"].([]string); ok {
		logger.Info(ctx, "Knowledge base contains %d entries:", len(keys))
		for _, key := range keys {
			logger.Info(ctx, "  - %s", key)
		}
	}

	// Wait for the temporary notice to expire
	logger.Info(ctx, "Waiting for temporary notice to expire...")
	time.Sleep(40 * time.Second)

	// Try to retrieve the expired entry
	expiredResult, err := workflow.Execute(ctx, map[string]any{
		"operation": "retrieve",
		"key":       "temporary_notice",
	})
	if err != nil {
		logger.Info(ctx, "As expected, temporary notice expired: %v", err)
	} else {
		logger.Info(ctx, "Unexpectedly, temporary notice is still available: %s", expiredResult["content"])
	}

	logger.Info(ctx, "=================================================")
}

// CreateKnowledgeBaseWorkflow creates a workflow for managing a knowledge base
func CreateKnowledgeBaseWorkflow(dspyConfig *core.DSPYConfig, redisStore agents.Memory) (*workflows.ChainWorkflow, error) {
	// Create a chain workflow with Redis storage
	workflow := workflows.NewChainWorkflow(redisStore)

	// Create the knowledge base operation step
	knowledgeBaseStep := &workflows.Step{
		ID:     "knowledge_base_operation",
		Module: NewKnowledgeBaseModule(dspyConfig),
	}

	// Add step to workflow
	if err := workflow.AddStep(knowledgeBaseStep); err != nil {
		return nil, fmt.Errorf("failed to add knowledge base step: %v", err)
	}

	return workflow, nil
}

// KnowledgeBaseModule handles operations on a knowledge base
type KnowledgeBaseModule struct {
	*modules.Predict
	Memory agents.Memory
}

// NewKnowledgeBaseModule creates a new knowledge base module
func NewKnowledgeBaseModule(dspyConfig *core.DSPYConfig) *KnowledgeBaseModule {
	signature := core.NewSignature(
		[]core.InputField{
			{Field: core.Field{Name: "operation"}}, // "store", "retrieve", "list", "store_with_ttl"
			{Field: core.Field{Name: "key"}},       // key to store/retrieve (optional for "list")
			{Field: core.Field{Name: "content"}},   // content to store (optional for "retrieve"/"list")
			{Field: core.Field{Name: "ttl"}},       // time-to-live in seconds (optional)
		},
		[]core.OutputField{
			{Field: core.Field{Name: "message"}}, // operation result message
			{Field: core.Field{Name: "content"}}, // retrieved content
			{Field: core.Field{Name: "keys"}},    // list of keys
		},
	).WithInstruction(`Process knowledge base operations.`)

	return &KnowledgeBaseModule{
		Predict: modules.NewPredict(signature, dspyConfig),
	}
}

// Process overrides the default Process method to handle knowledge base operations
func (m *KnowledgeBaseModule) Process(ctx context.Context, inputs map[string]any, opts ...core.Option) (map[string]any, error) {
	// Extract inputs
	operation, _ := inputs["operation"].(string)
	key, _ := inputs["key"].(string)
	content, _ := inputs["content"].(string)
	var ttl float64
	if t, ok := inputs["ttl"].(float64); ok {
		ttl = t
	}

	// Get the workflow's memory (Redis store)
	memory, ok := ctx.Value(MemoryKey).(agents.Memory)
	if !ok {
		return nil, fmt.Errorf("memory not found in context")
	}

	outputs := make(map[string]any)

	switch operation {
	case "store":
		if err := memory.Store(key, content); err != nil {
			return nil, fmt.Errorf("failed to store: %v", err)
		}
		outputs["message"] = fmt.Sprintf("Successfully stored content for key: %s", key)

	case "store_with_ttl":
		// Cast to TTLStore interface
		ttlStore, ok := memory.(TTLStore)
		if !ok {
			return nil, fmt.Errorf("memory store doesn't support TTL operations")
		}
		if err := ttlStore.StoreWithTTL(ctx, key, content, time.Duration(ttl)*time.Second); err != nil {
			return nil, fmt.Errorf("failed to store with TTL: %v", err)
		}
		outputs["message"] = fmt.Sprintf("Successfully stored content for key: %s with %d second TTL", key, int(ttl))

	case "retrieve":
		retrieved, err := memory.Retrieve(key)
		if err != nil {
			return nil, fmt.Errorf("failed to retrieve: %v", err)
		}
		content, ok := retrieved.(string)
		if !ok {
			return nil, fmt.Errorf("retrieved value is not a string")
		}
		outputs["content"] = content
		outputs["message"] = "Successfully retrieved content"

	case "list":
		keys, err := memory.List()
		if err != nil {
			return nil, fmt.Errorf("failed to list keys: %v", err)
		}
		outputs["keys"] = keys
		outputs["message"] = fmt.Sprintf("Found %d keys", len(keys))

	default:
		return nil, fmt.Errorf("unknown operation: %s", operation)
	}

	return outputs, nil
}

// RedisConfig holds Redis connection parameters
type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

func main() {
	// Parse command line arguments
	redisAddr := flag.String("redis-addr", "localhost:6379", "Redis server address")
	redisPassword := flag.String("redis-password", "", "Redis password")
	redisDB := flag.Int("redis-db", 0, "Redis database number")
	modelHost := flag.String("model-host", "http://localhost:11434", "LLM model host")
	modelName := flag.String("model-name", "llama3", "LLM model name")
	flag.Parse()

	// Setup logging
	output := logging.NewConsoleOutput(true, logging.WithColor(true))
	fileOutput, err := logging.NewFileOutput(
		"redis-example.log",
		logging.WithRotation(10*1024*1024, 5),
		logging.WithJSONFormat(true),
	)
	if err != nil {
		fmt.Printf("Failed to create file output: %v\n", err)
		os.Exit(1)
	}
	logger := logging.NewLogger(logging.Config{
		Severity: logging.INFO,
		Outputs:  []logging.Output{output, fileOutput},
	})
	logging.SetLogger(logger)

	// Create context with execution state
	ctx := core.WithExecutionState(context.Background())
	logger.Info(ctx, "Starting Redis agent example")

	// Create LLM
	llmConfig := llms.NewOllamaConfig(*modelHost, *modelName)
	llm, err := llms.NewLLM("", llmConfig)
	if err != nil {
		logger.Error(ctx, "Failed to create LLM: %v", err)
		os.Exit(1)
	}

	// Create DSPYConfig with the LLM
	dspyConfig := core.NewDSPYConfig().WithDefaultLLM(llm)

	// Redis configuration
	redisConfig := RedisConfig{
		Addr:     *redisAddr,
		Password: *redisPassword,
		DB:       *redisDB,
	}

	// Run the example
	RunRedisAgentExample(ctx, logger, dspyConfig, redisConfig)
}
