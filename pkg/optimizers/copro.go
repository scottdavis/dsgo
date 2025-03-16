package optimizers

import (
	"context"
	"fmt"

	"github.com/scottdavis/dsgo/pkg/core"
)

type Copro struct {
	Metric          func(example, prediction map[string]any, ctx context.Context) bool
	MaxBootstrapped int
	SubOptimizer    core.Optimizer
	Config          *core.DSPYConfig
}

func NewCopro(metric func(example, prediction map[string]any, ctx context.Context) bool, maxBootstrapped int, subOptimizer core.Optimizer, config *core.DSPYConfig) *Copro {
	if config == nil {
		config = core.NewDSPYConfig()
	}
	return &Copro{
		Metric:          metric,
		MaxBootstrapped: maxBootstrapped,
		SubOptimizer:    subOptimizer,
		Config:          config,
	}
}

// Compile optimizes a program using the provided dataset and metric.
func (c *Copro) Compile(ctx context.Context, program *core.Program, dataset core.Dataset, metric core.Metric) (*core.Program, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if core.GetExecutionState(ctx) == nil {
		ctx = core.WithExecutionState(ctx)
	}

	ctx, span := core.StartSpan(ctx, "CoPro.Compile")
	defer core.EndSpan(ctx)

	// Create a copy of the program to optimize
	compiledProgram := program.Clone()

	// Wrap the metric to handle the expected format
	wrappedMetric := func(ctx context.Context, result map[string]any) (bool, string) {
		return metric(ctx, result)
	}

	// Compile each module in the program
	for name, module := range compiledProgram.Modules {
		span.WithAnnotation("compiling_module", name)
		optimizedModule, err := c.compileModule(ctx, module, dataset, wrappedMetric)
		if err != nil {
			span.WithError(err)
			return compiledProgram, fmt.Errorf("error compiling module %s: %w", name, err)
		}
		compiledProgram.Modules[name] = optimizedModule
	}

	return compiledProgram, nil
}

// compileModule optimizes a single module using the provided dataset and metric.
func (c *Copro) compileModule(ctx context.Context, module core.Module, dataset core.Dataset, metric core.Metric) (core.Module, error) {
	// Create a program with just this module
	moduleProgram := core.NewProgram(
		map[string]core.Module{"module": module},
		func(ctx context.Context, inputs map[string]any) (map[string]any, error) {
			return module.Process(ctx, inputs)
		},
		c.Config,
	)

	// Optimize the module program using the SubOptimizer
	optimizedProgram, err := c.SubOptimizer.Compile(ctx, moduleProgram, dataset, metric)
	if err != nil {
		return module, err
	}

	// Return the optimized module
	return optimizedProgram.Modules["module"], nil
}
