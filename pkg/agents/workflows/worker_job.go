package workflows

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/scottdavis/dsgo/pkg/core"
)

// WorkerJob represents a job to be processed by a worker
type WorkerJob struct {
	// ID is the unique identifier for this job
	ID string `json:"id"`

	// Type is the module type signature
	Type string `json:"type"`

	// Payload contains the input data for the job
	Payload map[string]any `json:"payload"`

	// Queue is the name of the queue this job belongs to
	Queue string `json:"queue"`

	// CreatedAt is when the job was created
	CreatedAt time.Time `json:"created_at"`

	// RunAt specifies when the job should be executed
	RunAt time.Time `json:"run_at"`

	// RetryCount tracks how many times this job has been retried
	RetryCount int `json:"retry_count"`

	// MaxRetries is the maximum number of retries before failing
	MaxRetries int `json:"max_retries"`

	// LastError contains the last error message if the job failed
	LastError string `json:"last_error,omitempty"`
}

// NewWorkerJob creates a new job from a module and payload
func NewWorkerJob(module core.Module, payload map[string]any) *WorkerJob {
	// Extract the module type as a string
	moduleType := fmt.Sprintf("%T", module)
	
	job := &WorkerJob{
		ID:         uuid.New().String(),
		Type:       moduleType, // Use the type name as the signature
		Payload:    payload,
		CreatedAt:  time.Now(),
		RunAt:      time.Now(),
		MaxRetries: 3,
	}
	return job
}

// IsReady returns true if the job is ready to be executed
func (j *WorkerJob) IsReady() bool {
	return time.Now().After(j.RunAt) || time.Now().Equal(j.RunAt)
}

// Retry marks the job for retry with a backoff
func (j *WorkerJob) Retry(err error) {
	j.RetryCount++
	if err != nil {
		j.LastError = err.Error()
	}
	
	// Exponential backoff: 10s, 30s, 90s, etc.
	backoff := time.Duration(10*3^(j.RetryCount-1)) * time.Second
	if backoff > 1*time.Hour {
		backoff = 1 * time.Hour
	}
	
	j.RunAt = time.Now().Add(backoff)
}

// CanRetry returns true if the job can be retried
func (j *WorkerJob) CanRetry() bool {
	return j.RetryCount < j.MaxRetries
}

// ScheduleAt schedules the job to run at a specific time
func (j *WorkerJob) ScheduleAt(runAt time.Time) {
	j.RunAt = runAt
}

// Delay schedules the job to run after a delay
func (j *WorkerJob) Delay(duration time.Duration) {
	j.RunAt = time.Now().Add(duration)
}

// Marshal serializes the job to JSON
func (j *WorkerJob) Marshal() ([]byte, error) {
	return json.Marshal(j)
}

// Unmarshal deserializes a job from JSON
func (j *WorkerJob) Unmarshal(data []byte) error {
	return json.Unmarshal(data, j)
}

// String returns a string representation of the job
func (j *WorkerJob) String() string {
	return fmt.Sprintf("WorkerJob{ID: %s, Type: %s, RetryCount: %d/%d}", 
		j.ID, j.Type, j.RetryCount, j.MaxRetries)
} 