package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/scottdavis/dsgo/pkg/agents"
	"github.com/scottdavis/dsgo/pkg/agents/memory"
	"github.com/scottdavis/dsgo/pkg/agents/workflows"
	"github.com/scottdavis/dsgo/pkg/core"
)

// Client configuration
type clientConfig struct {
	RedisAddr string
	QueueName string
	Operation string
	ValA      float64
	ValB      float64
}

func main() {
	// Parse command-line flags
	cfg := clientConfig{}
	flag.StringVar(&cfg.RedisAddr, "redis-addr", "localhost:6379", "Redis address (host:port[:password])")
	flag.StringVar(&cfg.QueueName, "queue", "default", "Queue name")
	flag.StringVar(&cfg.Operation, "op", "addition", "Operation to perform (addition, subtraction, multiplication)")
	flag.Float64Var(&cfg.ValA, "a", 5.0, "First value")
	flag.Float64Var(&cfg.ValB, "b", 3.0, "Second value")
	flag.Parse()

	// Setup Redis store
	redisStore, err := setupRedisStore(cfg.RedisAddr)
	if err != nil {
		log.Fatalf("Failed to setup Redis store: %v", err)
	}

	// Create module registry
	registry := agents.NewModuleRegistry()
	registry.Register("AdditionModule", &AdditionModule{})
	
	// Create a worker client
	worker, err := workflows.NewDistributedWorker(&workflows.DistributedWorkerConfig{
		QueueStore:     redisStore,
		StateStore:     redisStore,
		QueueName:      cfg.QueueName,
		ModuleRegistry: registry,
		WorkerID:       "client",
	})
	if err != nil {
		log.Fatalf("Failed to create worker client: %v", err)
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Push a job
	module := &AdditionModule{}
	job, err := worker.PushJob(ctx, module, map[string]any{
		"a": cfg.ValA,
		"b": cfg.ValB,
	})
	if err != nil {
		log.Fatalf("Failed to push job: %v", err)
	}

	fmt.Printf("Job submitted with ID: %s\n", job.ID)
	fmt.Printf("Parameters: a=%.2f, b=%.2f\n", cfg.ValA, cfg.ValB)
	fmt.Printf("Job queue: %s\n", cfg.QueueName)
	
	// Monitor job status
	fmt.Println("Waiting for job to complete...")
	
	var result interface{}
	var jobStatus string
	maxAttempts := 30
	for i := 0; i < maxAttempts; i++ {
		time.Sleep(1 * time.Second)
		
		// Check job status
		jobStatus, err = worker.GetJobStatus(ctx, job.ID)
		fmt.Printf("Job status (attempt %d): %s\n", i+1, jobStatus)
		
		if err != nil {
			fmt.Printf("Error getting job status: %v\n", err)
			continue
		}
		
		if jobStatus == "completed" {
			// Try to get raw result directly from Redis
			resultKey := fmt.Sprintf("job:%s:result", job.ID)
			rawResult, err := redisStore.Retrieve(resultKey)
			if err != nil {
				fmt.Printf("Error getting raw job result: %v\n", err)
				continue
			}
			
			// Display the raw result for debugging
			fmt.Printf("Raw result type: %T\nRaw result: %v\n", rawResult, rawResult)
			
			result = rawResult
			break
		} else if jobStatus == "failed" {
			fmt.Println("Job failed.")
			os.Exit(1)
		}
	}
	
	if jobStatus != "completed" {
		fmt.Println("Job did not complete within the expected time.")
		os.Exit(1)
	}
	
	// Display result
	fmt.Println("Job completed successfully!")
	fmt.Printf("Result: %v\n", result)
	
	resultStr, _ := json.MarshalIndent(result, "", "  ")
	fmt.Printf("Full result:\n%s\n", resultStr)
}

// Setup Redis memory store (same as in main.go)
func setupRedisStore(addr string) (memory.ListMemory, error) {
	hostPort := addr
	password := ""
	
	parts := strings.Split(addr, ":")
	if len(parts) >= 3 {
		hostPort = parts[0] + ":" + parts[1]
		password = parts[2]
	}
	
	redisStore, err := memory.NewRedisListStore(hostPort, password, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to create Redis store: %w", err)
	}
	
	return redisStore, nil
}

// AdditionModule is the same as in main.go
type AdditionModule struct{}

func (m *AdditionModule) GetSignature() core.Signature {
	return core.Signature{
		Inputs: []core.InputField{
			{Field: core.Field{Name: "a"}},
			{Field: core.Field{Name: "b"}},
		},
		Outputs: []core.OutputField{
			{Field: core.Field{Name: "result"}},
		},
	}
}

func (m *AdditionModule) Process(ctx context.Context, input map[string]any, opts ...core.Option) (map[string]any, error) {
	a, aOk := input["a"].(float64)
	b, bOk := input["b"].(float64)
	
	if !aOk || !bOk {
		return nil, fmt.Errorf("invalid input types for addition")
	}
	
	result := a + b
	return map[string]any{
		"result": result,
	}, nil
}

func (m *AdditionModule) Clone() core.Module {
	return &AdditionModule{}
} 