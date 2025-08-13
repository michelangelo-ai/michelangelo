package main

import (
	"fmt"
	v2 "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	v2_ext "github.com/michelangelo-ai/michelangelo/proto/api/v2_ext"
)

func main() {
	// Test that v2 validation Register functions exist and work
	fmt.Println("Testing integration between validation and ext compilers...")

	// Create original v2 object
	originalObj := &v2.DataSchemaItem{
		Name:     "test_field",
		DataType: v2.DataType_DATA_TYPE_STRING,
		Shape:    []int32{10, 20},
	}

	// Validate original object (should pass)
	err := originalObj.Validate("")
	if err != nil {
		fmt.Printf("❌ Original validation failed: %v\n", err)
	} else {
		fmt.Println("✅ Original validation passed")
	}

	// Create ext object
	extObj := &v2_ext.DataSchemaItem_Ext{
		Name:     "valid_field_name",
		DataType: v2_ext.DataType_Ext_DATA_TYPE_STRING,
		Shape:    []int32{10, 20},
	}

	// Validate ext object (should pass)
	err = extObj.Validate("")
	if err != nil {
		fmt.Printf("❌ Ext validation failed: %v\n", err)
	} else {
		fmt.Println("✅ Ext validation passed")
	}

	// Test ext object with invalid data
	invalidExtObj := &v2_ext.DataSchemaItem_Ext{
		Name:     "123invalid", // Invalid: starts with number
		DataType: v2_ext.DataType_Ext_DATA_TYPE_STRING,
		Shape:    []int32{10, -5, 20}, // Invalid: negative shape value
	}

	err = invalidExtObj.Validate("")
	if err != nil {
		fmt.Printf("✅ Ext validation correctly caught error: %v\n", err)
	} else {
		fmt.Println("❌ Ext validation should have failed")
	}

	// Test ValidationRegistry
	err = v2_ext.Validate("DataSchemaItem_Ext", extObj, "")
	if err != nil {
		fmt.Printf("❌ Registry validation failed: %v\n", err)
	} else {
		fmt.Println("✅ Registry validation passed")
	}

	fmt.Println("Integration test completed!")
}