package v2_ext_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	v2_ext "github.com/michelangelo-ai/michelangelo/proto/api/v2_ext"
)

func TestDataSchemaExtValidation(t *testing.T) {
	t.Run("DataSchemaItem_Ext_RequiredFields", func(t *testing.T) {
		// Test that required fields are validated
		item := &v2_ext.DataSchemaItem_Ext{}

		// Should fail with empty name
		err := item.Validate("")
		assert.Error(t, err, "Should fail when name is empty")
		assert.Contains(t, err.Error(), "name")
		assert.Contains(t, err.Error(), "is required")

		// Should fail with invalid name pattern
		item.Name = "123invalid" // Starts with number
		item.DataType = v2_ext.DataType_Ext_DATA_TYPE_STRING
		err = item.Validate("")
		assert.Error(t, err, "Should fail with invalid name pattern")
		assert.Contains(t, err.Error(), "must match pattern")

		// Should pass with valid name
		item.Name = "valid_field_name"
		err = item.Validate("")
		assert.NoError(t, err, "Should pass with valid name and data type")
	})

	t.Run("DataSchemaItem_Ext_NameValidation", func(t *testing.T) {
		item := &v2_ext.DataSchemaItem_Ext{
			DataType: v2_ext.DataType_Ext_DATA_TYPE_INT,
		}

		// Test empty name fails
		item.Name = ""
		err := item.Validate("")
		assert.Error(t, err, "Should fail with empty name")
		assert.Contains(t, err.Error(), "is required")

		// Test invalid pattern fails
		item.Name = "123invalid" // Starts with number
		err = item.Validate("")
		assert.Error(t, err, "Should fail with invalid pattern")
		assert.Contains(t, err.Error(), "must match pattern")

		// Test valid name passes
		item.Name = "valid_name"
		err = item.Validate("")
		assert.NoError(t, err, "Should pass with valid name")
	})

	t.Run("DataSchemaItem_Ext_ShapeValidation", func(t *testing.T) {
		item := &v2_ext.DataSchemaItem_Ext{
			Name:     "tensor_field",
			DataType: v2_ext.DataType_Ext_DATA_TYPE_FLOAT,
		}

		// Test valid shape values
		item.Shape = []int32{10, 20, 30}
		err := item.Validate("")
		assert.NoError(t, err, "Should pass with valid shape values")

		// Test shape with negative value (should fail)
		item.Shape = []int32{10, -1, 30}
		err = item.Validate("")
		assert.Error(t, err, "Should fail with negative shape value")
		assert.Contains(t, err.Error(), "shape")
		assert.Contains(t, err.Error(), "must be greater than 0")

		// Test shape with value exceeding max
		item.Shape = []int32{10, 20000, 30}
		err = item.Validate("")
		assert.Error(t, err, "Should fail with shape value exceeding 10000")
		assert.Contains(t, err.Error(), "must be less than 10000")
	})

	t.Run("ValidationRegistry", func(t *testing.T) {
		// Test that the validation registry is populated
		assert.NotNil(t, v2_ext.ValidationRegistry)
		assert.Greater(t, len(v2_ext.ValidationRegistry), 0, "Registry should have entries")

		// Test registry lookup for DataSchemaItem_Ext
		item := &v2_ext.DataSchemaItem_Ext{
			Name:     "test_field",
			DataType: v2_ext.DataType_Ext_DATA_TYPE_STRING,
		}

		err := v2_ext.Validate("DataSchemaItem_Ext", item, "")
		assert.NoError(t, err, "Should validate through registry")

		// Test with invalid data through registry
		invalidItem := &v2_ext.DataSchemaItem_Ext{
			Name: "", // Invalid: empty name
		}
		err = v2_ext.Validate("DataSchemaItem_Ext", invalidItem, "")
		assert.Error(t, err, "Should fail validation through registry")
	})
}
