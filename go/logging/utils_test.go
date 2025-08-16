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

func TestMarshalToStringForLogging(t *testing.T) {
	t.Run("map with registered sensitive field", func(t *testing.T) {
		// Clean up after test
		defer ClearSensitiveFields()
		
		// Register the field as sensitive
		RegisterSensitiveField("sensitive_field")
		
		// Test map with mixed sensitive and non-sensitive fields
		requestMap := map[string]interface{}{
			"meta": map[string]interface{}{
				"name":       "test-project",
				"project_id": "proj-123",
			},
			"sensitive_field": "secret-api-key-data",
			"data_type":       "BATCH",
		}
		
		result := MarshalToStringForLogging(requestMap)
		
		// Should contain non-sensitive fields
		assert.Contains(t, result, `"name":"test-project"`)
		assert.Contains(t, result, `"project_id":"proj-123"`)
		assert.Contains(t, result, `"data_type":"BATCH"`)
		
		// Should redact sensitive_field since it was registered as sensitive
		assert.Contains(t, result, `"sensitive_field":"[REDACTED]"`)
		assert.NotContains(t, result, "secret-api-key-data")
	})
	
	t.Run("regular struct without sensitive fields", func(t *testing.T) {
		request := map[string]interface{}{
			"name": "test",
			"data": "normal data",
			"count": 42,
		}
		
		result := MarshalToStringForLogging(request)
		
		// Should preserve all fields for non-sensitive data
		assert.Contains(t, result, `"name":"test"`)
		assert.Contains(t, result, `"data":"normal data"`)
		assert.Contains(t, result, `"count":42`)
	})
}

func TestMarshalToStringForLogging_KeywordFields(t *testing.T) {
	t.Run("fields with sensitive keywords are NOT redacted unless explicitly registered", func(t *testing.T) {
		// Clean up after test
		defer ClearSensitiveFields()
		
		input := map[string]interface{}{
			"username": "user1",
			"password": "secret123",  // Should NOT be redacted
			"secret":   "hidden",     // Should NOT be redacted  
			"token":    "abc123",     // Should NOT be redacted
			"key":      "secret-key", // Should NOT be redacted
		}
		
		result := MarshalToStringForLogging(input)
		
		// All fields should be preserved (not redacted) since none are registered
		assert.Contains(t, result, `"password":"secret123"`)
		assert.Contains(t, result, `"secret":"hidden"`)
		assert.Contains(t, result, `"token":"abc123"`)
		assert.Contains(t, result, `"key":"secret-key"`)
		assert.Contains(t, result, `"username":"user1"`)
		
		// No field should be redacted
		assert.NotContains(t, result, `"[REDACTED]"`)
	})
	
	t.Run("only explicitly registered fields are redacted", func(t *testing.T) {
		// Clean up after test
		defer ClearSensitiveFields()
		
		// Only register "secret" as sensitive
		RegisterSensitiveField("secret")
		
		input := map[string]interface{}{
			"username": "user1",
			"password": "secret123",  // Should NOT be redacted
			"secret":   "hidden",     // Should be redacted
			"token":    "abc123",     // Should NOT be redacted
		}
		
		result := MarshalToStringForLogging(input)
		
		// Only "secret" should be redacted
		assert.Contains(t, result, `"password":"secret123"`)
		assert.Contains(t, result, `"secret":"[REDACTED]"`)
		assert.Contains(t, result, `"token":"abc123"`)
		assert.Contains(t, result, `"username":"user1"`)
	})
}

func TestSensitiveFieldRegistry_Integration(t *testing.T) {
	t.Run("field registration and cleanup", func(t *testing.T) {
		// Start with clean state
		ClearSensitiveFields()
		
		// Register some fields
		RegisterSensitiveField("test_field")
		RegisterSensitiveField("another_field")
		
		// Test with registered field
		testStruct := map[string]interface{}{
			"test_field":    "sensitive data",
			"another_field": "more sensitive data",
			"normal_field":  "normal data",
		}
		
		result := MarshalToStringForLogging(testStruct)
		
		assert.Contains(t, result, `"test_field":"[REDACTED]"`)
		assert.Contains(t, result, `"another_field":"[REDACTED]"`)
		assert.Contains(t, result, `"normal_field":"normal data"`)
		
		// Unregister one field
		UnregisterSensitiveField("test_field")
		
		result2 := MarshalToStringForLogging(testStruct)
		
		assert.NotContains(t, result2, `"test_field":"[REDACTED]"`)
		assert.Contains(t, result2, `"test_field":"sensitive data"`)
		assert.Contains(t, result2, `"another_field":"[REDACTED]"`)
		
		// Clear all
		ClearSensitiveFields()
		
		result3 := MarshalToStringForLogging(testStruct)
		
		assert.Contains(t, result3, `"test_field":"sensitive data"`)
		assert.Contains(t, result3, `"another_field":"more sensitive data"`)
	})
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
