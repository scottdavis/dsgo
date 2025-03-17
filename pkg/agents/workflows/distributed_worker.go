package workflows

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/scottdavis/dsgo/pkg/agents"
	"github.com/scottdavis/dsgo/pkg/agents/memory"
	"github.com/scottdavis/dsgo/pkg/core"
	"github.com/scottdavis/dsgo/pkg/errors"
)

// DistributedWorkerConfig configures the distributed worker
type DistributedWorkerConfig struct {
	// QueueStore is the distributed list storage (Redis recommended)
	QueueStore memory.ListMemory

	// StateStore is used to store job state
	StateStore agents.Memory

	// QueueName is the name of the queue to process
	QueueName string

	// ModuleRegistry contains the modules this worker can process
	ModuleRegistry *agents.ModuleRegistry

	// PollInterval is how often to poll for new jobs
	PollInterval time.Duration

	// Concurrency is the number of jobs to process concurrently
	Concurrency int

	// ErrorHandler is called when a job fails
	ErrorHandler func(job *WorkerJob, err error)

	// Logger is used for logging
	Logger *slog.Logger

	// WorkerID identifies this worker instance
	WorkerID string
}

// DistributedWorker processes jobs from a Redis queue
type DistributedWorker struct {
	config      *DistributedWorkerConfig
	workerID    string
	queueStore  memory.ListMemory
	stateStore  agents.Memory
	registry    *agents.ModuleRegistry
	stopCh      chan struct{}
	jobCh       chan *WorkerJob
	wg          sync.WaitGroup
	middlewares []WorkerMiddleware
	mu          sync.Mutex
	isRunning   bool
}

// WorkerMiddleware defines a function that wraps job processing
type WorkerMiddleware func(next WorkerHandler) WorkerHandler

// WorkerHandler defines the signature for processing a job
type WorkerHandler func(ctx context.Context, job *WorkerJob) error

// SetErrorHandler sets the error handler for the worker
func (w *DistributedWorker) SetErrorHandler(handler func(job *WorkerJob, err error)) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.config.ErrorHandler = handler
}

// NewDistributedWorker creates a new distributed worker
func NewDistributedWorker(config *DistributedWorkerConfig) (*DistributedWorker, error) {
	if config.QueueStore == nil {
		return nil, errors.New(errors.InvalidInput, "QueueStore is required")
	}
	if config.StateStore == nil {
		return nil, errors.New(errors.InvalidInput, "StateStore is required")
	}
	if config.ModuleRegistry == nil {
		return nil, errors.New(errors.InvalidInput, "ModuleRegistry is required")
	}
	if config.QueueName == "" {
		config.QueueName = "default"
	}
	if config.Concurrency <= 0 {
		config.Concurrency = 1
	}
	if config.PollInterval <= 0 {
		config.PollInterval = 5 * time.Second
	}
	if config.Logger == nil {
		config.Logger = slog.Default()
	}
	if config.WorkerID == "" {
		config.WorkerID = uuid.New().String()
	}

	return &DistributedWorker{
		config:     config,
		workerID:   config.WorkerID,
		queueStore: config.QueueStore,
		stateStore: config.StateStore,
		registry:   config.ModuleRegistry,
		stopCh:     make(chan struct{}),
		jobCh:      make(chan *WorkerJob, config.Concurrency),
		isRunning:  false,
	}, nil
}

// Use adds middleware to the worker
func (w *DistributedWorker) Use(middleware ...WorkerMiddleware) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.middlewares = append(w.middlewares, middleware...)
}

// Start begins processing jobs
func (w *DistributedWorker) Start(ctx context.Context) error {
	w.mu.Lock()
	if w.isRunning {
		w.mu.Unlock()
		return errors.New(errors.InvalidWorkflowState, "worker is already running")
	}
	w.isRunning = true
	w.mu.Unlock()

	w.config.Logger.Info("Starting distributed worker",
		"worker_id", w.workerID,
		"queue", w.config.QueueName,
		"concurrency", w.config.Concurrency)

	// Start the poller
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		w.pollQueue(ctx)
	}()

	// Start worker goroutines
	for i := 0; i < w.config.Concurrency; i++ {
		w.wg.Add(1)
		go func(workerNum int) {
			defer w.wg.Done()
			w.processJobs(ctx, workerNum)
		}(i)
	}

	return nil
}

// Stop halts all job processing
func (w *DistributedWorker) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.isRunning {
		return
	}

	w.config.Logger.Info("Stopping distributed worker", "worker_id", w.workerID)
	close(w.stopCh)
	w.isRunning = false
	w.wg.Wait()
	w.config.Logger.Info("Distributed worker stopped", "worker_id", w.workerID)
}

// QueueName returns the name of the queue this worker processes
func (w *DistributedWorker) QueueName() string {
	return w.config.QueueName
}

// WorkerID returns the ID of this worker
func (w *DistributedWorker) WorkerID() string {
	return w.workerID
}

// pollQueue continuously checks for new jobs in the queue
func (w *DistributedWorker) pollQueue(ctx context.Context) {
	ticker := time.NewTicker(w.config.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-w.stopCh:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Check for jobs and push to channel
			job, err := w.fetchJob(ctx)
			if err != nil {
				w.config.Logger.Error("Error fetching job", "error", err)
				continue
			}
			if job != nil {
				select {
				case w.jobCh <- job:
					// Job sent to worker
				case <-w.stopCh:
					// Put job back in queue
					if err := w.returnJobToQueue(ctx, job); err != nil {
						w.config.Logger.Error("Failed to return job to queue", "job_id", job.ID, "error", err)
					}
					return
				case <-ctx.Done():
					// Put job back in queue
					if err := w.returnJobToQueue(ctx, job); err != nil {
						w.config.Logger.Error("Failed to return job to queue", "job_id", job.ID, "error", err)
					}
					return
				}
			}
		}
	}
}

// fetchJob retrieves the next job from the queue
func (w *DistributedWorker) fetchJob(ctx context.Context) (*WorkerJob, error) {
	value, err := w.queueStore.PopList(w.config.QueueName)
	if err != nil {
		return nil, errors.Wrap(err, errors.Unknown, "failed to pop from job queue")
	}
	if value == nil {
		return nil, nil // No jobs in queue
	}

	// Convert to JSON bytes
	var jobBytes []byte
	switch v := value.(type) {
	case string:
		jobBytes = []byte(v)
	case []byte:
		jobBytes = v
	default:
		return nil, errors.WithFields(
			errors.New(errors.InvalidResponse, "unexpected value type from queue"),
			errors.Fields{"type": fmt.Sprintf("%T", value)},
		)
	}

	// Parse job
	var job WorkerJob
	if err := json.Unmarshal(jobBytes, &job); err != nil {
		return nil, errors.Wrap(err, errors.InvalidResponse, "failed to unmarshal job")
	}

	// Check if job is ready to run
	if !job.IsReady() {
		// Put it back in queue if not ready
		if err := w.returnJobToQueue(ctx, &job); err != nil {
			return nil, err
		}
		return nil, nil
	}

	return &job, nil
}

// returnJobToQueue puts a job back in the queue
func (w *DistributedWorker) returnJobToQueue(ctx context.Context, job *WorkerJob) error {
	jobData, err := json.Marshal(job)
	if err != nil {
		return errors.Wrap(err, errors.InvalidInput, "failed to marshal job")
	}

	if err := w.queueStore.PushList(w.config.QueueName, string(jobData)); err != nil {
		return errors.Wrap(err, errors.Unknown, "failed to push job back to queue")
	}

	return nil
}

// processJobs handles jobs from the job channel
func (w *DistributedWorker) processJobs(ctx context.Context, workerNum int) {
	for {
		select {
		case <-w.stopCh:
			return
		case <-ctx.Done():
			return
		case job := <-w.jobCh:
			// Process the job with middleware
			jobCtx := context.WithValue(ctx, "worker_id", w.workerID)
			jobCtx = context.WithValue(jobCtx, "worker_num", workerNum)
			
			w.config.Logger.Info("Processing job", 
				"job_id", job.ID, 
				"job_type", job.Type, 
				"worker_id", w.workerID, 
				"worker_num", workerNum)
			
			var handler WorkerHandler = w.processJob
			
			// Apply middleware in reverse order
			for i := len(w.middlewares) - 1; i >= 0; i-- {
				handler = w.middlewares[i](handler)
			}
			
			err := handler(jobCtx, job)
			
			if err != nil {
				w.config.Logger.Error("Job processing failed", 
					"job_id", job.ID, 
					"error", err,
					"retry_count", job.RetryCount)
				
				if w.config.ErrorHandler != nil {
					w.config.ErrorHandler(job, err)
				}
				
				if job.CanRetry() {
					job.Retry(err)
					if err := w.returnJobToQueue(ctx, job); err != nil {
						w.config.Logger.Error("Failed to requeue job for retry", 
							"job_id", job.ID, 
							"error", err)
					}
				} else {
					// Store failed job state
					stateKey := fmt.Sprintf("job:%s:failed", job.ID)
					if err := w.stateStore.Store(stateKey, job); err != nil {
						w.config.Logger.Error("Failed to store failed job state", 
							"job_id", job.ID, 
							"error", err)
					}
				}
			} else {
				w.config.Logger.Info("Job completed successfully", "job_id", job.ID)
				
				// Store completed job state
				stateKey := fmt.Sprintf("job:%s:completed", job.ID)
				if err := w.stateStore.Store(stateKey, job); err != nil {
					w.config.Logger.Error("Failed to store completed job state", 
						"job_id", job.ID, 
						"error", err)
				}
			}
		}
	}
}

// processJob executes the actual job
func (w *DistributedWorker) processJob(ctx context.Context, job *WorkerJob) error {
	// Find module in registry
	module, err := w.registry.GetBySignature(job.Type)
	if err != nil {
		return errors.WithFields(
			errors.Wrap(err, errors.ResourceNotFound, "module not found in registry"),
			errors.Fields{"module_type": job.Type},
		)
	}
	
	// Create a cloned instance of the module
	instance := module.Clone()
	
	// Process the job
	w.config.Logger.Debug("Processing job with payload", "payload", job.Payload, "module_type", job.Type)
	result, err := instance.Process(ctx, job.Payload)
	if err != nil {
		return errors.WithFields(
			errors.Wrap(err, errors.Unknown, "module processing failed"),
			errors.Fields{"module_type": job.Type},
		)
	}
	
	// Store the result
	w.config.Logger.Debug("Storing job result", "result", result, "job_id", job.ID)
	resultKey := fmt.Sprintf("job:%s:result", job.ID)
	if err := w.stateStore.Store(resultKey, result); err != nil {
		return errors.WithFields(
			errors.Wrap(err, errors.Unknown, "failed to store job result"),
			errors.Fields{"job_id": job.ID},
		)
	}
	
	return nil
}

// PushJob adds a job to the queue
func (w *DistributedWorker) PushJob(ctx context.Context, module core.Module, payload map[string]any) (*WorkerJob, error) {
	job := NewWorkerJob(module, payload)
	job.Queue = w.config.QueueName
	
	jobData, err := json.Marshal(job)
	if err != nil {
		return nil, errors.Wrap(err, errors.InvalidInput, "failed to marshal job")
	}
	
	if err := w.queueStore.PushList(w.config.QueueName, string(jobData)); err != nil {
		return nil, errors.Wrap(err, errors.Unknown, "failed to push job to queue")
	}
	
	w.config.Logger.Info("Job pushed to queue", 
		"job_id", job.ID, 
		"job_type", job.Type, 
		"queue", w.config.QueueName)
	
	return job, nil
}

// GetJobResult retrieves the result of a completed job
func (w *DistributedWorker) GetJobResult(ctx context.Context, jobID string) (map[string]any, error) {
	resultKey := fmt.Sprintf("job:%s:result", jobID)
	result, err := w.stateStore.Retrieve(resultKey)
	if err != nil {
		return nil, errors.WithFields(
			errors.Wrap(err, errors.ResourceNotFound, "job result not found"),
			errors.Fields{"job_id": jobID},
		)
	}
	
	resultMap, ok := result.(map[string]any)
	if !ok {
		return nil, errors.WithFields(
			errors.New(errors.InvalidResponse, "job result is not a map"),
			errors.Fields{"job_id": jobID, "result_type": fmt.Sprintf("%T", result)},
		)
	}
	
	return resultMap, nil
}

// GetJobStatus returns the status of a job
func (w *DistributedWorker) GetJobStatus(ctx context.Context, jobID string) (string, error) {
	// Check completed jobs
	completedKey := fmt.Sprintf("job:%s:completed", jobID)
	_, err := w.stateStore.Retrieve(completedKey)
	if err == nil {
		return "completed", nil
	}
	
	// Check failed jobs
	failedKey := fmt.Sprintf("job:%s:failed", jobID)
	_, err = w.stateStore.Retrieve(failedKey)
	if err == nil {
		return "failed", nil
	}
	
	// Check if job is in queue
	queueItems, err := w.queueStore.ListItems(w.config.QueueName, 0, -1)
	if err != nil {
		return "", errors.Wrap(err, errors.Unknown, "failed to list queue items")
	}
	
	for _, item := range queueItems {
		var jobBytes []byte
		switch v := item.(type) {
		case string:
			jobBytes = []byte(v)
		case []byte:
			jobBytes = v
		default:
			continue
		}
		
		var job WorkerJob
		if err := json.Unmarshal(jobBytes, &job); err != nil {
			continue
		}
		
		if job.ID == jobID {
			if job.RetryCount > 0 {
				return "retrying", nil
			}
			return "queued", nil
		}
	}
	
	return "unknown", errors.WithFields(
		errors.New(errors.ResourceNotFound, "job not found"),
		errors.Fields{"job_id": jobID},
	)
}

// QueueStats returns statistics about the queue
func (w *DistributedWorker) QueueStats(ctx context.Context) (QueueStats, error) {
	length, err := w.queueStore.ListLength(w.config.QueueName)
	if err != nil {
		return QueueStats{}, errors.Wrap(err, errors.Unknown, "failed to get queue length")
	}
	
	return QueueStats{
		QueueName:  w.config.QueueName,
		Count:      int64(length),
		Timestamp:  time.Now(),
	}, nil
}

// WithJobTimeout middleware adds a timeout to the job processing
func WithJobTimeout(timeout time.Duration) WorkerMiddleware {
	return func(next WorkerHandler) WorkerHandler {
		return func(ctx context.Context, job *WorkerJob) error {
			ctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			return next(ctx, job)
		}
	}
}

// WithJobLogging middleware logs job processing metrics
func WithJobLogging(logger *slog.Logger) WorkerMiddleware {
	return func(next WorkerHandler) WorkerHandler {
		return func(ctx context.Context, job *WorkerJob) error {
			start := time.Now()
			logger.Info("Starting job execution", 
				"job_id", job.ID, 
				"job_type", job.Type)
			
			err := next(ctx, job)
			
			duration := time.Since(start)
			logEvent := logger.With(
				"job_id", job.ID,
				"job_type", job.Type,
				"duration_ms", duration.Milliseconds(),
			)
			
			if err != nil {
				logEvent.Error("Job execution failed", "error", err)
			} else {
				logEvent.Info("Job execution completed")
			}
			
			return err
		}
	}
} 