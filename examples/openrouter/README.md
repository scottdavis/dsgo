# OpenRouter Example

This example demonstrates how to use the OpenRouter integration with dsgo.

## What is OpenRouter?

[OpenRouter](https://openrouter.ai) is a unified API that provides access to hundreds of AI models through a single endpoint. It automatically handles fallbacks and selects the most cost-effective options for your use case.

## Setup

1. Sign up for an account at [OpenRouter](https://openrouter.ai) and get an API key.
2. Set your API key as an environment variable:
   ```bash
   export OPENROUTER_API_KEY=your_api_key_here
   ```

## Running the Example

```bash
go run examples/openrouter/main.go
```

## Available Models

OpenRouter supports a wide range of models from different providers, including:

- Anthropic (Claude 3 Opus, Claude 3 Sonnet, Claude 3 Haiku)
- OpenAI (GPT-4o, GPT-4)
- Google (Gemini)
- Meta (Llama 3)
- Mistral AI
- And many more

For a complete list of available models, see the [OpenRouter Models page](https://openrouter.ai/models).

## Using the OpenRouter Integration

### Method 1: Using OpenRouterConfig

```go
// Create an OpenRouter config with a specific model
config := llms.NewOpenRouterConfig("anthropic/claude-3-opus-20240229")

// Create the LLM using the factory
llm, err := llms.NewLLM(apiKey, config)
```

### Method 2: Using ModelID String Format

```go
// Use the string format "openrouter:<model_name>"
modelID := core.ModelID("openrouter:anthropic/claude-3-haiku-20240307")

// Create the LLM using the factory with the string format
llm, err := llms.NewLLM(apiKey, modelID)
```

Both methods provide the same functionality, choose whichever is more convenient for your use case. 