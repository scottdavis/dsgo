package modules

import (
	"context"

	"github.com/scottdavis/dsgo/pkg/core"
)

type ChainOfThought struct {
	Predict *Predict
}

// Ensure ChainOfThought implements Composable.
var _ core.Composable = (*ChainOfThought)(nil)

func NewChainOfThought(signature core.Signature, config *core.DSPYConfig) *ChainOfThought {
	modifiedSignature := appendRationaleField(signature)
	return &ChainOfThought{
		Predict: NewPredict(modifiedSignature, config),
	}
}

// WithDefaultOptions sets default options by configuring the underlying Predict module.
func (c *ChainOfThought) WithDefaultOptions(opts ...core.Option) *ChainOfThought {
	// Simply delegate to the Predict module's WithDefaultOptions
	c.Predict.WithDefaultOptions(opts...)
	return c
}

func (c *ChainOfThought) Process(ctx context.Context, inputs map[string]any, opts ...core.Option) (map[string]any, error) {
	ctx, span := core.StartSpan(ctx, "ChainOfThought")
	defer core.EndSpan(ctx)

	span.WithAnnotation("inputs", inputs)
	outputs, err := c.Predict.Process(ctx, inputs, opts...)
	if err != nil {
		span.WithError(err)
		return nil, err
	}
	span.WithAnnotation("outputs", outputs)

	return outputs, nil
}

func (c *ChainOfThought) GetSignature() core.Signature {
	return c.Predict.GetSignature()
}

func (c *ChainOfThought) Clone() core.Module {
	return &ChainOfThought{
		Predict: c.Predict.Clone().(*Predict),
	}
}

func (c *ChainOfThought) Compose(next core.Module) core.Module {
	return &core.ModuleChain{
		Modules: []core.Module{c, next},
	}
}

func (c *ChainOfThought) GetSubModules() []core.Module {
	return []core.Module{c.Predict}
}

func (c *ChainOfThought) SetSubModules(modules []core.Module) {
	if len(modules) > 0 {
		if predict, ok := modules[0].(*Predict); ok {
			c.Predict = predict
		}
	}
}

func appendRationaleField(signature core.Signature) core.Signature {
	newSignature := signature
	rationaleField := core.OutputField{
		Field: core.NewField("rationale",
			core.WithDescription("Step-by-step reasoning process"),
		),
	}
	newSignature.Outputs = append([]core.OutputField{rationaleField}, newSignature.Outputs...)

	return newSignature
}
