package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/scottdavis/dsgo/pkg/agents"
	"github.com/scottdavis/dsgo/pkg/agents/workflows"
	"github.com/scottdavis/dsgo/pkg/core"
)

// This example demonstrates the supervisor resilience pattern for workflows.
// It creates a workflow with multiple steps and shows how the supervisor
// detects and recovers from stalled workflows.

// SimpleModule is a basic module that logs its operations
type SimpleModule struct {
	core.BaseModule
	Name    string
	Fail    bool  // Whether this module should fail
	Stall   bool  // Whether this module should stall
	Timeout int64 // Seconds to sleep (to simulate work or stall)
}

// NewSimpleModule creates a new SimpleModule with the given configuration
func NewSimpleModule(name string, fail bool, stall bool, timeout int64) *SimpleModule {
	// Create a simple input/output signature
	signature := core.Signature{
		Inputs: []core.InputField{
			{Field: core.Field{Name: "input", Description: "Input data"}},
		},
		Outputs: []core.OutputField{
			{Field: core.Field{Name: "result", Description: "Output result"}},
			{Field: core.Field{Name: "time", Description: "Processing time"}},
		},
	}
	
	return &SimpleModule{
		BaseModule: core.BaseModule{
			Signature: signature,
			Config:    core.NewDSPYConfig(),
		},
		Name:    name,
		Fail:    fail,
		Stall:   stall,
		Timeout: timeout,
	}
}

// Process implements the core.Module interface
func (m *SimpleModule) Process(ctx context.Context, inputs map[string]any, opts ...core.Option) (map[string]any, error) {
	log.Printf("Processing module %s (Fail: %v, Stall: %v, Timeout: %d)", m.Name, m.Fail, m.Stall, m.Timeout)
	
	// Sleep to simulate work
	if m.Timeout > 0 {
		log.Printf("Module %s sleeping for %d seconds", m.Name, m.Timeout)
		time.Sleep(time.Duration(m.Timeout) * time.Second)
	}
	
	// Simulate failure if configured to fail
	if m.Fail {
		return nil, fmt.Errorf("module %s failed as configured", m.Name)
	}
	
	// If configured to stall, don't return and don't progress workflow
	if m.Stall {
		log.Printf("Module %s is stalling (not returning)", m.Name)
		// Block indefinitely
		<-ctx.Done()
		return nil, ctx.Err()
	}
	
	// Normal successful output
	log.Printf("Module %s completed successfully", m.Name)
	return map[string]any{
		"result": fmt.Sprintf("Output from %s", m.Name),
		"time":   time.Now().Format(time.RFC3339),
	}, nil
}

// Clone implements the core.Module interface
func (m *SimpleModule) Clone() core.Module {
	return &SimpleModule{
		BaseModule: m.BaseModule,
		Name:       m.Name,
		Fail:       m.Fail,
		Stall:      m.Stall,
		Timeout:    m.Timeout,
	}
}

// RecoveryCallback handles workflow recovery notifications from the supervisor
func RecoveryCallback(ctx context.Context, workflowID string, state *workflows.WorkflowState) error {
	log.Printf("RECOVERY CALLBACK: Workflow %s needs recovery (status: %s)", workflowID, state.Status)
	log.Printf("RECOVERY CALLBACK: Current step ID: %s", state.CurrentStepID)
	log.Printf("RECOVERY CALLBACK: Last heartbeat: %v", state.LastHeartbeat)
	
	// You could implement custom recovery logic here
	// For this example, we leave the actual recovery to the supervisor
	// But in real applications, you could implement more sophisticated recovery strategies
	
	return nil
}

// RunResilientWorkflowExample demonstrates the supervisor resilience pattern
func RunResilientWorkflowExample() {
	// Set up logging
	logger := log.New(os.Stdout, "EXAMPLE: ", log.LstdFlags|log.Lmicroseconds)
	logger.Printf("Starting resilient workflow example")
	
	// Create memory store
	memory := agents.NewInMemoryStore()
	
	// Create in-memory job queue for example
	queue := &TestJobQueue{
		jobs: make(map[string]*workflows.Job),
	}
	
	// Create module registry with our test modules
	moduleRegistry := map[string]core.Module{
		"step1": NewSimpleModule("Step 1", false, false, 2),
		"step2": NewSimpleModule("Step 2", false, true, 10),  // This step will stall
		"step3": NewSimpleModule("Step 3", false, false, 1),
		"step4": NewSimpleModule("Step 4", true, false, 1),   // This step will fail
		"step5": NewSimpleModule("Step 5", false, false, 1),
	}
	
	// Create workflow supervisor
	supervisor := workflows.NewWorkflowSupervisor(workflows.SupervisorConfig{
		Memory:               memory,
		JobQueue:             queue,
		CheckInterval:        5 * time.Second,    // Check for stalled workflows every 5 seconds
		StallThreshold:       10 * time.Second,   // Consider a workflow stalled after 10 seconds of inactivity
		RetentionPeriod:      1 * time.Hour,      // Keep completed workflows for 1 hour
		EnableAutoRecovery:   true,               // Automatically recover stalled workflows
		RecoveryCallback:     RecoveryCallback,   // Also notify our callback
		MaxConcurrentRecoveries: 2,               // Maximum of 2 recoveries at a time
		DeadLetterQueue:      "example_dlq",      // Dead letter queue name
		Logger:               logger,
	})
	
	// Create worker
	worker := workflows.NewWorkflowWorker(workflows.WorkerConfig{
		JobQueue:        queue,
		Memory:          memory,
		ModuleRegistry:  moduleRegistry,
		PollInterval:    100 * time.Millisecond,
		MaxConcurrentJobs: 2,
		ErrorHandler: func(ctx context.Context, workflowID, stepID string, err error) error {
			logger.Printf("ERROR HANDLER: Workflow %s, Step %s failed: %v", workflowID, stepID, err)
			return nil
		},
		Logger:          logger,
	})
	
	// Create async workflow
	workflow := workflows.NewAsyncChainWorkflow(memory, &workflows.AsyncChainConfig{
		JobQueue:          queue,
		JobPrefix:         "resilient",
		WaitForCompletion: true,
		CompletionTimeout: 3 * time.Minute,
		HeartbeatInterval: 5 * time.Second,
		DefaultErrorPolicy: workflows.ErrorPolicyRetry,
		EnableRetries:     true,
		MaxRetryAttempts:  2,
		RetryBackoff:      5 * time.Second,
	})
	
	// Add steps to the workflow
	for _, stepID := range []string{"step1", "step2", "step3", "step4", "step5"} {
		// Create a step with the corresponding module
		step := &workflows.Step{
			ID:     stepID,
			Module: moduleRegistry[stepID],
		}
		if err := workflow.AddStep(step); err != nil {
			logger.Printf("Error adding step %s: %v", stepID, err)
		}
	}
	
	// Start supervisor and worker
	supervisor.Start()
	worker.Start()
	
	// Handle graceful shutdown
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM)
	
	go func() {
		// Execute the workflow with input data
		result, err := workflow.Execute(context.Background(), map[string]any{
			"input":  "This is the example input",
			"config": map[string]any{"option1": true, "option2": "value"},
		})
		
		if err != nil {
			logger.Printf("Workflow execution error: %v", err)
		} else {
			logger.Printf("Workflow execution result: %v", result)
		}
	}()
	
	// Provide some status updates while running
	statusTicker := time.NewTicker(3 * time.Second)
	defer statusTicker.Stop()
	
	for {
		select {
		case <-shutdown:
			logger.Printf("Shutting down...")
			worker.Stop()
			supervisor.Stop()
			queue.Close()
			memory.Close()
			return
			
		case <-statusTicker.C:
			// List all keys in memory to see the workflow state
			keys, err := memory.List()
			if err != nil {
				logger.Printf("Error listing keys: %v", err)
				continue
			}
			
			var stateKey string
			for _, key := range keys {
				if strings.HasPrefix(key, "wf:") && strings.HasSuffix(key, ":state") {
					stateKey = key
					break
				}
			}
			
			if stateKey == "" {
				continue
			}
			
			// Get the workflow state
			value, err := memory.Retrieve(stateKey)
			if err != nil {
				logger.Printf("Error retrieving workflow state: %v", err)
				continue
			}
			
			state, err := workflows.DeserializeWorkflowState(value.(string))
			if err != nil {
				logger.Printf("Error deserializing workflow state: %v", err)
				continue
			}
			
			logger.Printf("Workflow status: %s, Current step: %s, Progress: %d%%", 
				state.Status, state.CurrentStepID, state.Progress)
			logger.Printf("Completed steps: %v, Failed steps: %v", 
				state.CompletedStepIDs, state.FailedStepIDs)
			
			if state.Status == workflows.WorkflowStatusCompleted || 
			   state.Status == workflows.WorkflowStatusFailed {
				logger.Printf("Workflow has finished. Press Ctrl+C to exit.")
			}
		}
	}
}

// TestJobQueue is a simple in-memory job queue for testing
type TestJobQueue struct {
	jobs map[string]*workflows.Job
	mu   sync.Mutex
}

// Push adds a job to the queue
func (q *TestJobQueue) Push(ctx context.Context, jobID, stepID string, payload map[string]any) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	
	q.jobs[jobID] = &workflows.Job{
		ID:         jobID,
		StepID:     stepID,
		Payload:    payload,
		EnqueuedAt: time.Now(),
	}
	
	return nil
}

// Pop retrieves a job from the queue
func (q *TestJobQueue) Pop(ctx context.Context) (*workflows.Job, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	
	// Return error if no jobs
	if len(q.jobs) == 0 {
		return nil, fmt.Errorf("no jobs available")
	}
	
	// Get the first job
	var job *workflows.Job
	for id, j := range q.jobs {
		job = j
		delete(q.jobs, id)
		break
	}
	
	return job, nil
}

// Close releases resources
func (q *TestJobQueue) Close() error {
	q.mu.Lock()
	defer q.mu.Unlock()
	
	q.jobs = make(map[string]*workflows.Job)
	return nil
}

// Ping verifies connectivity
func (q *TestJobQueue) Ping() error {
	return nil
} 