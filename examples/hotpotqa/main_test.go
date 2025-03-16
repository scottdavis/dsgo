package main

import (
	"math"
	"testing"

	"github.com/scottdavis/dsgo/pkg/core"
	"github.com/scottdavis/dsgo/pkg/llms"
)

// floatEquals checks if two floats are equal within a small epsilon
func floatEquals(a, b float64) bool {
	epsilon := 1e-6
	return math.Abs(a-b) < epsilon
}

func TestComputeF1(t *testing.T) {
	tests := []struct {
		name        string
		prediction  string
		groundTruth string
		expected    float64
	}{
		{
			name:        "Exact match",
			prediction:  "Albert Einstein",
			groundTruth: "Albert Einstein",
			expected:    1.0,
		},
		{
			name:        "No match",
			prediction:  "Isaac Newton",
			groundTruth: "Albert Einstein",
			expected:    0.0,
		},
		{
			name:        "Partial match",
			prediction:  "Albert Einstein was a physicist",
			groundTruth: "Albert Einstein was born in Germany",
			expected:    0.545455, // Use actual result from F1 calculation
		},
		{
			name:        "Empty prediction",
			prediction:  "",
			groundTruth: "Albert Einstein",
			expected:    0.0,
		},
		{
			name:        "Empty ground truth",
			prediction:  "Albert Einstein",
			groundTruth: "",
			expected:    0.0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f1 := computeF1(tc.prediction, tc.groundTruth)
			if !floatEquals(f1, tc.expected) {
				t.Errorf("computeF1(%q, %q) = %f, want %f", tc.prediction, tc.groundTruth, f1, tc.expected)
			}
		})
	}
}

func TestOpenRouterConfig(t *testing.T) {
	// This test just verifies that the OpenRouterConfig can be created
	// without runtime panics. It doesn't actually test API calls.
	config := llms.NewOpenRouterConfig("google/gemma-3-27b-it:free")
	if config.ModelName != "google/gemma-3-27b-it:free" {
		t.Errorf("Expected model name to be 'google/gemma-3-27b-it:free', got %q", config.ModelName)
	}
}

// TestDSPYConfig confirms that method chaining on DSPYConfig works correctly
func TestDSPYConfig(t *testing.T) {
	config := core.NewDSPYConfig()
	config = config.WithConcurrencyLevel(10)
	
	if config.ConcurrencyLevel != 10 {
		t.Errorf("Expected concurrency level to be 10, got %d", config.ConcurrencyLevel)
	}
} 