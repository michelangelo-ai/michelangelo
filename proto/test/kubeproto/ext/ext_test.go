package kubeproto_ext_test

import (
	"testing"

	ext "github.com/michelangelo-ai/michelangelo/proto/test/kubeproto/ext"
	"github.com/stretchr/testify/assert"
)

func TestExtValidation(t *testing.T) {
	t.Run("ValidationMsg1_RequiredFields", func(t *testing.T) {
		msg := &ext.ValidationMsg1{}

		// Test that required fields are validated
		err := msg.Validate("")
		assert.Error(t, err, "Should fail when required fields are missing")
		assert.Contains(t, err.Error(), "is required")

		// Test with valid values
		msg.F1 = 50 // Between 10 and 100
		msg.F3 = "test value"
		msg.F4 = ext.ENUM1_VALUE2 // Must be >= 2
		err = msg.Validate("")
		assert.NoError(t, err, "Should pass with valid values")

		// Test min/max validation
		msg.F1 = 5 // Less than min
		err = msg.Validate("")
		assert.Error(t, err, "Should fail when value is less than min")

		msg.F1 = 150 // Greater than max
		err = msg.Validate("")
		assert.Error(t, err, "Should fail when value is greater than max")
	})

		t.Run("ValidationMsg5_PatternValidation", func(t *testing.T) {
		msg := &ext.ValidationMsg5{}

		// Test pattern validation
		msg.F1 = "xyz" // Should not match [ab]+
		err := msg.Validate("")
		assert.Error(t, err, "Should fail with pattern not matching")

		msg.F1 = "abab" // Should match [ab]+
		msg.F2 = "123" // Should match [1-9][0-9]*
		err = msg.Validate("")
		assert.NoError(t, err, "Should pass with valid pattern")

		// Reset and test invalid F2
		msg = &ext.ValidationMsg5{}
		msg.F1 = "abab" // Keep F1 valid
		msg.F2 = "0123" // Should not match (starts with 0)
		err = msg.Validate("")
		if assert.Error(t, err, "Should fail with invalid number pattern") {
			assert.Contains(t, err.Error(), "must be a positive number")
		}
	})

	t.Run("ValidationMsg7_WellKnownFormats", func(t *testing.T) {
		msg := &ext.ValidationMsg7{}

		// Test UUID validation
		msg.F1 = "not-a-uuid"
		err := msg.Validate("")
		assert.Error(t, err, "Should fail with invalid UUID")

		msg.F1 = "550e8400-e29b-41d4-a716-446655440000"
		msg.F2 = "test@example.com" // Valid email
		msg.F3 = "https://example.com" // Valid URI
		msg.F4 = "192.168.1.1" // Valid IPv4
		msg.F5 = "::1" // Valid IPv6
		msg.F6 = "192.168.1.1" // Valid IP
		err = msg.Validate("")
		assert.NoError(t, err, "Should pass with all valid formats")

		// Test email validation failure
		msg.F2 = "invalid-email"
		err = msg.Validate("")
		assert.Error(t, err, "Should fail with invalid email")

		msg.F4 = "::1" // IPv6 address
		err = msg.Validate("")
		assert.Error(t, err, "Should fail with IPv6 when expecting IPv4")
	})

	t.Run("ValidationRegistry", func(t *testing.T) {
		// Test that the validation registry is populated
		assert.NotNil(t, ext.ValidationRegistry)
		assert.Greater(t, len(ext.ValidationRegistry), 0, "Registry should have entries")

		// Test Validate function using the registry
		msg := &ext.ValidationMsg1{F1: 50, F3: "test", F4: ext.ENUM1_VALUE2}
		err := ext.Validate("ValidationMsg1", msg, "")
		assert.NoError(t, err, "Should validate through registry")

		// Test with invalid data through registry
		invalidMsg := &ext.ValidationMsg1{F1: 5, F3: "test", F4: ext.ENUM1_VALUE2}
		err = ext.Validate("ValidationMsg1", invalidMsg, "")
		assert.Error(t, err, "Should fail validation through registry")
	})
}
