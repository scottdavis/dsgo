package main

import (
	"context"
	"flag"
	"fmt"
	"log"

	"github.com/scottdavis/dsgo/examples/utils"
	"github.com/scottdavis/dsgo/pkg/core"
	"github.com/scottdavis/dsgo/pkg/datasets"
	"github.com/scottdavis/dsgo/pkg/modules"
	"github.com/scottdavis/dsgo/pkg/optimizers"
)

func RunGSM8KExample(apiKey string) {
	// Setup LLM and create config
	dspyConfig := utils.SetupLLM(apiKey, "llamacpp:local")

	// Load GSM8K dataset
	examples, err := datasets.LoadGSM8K()
	if err != nil {
		log.Fatalf("Failed to load GSM8K dataset: %v", err)
	}

	// Create signature for ChainOfThought
	signature := core.NewSignature(
		[]core.InputField{{Field: core.Field{Name: "question"}}},
		[]core.OutputField{{Field: core.Field{Name: "answer"}}},
	)

	// Create ChainOfThought module
	cot := modules.NewChainOfThought(signature, dspyConfig)

	// Create program
	program := core.NewProgram(
		map[string]core.Module{"cot": cot},
		func(ctx context.Context, inputs map[string]any) (map[string]any, error) {
			return cot.Process(ctx, inputs, core.WithGenerateOptions(
				core.WithTemperature(0.7),
				core.WithMaxTokens(8192),
			))
		},
		dspyConfig,
	)

	// Create optimizer
	optimizer := optimizers.NewBootstrapFewShot(
		func(example, prediction map[string]any, ctx context.Context) bool {
			return example["answer"] == prediction["answer"]
		},
		5,
		dspyConfig,
	)

	// Prepare training set
	trainset := make([]map[string]any, len(examples[:10]))
	for i, ex := range examples[:10] {
		trainset[i] = map[string]any{
			"question": ex.Question,
			"answer":   ex.Answer,
		}
	}

	// Compile the program
	compiledProgram, err := optimizer.Compile(context.Background(), program, program, trainset)
	if err != nil {
		log.Fatalf("Failed to compile program: %v", err)
	}

	// Test the compiled program
	for _, ex := range examples[10:15] {
		result, err := compiledProgram.Execute(context.Background(), map[string]any{"question": ex.Question})
		if err != nil {
			log.Printf("Error executing program: %v", err)
			continue
		}
		fmt.Printf("Question: %s\n", ex.Question)
		fmt.Printf("Predicted Answer: %s\n", result["answer"])
		fmt.Printf("Actual Answer: %s\n\n", ex.Answer)
	}
}

func main() {
	apiKey := flag.String("api-key", "", "Anthropic API Key")

	RunGSM8KExample(*apiKey)
}
