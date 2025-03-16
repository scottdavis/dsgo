package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDSPYConfig(t *testing.T) {
	t.Run("NewDSPYConfig", func(t *testing.T) {
		config := NewDSPYConfig()
		assert.NotNil(t, config)
		assert.Equal(t, 1, config.ConcurrencyLevel)
		assert.Nil(t, config.DefaultLLM)
		assert.Nil(t, config.TeacherLLM)
	})

	t.Run("WithDefaultLLM", func(t *testing.T) {
		config := NewDSPYConfig()
		mockLLM := &MockLLM{}

		config = config.WithDefaultLLM(mockLLM)
		assert.Equal(t, mockLLM, config.DefaultLLM)
	})

	t.Run("WithTeacherLLM", func(t *testing.T) {
		config := NewDSPYConfig()
		mockLLM := &MockLLM{}

		config = config.WithTeacherLLM(mockLLM)
		assert.Equal(t, mockLLM, config.TeacherLLM)
	})

	t.Run("WithConcurrencyLevel", func(t *testing.T) {
		config := NewDSPYConfig()

		// Valid concurrency level
		config = config.WithConcurrencyLevel(5)
		assert.Equal(t, 5, config.ConcurrencyLevel)

		// Invalid concurrency level should reset to 1
		config = config.WithConcurrencyLevel(0)
		assert.Equal(t, 1, config.ConcurrencyLevel)

		config = config.WithConcurrencyLevel(-1)
		assert.Equal(t, 1, config.ConcurrencyLevel)
	})
}
