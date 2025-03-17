package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/scottdavis/dsgo/pkg/agents"
	"github.com/scottdavis/dsgo/pkg/agents/memory"
	"github.com/scottdavis/dsgo/pkg/agents/workflows"
	"github.com/scottdavis/dsgo/pkg/core"
)

// Configuration options
type config struct {
	RedisAddr             string
	Role                  string
	WorkerID              string
	QueueName             string
	PollInterval          time.Duration
	Concurrency           int
	WorkersToMonitor      []string
	StalledWorkflowThreshold time.Duration
}

// Parse environment variables or flags
func parseConfig() config {
	cfg := config{}

	// Define flags with environment variable fallbacks
	flag.StringVar(&cfg.RedisAddr, "redis-addr", getEnv("REDIS_ADDR", "localhost:6379"), "Redis address")
	flag.StringVar(&cfg.Role, "role", getEnv("ROLE", "worker"), "Role (worker or supervisor)")
	flag.StringVar(&cfg.WorkerID, "worker-id", getEnv("WORKER_ID", "worker1"), "Worker ID")
	flag.StringVar(&cfg.QueueName, "queue-name", getEnv("QUEUE_NAME", "default"), "Queue name")
	
	pollIntervalStr := getEnv("POLL_INTERVAL", "1s")
	pollInterval, err := time.ParseDuration(pollIntervalStr)
	if err != nil {
		pollInterval = 1 * time.Second
	}
	flag.DurationVar(&cfg.PollInterval, "poll-interval", pollInterval, "Poll interval")
	
	concurrencyStr := getEnv("CONCURRENCY", "5")
	concurrency := 5
	if c, err := fmt.Sscanf(concurrencyStr, "%d", &concurrency); err == nil && c > 0 {
		// Do nothing, we parsed it correctly
	}
	flag.IntVar(&cfg.Concurrency, "concurrency", concurrency, "Concurrency")

	workersToMonitorStr := getEnv("WORKERS_TO_MONITOR", "")
	if workersToMonitorStr != "" {
		cfg.WorkersToMonitor = strings.Split(workersToMonitorStr, ",")
	}
	
	stalledThresholdStr := getEnv("STALLED_WORKFLOW_THRESHOLD", "30s")
	stalledThreshold, err := time.ParseDuration(stalledThresholdStr)
	if err != nil {
		stalledThreshold = 30 * time.Second
	}
	flag.DurationVar(&cfg.StalledWorkflowThreshold, "stalled-threshold", stalledThreshold, "Stalled workflow threshold")

	flag.Parse()
	return cfg
}

// Helper to get environment variable with fallback
func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}

// Setup Redis memory store
func setupRedisStore(addr string) (memory.ListMemory, error) {
	// Split Redis address into host and password if provided in format host:port:password
	hostPort := addr
	password := ""
	
	parts := strings.Split(addr, ":")
	if len(parts) >= 3 {
		// Format is likely host:port:password
		hostPort = parts[0] + ":" + parts[1]
		password = parts[2]
	}
	
	// Create Redis list store
	redisStore, err := memory.NewRedisListStore(hostPort, password, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to create Redis store: %w", err)
	}
	
	return redisStore, nil
}

// Example module that adds two numbers
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

func main() {
	// Set up logging
	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	logger := slog.New(handler)
	slog.SetDefault(logger)

	// Parse config
	cfg := parseConfig()

	// Set up the Redis store for both queue and state
	redisStore, err := setupRedisStore(cfg.RedisAddr)
	if err != nil {
		logger.Error("Failed to set up Redis store", "error", err)
		os.Exit(1)
	}

	// Create a registry and register modules
	registry := agents.NewModuleRegistry()
	registry.Register("AdditionModule", &AdditionModule{})

	// Create a context that can be canceled
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigChan
		logger.Info("Received shutdown signal", "signal", sig)
		cancel()
	}()

	switch cfg.Role {
	case "worker":
		runWorker(ctx, cfg, redisStore, registry, logger)
	case "supervisor":
		runSupervisor(ctx, cfg, redisStore, logger)
	default:
		logger.Error("Unknown role", "role", cfg.Role)
		os.Exit(1)
	}
}

func runWorker(ctx context.Context, cfg config, store memory.ListMemory, registry *agents.ModuleRegistry, logger *slog.Logger) {
	worker, err := workflows.NewDistributedWorker(&workflows.DistributedWorkerConfig{
		QueueStore:     store,
		StateStore:     store,
		QueueName:      cfg.QueueName,
		ModuleRegistry: registry,
		PollInterval:   cfg.PollInterval,
		Concurrency:    cfg.Concurrency,
		WorkerID:       cfg.WorkerID,
		Logger:         logger,
	})
	if err != nil {
		logger.Error("Failed to create worker", "error", err)
		os.Exit(1)
	}

	// Add logging middleware
	worker.Use(workflows.WithJobLogging(logger))
	
	// Add timeout middleware (5 minutes per job)
	worker.Use(workflows.WithJobTimeout(5 * time.Minute))

	logger.Info("Starting worker", 
		"worker_id", cfg.WorkerID,
		"queue", cfg.QueueName,
		"concurrency", cfg.Concurrency)

	// Start the worker
	if err := worker.Start(ctx); err != nil {
		logger.Error("Failed to start worker", "error", err)
		os.Exit(1)
	}

	<-ctx.Done()
	worker.Stop()
	logger.Info("Worker stopped gracefully")
}

func runSupervisor(ctx context.Context, cfg config, store memory.ListMemory, logger *slog.Logger) {
	supervisor := workflows.NewWorkflowSupervisor(workflows.SupervisorConfig{
		Memory:         store,
		CheckInterval:  30 * time.Second,
		StallThreshold: cfg.StalledWorkflowThreshold,
		RetentionPeriod: 24 * time.Hour,
		JobQueue:       &mockJobQueue{}, // Mock job queue for example purposes
		EnableAutoRecovery: true,
		DeadLetterQueue: "dead_letter_queue",
		Logger:         log.New(os.Stdout, "SUPERVISOR: ", log.LstdFlags),
	})

	logger.Info("Starting supervisor",
		"monitored_workers", strings.Join(cfg.WorkersToMonitor, ","),
		"stalled_threshold", cfg.StalledWorkflowThreshold)

	// Start the supervisor
	supervisor.Start()
	
	<-ctx.Done()
	supervisor.Stop()
	logger.Info("Supervisor stopped gracefully")
}

// mockJobQueue implements a basic job queue for example purposes
type mockJobQueue struct{}

func (q *mockJobQueue) Push(ctx context.Context, jobID, stepID string, payload map[string]any) error {
	fmt.Printf("Would push job %s, step %s with payload %v\n", jobID, stepID, payload)
	return nil
}

func (q *mockJobQueue) Pop(ctx context.Context) (*workflows.Job, error) {
	return nil, fmt.Errorf("not implemented")
}

func (q *mockJobQueue) Close() error {
	return nil
}

func (q *mockJobQueue) Ping() error {
	return nil
} 