package handler

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrlRTClient "sigs.k8s.io/controller-runtime/pkg/client"
)

// MockMetadataHandler for testing the metadata handler integration
type MockMetadataHandler struct {
	mock.Mock
}

func (m *MockMetadataHandler) GetFromMetadata(ctx context.Context, namespace, name string, obj ctrlRTClient.Object) error {
	args := m.Called(ctx, namespace, name, obj)
	return args.Error(0)
}

func (m *MockMetadataHandler) UpdateInMetadata(ctx context.Context, obj ctrlRTClient.Object) error {
	args := m.Called(ctx, obj)
	return args.Error(0)
}

func (m *MockMetadataHandler) DeleteFromMetadata(ctx context.Context, obj ctrlRTClient.Object) error {
	args := m.Called(ctx, obj)
	return args.Error(0)
}

func (m *MockMetadataHandler) ListFromMetadata(ctx context.Context, namespace string, opts *metav1.ListOptions, listOptionsExt *apipb.ListOptionsExt, list ctrlRTClient.ObjectList) error {
	args := m.Called(ctx, namespace, opts, listOptionsExt, list)
	return args.Error(0)
}

// MockObjectList for testing
type MockObjectList struct {
	metav1.TypeMeta
	metav1.ListMeta
	Items []MockObject
}

func (m *MockObjectList) DeepCopyObject() runtime.Object {
	return &MockObjectList{TypeMeta: m.TypeMeta, ListMeta: m.ListMeta, Items: m.Items}
}

func (m *MockObjectList) GetObjectKind() runtime.Object {
	return m
}

func TestMetadataHandlerRefactoring(t *testing.T) {
	ctx := context.Background()

	t.Run("GetFromMetadata is called instead of direct metadataStorage", func(t *testing.T) {
		mockMetadata := &MockMetadataHandler{}
		handler := &apiHandler{
			metadataHandler: mockMetadata,
		}

		obj := &MockObject{
			ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		}

		// Mock the expected call
		mockMetadata.On("GetFromMetadata", ctx, "default", "test", obj).Return(nil)

		// Call the method that should use metadataHandler
		err := handler.metadataHandler.GetFromMetadata(ctx, "default", "test", obj)

		assert.NoError(t, err)
		mockMetadata.AssertExpectations(t)
	})

	t.Run("UpdateInMetadata is called instead of handleUpdate", func(t *testing.T) {
		mockMetadata := &MockMetadataHandler{}
		handler := &apiHandler{
			metadataHandler: mockMetadata,
		}

		obj := &MockObject{
			ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		}

		// Mock the expected call
		mockMetadata.On("UpdateInMetadata", ctx, obj).Return(nil)

		// Call the method that should use metadataHandler
		err := handler.metadataHandler.UpdateInMetadata(ctx, obj)

		assert.NoError(t, err)
		mockMetadata.AssertExpectations(t)
	})

	t.Run("DeleteFromMetadata is called instead of handleDelete", func(t *testing.T) {
		mockMetadata := &MockMetadataHandler{}
		handler := &apiHandler{
			metadataHandler: mockMetadata,
		}

		obj := &MockObject{
			ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		}

		// Mock the expected call
		mockMetadata.On("DeleteFromMetadata", ctx, obj).Return(nil)

		// Call the method that should use metadataHandler
		err := handler.metadataHandler.DeleteFromMetadata(ctx, obj)

		assert.NoError(t, err)
		mockMetadata.AssertExpectations(t)
	})

	t.Run("ListFromMetadata supports listOptionsExt parameter", func(t *testing.T) {
		mockMetadata := &MockMetadataHandler{}
		handler := &apiHandler{
			metadataHandler: mockMetadata,
		}

		list := &MockObjectList{}
		opts := &metav1.ListOptions{}
		listOptionsExt := &apipb.ListOptionsExt{}

		// Mock the expected call with the new signature
		mockMetadata.On("ListFromMetadata", ctx, "default", opts, listOptionsExt, list).Return(nil)

		// Call the method that should use metadataHandler with extended options
		err := handler.metadataHandler.ListFromMetadata(ctx, "default", opts, listOptionsExt, list)

		assert.NoError(t, err)
		mockMetadata.AssertExpectations(t)
	})

	t.Run("Error handling works correctly", func(t *testing.T) {
		mockMetadata := &MockMetadataHandler{}
		handler := &apiHandler{
			metadataHandler: mockMetadata,
		}

		obj := &MockObject{
			ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		}

		expectedError := errors.New("metadata operation failed")
		mockMetadata.On("GetFromMetadata", ctx, "default", "test", obj).Return(expectedError)

		err := handler.metadataHandler.GetFromMetadata(ctx, "default", "test", obj)

		assert.Error(t, err)
		assert.Equal(t, expectedError, err)
		mockMetadata.AssertExpectations(t)
	})
}

func TestNullMetadataHandlerCompatibility(t *testing.T) {
	ctx := context.Background()
	nullHandler := &NullMetadataHandler{}

	t.Run("NullMetadataHandler supports new interface signature", func(t *testing.T) {
		list := &MockObjectList{}
		opts := &metav1.ListOptions{}
		listOptionsExt := &apipb.ListOptionsExt{}

		// This should not panic and should return a NotFound error
		err := nullHandler.ListFromMetadata(ctx, "default", opts, listOptionsExt, list)

		assert.Error(t, err)
		// Should be a NotFound error as per the implementation
	})

	t.Run("All NullMetadataHandler methods work", func(t *testing.T) {
		obj := &MockObject{}

		// Test all methods don't panic
		getErr := nullHandler.GetFromMetadata(ctx, "default", "test", obj)
		updateErr := nullHandler.UpdateInMetadata(ctx, obj)
		deleteErr := nullHandler.DeleteFromMetadata(ctx, obj)

		// GetFromMetadata should return NotFound
		assert.Error(t, getErr)

		// UpdateInMetadata and DeleteFromMetadata should return nil (no-op)
		assert.NoError(t, updateErr)
		assert.NoError(t, deleteErr)
	})
}

func TestNewMetadataHandlerNilStorage(t *testing.T) {
	t.Run("NewMetadataHandler returns NullMetadataHandler when storage is nil", func(t *testing.T) {
		// Simulate the case when metadata storage is disabled
		handler := NewMetadataHandler(nil, nil, nil)

		// Should return NullMetadataHandler, not nil
		assert.NotNil(t, handler)

		// Should be able to call methods without panic
		ctx := context.Background()
		obj := &MockObject{}

		err := handler.GetFromMetadata(ctx, "default", "test", obj)
		assert.Error(t, err) // Should return NotFound error

		err = handler.UpdateInMetadata(ctx, obj)
		assert.NoError(t, err) // Should be no-op

		err = handler.DeleteFromMetadata(ctx, obj)
		assert.NoError(t, err) // Should be no-op
	})
}

func TestMetadataStorageDisabledCompatibility(t *testing.T) {
	t.Run("Operations work when metadata storage is disabled", func(t *testing.T) {
		// Simulate a handler built when metadata storage is disabled
		nullHandler := NewMetadataHandler(nil, nil, nil) // This returns NullMetadataHandler
		
		handler := &apiHandler{
			metadataHandler: nullHandler,
			conf: storage.MetadataStorageConfig{
				EnableMetadataStorage: false,
			},
		}

		ctx := context.Background()
		obj := &MockObject{
			ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		}

		// Test that calling metadataHandler methods doesn't panic
		// Even when metadata storage is conceptually "disabled"
		err := handler.metadataHandler.GetFromMetadata(ctx, "default", "test", obj)
		// Should return NotFound (safe fallback behavior)
		assert.Error(t, err)

		err = handler.metadataHandler.UpdateInMetadata(ctx, obj)
		// Should be no-op and not fail
		assert.NoError(t, err)

		err = handler.metadataHandler.DeleteFromMetadata(ctx, obj)
		// Should be no-op and not fail  
		assert.NoError(t, err)

		// Test list operation with extended options
		list := &MockObjectList{}
		opts := &metav1.ListOptions{}
		listOptionsExt := &apipb.ListOptionsExt{}

		err = handler.metadataHandler.ListFromMetadata(ctx, "default", opts, listOptionsExt, list)
		// Should return NotFound (safe fallback behavior)
		assert.Error(t, err)
	})
}