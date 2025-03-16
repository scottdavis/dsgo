package agents

import (
	"context"

	"github.com/scottdavis/dsgo/pkg/core"
)

type Agent interface {
	// Execute runs the agent's task with given input and returns output
	Execute(ctx context.Context, input map[string]any) (map[string]any, error)

	// GetCapabilities returns the tools/capabilities available to this agent
	GetCapabilities() []core.Tool

	// GetMemory returns the agent's memory store
	GetMemory() Memory
}
