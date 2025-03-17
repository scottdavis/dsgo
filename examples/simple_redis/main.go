package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/scottdavis/dsgo/pkg/agents"
	"github.com/scottdavis/dsgo/pkg/agents/memory"
	"github.com/scottdavis/dsgo/pkg/agents/workflows"
	"github.com/scottdavis/dsgo/pkg/core"
	"github.com/scottdavis/dsgo/pkg/llms"
	"github.com/scottdavis/dsgo/pkg/logging"
)

func main() {
	// Parse command line flags
	redisAddr := flag.String("redis-addr", "localhost:6379", "Redis server address")
	redisPassword := flag.String("redis-password", "", "Redis password")
	redisDB := flag.Int("redis-db", 0, "Redis database number")
	url := flag.String("url", "https://raw.githubusercontent.com/scottdavis/dsgo/main/README.md", "URL to download")
	flag.Parse()

	// Setup DSP-GO logging
	output := logging.NewConsoleOutput(true, logging.WithColor(true))
	logger := logging.NewLogger(logging.Config{
		Severity: logging.INFO,
		Outputs:  []logging.Output{output},
	})
	logging.SetLogger(logger)

	// Create basic context for LLM initialization
	basicCtx := context.Background()
	
	// Initialize LLM
	// Create an Ollama config specifically for llama3
	ollamaConfig := llms.NewOllamaConfig("http://localhost:11434", "llama3")
	llm, err := llms.NewLLM("", ollamaConfig)
	if err != nil {
		logger.Error(basicCtx, "Failed to initialize LLM: %v", err)
		os.Exit(1)
	}

	// Create proper DSP-GO context with execution state
	ctx := core.WithExecutionState(basicCtx)
	logger.Info(ctx, "=============== DSP-GO Redis Workflow Example ===============")
	logger.Info(ctx, "Using Ollama llama3 model from localhost")
	logger.Info(ctx, "Connecting to Redis at %s...", *redisAddr)

	// Create a Redis store
	store, err := memory.NewRedisStore(*redisAddr, *redisPassword, *redisDB)
	if err != nil {
		logger.Error(ctx, "Failed to connect to Redis: %v", err)
		logger.Info(ctx, "Is Redis running? Try: docker run -d --name redis-example -p 6379:6379 redis:latest")
		os.Exit(1)
	}
	defer store.Close()
	logger.Info(ctx, "Connected to Redis successfully")

	// Clear any existing data
	if err := store.Clear(); err != nil {
		logger.Error(ctx, "Failed to clear Redis store: %v", err)
		os.Exit(1)
	}
	logger.Info(ctx, "Redis store cleared")

	// Step 1: Download content from URL
	logger.Info(ctx, "\n=== Step 1: Downloading content from URL ===")
	content, err := downloadURL(*url)
	if err != nil {
		logger.Error(ctx, "Failed to download content: %v", err)
		os.Exit(1)
	}
	logger.Info(ctx, "Successfully downloaded %d bytes from %s", len(content), *url)

	// Step 2: Setting up NLP workflow with Redis memory
	logger.Info(ctx, "\n=== Step 2: Setting up NLP workflow with Redis memory ===")
	
	// Create a workflow with Redis store for shared memory
	workflow := workflows.NewChainWorkflow(store)
	
	// Add a content processing step that summarizes the text
	summaryStep := createSummaryStep(llm)
	if err := workflow.AddStep(summaryStep); err != nil {
		logger.Error(ctx, "Failed to add summary step: %v", err)
		os.Exit(1)
	}
	
	// Add a translation step that translates the summary to Spanish
	translationStep := createTranslationStep(llm)
	if err := workflow.AddStep(translationStep); err != nil {
		logger.Error(ctx, "Failed to add translation step: %v", err)
		os.Exit(1)
	}
	
	// Execute the workflow
	logger.Info(ctx, "Executing workflow...")
	result, err := workflow.Execute(ctx, map[string]any{
		"content": content,  // Pass downloaded content directly to the workflow
	})
	if err != nil {
		logger.Error(ctx, "Failed to execute workflow: %v", err)
		os.Exit(1)
	}

	// Display workflow results
	logger.Info(ctx, "\n=== Workflow Results ===")
	logger.Info(ctx, "English Summary:")
	logger.Info(ctx, "%s", result["summary"])
	logger.Info(ctx, "\nSpanish Translation:")
	logger.Info(ctx, "%s", result["translation"])
	
	// Get all keys in Redis to show what was stored during workflow
	keys, err := store.List()
	if err != nil {
		logger.Error(ctx, "Failed to list Redis keys: %v", err)
		os.Exit(1)
	}
	logger.Info(ctx, "\nRedis Keys Created During Workflow:")
	for _, key := range keys {
		logger.Info(ctx, "  - %s", key)
	}

	// Step 4: Demonstrate TTL functionality
	logger.Info(ctx, "\n=== Step 3: Demonstrating TTL functionality ===")
	
	// Generate a unique key with workflow and step ID
	tempKey := fmt.Sprintf("workflow:temp:%s", time.Now().Format(time.RFC3339))
	
	// Store the summary with TTL
	summary, ok := result["summary"].(string)
	if !ok {
		logger.Error(ctx, "Failed to convert summary to string")
		os.Exit(1)
	}
	
	// Note that users don't need to worry about the key - the WithTTL option is what matters
	err = store.Store(tempKey, summary, agents.WithTTL(5*time.Second))
	if err != nil {
		logger.Error(ctx, "Failed to store summary with TTL: %v", err)
		os.Exit(1)
	}
	logger.Info(ctx, "Stored summary with 5-second TTL")
	
	// Verify content is stored - users don't need to know the key
	_, err = store.Retrieve(tempKey)
	if err != nil {
		logger.Error(ctx, "Failed to retrieve temporary summary: %v", err)
		os.Exit(1)
	}
	logger.Info(ctx, "Temporary data is available in the store")
	
	// Wait for TTL to expire
	logger.Info(ctx, "Waiting for 6 seconds for TTL to expire...")
	time.Sleep(6 * time.Second)
	
	// Try to retrieve expired content - again, in a real workflow users wouldn't
	// need to worry about this specific key
	_, err = store.Retrieve(tempKey)
	if err != nil {
		logger.Info(ctx, "As expected, the temporary data has expired after TTL period")
	} else {
		logger.Info(ctx, "Unexpected: temporary data is still available")
	}

	// Step 5: Clean up
	logger.Info(ctx, "\n=== Step 4: Cleaning up ===")
	err = store.Clear()
	if err != nil {
		logger.Error(ctx, "Failed to clear Redis store: %v", err)
		os.Exit(1)
	}
	logger.Info(ctx, "Successfully cleared Redis store")
	
	logger.Info(ctx, "\nDSP-GO Redis Workflow example completed successfully")
	logger.Info(ctx, "===============================================")
}

// downloadURL fetches content from a given URL and returns it as a string
func downloadURL(url string) (string, error) {
	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 10 * time.Second,
	}
	
	// Make the request
	resp, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to GET URL: %w", err)
	}
	defer resp.Body.Close()
	
	// Check if the response was successful
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP request failed with status: %s", resp.Status)
	}
	
	// Read the body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}
	
	return string(body), nil
}

// SummaryModule summarizes content using LLM
type SummaryModule struct {
	signature core.Signature
	llm       core.LLM
}

// NewSummaryModule creates a new summary module with the given LLM
func NewSummaryModule(signature core.Signature, llm core.LLM) *SummaryModule {
	return &SummaryModule{
		signature: signature,
		llm:       llm,
	}
}

// Process implements the core.Module interface
func (m *SummaryModule) Process(ctx context.Context, inputs map[string]any, opts ...core.Option) (map[string]any, error) {
	// Extract content from inputs
	content, ok := inputs["content"].(string)
	if !ok {
		return nil, fmt.Errorf("content field is required and must be a string")
	}
	
	// Create a prompt for summarization
	prompt := fmt.Sprintf("Please summarize the following content in 2-3 paragraphs:\n\n%s", content)
	
	// Generate summary using LLM
	response, err := m.llm.Generate(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("failed to generate summary: %w", err)
	}
	
	// Store summary in Redis
	mem, err := memory.GetMemoryStore(ctx)
	if err != nil {
		return nil, err
	}
	
	// Create a unique key for the summary
	summaryKey := fmt.Sprintf("workflow:summary:%s", time.Now().Format(time.RFC3339))
	
	// Store the summary
	if err := mem.Store(summaryKey, response.Content); err != nil {
		return nil, fmt.Errorf("failed to store summary: %w", err)
	}
	
	// Return the summary as output
	return map[string]any{
		"summary": response.Content,
	}, nil
}

// GetSignature implements the core.Module interface
func (m *SummaryModule) GetSignature() core.Signature {
	return m.signature
}

// Clone implements the core.Module interface
func (m *SummaryModule) Clone() core.Module {
	return &SummaryModule{
		signature: m.signature,
		llm:       m.llm,
	}
}

// TranslationModule translates content to Spanish using LLM
type TranslationModule struct {
	signature core.Signature
	llm       core.LLM
}

// NewTranslationModule creates a new translation module with the given LLM
func NewTranslationModule(signature core.Signature, llm core.LLM) *TranslationModule {
	return &TranslationModule{
		signature: signature,
		llm:       llm,
	}
}

// Process implements the core.Module interface
func (m *TranslationModule) Process(ctx context.Context, inputs map[string]any, opts ...core.Option) (map[string]any, error) {
	// Extract summary from inputs
	summary, ok := inputs["summary"].(string)
	if !ok {
		return nil, fmt.Errorf("summary field is required and must be a string")
	}
	
	// Create a prompt for translation
	prompt := fmt.Sprintf("Translate the following English text to Spanish:\n\n%s", summary)
	
	// Generate translation using LLM
	response, err := m.llm.Generate(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("failed to generate translation: %w", err)
	}
	
	// Clean up the translation (remove potential prefixes like "Spanish:" or "Translation:")
	translation := cleanTranslationOutput(response.Content)
	
	// Store translation in Redis
	mem, err := memory.GetMemoryStore(ctx)
	if err != nil {
		return nil, err
	}
	
	// Create a unique key for the translation
	translationKey := fmt.Sprintf("workflow:translation:%s", time.Now().Format(time.RFC3339))
	
	// Store the translation
	if err := mem.Store(translationKey, translation); err != nil {
		return nil, fmt.Errorf("failed to store translation: %w", err)
	}
	
	// Return the translation as output
	return map[string]any{
		"translation": translation,
	}, nil
}

// GetSignature implements the core.Module interface
func (m *TranslationModule) GetSignature() core.Signature {
	return m.signature
}

// Clone implements the core.Module interface
func (m *TranslationModule) Clone() core.Module {
	return &TranslationModule{
		signature: m.signature,
		llm:       m.llm,
	}
}

// Helper function to clean translation output from common prefixes
func cleanTranslationOutput(translation string) string {
	// Remove common prefixes that LLMs might add
	prefixes := []string{
		"Translation:", "Spanish:", "Spanish Translation:", 
		"Traducción:", "En español:", "Translated text:",
	}
	
	result := strings.TrimSpace(translation)
	for _, prefix := range prefixes {
		if strings.HasPrefix(strings.ToLower(result), strings.ToLower(prefix)) {
			result = strings.TrimSpace(result[len(prefix):])
			break
		}
	}
	
	return result
}

// createSummaryStep creates a workflow step that summarizes content
func createSummaryStep(llm core.LLM) *workflows.Step {
	// Create signature for summarization
	signature := core.NewSignature(
		[]core.InputField{
			{Field: core.Field{Name: "content", Description: "Content to summarize"}},
		},
		[]core.OutputField{
			{Field: core.Field{Name: "summary", Description: "Summarized content"}},
		},
	).WithInstruction("Create a concise summary of the content in 2-3 paragraphs")
	
	// Create a custom module with explicit LLM
	module := NewSummaryModule(signature, llm)
	
	return &workflows.Step{
		ID:     "summarize",
		Module: module,
	}
}

// createTranslationStep creates a workflow step that translates content to Spanish
func createTranslationStep(llm core.LLM) *workflows.Step {
	// Create signature for translation
	signature := core.NewSignature(
		[]core.InputField{
			{Field: core.Field{Name: "summary", Description: "English summary to translate"}},
		},
		[]core.OutputField{
			{Field: core.Field{Name: "translation", Description: "Spanish translation"}},
		},
	).WithInstruction("Translate the English summary into Spanish")
	
	// Create a custom module with explicit LLM
	module := NewTranslationModule(signature, llm)
	
	return &workflows.Step{
		ID:     "translate",
		Module: module,
	}
} 