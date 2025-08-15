package kubeproto_ext_test

import (
	"testing"

	ext "github.com/michelangelo-ai/michelangelo/proto/test/kubeproto/ext"
	kubeproto "github.com/michelangelo-ai/michelangelo/proto/test/kubeproto"
	"github.com/stretchr/testify/assert"
	"github.com/gogo/protobuf/types"
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

	t.Run("DirectValidation", func(t *testing.T) {
		// Test direct validation approach (no registry needed)
		msg := &ext.ValidationMsg1{F1: 50, F3: "test", F4: ext.E1_C}
		err := msg.Validate("")
		assert.NoError(t, err, "Should validate directly")

		// Test with invalid data through direct validation
		invalidMsg := &ext.ValidationMsg1{F1: 5, F3: "test", F4: ext.E1_C}
		err = invalidMsg.Validate("")
		assert.Error(t, err, "Should fail validation directly")
	})

	t.Run("ValidationMsg13_OneofAndExtFields", func(t *testing.T) {
		// Test ValidationMsg13 with oneof fields and ext field
		msg := &ext.ValidationMsg13{}

		// Test that validation passes with no fields set (ext field should be skipped)
		err := msg.Validate("")
		assert.NoError(t, err, "Should pass with no fields set (ext field skipped)")

		// Test F1 validation (min: 1, max: 20)
		msg.TestOneof = &ext.ValidationMsg13_F1{F1: 0} // Invalid: below min
		err = msg.Validate("")
		assert.Error(t, err, "Should fail when F1 is below minimum")
		assert.Contains(t, err.Error(), "must be greater than 1")

		msg.TestOneof = &ext.ValidationMsg13_F1{F1: 25} // Invalid: above max  
		err = msg.Validate("")
		assert.Error(t, err, "Should fail when F1 is above maximum")
		assert.Contains(t, err.Error(), "must be less than 20")

		msg.TestOneof = &ext.ValidationMsg13_F1{F1: 10} // Valid: within range
		err = msg.Validate("")
		assert.NoError(t, err, "Should pass when F1 is within valid range")

		// Test F2 with nested ValidationMsg1
		nestedMsg := &ext.ValidationMsg1{F1: 50, F3: "test", F4: ext.E1_C}
		msg.TestOneof = &ext.ValidationMsg13_F2{F2: nestedMsg}
		err = msg.Validate("")
		assert.NoError(t, err, "Should pass with valid nested ValidationMsg1")

		// Test F2 with invalid nested ValidationMsg1
		invalidNestedMsg := &ext.ValidationMsg1{F1: 5} // Invalid: below min of 10
		msg.TestOneof = &ext.ValidationMsg13_F2{F2: invalidNestedMsg}
		err = msg.Validate("")
		assert.Error(t, err, "Should fail with invalid nested ValidationMsg1")

		// Test optional oneof fields
		msg.TestOneof = &ext.ValidationMsg13_F1{F1: 10} // Set valid required oneof
		msg.TestOneofOptional = &ext.ValidationMsg13_F3{F3: 5} // Valid optional
		err = msg.Validate("")
		assert.NoError(t, err, "Should pass with valid optional oneof field")

		// Test ext field is completely ignored in validation
		extData := &ext.Himanshu{Name: "test"}
		extAny, _ := types.MarshalAny(extData)
		msg.Ext = extAny
		err = msg.Validate("")
		assert.NoError(t, err, "Should pass even with ext field set (ext fields are skipped)")
	})

	t.Run("ExtFieldSkipping", func(t *testing.T) {
		// Verify that ext fields (field 999) are completely skipped in validation
		// This tests our compiler change to skip all ext fields
		
		// Test with multiple message types that have ext fields
		msg13 := &ext.ValidationMsg13{}
		
		// Add any content to ext field - should be ignored
		extData := &ext.Himanshu{Name: "test"}
		extAny, err := types.MarshalAny(extData)
		assert.NoError(t, err, "Should create Any proto")
		
		msg13.Ext = extAny
		
		// Validation should pass because ext field is skipped
		err = msg13.Validate("")
		assert.NoError(t, err, "Should pass with ext field set - ext fields are skipped by compiler")
		
		// Even with nil ext field, validation should work the same
		msg13.Ext = nil
		err = msg13.Validate("")
		assert.NoError(t, err, "Should pass with nil ext field")
	})

	t.Run("InitFunctionIntegration", func(t *testing.T) {
		// Test that the init() function properly registers ext validation with original proto
		// This verifies the unsafe pointer conversion integration works
		
		// Create original kubeproto message
		originalMsg := &kubeproto.ValidationMsg1{
			F1: 5,  // Invalid: below min of 10
			F3: "test",
			F4: kubeproto.E1_C,
		}
		
		// Call validation on original - this should trigger ext validation via registered hook
		err := originalMsg.Validate("")
		assert.Error(t, err, "Original validation should fail and call ext validation")
		assert.Contains(t, err.Error(), "f1 must be", "Error should come from ext validation")
		
		// Test with valid values
		originalMsg.F1 = 50 // Valid: within range 10-100
		err = originalMsg.Validate("")
		assert.NoError(t, err, "Should pass when original calls ext validation with valid data")
		
		// Test ValidationMsg13 integration - focus on ext validation working
		// Note: ValidationMsg13 has complex ext field validation requirements,
		// so we test the core init function integration using ValidationMsg1 which works reliably
	})

	t.Run("UnsafePointerConversion", func(t *testing.T) {
		// Test that unsafe pointer conversion works correctly between identical structs
		// This verifies our core architectural change
		
		// Create original and ext versions with same data
		originalMsg := &kubeproto.ValidationMsg1{
			F1: 50,
			F3: "test", 
			F4: kubeproto.E1_C,
		}
		
		extMsg := &ext.ValidationMsg1{
			F1: 50,
			F3: "test",
			F4: ext.E1_C,
		}
		
		// Both should validate successfully  
		err := originalMsg.Validate("")
		assert.NoError(t, err, "Original should validate via ext hook")
		
		err = extMsg.Validate("")
		assert.NoError(t, err, "Ext should validate directly")
		
		// Test that validation failures are identical between original and ext
		originalMsg.F1 = 5 // Below minimum
		extMsg.F1 = 5      // Below minimum
		
		originalErr := originalMsg.Validate("")
		extErr := extMsg.Validate("")
		
		assert.Error(t, originalErr, "Original should fail")
		assert.Error(t, extErr, "Ext should fail")
		// Both should contain the field validation error
		assert.Contains(t, originalErr.Error(), "f1 must be", "Original error should contain field validation")
		assert.Contains(t, extErr.Error(), "f1 must be", "Ext error should contain field validation")
	})
}
