package utils

import (
	"log"

	"github.com/scottdavis/dsgo/pkg/core"
	"github.com/scottdavis/dsgo/pkg/llms"
)

// SetupLLM creates a new LLM and returns a DSPYConfig with that LLM configured as default
// This function accepts either:
// 1. An API key and a ModelID, or
// 2. An API key and an OllamaConfig, or
// 3. An API key and an OpenRouterConfig
func SetupLLM(apiKey string, modelOrConfig any) *core.DSPYConfig {
	var llm core.LLM
	var err error

	switch v := modelOrConfig.(type) {
	case core.ModelID:
		// Use the model ID directly
		llm, err = llms.NewLLM(apiKey, v)
	case *llms.OllamaConfig:
		// Use a pre-configured Ollama config 
		llm, err = llms.NewLLM(apiKey, v)
	case *llms.OpenRouterConfig:
		// Use a pre-configured OpenRouter config
		llm, err = llms.NewLLM(apiKey, v)
	default:
		log.Fatalf("Invalid argument type for modelOrConfig: %T", modelOrConfig)
	}

	if err != nil {
		log.Fatalf("Failed to create LLM: %v", err)
	}
	
	return core.NewDSPYConfig().WithDefaultLLM(llm)
}
