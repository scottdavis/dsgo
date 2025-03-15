package core

// DSPYConfig holds all configuration for the DSPy library
type DSPYConfig struct {
	DefaultLLM       LLM
	TeacherLLM       LLM
	ConcurrencyLevel int
}

// NewDSPYConfig creates a new configuration with default values
func NewDSPYConfig() *DSPYConfig {
	return &DSPYConfig{
		ConcurrencyLevel: 1, // default concurrency level
	}
}

// WithDefaultLLM sets the default LLM
func (c *DSPYConfig) WithDefaultLLM(llm LLM) *DSPYConfig {
	c.DefaultLLM = llm
	return c
}

// WithTeacherLLM sets the teacher LLM
func (c *DSPYConfig) WithTeacherLLM(llm LLM) *DSPYConfig {
	c.TeacherLLM = llm
	return c
}

// WithConcurrencyLevel sets the concurrency level
func (c *DSPYConfig) WithConcurrencyLevel(level int) *DSPYConfig {
	if level > 0 {
		c.ConcurrencyLevel = level
	} else {
		c.ConcurrencyLevel = 1 // Reset to default value for invalid inputs
	}
	return c
}
