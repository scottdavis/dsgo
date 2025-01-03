package main

import (
	"context"
	"flag"
	"fmt"
	"strings"

	"github.com/XiaoConstantine/dspy-go/pkg/config"
	"github.com/XiaoConstantine/dspy-go/pkg/core"

	workflows "github.com/XiaoConstantine/dspy-go/pkg/agents/workflows"
	"github.com/XiaoConstantine/dspy-go/pkg/logging"
)

func RunChainExample(ctx context.Context, logger *logging.Logger) {
	logger.Info(ctx, "============ Example 1: Chain workflow for structured data extraction and formatting ==============")
	dataWorkflow, err := CreateDataProcessingWorkflow()
	if err != nil {
		logger.Error(ctx, "Failed to create data workflow: %v", err)
	}

	report := `Q3 Performance Summary:Our customer satisfaction score rose to 92 points this quarter.Revenue grew by 45% compared to last year.Market share is now at 23% in our primary market.Customer churn decreased to 5% from 8%.New user acquisition cost is $43 per user.Product adoption rate increased to 78%.Employee satisfaction is at 87 points.Operating margin improved to 34%.`
	result, err := dataWorkflow.Execute(ctx, map[string]interface{}{
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
			logger.Info(ctx, strings.TrimSpace(line))
		}
	} else {
		logger.Error(ctx, "invalid table format in result")
	}
	logger.Info(ctx, "================================================================================")
}

func RunParallelExample(ctx context.Context, logger *logging.Logger) {
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
	workflow, inputs, err := CreateParallelWorkflow(stakeholders)
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

func RunRouteExample(ctx context.Context, logger *logging.Logger) {
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
	router := CreateRouterWorkflow()

	for routeType, prompt := range supportRoutes {
		routeStep := CreateHandlerStep(routeType, prompt)
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
		logger.Info(ctx, strings.Repeat("-", 40))
		logger.Info(ctx, ticket)
		logger.Info(ctx, "\nResponse:")
		logger.Info(ctx, strings.Repeat("-", 40))

		response, err := router.Execute(ctx, map[string]interface{}{"input": ticket})
		if err != nil {
			logger.Info(ctx, "Error processing ticket %d: %v", i+1, err)
			continue
		}
		logger.Info(ctx, response["response"].(string))
	}
	logger.Info(ctx, "=================================================")

}

func main() {
	output := logging.NewConsoleOutput(true, logging.WithColor(true))

	logger := logging.NewLogger(logging.Config{
		Severity: logging.DEBUG,
		Outputs:  []logging.Output{output},
	})
	logging.SetLogger(logger)
	apiKey := flag.String("api-key", "", "Anthropic API Key")

	ctx := context.Background()
	err := config.ConfigureDefaultLLM(*apiKey, core.ModelAnthropicSonnet)
	if err != nil {
		logger.Error(ctx, "Failed to configure LLM: %v", err)
	}
	RunChainExample(ctx, logger)
	RunParallelExample(ctx, logger)
	RunRouteExample(ctx, logger)
}
