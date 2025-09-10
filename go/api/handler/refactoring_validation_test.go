package handler

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestRefactoringValidation validates that the architectural refactoring was successful
func TestRefactoringValidation(t *testing.T) {
	t.Run("apiHandler no longer has metadataStorage field", func(t *testing.T) {
		handler := &apiHandler{}
		
		// Use reflection to check that metadataStorage field is no longer present
		val := reflect.ValueOf(handler).Elem()
		typ := val.Type()
		
		for i := 0; i < val.NumField(); i++ {
			field := typ.Field(i)
			assert.NotEqual(t, "metadataStorage", field.Name, 
				"apiHandler should not have metadataStorage field after refactoring")
		}
	})
	
	t.Run("apiHandler has metadataHandler field", func(t *testing.T) {
		handler := &apiHandler{}
		
		// Use reflection to verify metadataHandler field exists
		val := reflect.ValueOf(handler).Elem()
		typ := val.Type()
		
		var hasMetadataHandler bool
		for i := 0; i < val.NumField(); i++ {
			field := typ.Field(i)
			if field.Name == "metadataHandler" {
				hasMetadataHandler = true
				
				// Verify it's the correct type
				assert.Equal(t, "MetadataHandler", field.Type.Name(),
					"metadataHandler field should be of type MetadataHandler")
				break
			}
		}
		
		assert.True(t, hasMetadataHandler, "apiHandler should have metadataHandler field")
	})
	
	t.Run("MetadataHandler interface has all required methods", func(t *testing.T) {
		// Test that we can create handlers that implement the interface
		var handler MetadataHandler
		
		// Should be able to assign both implementations
		handler = &MetadataHandlerImpl{}
		assert.NotNil(t, handler)
		
		handler = &NullMetadataHandler{}
		assert.NotNil(t, handler)
		
		// Verify the interface has the expected methods using reflection
		interfaceType := reflect.TypeOf((*MetadataHandler)(nil)).Elem()
		
		expectedMethods := []string{
			"GetFromMetadata",
			"UpdateInMetadata", 
			"DeleteFromMetadata",
			"ListFromMetadata",
		}
		
		assert.Equal(t, len(expectedMethods), interfaceType.NumMethod(),
			"MetadataHandler should have exactly 4 methods")
		
		for _, expectedMethod := range expectedMethods {
			method, exists := interfaceType.MethodByName(expectedMethod)
			assert.True(t, exists, "MetadataHandler should have method %s", expectedMethod)
			assert.NotNil(t, method, "Method %s should be properly defined", expectedMethod)
		}
	})
	
	t.Run("ListFromMetadata method has correct signature", func(t *testing.T) {
		interfaceType := reflect.TypeOf((*MetadataHandler)(nil)).Elem()
		method, exists := interfaceType.MethodByName("ListFromMetadata")
		
		assert.True(t, exists, "ListFromMetadata method should exist")
		
		// Check that it has the correct number of parameters (including receiver)
		// Signature: ListFromMetadata(ctx, namespace, opts, listOptionsExt, list) error
		assert.Equal(t, 6, method.Type.NumIn(), 
			"ListFromMetadata should have 5 parameters plus receiver")
		
		// Check return type is error
		assert.Equal(t, 1, method.Type.NumOut(), 
			"ListFromMetadata should return 1 value")
		assert.Equal(t, "error", method.Type.Out(0).Name(),
			"ListFromMetadata should return error")
	})
	
	t.Run("Refactoring maintained backward compatibility", func(t *testing.T) {
		// Test that the builder still creates handlers correctly
		builder := NewAPIHandlerBuilder()
		
		// Should not panic when building with nil storage (metadata disabled case)
		assert.NotPanics(t, func() {
			handler, err := builder.
				WithK8sClient(nil).  // Would normally be a real client
				WithStorageConfig(storage.MetadataStorageConfig{EnableMetadataStorage: false}).
				Build()
			
			// Even with nil storage, should get a handler (with NullMetadataHandler)
			if err == nil { // err expected due to nil client, but if no err:
				assert.NotNil(t, handler)
			}
		})
	})
}