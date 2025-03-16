package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/XiaoConstantine/dspy-go/pkg/agents"
	"github.com/XiaoConstantine/dspy-go/pkg/core"
	"github.com/XiaoConstantine/dspy-go/pkg/modules"

	workflows "github.com/XiaoConstantine/dspy-go/pkg/agents/workflows"
	"github.com/XiaoConstantine/dspy-go/pkg/llms"
	"github.com/XiaoConstantine/dspy-go/pkg/logging"
)

func RunChainExample(ctx context.Context, logger *logging.Logger, dspyConfig *core.DSPYConfig) {
	logger.Info(ctx, "============ Example 1: Chain workflow for structured data extraction and formatting ==============")
	dataWorkflow, err := CreateDataProcessingWorkflow(dspyConfig)
	if err != nil {
		logger.Error(ctx, "Failed to create data workflow: %v", err)
	}

	report := `Q3 Performance Summary:Our customer satisfaction score rose to 92 points this quarter.Revenue grew by 45% compared to last year.Market share is now at 23% in our primary market.Customer churn decreased to 5% from 8%.New user acquisition cost is $43 per user.Product adoption rate increased to 78%.Employee satisfaction is at 87 points.Operating margin improved to 34%.`
	result, err := dataWorkflow.Execute(ctx, map[string]any{
		"raw_text": report,
	})
	if err != nil {
		logger.Error(ctx, "Data processing failed: %v", err)
	}
	// Format the output nicely
	if table, ok := result["markdown_table"].(string); ok {
		// clean up any extra whitespace
		lines := strings.Split(strings.TrimSpace(table), "\n")
		for _, line := range lines {
			logger.Info(ctx, "%s", strings.TrimSpace(line))
		}
	} else {
		logger.Error(ctx, "invalid table format in result")
	}
	logger.Info(ctx, "================================================================================")
}

func RunParallelExample(ctx context.Context, logger *logging.Logger, config *core.DSPYConfig) {
	logger.Info(ctx, "============ Example 2: Parallelization workflow for stakeholder impact analysis ==============")
	stakeholders := []string{
		`customers:
	       - price sensitive
	       - want better tech
	       - environmental concerns`,

		`employees:
	       - job security worries
	       - need new skills
	       - want clear direction`,

		`investors:
	       - expect growth
	       - want cost control
	       - risk concerns`,

		`suppliers:
	       - capacity constraints
	       - price pressures
	       - tech transitions`,
	}
	workflow, inputs, err := CreateParallelWorkflow(stakeholders, config)
	if err != nil {
		logger.Error(ctx, "Failed to create parallel workflow: %v", err)
		return
	}
	logger.Info(ctx, "Inputs: %v", inputs)
	results, err := workflow.Execute(ctx, inputs)
	if err != nil {
		logger.Error(ctx, "Workflow execution failed: %v", err)
	}
	// Print results
	for i := range stakeholders {
		analysisKey := fmt.Sprintf("analyze_stakeholder_%d_analysis", i)
		if analysis, ok := results[analysisKey]; ok {
			fmt.Printf("\n=== Stakeholder Analysis %d ===\n", i+1)
			fmt.Println(analysis)
		}
	}
	logger.Info(ctx, "=================================================")
}

func RunRouteExample(ctx context.Context, logger *logging.Logger, dspyConfig *core.DSPYConfig) {
	logger.Info(ctx, "============ Example 3: Route workflow for customer support ticket handling ==============")

	supportRoutes := map[string]string{
		"billing": `You are a billing support specialist. Follow these guidelines:
            1. Always start with "Billing Support Response:"
            2. First acknowledge the specific billing issue
            3. Explain any charges or discrepancies clearly
            4. List concrete next steps with timeline
            5. End with payment options if relevant
            
            Keep responses professional but friendly.,
	    Input:`,

		"technical": `You are a technical support engineer. Follow these guidelines:
            1. Always start with "Technical Support Response:"
            2. List exact steps to resolve the issue
            3. Include system requirements if relevant
            4. Provide workarounds for common problems
            5. End with escalation path if needed
            
            Use clear, numbered steps and technical details.
		Input:`,
		"account": `You are an account security specialist. Follow these guidelines:
		1. Always start with "Account Support Response:"
		2. Prioritize account security and verification
	        3. Provide clear steps for account recovery/changes
	        4. Include security tips and warnings
		5. Set clear expectations for resolution time
    
		Maintain a serious, security-focused tone.
		Input:`,
		"product": `You are a product specialist. Follow these guidelines:
    1. Always start with "Product Support Response:"
    2. Focus on feature education and best practices
    3. Include specific examples of usage
    4. Link to relevant documentation sections
    5. Suggest related features that might help
    
    Be educational and encouraging in tone.
		Input:`,
	}
	tickets := []string{
		`Subject: Can't access my account
        Message: Hi, I've been trying to log in for the past hour but keep getting an 'invalid password' error. 
        I'm sure I'm using the right password. Can you help me regain access? This is urgent as I need to 
        submit a report by end of day.
        - John`,
		`Subject: Unexpected charge on my card
    Message: Hello, I just noticed a charge of $49.99 on my credit card from your company, but I thought
    I was on the $29.99 plan. Can you explain this charge and adjust it if it's a mistake?
    Thanks,
    Sarah`,
		`Subject: How to export data?
		Message: I need to export all my project data to Excel. I've looked through the docs but can't
		figure out how to do a bulk export. Is this possible? If so, could you walk me through the steps?:
		Best regards,
		Mike`,
	}
	router := CreateRouterWorkflow(dspyConfig)

	for routeType, prompt := range supportRoutes {
		routeStep := CreateHandlerStep(routeType, prompt, dspyConfig)
		if err := router.AddStep(routeStep); err != nil {
			logger.Error(ctx, "Failed to add step %s: %v", routeType, err)
			continue
		}
		if err := router.AddRoute(routeType, []*workflows.Step{routeStep}); err != nil {
			logger.Error(ctx, "Failed to add route %s: %v", routeType, err)
		}
	}
	logger.Info(ctx, "Processing support tickets...\n")
	for i, ticket := range tickets {
		logger.Info(ctx, "\nTicket %d:\n", i+1)
		logger.Info(ctx, "%s", strings.Repeat("-", 40))
		logger.Info(ctx, "%s", ticket)
		logger.Info(ctx, "\nResponse:")
		logger.Info(ctx, "%s", strings.Repeat("-", 40))

		response, err := router.Execute(ctx, map[string]interface{}{"input": ticket})
		if err != nil {
			logger.Info(ctx, "Error processing ticket %d: %v", i+1, err)
			continue
		}
		logger.Info(ctx, "%s", response["response"].(string))
	}
	logger.Info(ctx, "=================================================")
}

func RunEvalutorOptimizerExample(ctx context.Context, logger *logging.Logger, config *core.DSPYConfig) {
	// Create signature for our task - simpler now that we don't need dataset support
	signature := core.NewSignature(
		[]core.InputField{
			{Field: core.Field{Name: "task"}},
		},
		[]core.OutputField{
			{Field: core.Field{Name: "solution", Prefix: "solution"}},
		},
	).WithInstruction(`
        Your task is to implement the requested data structure.
        Consider time and space complexity.
        Provide a complete, correct implementation in Go.
        If context is provided, learn from previous attempts and feedback.
    `)

	// Create predict module with the signature
	predict := modules.NewPredict(signature, config)

	// Create program that uses predict module
	program := core.NewProgram(
		map[string]core.Module{"predict": predict},
		func(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
			return predict.Process(ctx, inputs)
		},
		config,
	)

	// Create evaluation metric that matches Python implementation
	metric := func(ctx context.Context, result map[string]interface{}) (bool, string) {
		solution, ok := result["solution"].(string)
		if !ok {
			return false, "No solution provided"
		}

		// Check implementation quality
		issues := make([]string, 0)

		if !strings.Contains(solution, "type MinStack struct") {
			issues = append(issues, "Missing MinStack struct definition")
		}

		if !strings.Contains(solution, "min []int") && !strings.Contains(solution, "minStack []int") {
			issues = append(issues, "Need a way to track minimum values")
		}

		if strings.Contains(solution, "for") || strings.Contains(solution, "range") {
			issues = append(issues, "Operations should be O(1) - avoid loops")
		}

		if !strings.Contains(solution, "func (s *MinStack) GetMin()") {
			issues = append(issues, "Missing GetMin() method")
		}

		// Pass only if no issues found
		if len(issues) == 0 {
			return true, "Implementation meets all requirements"
		}

		return false, strings.Join(issues, ". ")
	}

	ctx = core.WithExecutionState(ctx)
	// Create and use PromptingOptimizer
	optimizer := NewPromptingOptimizer(metric, 5) // Allow 5 attempts

	logger.Info(ctx, "Starting optimization process...")
	optimizedProgram, err := optimizer.Compile(ctx, program, nil, nil)
	if err != nil {
		logger.Error(ctx, "Optimization failed: %v", err)
		return
	}

	// Test the optimized program
	result, err := optimizedProgram.Execute(ctx, map[string]interface{}{
		"task": "Implement MinStack with O(1) operations for push, pop, and getMin",
	})
	if err != nil {
		logger.Error(ctx, "Final execution failed: %v", err)
		return
	}

	if solution, ok := result["solution"].(string); ok {
		logger.Info(ctx, "Final MinStack Implementation:\n%s", solution)
	} else {
		logger.Error(ctx, "No solution in final result")
	}
}

func RunOrchestratorExample(ctx context.Context, logger *logging.Logger, dspyConfig *core.DSPYConfig) {
	// Provide a more detailed XML example to help the LLM generate correct format
	xmlFormat := `Format the tasks section in XML with the following structure EXACTLY as shown:
    <tasks>
        <task id="task_1" type="analysis" processor="general" priority="1">
            <description>Analyze input data</description>
            <dependencies></dependencies>
            <metadata>
                <item key="resource">cpu</item>
            </metadata>
        </task>
        <task id="task_2" type="formatting" processor="general" priority="2">
            <description>Format results into report</description>
            <dependencies>
                <dependency>task_1</dependency>
            </dependencies>
            <metadata>
                <item key="output">report</item>
            </metadata>
        </task>
    </tasks>
    
    Keep your XML well-formed with proper closing tags for every opening tag.
    Every task MUST have an id, type, and processor attribute.`

	parser := &agents.XMLTaskParser{
		RequiredFields: []string{"id", "type", "processor"},
	}

	planner := agents.NewDependencyPlanCreator(5) // Max 5 tasks per phase

	// Define streaming handler
	streamHandler := func(chunk core.StreamChunk) error {
		logger := logging.GetLogger()
		ctx := context.Background()
		if chunk.Error != nil {
			logger.Info(ctx, "Error: %v\n", chunk.Error)
			return chunk.Error
		}

		if chunk.Done {
			logger.Info(ctx, "\n[DONE]")
			return nil
		}

		// Display chunks as they arrive
		logger.Info(ctx, "Get chunk: %v", chunk.Content)
		return nil
	}

	// Create orchestrator configuration (separate from DSPYConfig)
	orchConfig := agents.OrchestrationConfig{
		MaxConcurrent:  3,
		DefaultTimeout: 30 * time.Second,
		RetryConfig: &agents.RetryConfig{
			MaxAttempts:       3,
			BackoffMultiplier: 2.0,
		},
		// Use custom parser and planner
		TaskParser:  parser,
		PlanCreator: planner,
		// Add your custom processors
		CustomProcessors: map[string]agents.TaskProcessor{
			"general": &ExampleProcessor{},
		},
		AnalyzerConfig: agents.AnalyzerConfig{
			FormatInstructions: xmlFormat,
			Considerations: []string{
				"Task dependencies and optimal execution order",
				"Opportunities for parallel execution",
				"Required processor types for each task",
				"Task priorities and resource requirements",
			},
		},
		Options: core.WithOptions(
			core.WithGenerateOptions(
				core.WithTemperature(0.2), // Lower temperature for more precise XML generation
				core.WithMaxTokens(8192),
			),
			core.WithStreamHandler(streamHandler),
		),
	}

	// Create orchestrator
	orchestrator := agents.NewFlexibleOrchestrator(agents.NewInMemoryStore(), orchConfig, dspyConfig)

	// The analyzer will return tasks in XML format that our parser understands
	task := "Create a summary report of quarterly sales data, identifying top products and regional performance trends"
	contextData := map[string]any{
		"key": "value",
	}

	// Process the task
	ctx = core.WithExecutionState(ctx)
	result, err := orchestrator.Process(ctx, task, contextData)
	if err != nil {
		logger.Error(ctx, "Orchestration failed: %v", err)
		// Safe handling of results to avoid nil pointer panic
		if result != nil {
			// Log more details about failed tasks
			for taskID, taskErr := range result.FailedTasks {
				logger.Error(ctx, "Task %s failed: %v", taskID, taskErr)
			}

			// Log successful tasks with details
			for taskID, taskResult := range result.CompletedTasks {
				logger.Info(ctx, "Task %s completed successfully with result: %v", taskID, taskResult)
			}

			// Handle results
			logger.Info(ctx, "Orchestration completed with %d successful tasks and %d failures",
				len(result.CompletedTasks), len(result.FailedTasks))
		} else {
			logger.Error(ctx, "No result returned from orchestrator")
		}
		return
	}

	// Only try to access results if there was no error
	// Log successful tasks with details
	for taskID, taskResult := range result.CompletedTasks {
		logger.Info(ctx, "Task %s completed successfully with result: %v", taskID, taskResult)
	}

	// Handle results
	logger.Info(ctx, "Orchestration completed with %d successful tasks and %d failures",
		len(result.CompletedTasks), len(result.FailedTasks))
}

func main() {
	output := logging.NewConsoleOutput(true, logging.WithColor(true))

	fileOutput, err := logging.NewFileOutput(
		filepath.Join(".", "dspy.log"),
		logging.WithRotation(10*1024*1024, 5), // 10MB max size, keep 5 files
		logging.WithJSONFormat(true),          // Use JSON format
	)
	if err != nil {
		fmt.Printf("Failed to create file output: %v\n", err)
		os.Exit(1)
	}
	logger := logging.NewLogger(logging.Config{
		Severity: logging.INFO,
		Outputs:  []logging.Output{output, fileOutput},
	})
	logging.SetLogger(logger)
	//apiKey := flag.String("api-key", "", "Anthropic API Key")

	ctx := core.WithExecutionState(context.Background())
	logger.Info(ctx, "Starting application")
	logger.Debug(ctx, "This is a debug message")
	logger.Warn(ctx, "This is a warning message")

	// Create an Ollama configuration with custom host and model
	//ollamaConfig := llms.NewOllamaConfig("http://192.168.1.199:11434", "gemma3:27b")

	// Get API key from environment
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		fmt.Println("Please set OPENROUTER_API_KEY environment variable")
		os.Exit(1)
	}

	// Example 1: Using OpenRouterConfig
	fmt.Println("=== Example 1: Using OpenRouterConfig ===")
	// Create a model ID string with the openrouter prefix
	modelID := core.ModelID("openrouter:deepseek/deepseek-r1:free")

	// Create the LLM directly using the model ID
	llm, err := llms.NewLLM(apiKey, modelID)
	if err != nil {
		fmt.Printf("Error creating LLM: %v\n", err)
		os.Exit(1)
	}

	// Create DSPYConfig with the LLM
	dspyConfig := core.NewDSPYConfig().WithDefaultLLM(llm)

	// Pass the config to all example functions
	RunChainExample(ctx, logger, dspyConfig)
	RunParallelExample(ctx, logger, dspyConfig)
	RunRouteExample(ctx, logger, dspyConfig)
	RunEvalutorOptimizerExample(ctx, logger, dspyConfig)
	RunOrchestratorExample(ctx, logger, dspyConfig)
}

func CreateClassifierStep(dspyConfig *core.DSPYConfig) *workflows.Step {
	// Create a signature for classification that captures reasoning and selection
	signature := core.NewSignature(
		[]core.InputField{
			{Field: core.Field{Name: "input"}},
		},
		[]core.OutputField{
			{Field: core.Field{Name: "reasoning", Prefix: "reasoning"}},
			{Field: core.Field{Name: "selection", Prefix: "selection"}},
			{Field: core.Field{Name: "classification", Prefix: "classification"}}, // Required by RouterWorkflow
		},
	).WithInstruction(`Analyze the input and select the most appropriate support team.
    First explain your reasoning, then provide your selection.
    You must classify the ticket into exactly one of these categories: "billing", "technical", "account", or "product".
    Do not use any other classification values.`)

	// Create a specialized predict module that formats the response correctly
	predictModule := modules.NewPredict(signature, dspyConfig)

	return &workflows.Step{
		ID:     "support_classifier",
		Module: predictModule,
	}
}

func CreateHandlerStep(routeType string, prompt string, dspyConfig *core.DSPYConfig) *workflows.Step {
	// Create signature for handling tickets
	signature := core.NewSignature(
		[]core.InputField{
			{Field: core.Field{Name: "input"}},
		},
		[]core.OutputField{
			{Field: core.Field{Name: "response"}},
		},
	).WithInstruction(prompt)

	return &workflows.Step{
		ID:     fmt.Sprintf("%s_handler", routeType),
		Module: modules.NewPredict(signature, dspyConfig),
	}
}

func CreateRouterWorkflow(dspyConfig *core.DSPYConfig) *workflows.RouterWorkflow {
	routerWorkflow := workflows.NewRouterWorkflow(agents.NewInMemoryStore(), CreateClassifierStep(dspyConfig))
	return routerWorkflow
}

func CreateParallelWorkflow(stakeholders []string, dspyConfig *core.DSPYConfig) (*workflows.ParallelWorkflow, map[string]interface{}, error) {
	// Create a new parallel workflow with in-memory storage
	workflow := workflows.NewParallelWorkflow(agents.NewInMemoryStore(), 3)
	inputs := make(map[string]interface{})

	for i, stakeholder := range stakeholders {
		step := &workflows.Step{
			ID:     fmt.Sprintf("analyze_stakeholder_%d", i),
			Module: NewStakeholderAnalysis(i, dspyConfig),
		}

		if err := workflow.AddStep(step); err != nil {
			return nil, nil, fmt.Errorf("failed to add step: %v", err)
		}
		inputKey := fmt.Sprintf("analyze_stakeholder_%d_stakeholder_info", i)
		logging.GetLogger().Info(context.Background(), "input key: %s", inputKey)

		inputs[inputKey] = stakeholder
	}

	return workflow, inputs, nil
}

func NewStakeholderAnalysis(index int, dspyConfig *core.DSPYConfig) core.Module {
	signature := core.NewSignature(
		[]core.InputField{
			{Field: core.Field{Name: fmt.Sprintf("analyze_stakeholder_%d_stakeholder_info", index)}},
		},
		[]core.OutputField{
			{Field: core.Field{Name: "analysis", Prefix: "analysis"}},
		},
	).WithInstruction(`Analyze how market changes will impact this stakeholder group.
        Provide specific impacts and recommended actions.
        Format with clear sections and priorities.`)

	return modules.NewPredict(signature, dspyConfig)
}

func CreateDataProcessingWorkflow(dspyConfig *core.DSPYConfig) (*workflows.ChainWorkflow, error) {
	// Create a new chain workflow with in-memory storage
	workflow := workflows.NewChainWorkflow(agents.NewInMemoryStore())

	// Step 1: Extract numerical values
	extractSignature := core.NewSignature(
		[]core.InputField{{Field: core.Field{Name: "raw_text"}}},
		[]core.OutputField{{Field: core.Field{Name: "extracted_values", Prefix: "extracted_values:"}}},
	).WithInstruction(`Extract only the numerical values and their associated metrics from the text.
        Format each as 'value: metric' on a new line.
		Example: extracted_values:
        92: customer satisfaction
        45%: revenue growth`)
	extractStep := &workflows.Step{
		ID:     "extract_numbers",
		Module: modules.NewPredict(extractSignature, dspyConfig),
	}

	// Step 2: Standardize to percentages
	standardizeSignature := core.NewSignature(
		[]core.InputField{{Field: core.Field{Name: "extracted_values"}}},
		[]core.OutputField{{Field: core.Field{Name: "standardized_values", Prefix: "standardized_values:"}}},
	).WithInstruction(`Convert all numerical values to percentages where possible.
        If not a percentage or points, convert to decimal (e.g., 92 points -> 92%).
        Keep one number per line.
	Example format:
	standardized_values:
	92%: customer satisfaction
	87%: employee satisfaction
	`)

	standardizeStep := &workflows.Step{
		ID:     "standardize_values",
		Module: modules.NewPredict(standardizeSignature, dspyConfig),
	}

	// Step 3: Sort values
	sortSignature := core.NewSignature(
		[]core.InputField{{Field: core.Field{Name: "standardized_values"}}},
		[]core.OutputField{{Field: core.Field{Name: "sorted_values", Prefix: "sorted_values:"}}},
	).WithInstruction(`Sort all lines in descending order by numerical value.
                Keep the format 'value: metric' on each line.
		`)
	sortStep := &workflows.Step{
		ID:     "sort_values",
		Module: modules.NewPredict(sortSignature, dspyConfig),
	}

	// Step 4: Format as table
	tableSignature := core.NewSignature(
		[]core.InputField{{Field: core.Field{Name: "sorted_values"}}},
		[]core.OutputField{{Field: core.Field{Name: "markdown_table", Prefix: "markdown_table:"}}},
	).WithInstruction(`Format the sorted values as a markdown table with two columns:
    - First column: Metric
    - Second column: Value
Format example:
| Metric | Value |
|:--|--:|
|--------|-------|
| Customer Satisfaction | 92% |`)
	tableStep := &workflows.Step{
		ID:     "format_table",
		Module: modules.NewPredict(tableSignature, dspyConfig),
	}

	// Add steps to workflow
	if err := workflow.AddStep(extractStep); err != nil {
		return nil, fmt.Errorf("failed to add extract step: %w", err)
	}
	if err := workflow.AddStep(standardizeStep); err != nil {
		return nil, fmt.Errorf("failed to add standardize step: %w", err)
	}
	if err := workflow.AddStep(sortStep); err != nil {
		return nil, fmt.Errorf("failed to add sort step: %w", err)
	}
	if err := workflow.AddStep(tableStep); err != nil {
		return nil, fmt.Errorf("failed to add table step: %w", err)
	}

	return workflow, nil
}

// A simple optimizer following the pattern from the Python implementation.
type PromptingOptimizer struct {
	// Metric evaluates solution quality and returns PASS/FAIL with feedback
	Metric      func(ctx context.Context, solution map[string]interface{}) (pass bool, feedback string)
	MaxAttempts int
	Memory      []string // Track previous attempts like Python version
	Logger      *logging.Logger
}

func NewPromptingOptimizer(
	metric func(ctx context.Context, solution map[string]interface{}) (bool, string),
	maxAttempts int,
) *PromptingOptimizer {
	return &PromptingOptimizer{
		Metric:      metric,
		MaxAttempts: maxAttempts,
		Memory:      make([]string, 0),
		Logger:      logging.GetLogger(),
	}
}

func (o *PromptingOptimizer) Compile(
	ctx context.Context,
	program *core.Program,
	dataset core.Dataset, // Kept for interface compatibility but not used
	metric core.Metric, // Kept for interface compatibility but not used
) (*core.Program, error) {
	ctx, span := core.StartSpan(ctx, "PromptingOptimization")
	defer core.EndSpan(ctx)

	// Initial attempt without context
	o.Logger.Info(ctx, "Making initial attempt...")
	result, err := program.Execute(ctx, map[string]interface{}{
		"task": "Implement MinStack with O(1) operations",
	})
	if err != nil {
		span.WithError(err)
		return program, fmt.Errorf("initial attempt failed: %w", err)
	}

	// Store first attempt
	if solution, ok := result["solution"].(string); ok {
		o.Memory = append(o.Memory, solution)
		o.Logger.Debug(ctx, "Initial solution:\n%s", solution)
	}

	// Improvement loop
	for attempt := 0; attempt < o.MaxAttempts; attempt++ {
		attemptCtx, attemptSpan := core.StartSpan(ctx, fmt.Sprintf("Attempt_%d", attempt))

		// Evaluate current solution
		pass, feedback := o.Metric(attemptCtx, result)
		attemptSpan.WithAnnotation("feedback", feedback)
		attemptSpan.WithAnnotation("pass", pass)

		o.Logger.Info(ctx, "Attempt %d evaluation - Pass: %v, Feedback: %s", attempt, pass, feedback)

		if pass {
			o.Logger.Info(ctx, "Found satisfactory solution on attempt %d", attempt)
			core.EndSpan(attemptCtx)
			return program, nil
		}

		// Build context from memory like Python version
		context := "Previous attempts:\n"
		for _, m := range o.Memory {
			context += fmt.Sprintf("- %s\n", m)
		}
		context += fmt.Sprintf("\nFeedback: %s", feedback)

		// Try again with feedback
		result, err = program.Execute(attemptCtx, map[string]interface{}{
			"task":    "Implement MinStack with O(1) operations",
			"context": context,
		})
		if err != nil {
			core.EndSpan(attemptCtx)
			return program, fmt.Errorf("attempt %d failed: %w", attempt, err)
		}

		// Store this attempt
		if solution, ok := result["solution"].(string); ok {
			o.Memory = append(o.Memory, solution)
			o.Logger.Debug(ctx, "Attempt %d solution:\n%s", attempt, solution)
		}

		core.EndSpan(attemptCtx)
	}

	return program, fmt.Errorf("failed to find satisfactory solution in %d attempts", o.MaxAttempts)
}

// Example processor implementation
type ExampleProcessor struct{}

func (p *ExampleProcessor) Process(ctx context.Context, task agents.Task, taskContext map[string]interface{}) (interface{}, error) {
	// Create a logger to help us understand what's happening
	logger := logging.GetLogger()
	logger.Info(ctx, "Processing task: %s (Type: %s)", task.ID, task.Type)

	// Process different task types
	switch task.Type {
	case "analysis":
		return p.handleAnalysisTask(task, taskContext)
	case "formatting":
		return p.handleFormattingTask(task, taskContext)
	case "decomposition":
		return p.handleDecompositionTask(task, taskContext)
	default:
		// Instead of returning an error, let's handle any task type
		return p.handleGenericTask(task, taskContext)
	}
}

func (p *ExampleProcessor) handleAnalysisTask(task agents.Task, taskContext map[string]interface{}) (interface{}, error) {
	// Simulate analysis work
	result := map[string]interface{}{
		"status":       "completed",
		"analysis":     "Sample analysis of quarterly data",
		"top_products": []string{"Product A", "Product B", "Product C"},
		"trends": map[string]string{
			"north": "increasing",
			"south": "stable",
			"east":  "declining",
			"west":  "rapidly increasing",
		},
	}
	return result, nil
}

func (p *ExampleProcessor) handleDecompositionTask(task agents.Task, taskContext map[string]interface{}) (interface{}, error) {
	// Simulate decomposition work
	result := map[string]interface{}{
		"status":       "completed",
		"subtasks":     []string{"data collection", "data cleaning", "analysis", "reporting"},
		"dependencies": []string{"data availability", "team allocation"},
	}
	return result, nil
}

func (p *ExampleProcessor) handleFormattingTask(task agents.Task, taskContext map[string]interface{}) (interface{}, error) {
	// Check for dependencies in the task context
	var analysisResult map[string]interface{}

	// Look for analysis results from other tasks
	for taskID, result := range taskContext {
		if strings.Contains(taskID, "analysis") {
			if resultMap, ok := result.(map[string]interface{}); ok {
				analysisResult = resultMap
				break
			}
		}
	}

	// Generate report based on analysis or with placeholder data if not found
	report := ""
	if analysisResult != nil {
		report = "# Quarterly Sales Report\n\n"

		// Add top products if available
		if products, ok := analysisResult["top_products"].([]string); ok && len(products) > 0 {
			report += "## Top Performing Products\n"
			for i, product := range products {
				report += fmt.Sprintf("%d. %s\n", i+1, product)
			}
			report += "\n"
		}

		// Add trends if available
		if trends, ok := analysisResult["trends"].(map[string]string); ok && len(trends) > 0 {
			report += "## Regional Trends\n"
			for region, trend := range trends {
				report += fmt.Sprintf("- %s: %s\n", region, trend)
			}
		}
	} else {
		// If no analysis data, create a placeholder report
		report = "# Quarterly Sales Report\n\n"
		report += "## Overview\n"
		report += "This is a placeholder report as analysis data was not available.\n\n"
		report += "## Recommendations\n"
		report += "- Conduct detailed analysis\n"
		report += "- Gather more data\n"
		report += "- Schedule follow-up meeting\n"
	}

	// Return formatted report
	return map[string]interface{}{
		"status": "completed",
		"report": report,
		"format": "markdown",
	}, nil
}

func (p *ExampleProcessor) handleGenericTask(task agents.Task, taskContext map[string]interface{}) (interface{}, error) {
	// Handle any other task type
	result := map[string]interface{}{
		"status":     "completed",
		"task_id":    task.ID,
		"task_type":  task.Type,
		"message":    fmt.Sprintf("Processed generic task %s", task.ID),
		"created_at": time.Now().Format(time.RFC3339),
	}
	return result, nil
}
