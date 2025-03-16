package main

import (
	"testing"

	"github.com/scottdavis/dsgo/pkg/core"
)

func TestCreateDataProcessingWorkflow(t *testing.T) {
	// Create a basic DSPYConfig
	config := core.NewDSPYConfig()
	
	// Create workflow and verify it doesn't error out
	_, err := CreateDataProcessingWorkflow(config)
	if err != nil {
		t.Fatalf("Failed to create workflow: %v", err)
	}
}

func TestCreateClassifierStep(t *testing.T) {
	// Create a basic DSPYConfig
	config := core.NewDSPYConfig()
	
	// Create a classifier step
	step := CreateClassifierStep(config)
	
	// Basic validation
	if step == nil {
		t.Fatal("Expected non-nil step")
	}
	
	if step.ID != "support_classifier" {
		t.Errorf("Expected classifier step ID 'support_classifier', got '%s'", step.ID)
	}
}

func TestCreateHandlerStep(t *testing.T) {
	// Create a basic DSPYConfig
	config := core.NewDSPYConfig()
	
	// Create a handler step
	step := CreateHandlerStep("test", "test prompt", config)
	
	// Basic validation
	if step == nil {
		t.Error("Expected non-nil step")
	}
	
	if step.ID != "test_handler" {
		t.Errorf("Expected handler step ID 'test_handler', got '%s'", step.ID)
	}
}

func TestCreateRouterWorkflow(t *testing.T) {
	// Create a basic DSPYConfig
	config := core.NewDSPYConfig()
	
	// Create workflow
	workflow := CreateRouterWorkflow(config)
	
	// Basic validation
	if workflow == nil {
		t.Fatal("Expected non-nil workflow")
	}
} 