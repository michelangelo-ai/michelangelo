package logging

import (
	"testing"

	"github.com/stretchr/testify/assert"
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

func TestMarshalToStringError(t *testing.T) {
	// Test with unmarshalable type (channels cannot be marshaled to JSON)
	ch := make(chan int)
	result := MarshalToString(ch)
	assert.Contains(t, result, "<error marshaling to JSON:", "Should return error message for unmarshalable type")
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
