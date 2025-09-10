package handler

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestHandlerRedundancyElimination validates that we eliminated the architectural redundancies
// similar to the metadata issue that was fixed
func TestHandlerRedundancyElimination(t *testing.T) {
	t.Run("apiHandler no longer has k8sClient field", func(t *testing.T) {
		handler := &apiHandler{}
		
		// Use reflection to check that k8sClient field is no longer present
		val := reflect.ValueOf(handler).Elem()
		typ := val.Type()
		
		for i := 0; i < val.NumField(); i++ {
			field := typ.Field(i)
			assert.NotEqual(t, "k8sClient", field.Name, 
				"apiHandler should not have k8sClient field after refactoring - should use k8sHandler instead")
		}
	})
	
	t.Run("apiHandler no longer has blobStorage field", func(t *testing.T) {
		handler := &apiHandler{}
		
		// Use reflection to check that blobStorage field is no longer present
		val := reflect.ValueOf(handler).Elem()
		typ := val.Type()
		
		for i := 0; i < val.NumField(); i++ {
			field := typ.Field(i)
			assert.NotEqual(t, "blobStorage", field.Name, 
				"apiHandler should not have blobStorage field after refactoring - should use blobHandler instead")
		}
	})
	
	t.Run("apiHandler has all handler abstractions", func(t *testing.T) {
		handler := &apiHandler{}
		
		// Use reflection to verify all handler fields exist
		val := reflect.ValueOf(handler).Elem()
		typ := val.Type()
		
		expectedHandlers := map[string]string{
			"k8sHandler":        "K8sHandler",
			"metadataHandler":   "MetadataHandler", 
			"blobHandler":       "BlobHandler",
			"validationHandler": "ValidationHandler",
		}
		
		foundHandlers := make(map[string]bool)
		for i := 0; i < val.NumField(); i++ {
			field := typ.Field(i)
			if expectedType, exists := expectedHandlers[field.Name]; exists {
				foundHandlers[field.Name] = true
				assert.Equal(t, expectedType, field.Type.Name(),
					"Handler field %s should be of type %s", field.Name, expectedType)
			}
		}
		
		for handlerName := range expectedHandlers {
			assert.True(t, foundHandlers[handlerName], 
				"apiHandler should have %s field", handlerName)
		}
	})
	
	t.Run("apiHandler follows single abstraction principle", func(t *testing.T) {
		// Test that we don't have both direct storage and handler abstractions
		handler := &apiHandler{}
		val := reflect.ValueOf(handler).Elem()
		typ := val.Type()
		
		// Check that we don't have conflicting patterns
		hasK8sClient := false
		hasK8sHandler := false
		hasBlobStorage := false
		hasBlobHandler := false
		hasMetadataStorage := false
		hasMetadataHandler := false
		
		for i := 0; i < val.NumField(); i++ {
			field := typ.Field(i)
			switch field.Name {
			case "k8sClient":
				hasK8sClient = true
			case "k8sHandler":
				hasK8sHandler = true
			case "blobStorage":
				hasBlobStorage = true
			case "blobHandler":
				hasBlobHandler = true
			case "metadataStorage":
				hasMetadataStorage = true
			case "metadataHandler":
				hasMetadataHandler = true
			}
		}
		
		// Assert single abstraction principle - only handlers, not direct storage
		assert.False(t, hasK8sClient, "Should not have k8sClient - use k8sHandler instead")
		assert.True(t, hasK8sHandler, "Should have k8sHandler for K8s operations")
		
		assert.False(t, hasBlobStorage, "Should not have blobStorage - use blobHandler instead")
		assert.True(t, hasBlobHandler, "Should have blobHandler for blob operations")
		
		assert.False(t, hasMetadataStorage, "Should not have metadataStorage - use metadataHandler instead")
		assert.True(t, hasMetadataHandler, "Should have metadataHandler for metadata operations")
	})
}

// TestFakeHandlerCompatibility ensures our refactoring maintains backward compatibility
func TestFakeHandlerCompatibility(t *testing.T) {
	t.Run("NewFakeAPIHandler creates handlers correctly", func(t *testing.T) {
		// Test that the builder still creates handlers correctly
		assert.NotPanics(t, func() {
			handler := NewFakeAPIHandler(nil) // Would normally be a real client
			assert.NotNil(t, handler)
			
			// Verify it's an apiHandler with proper handlers set
			if apiHandler, ok := handler.(*apiHandler); ok {
				assert.NotNil(t, apiHandler.k8sHandler, "Should have k8sHandler")
				assert.NotNil(t, apiHandler.metadataHandler, "Should have metadataHandler")
				assert.NotNil(t, apiHandler.blobHandler, "Should have blobHandler")
				assert.NotNil(t, apiHandler.validationHandler, "Should have validationHandler")
			}
		})
	})
}