package llms

import (
	"fmt"
	"strings"

	"github.com/XiaoConstantine/anthropic-go/anthropic"
	"github.com/XiaoConstantine/dspy-go/pkg/core"
)

// OllamaConfig represents configuration for an Ollama model
type OllamaConfig struct {
	// Host is the Ollama server endpoint (e.g., "http://localhost:11434")
	Host string
	
	// ModelName is the name of the Ollama model (e.g., "llama2", "llama3")
	ModelName string
}

// NewOllamaConfig creates a new OllamaConfig with the specified host and model name
func NewOllamaConfig(host, modelName string) *OllamaConfig {
	// Ensure host doesn't have trailing slash
	host = strings.TrimSuffix(host, "/")
	
	return &OllamaConfig{
		Host:      host,
		ModelName: modelName,
	}
}

// DefaultLLMFactory implements the LLMFactory interface
type DefaultLLMFactory struct{}

// LLMFactory defines a simple interface for creating LLM instances.
// This maintains compatibility with existing code while allowing for configuration.
type LLMFactory interface {
	// CreateLLM creates a new LLM instance.
	CreateLLM(apiKey string, modelIDOrConfig interface{}) (core.LLM, error)
}

// NewLLM creates a new LLM instance based on the provided model ID or config.
// This function can accept either a core.ModelID or an OllamaConfig.
func NewLLM(apiKey string, modelIDOrConfig interface{}) (core.LLM, error) {
	factory := &DefaultLLMFactory{}
	return factory.CreateLLM(apiKey, modelIDOrConfig)
}

// Implement the LLMFactory interface.
func (f *DefaultLLMFactory) CreateLLM(apiKey string, modelIDOrConfig interface{}) (core.LLM, error) {
	var llm core.LLM
	var err error

	// Handle different types of model identifiers
	switch config := modelIDOrConfig.(type) {
	case *OllamaConfig:
		// Handle OllamaConfig directly
		llm, err = NewOllamaLLM(config.Host, config.ModelName)
	
	case *OpenRouterConfig:
		// Handle OpenRouterConfig directly
		llm, err = NewOpenRouterLLM(apiKey, config.ModelName)
	
	case core.ModelID:
		// Handle standard model IDs
		modelID := config
		switch {
		case modelID == core.ModelAnthropicHaiku || modelID == core.ModelAnthropicSonnet || modelID == core.ModelAnthropicOpus:
			llm, err = NewAnthropicLLM(apiKey, anthropic.ModelID(modelID))
			
		case modelID == core.ModelGoogleGeminiFlash || modelID == core.ModelGoogleGeminiPro || 
			modelID == core.ModelGoogleGeminiFlashThinking || modelID == core.ModelGoogleGeminiFlashLite:
			llm, err = NewGeminiLLM(apiKey, modelID)
			
		case strings.HasPrefix(string(modelID), "ollama:"):
			// Support legacy format for backward compatibility
			parts := strings.SplitN(string(modelID), ":", 2)
			if len(parts) != 2 || parts[1] == "" {
				return nil, fmt.Errorf("invalid Ollama model ID format. Use 'ollama:<model_name>'")
			}
			llm, err = NewOllamaLLM("http://localhost:11434", parts[1])
			
		case strings.HasPrefix(string(modelID), "llamacpp:"):
			return NewLlamacppLLM("http://localhost:8080")
			
		case strings.HasPrefix(string(modelID), "openrouter:"):
			// Support OpenRouter model format
			parts := strings.SplitN(string(modelID), ":", 2)
			if len(parts) != 2 || parts[1] == "" {
				return nil, fmt.Errorf("invalid OpenRouter model ID format. Use 'openrouter:<model_name>'")
			}
			llm, err = NewOpenRouterLLM(apiKey, parts[1])
			
		default:
			return nil, fmt.Errorf("unsupported model ID: %s", modelID)
		}
		
	default:
		return nil, fmt.Errorf("unsupported model configuration type: %T", modelIDOrConfig)
	}

	if err != nil {
		return nil, err
	}
	
	return core.Chain(llm,
		func(l core.LLM) core.LLM { return core.NewModelContextDecorator(l) },
	), nil
}
