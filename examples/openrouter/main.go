package main

import (
	"context"
	"fmt"
	"os"

	"github.com/XiaoConstantine/dspy-go/pkg/core"
	"github.com/XiaoConstantine/dspy-go/pkg/llms"
)

func main() {
	// Get API key from environment
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		fmt.Println("Please set OPENROUTER_API_KEY environment variable")
		os.Exit(1)
	}

	// Example 1: Using OpenRouterConfig
	fmt.Println("=== Example 1: Using OpenRouterConfig ===")
	// Create an OpenRouter config with a specific model
	config := llms.NewOpenRouterConfig("anthropic/claude-3-opus-20240229")
	
	// Create the LLM using the factory
	llm, err := llms.NewLLM(apiKey, config)
	if err != nil {
		fmt.Printf("Error creating LLM: %v\n", err)
		os.Exit(1)
	}

	// Generate text using the LLM
	response, err := llm.Generate(context.Background(), "What are three interesting applications of LLMs in healthcare?", 
		core.WithMaxTokens(300),
		core.WithTemperature(0.7),
	)
	if err != nil {
		fmt.Printf("Error generating text: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Response from %s:\n%s\n\n", llm.ModelID(), response.Content)
	fmt.Printf("Token usage - Prompt: %d, Completion: %d, Total: %d\n\n", 
		response.Usage.PromptTokens, 
		response.Usage.CompletionTokens, 
		response.Usage.TotalTokens,
	)

	// Example 2: Using the string format
	fmt.Println("=== Example 2: Using ModelID string format ===")
	// Alternatively, you can use the string format
	modelID := core.ModelID("openrouter:anthropic/claude-3-haiku-20240307")
	
	// Create the LLM using the factory with the string format
	llm2, err := llms.NewLLM(apiKey, modelID)
	if err != nil {
		fmt.Printf("Error creating LLM: %v\n", err)
		os.Exit(1)
	}

	// Generate text using the LLM
	response2, err := llm2.Generate(context.Background(), "Explain quantum computing in simple terms", 
		core.WithMaxTokens(200),
		core.WithTemperature(0.5),
	)
	if err != nil {
		fmt.Printf("Error generating text: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Response from %s:\n%s\n\n", llm2.ModelID(), response2.Content)
	fmt.Printf("Token usage - Prompt: %d, Completion: %d, Total: %d\n", 
		response2.Usage.PromptTokens, 
		response2.Usage.CompletionTokens, 
		response2.Usage.TotalTokens,
	)
} 