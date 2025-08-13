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
		msg.F4 = ext.E1_C // Must be >= 2
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

	t.Run("ValidationMsg4_LengthValidation", func(t *testing.T) {
		msg := &ext.ValidationMsg4{}

		// Test string length validation
		msg.F1 = "ab" // Too short (min 3)
		err := msg.Validate("")
		assert.Error(t, err, "Should fail with string too short")

		msg.F1 = "abc" // Valid length
		msg.F2 = []int32{1, 2} // Valid count (min 2)
		msg.F3 = []byte("test") // Valid byte length
		msg.F4 = map[int64]string{1: "test"} // Valid map with at least 1 item
		err = msg.Validate("")
		assert.NoError(t, err, "Should pass with valid lengths")

		// Test max validation
		msg.F1 = "abcdefghijklmnopqrstuvwxyz" // Too long (max 10)
		err = msg.Validate("")
		assert.Error(t, err, "Should fail with string too long")
	})

	t.Run("ValidationRegistry", func(t *testing.T) {
		// Test that the validation registry is populated
		assert.NotNil(t, ext.ValidationRegistry)
		assert.Greater(t, len(ext.ValidationRegistry), 0, "Registry should have entries")

		// Test Validate function using the registry
		msg := &ext.ValidationMsg1{F1: 50, F3: "test", F4: ext.E1_C}
		err := ext.Validate("ValidationMsg1", msg, "")
		assert.NoError(t, err, "Should validate through registry")

		// Test with invalid data through registry
		invalidMsg := &ext.ValidationMsg1{F1: 5, F3: "test", F4: ext.E1_C}
		err = ext.Validate("ValidationMsg1", invalidMsg, "")
		assert.Error(t, err, "Should fail validation through registry")
	})
}
