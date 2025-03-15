package utils

import (
	"log"

	"github.com/XiaoConstantine/dspy-go/pkg/core"
	"github.com/XiaoConstantine/dspy-go/pkg/llms"
)

// SetupLLM creates a new LLM and returns a DSPYConfig with that LLM configured as default
// This function accepts either:
// 1. An API key and a ModelID, or
// 2. An API key and an OllamaConfig
func SetupLLM(apiKey string, modelOrConfig interface{}) *core.DSPYConfig {
	var llm core.LLM
	var err error

	switch v := modelOrConfig.(type) {
	case core.ModelID:
		// Use the model ID directly
		llm, err = llms.NewLLM(apiKey, v)
	case *llms.OllamaConfig:
		// Use a pre-configured Ollama config 
		llm, err = llms.NewLLM(apiKey, v)
	default:
		log.Fatalf("Invalid argument type for modelOrConfig: %T", modelOrConfig)
	}

	if err != nil {
		log.Fatalf("Failed to create LLM: %v", err)
	}
	
	return core.NewDSPYConfig().WithDefaultLLM(llm)
}
