package optimizers

import (
	"context"
	"log"
	"sync"
	"sync/atomic"

	"github.com/XiaoConstantine/dspy-go/pkg/core"
	"github.com/sourcegraph/conc/pool"
)

type BootstrapFewShot struct {
	Metric          func(example map[string]any, prediction map[string]any, ctx context.Context) bool
	MaxBootstrapped int
	Config          *core.DSPYConfig
}

func NewBootstrapFewShot(metric func(example map[string]any, prediction map[string]any, ctx context.Context) bool, maxBootstrapped int, config *core.DSPYConfig) *BootstrapFewShot {
	if config == nil {
		config = core.NewDSPYConfig()
	}
	return &BootstrapFewShot{
		Metric:          metric,
		MaxBootstrapped: maxBootstrapped,
		Config:          config,
	}
}

func (b *BootstrapFewShot) Compile(ctx context.Context, student, teacher *core.Program, trainset []map[string]any) (*core.Program, error) {
	compiledStudent := student.Clone()
	
	// Use the teacher LLM from config if available, otherwise use default LLM
	teacherLLM := b.Config.TeacherLLM
	if teacherLLM == nil {
		teacherLLM = b.Config.DefaultLLM
	}
	
	if ctx == nil {
		ctx = context.Background()
	}
	if core.GetExecutionState(ctx) == nil {
		ctx = core.WithExecutionState(ctx)
	}

	ctx = core.WithExecutionState(ctx)
	ctx, _ = core.StartSpan(ctx, "Compilation")

	defer core.EndSpan(ctx)

	var (
		resultsMu sync.Mutex
		results   []struct {
			demo core.Example
			ctx  context.Context
		}
		numProcessed int32
	)

	// Check if we already have enough bootstrapped demonstrations
	if b.enoughBootstrappedDemos(compiledStudent) {
		return compiledStudent, nil
	}

	// Process examples in parallel
	p := pool.New().WithMaxGoroutines(10)

	for _, example := range trainset {
		example := example // Capture for goroutine
		p.Go(func() {
			exampleCtx, exampleSpan := core.StartSpan(ctx, "ProcessExample")
			defer core.EndSpan(exampleCtx)

			// Try to predict with the student program
			studentPrediction, err := compiledStudent.Execute(exampleCtx, example)
			if err != nil {
				exampleSpan.WithError(err)
				log.Printf("Error executing student program: %v", err)
				return
			}

			// Check if the student's prediction is correct
			if b.Metric(example, studentPrediction, exampleCtx) {
				return // Student already gets this example right
			}

			// Get the teacher's prediction
			teacherPrediction, err := b.predictWithTeacher(exampleCtx, teacher, teacherLLM, example)
			if err != nil {
				exampleSpan.WithError(err)
				log.Printf("Error executing teacher program: %v", err)
				return
			}

			// Check if the teacher's prediction is correct
			if !b.Metric(example, teacherPrediction, exampleCtx) {
				return // Teacher also gets this wrong, skip
			}

			// Create a demonstration from this example
			demo := core.Example{
				Input:  example,
				Output: teacherPrediction,
			}

			resultsMu.Lock()
			results = append(results, struct {
				demo core.Example
				ctx  context.Context
			}{demo, exampleCtx})
			resultsMu.Unlock()

			atomic.AddInt32(&numProcessed, 1)
		})
	}

	p.Wait()

	// Add demonstrations to the program
	for _, result := range results {
		if err := b.addDemonstrations(compiledStudent, result.demo, result.ctx); err != nil {
			return compiledStudent, err
		}

		// Check if we have enough demonstrations
		if b.enoughBootstrappedDemos(compiledStudent) {
			break
		}
	}

	return compiledStudent, nil
}

func (b *BootstrapFewShot) predictWithTeacher(ctx context.Context, teacher *core.Program, teacherLLM core.LLM, example map[string]any) (map[string]any, error) {
	// Create a temporary config with the teacher LLM
	tempConfig := core.NewDSPYConfig().WithDefaultLLM(teacherLLM)
	
	// Set the config on the teacher program
	teacherClone := teacher.Clone()
	teacherClone.Config = tempConfig
	
	// Execute the teacher program
	return teacherClone.Execute(ctx, example)
}

func (b *BootstrapFewShot) enoughBootstrappedDemos(program *core.Program) bool {
	if b.MaxBootstrapped <= 0 {
		return false
	}

	return len(program.Demonstrations) >= b.MaxBootstrapped
}

func (b *BootstrapFewShot) addDemonstrations(program *core.Program, demo core.Example, ctx context.Context) error {
	program.Demonstrations = append(program.Demonstrations, demo)
	return nil
}
