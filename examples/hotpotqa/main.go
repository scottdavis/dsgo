package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/XiaoConstantine/dspy-go/examples/utils"
	"github.com/XiaoConstantine/dspy-go/pkg/core"
	"github.com/XiaoConstantine/dspy-go/pkg/datasets"
	"github.com/XiaoConstantine/dspy-go/pkg/modules"
	"github.com/XiaoConstantine/dspy-go/pkg/optimizers"
	"github.com/sourcegraph/conc/pool"
)

func computeF1(prediction, groundTruth string) float64 {
	predTokens := strings.Fields(strings.ToLower(prediction))
	truthTokens := strings.Fields(strings.ToLower(groundTruth))

	common := make(map[string]bool)
	for _, token := range predTokens {
		for _, truthToken := range truthTokens {
			if token == truthToken {
				common[token] = true
				break
			}
		}
	}

	if len(predTokens) == 0 || len(truthTokens) == 0 {
		return 0.0
	}

	precision := float64(len(common)) / float64(len(predTokens))
	recall := float64(len(common)) / float64(len(truthTokens))

	if precision+recall == 0 {
		return 0.0
	}

	return 2 * precision * recall / (precision + recall)
}

func evaluateModel(program *core.Program, examples []datasets.HotPotQAExample) (float64, float64) {
	var totalF1, exactMatch float64
	var validExamples int32
	ctx := context.Background()

	results := make(chan struct {
		f1         float64
		exactMatch float64
	}, len(examples))

	total := len(examples)
	var processed int32 = 0

	// Start a goroutine to periodically report progress
	go func() {
		ticker := time.NewTicker(time.Second * 10)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				current := atomic.LoadInt32(&processed)
				fmt.Printf("Progress: %d/%d (%.2f%%)\n", current, total, float64(current)/float64(total)*100)
			}
		}
	}()

	// Set appropriate concurrency level
	concurrencyLevel := 1 // Default concurrency level
	if program.Config != nil {
		concurrencyLevel = program.Config.ConcurrencyLevel
	}

	p := pool.New().WithMaxGoroutines(concurrencyLevel)
	for _, ex := range examples {
		example := ex
		p.Go(func() {
			fmt.Println("Starting new coroutine")
			result, err := program.Execute(context.Background(), map[string]any{"question": example.Question})
			if err != nil {
				log.Printf("Error executing program: %v", err)
				return
			}

			predictedAnswer, ok := result["answer"].(string)
			if !ok {
				log.Printf("Error: predicted answer is not a string or is nil")
				return
			}

			f1 := computeF1(predictedAnswer, example.Answer)
			exactMatch := 0.0
			if predictedAnswer == example.Answer {
				exactMatch = 1.0
			}
			results <- struct {
				f1         float64
				exactMatch float64
			}{f1: f1, exactMatch: exactMatch}
			atomic.AddInt32(&processed, 1)
		})
	}
	go func() {
		p.Wait()
		close(results)
	}()
	for result := range results {
		totalF1 += result.f1
		exactMatch += result.exactMatch
		atomic.AddInt32(&validExamples, 1)
	}
	if validExamples == 0 {
		log.Printf("Warning: No valid examples processed")
		return 0, 0
	}

	avgF1 := totalF1 / float64(validExamples)
	exactMatchAccuracy := exactMatch / float64(validExamples)

	return avgF1, exactMatchAccuracy
}

func RunHotPotQAExample() {
	// Get API key from environment
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		fmt.Println("Please set OPENROUTER_API_KEY environment variable")
		os.Exit(1)
	}

	// Example 1: Using OpenRouterConfig
	fmt.Println("=== Example 1: Using OpenRouterConfig ===")
	
	// Define multiple models to try in case of failure
	modelOptions := []core.ModelID{
		core.ModelID("openrouter:google/gemma-3-1b-it:free"),
	}

	var dspyConfig *core.DSPYConfig
	var configError error
	var workingModel core.ModelID

	// Try each model in sequence until one works
	for _, model := range modelOptions {
		fmt.Printf("Attempting to set up model: %s\n", model)
		dspyConfig = utils.SetupLLM(apiKey, model)
		
		// Test the model with a simple query
		llm := dspyConfig.DefaultLLM
		testCtx := context.Background()
		
		// Create a cancellable context with timeout
		ctx, cancel := context.WithTimeout(testCtx, 30*time.Second)
		testResp, err := llm.Generate(ctx, "Hello, can you respond with a short greeting?")
		cancel()
		
		if err != nil {
			if strings.Contains(err.Error(), "rate limited") {
				fmt.Printf("Error with model %s: %v\n", model, err)
				fmt.Printf("Model %s is rate limited, trying next model\n", model)
				time.Sleep(1 * time.Second) // Brief pause before trying next model
				configError = err
				continue // Try the next model
			}
			
			// Log other types of errors
			fmt.Printf("Error with model %s: %v\n", model, err)
			configError = err
			continue // Try the next model
		}
		
		// Verify that the response has actual content
		if testResp != nil && testResp.Content != "" {
			fmt.Printf("Successfully connected to model: %s\n", model)
			workingModel = model
			configError = nil
			break // Found a working model, stop trying
		} else {
			fmt.Printf("Model %s returned empty response\n", model)
			configError = fmt.Errorf("empty response from model")
		}
	}

	// If all models failed, exit with error
	if configError != nil || workingModel == "" {
		fmt.Printf("Failed to set up any model. Last error: %v\n", configError)
		os.Exit(1)
	}

	fmt.Printf("Using model: %s\n", workingModel)
	dspyConfig.WithTeacherLLM(dspyConfig.DefaultLLM)
	// Set concurrency level in the config
	dspyConfig = dspyConfig.WithConcurrencyLevel(1)

	examples, err := datasets.LoadHotpotQA()
	if err != nil {
		log.Fatalf("Failed to load HotPotQA dataset: %v", err)
	}

	rand.Shuffle(len(examples), func(i, j int) { examples[i], examples[j] = examples[j], examples[i] })

	trainSize := int(0.025 * float64(len(examples)))
	valSize := int(0.1 * float64(len(examples)))
	trainExamples := examples[:trainSize]
	valExamples := examples[trainSize : trainSize+valSize]
	testExamples := examples[trainSize+valSize:]

	signature := core.NewSignature(
		[]core.InputField{{Field: core.Field{Name: "question"}}},
		[]core.OutputField{{Field: core.Field{Name: "answer"}}},
	)

	cot := modules.NewChainOfThought(signature, dspyConfig)

	program := core.NewProgram(map[string]core.Module{"cot": cot}, func(ctx context.Context, inputs map[string]any) (map[string]any, error) {
		return cot.Process(ctx, inputs)
	}, dspyConfig)

	metric := func(example, prediction map[string]any, ctx context.Context) bool {
		return computeF1(prediction["answer"].(string), example["answer"].(string)) > 0.5
	}

	optimizer := optimizers.NewBootstrapFewShot(metric, 5, dspyConfig)

	trainset := make([]map[string]any, len(trainExamples))
	for i, ex := range trainExamples {
		trainset[i] = map[string]any{
			"question": ex.Question,
			"answer":   ex.Answer,
		}
	}

	compiledProgram, err := optimizer.Compile(context.Background(), program, program, trainset)
	if err != nil {
		log.Fatalf("Failed to compile program: %v", err)
	}

	valF1, valExactMatch := evaluateModel(compiledProgram, valExamples)
	fmt.Printf("Validation Results - F1: %.4f, Exact Match: %.4f\n", valF1, valExactMatch)

	testF1, testExactMatch := evaluateModel(compiledProgram, testExamples)
	fmt.Printf("Test Results - F1: %.4f, Exact Match: %.4f\n", testF1, testExactMatch)

	// Example predictions
	for _, ex := range testExamples[:5] {
		result, err := compiledProgram.Execute(context.Background(), map[string]any{"question": ex.Question})
		if err != nil {
			log.Printf("Error executing program: %v", err)
			continue
		}
		fmt.Printf("Question: %s\n", ex.Question)
		fmt.Printf("Predicted Answer: %s\n", result["answer"])
		fmt.Printf("Actual Answer: %s\n", ex.Answer)
		fmt.Printf("F1 Score: %.4f\n\n", computeF1(result["answer"].(string), ex.Answer))
	}
}

func TestOpenRouterSetup() {
	// Get API key from environment
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		fmt.Println("Please set OPENROUTER_API_KEY environment variable")
		os.Exit(1)
	}

	fmt.Println("=== Testing OpenRouter Setup ===")
	
	// Define multiple models to try in case of failure
	modelOptions := []core.ModelID{
		core.ModelID("openrouter:google/gemma-3-1b-it:free"),
		core.ModelID("openrouter:anthropic/claude-3-haiku:free"),
		core.ModelID("openrouter:mistralai/mistral-large:free"),
		core.ModelID("openrouter:anthropic/claude-3-sonnet:free"),
		core.ModelID("openrouter:mistralai/mistral-small:free"),
	}

	var dspyConfig *core.DSPYConfig
	var configError error
	var workingModel core.ModelID

	// Try each model in sequence until one works
	for _, model := range modelOptions {
		fmt.Printf("Attempting to set up model: %s\n", model)
		dspyConfig = utils.SetupLLM(apiKey, model)
		
		// Test the model with a simple query
		llm := dspyConfig.DefaultLLM
		testCtx := context.Background()
		
		// Create a cancellable context with timeout
		ctx, cancel := context.WithTimeout(testCtx, 30*time.Second)
		testResp, err := llm.Generate(ctx, "Hello, can you respond with a short greeting?")
		cancel()
		
		if err != nil {
			if strings.Contains(err.Error(), "rate limited") {
				fmt.Printf("Error with model %s: %v\n", model, err)
				fmt.Printf("Model %s is rate limited, trying next model\n", model)
				time.Sleep(1 * time.Second) // Brief pause before trying next model
				configError = err
				continue // Try the next model
			}
			
			// Log other types of errors
			fmt.Printf("Error with model %s: %v\n", model, err)
			configError = err
			continue // Try the next model
		}
		
		// Verify that the response has actual content
		if testResp != nil && testResp.Content != "" {
			fmt.Printf("Successfully connected to model: %s\n", model)
			fmt.Printf("Response: %s\n", testResp.Content)
			workingModel = model
			configError = nil
			break // Found a working model, stop trying
		} else {
			fmt.Printf("Model %s returned empty response\n", model)
			configError = fmt.Errorf("empty response from model")
		}
	}

	// If all models failed, exit with error
	if configError != nil || workingModel == "" {
		fmt.Printf("Failed to set up any model. Last error: %v\n", configError)
		os.Exit(1)
	}

	fmt.Printf("Using model: %s\n", workingModel)
}

func main() {
	RunHotPotQAExample()
}
