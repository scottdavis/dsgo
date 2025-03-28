package optimizers

import (
	"context"
	"fmt"
	"math/rand"

	"github.com/scottdavis/dsgo/pkg/core"
	"github.com/scottdavis/dsgo/pkg/logging"
	"github.com/scottdavis/dsgo/pkg/modules"
)

type MIPRO struct {
	Metric               func(example, prediction map[string]any, ctx context.Context) float64
	NumCandidates        int
	MaxBootstrappedDemos int
	MaxLabeledDemos      int
	NumTrials            int
	PromptModel          core.LLM
	TaskModel            core.LLM
	MiniBatchSize        int
	FullEvalSteps        int
	Verbose              bool
}

func NewMIPRO(metric func(example, prediction map[string]any, ctx context.Context) float64, opts ...MIPROOption) *MIPRO {
	m := &MIPRO{
		Metric:               metric,
		NumCandidates:        10,
		MaxBootstrappedDemos: 5,
		MaxLabeledDemos:      5,
		NumTrials:            100,
		MiniBatchSize:        32,
		FullEvalSteps:        10,
		Verbose:              false,
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

type MIPROOption func(*MIPRO)

func WithNumCandidates(n int) MIPROOption {
	return func(m *MIPRO) { m.NumCandidates = n }
}

func WithMaxBootstrappedDemos(n int) MIPROOption {
	return func(m *MIPRO) { m.MaxBootstrappedDemos = n }
}

func WithMaxLabeledDemos(n int) MIPROOption {
	return func(m *MIPRO) { m.MaxLabeledDemos = n }
}

func WithNumTrials(n int) MIPROOption {
	return func(m *MIPRO) { m.NumTrials = n }
}

func WithPromptModel(model core.LLM) MIPROOption {
	return func(m *MIPRO) { m.PromptModel = model }
}

func WithTaskModel(model core.LLM) MIPROOption {
	return func(m *MIPRO) { m.TaskModel = model }
}

func WithMiniBatchSize(n int) MIPROOption {
	return func(m *MIPRO) { m.MiniBatchSize = n }
}

func WithFullEvalSteps(n int) MIPROOption {
	return func(m *MIPRO) { m.FullEvalSteps = n }
}

func WithVerbose(v bool) MIPROOption {
	return func(m *MIPRO) { m.Verbose = v }
}

type Trial struct {
	Params map[string]int
	Score  float64
}

func (m *MIPRO) Compile(ctx context.Context, program *core.Program, dataset core.Dataset, metric core.Metric) (*core.Program, error) {
	compileCtx, compilationSpan := core.StartSpan(ctx, "MIPROCompilation")
	defer core.EndSpan(compileCtx)

	dataset.Reset()
	instructionCandidates, err := m.generateInstructionCandidates(compileCtx, program, dataset)
	if err != nil {

		compilationSpan.WithError(err)
		return program, fmt.Errorf("failed to generate instruction candidates: %w", err)
	}

	demoCandidates, err := m.generateDemoCandidates(compileCtx, program, dataset)
	if err != nil {

		compilationSpan.WithError(err)
		return program, fmt.Errorf("failed to generate demo candidates: %w", err)
	}

	var bestTrial Trial
	var bestScore float64
	var compilationError error
	trialsCtx, trialsSpan := core.StartSpan(ctx, "OptimizationTrials")
	defer core.EndSpan(trialsCtx)
	for i := 0; i < m.NumTrials; i++ {

		trialCtx, trialSpan := core.StartSpan(trialsCtx, fmt.Sprintf("Trial_%d", i))
		trial := m.generateTrial(program.GetModules(), len(instructionCandidates), len(demoCandidates))
		candidateProgram := m.constructProgram(program, trial, instructionCandidates, demoCandidates)
		// Reset dataset before evaluation to ensure consistent starting point
		if i == 0 || i%m.FullEvalSteps == 0 {
			dataset.Reset()
		}

		score, err := m.evaluateProgram(trialsCtx, candidateProgram, dataset, metric)

		if err != nil {
			trialSpan.WithError(err)
			compilationError = err
			core.EndSpan(trialCtx)
			continue
		}

		trial.Score = score

		trialSpan.WithAnnotation("score", score)

		if score > bestTrial.Score || i == 0 {
			bestTrial = trial
			bestScore = score
			trialSpan.WithAnnotation("new_best", true)
		}

		if m.Verbose {
			m.logTrialResult(i, trial)
		}
		core.EndSpan(trialCtx)

	}
	trialsSpan.WithAnnotation("best_score", bestScore)

	if bestTrial.Params == nil {
		if compilationError != nil {
			compilationSpan.WithError(compilationError)
			return program, fmt.Errorf("compilation failed: %w", compilationError)
		}
		return program, fmt.Errorf("no successful trials")
	}
	result := m.constructProgram(program, bestTrial, instructionCandidates, demoCandidates)
	compilationSpan.WithAnnotation("final_score", bestScore)
	return result, nil
}

func (m *MIPRO) generateTrial(modules []core.Module, numInstructions, numDemos int) Trial {
	params := make(map[string]int)
	for i := range modules {
		params[fmt.Sprintf("instruction_%d", i)] = rand.Intn(numInstructions)
		params[fmt.Sprintf("demo_%d", i)] = rand.Intn(numDemos)
	}
	return Trial{Params: params}
}

func (m *MIPRO) constructProgram(baseProgram *core.Program, trial Trial, instructionCandidates [][]string, demoCandidates [][][]core.Example) *core.Program {
	program := baseProgram.Clone()
	modulesList := program.GetModules()

	for i, module := range modulesList {
		if predictor, ok := module.(*modules.Predict); ok {
			instructionIdx := trial.Params[fmt.Sprintf("instruction_%d", i)]
			demoIdx := trial.Params[fmt.Sprintf("demo_%d", i)]

			// Ensure we're using the correct index for instructionCandidates and demoCandidates
			if i < len(instructionCandidates) && instructionIdx < len(instructionCandidates[i]) {
				predictor.SetSignature(predictor.GetSignature().WithInstruction(instructionCandidates[i][instructionIdx]))
			}

			if i < len(demoCandidates) && demoIdx < len(demoCandidates[i]) {
				predictor.SetDemos(demoCandidates[i][demoIdx])
			}
		}
	}
	return program
}

func (m *MIPRO) evaluateProgram(ctx context.Context, program *core.Program, dataset core.Dataset, metric core.Metric) (float64, error) {
	// Evaluate on a mini-batch
	var totalScore float64
	var numExamples int

	dataset.Reset()
	for numExamples < m.MiniBatchSize {
		example, hasMore := dataset.Next()
		if !hasMore {
			break
		}

		// Execute the program on the example
		prediction, err := program.Execute(ctx, example.Input)
		if err != nil {
			return 0, err
		}

		// Evaluate the prediction
		score, _ := metric(ctx, prediction)
		if score {
			totalScore += 1.0
		}
		numExamples++
	}

	if numExamples == 0 {
		return 0, nil
	}

	return totalScore / float64(numExamples), nil
}

func (m *MIPRO) logTrialResult(trialNum int, trial Trial) {
	fmt.Printf("Trial %d: Score %.4f\n", trialNum, trial.Score)
}

func (m *MIPRO) generateInstructionCandidates(ctx context.Context, program *core.Program, dataset core.Dataset) ([][]string, error) {
	candidates := make([][]string, len(program.GetModules()))
	for i, module := range program.GetModules() {
		if predictor, ok := module.(*modules.Predict); ok {
			candidates[i] = make([]string, m.NumCandidates)
			for j := 0; j < m.NumCandidates; j++ {
				instruction, err := m.PromptModel.Generate(ctx, fmt.Sprintf("Generate an instruction for the following signature: %s", predictor.GetSignature()))
				if err != nil {
					return nil, err
				}
				candidates[i][j] = instruction.Content
			}
		}
	}
	return candidates, nil
}

func (m *MIPRO) generateDemoCandidates(ctx context.Context, program *core.Program, dataset core.Dataset) ([][][]core.Example, error) {
	candidates := make([][][]core.Example, len(program.GetModules()))
	logger := logging.GetLogger()

	// Collect examples just once
	var allExamples []core.Example
	maxNeeded := m.MaxBootstrappedDemos + m.MaxLabeledDemos

	// We won't reset the dataset here - it should already be at the beginning
	for i := 0; i < maxNeeded*m.NumCandidates; i++ {
		example, ok := dataset.Next()
		if !ok {
			break
		}
		if len(example.Output) > 0 {
			allExamples = append(allExamples, example)
		}
	}

	if len(allExamples) == 0 {
		return nil, fmt.Errorf("no valid examples found in dataset")
	}

	logger.Debug(ctx, "Collected %d examples for demo generation", len(allExamples))

	for i, module := range program.GetModules() {
		if _, ok := module.(*modules.Predict); ok {
			candidates[i] = make([][]core.Example, m.NumCandidates)
			for j := 0; j < m.NumCandidates; j++ {

				demos := make([]core.Example, 0, maxNeeded)

				startIdx := (j * maxNeeded) % len(allExamples)
				for k := 0; k < min(maxNeeded, len(allExamples)); k++ {
					idx := (startIdx + k) % len(allExamples)
					demos = append(demos, allExamples[idx])
				}

				candidates[i][j] = demos
			}
		}
	}
	return candidates, nil
}
