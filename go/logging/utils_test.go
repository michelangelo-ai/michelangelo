package logging

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestMarshalToString(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected string
	}{
		{
			name:     "Marshal map",
			input:    map[string]interface{}{"key": "value", "num": 42},
			expected: `{"key":"value","num":42}`,
		},
		{
			name:     "Marshal string",
			input:    "test",
			expected: `"test"`,
		},
		{
			name:     "Marshal int",
			input:    123,
			expected: `123`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MarshalToString(tt.input)
			if result != tt.expected {
				t.Errorf("Expected: %s, got: %s", tt.expected, result)
			}
		})
	}
}

func TestGetLogrLoggerOrPanic(t *testing.T) {
	t.Run("successful logger creation", func(t *testing.T) {
		// This should not panic
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("GetLogrLoggerOrPanic() should not panic but did: %v", r)
			}
		}()

		logger := GetLogrLoggerOrPanic()
		assert.NotNil(t, logger, "Logger should not be nil")

		// Test that the logger can be used
		// This shouldn't panic
		logger.Info("test message", "key", "value")
	})
}
