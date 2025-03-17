package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/scottdavis/dsgo/pkg/core"
	"github.com/scottdavis/dsgo/pkg/llms"
	"github.com/scottdavis/dsgo/pkg/logging"
	"github.com/scottdavis/dsgo/pkg/modules"
)

func main() {
	// Setup DSP-GO logging
	output := logging.NewConsoleOutput(true, logging.WithColor(true))
	logger := logging.NewLogger(logging.Config{
		Severity: logging.INFO,
		Outputs:  []logging.Output{output},
	})
	logging.SetLogger(logger)

	// Create basic context
	basicCtx := context.Background()

	// Initialize LLM (example with Ollama)
	ollamaConfig := llms.NewOllamaConfig("http://localhost:11434", "llama3")
	llm, err := llms.NewLLM("", ollamaConfig)
	if err != nil {
		log.Fatalf("Failed to initialize LLM: %v", err)
		os.Exit(1)
	}

	// Create DSP-GO context with execution state
	ctx := core.WithExecutionState(basicCtx)

	// Create a signature for question answering
	signature := core.NewSignature(
		[]core.InputField{{Field: core.Field{Name: "question"}}},
		[]core.OutputField{{Field: core.Field{Name: "answer"}}},
	)

	// Create a DSPYConfig with the LLM
	dspyConfig := core.NewDSPYConfig().WithDefaultLLM(llm)

	// Create a ChainOfThought module with the config
	cot := modules.NewChainOfThought(signature, dspyConfig)

	// Create a program
	program := core.NewProgram(
		map[string]core.Module{"cot": cot},
		func(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
			return cot.Process(ctx, inputs)
		},
		dspyConfig,
	)

	// Execute the program
	result, err := program.Execute(ctx, map[string]interface{}{
		"question": "What is the capital of France?",
	})
	if err != nil {
		log.Fatalf("Error executing program: %v", err)
	}

	fmt.Printf("Answer: %s\n", result["answer"])
} 