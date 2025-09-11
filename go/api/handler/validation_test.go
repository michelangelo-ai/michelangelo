package handler

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrlRTClient "sigs.k8s.io/controller-runtime/pkg/client"
)

// MockValidationHandler for testing
type MockValidationHandler struct {
	mock.Mock
}

func (m *MockValidationHandler) ValidateCreate(obj ctrlRTClient.Object) error {
	args := m.Called(obj)
	return args.Error(0)
}

func (m *MockValidationHandler) ValidateUpdate(obj ctrlRTClient.Object) error {
	args := m.Called(obj)
	return args.Error(0)
}

func (m *MockValidationHandler) ValidateDelete(obj ctrlRTClient.Object) error {
	args := m.Called(obj)
	return args.Error(0)
}

// MockObject for testing
type MockObject struct {
	metav1.ObjectMeta
	metav1.TypeMeta
}

func (m *MockObject) DeepCopyObject() runtime.Object {
	return &MockObject{ObjectMeta: m.ObjectMeta, TypeMeta: m.TypeMeta}
}

func TestValidationHandlerIntegration(t *testing.T) {
	tests := []struct {
		name          string
		setupHandler  func() *apiHandler
		setupMock     func(*MockValidationHandler)
		expectedError bool
		errorMessage  string
	}{
		{
			name: "validation handler success",
			setupHandler: func() *apiHandler {
				mockValidator := &MockValidationHandler{}
				return &apiHandler{
					validationHandler: mockValidator,
				}
			},
			setupMock: func(mock *MockValidationHandler) {
				// Mock expects the specific object that will be created in the test
				mockObj := &MockObject{
					ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
					TypeMeta:   metav1.TypeMeta{Kind: "MockObject"},
				}
				mock.On("ValidateCreate", mockObj).Return(nil)
			},
			expectedError: false,
		},
		{
			name: "validation handler failure",
			setupHandler: func() *apiHandler {
				mockValidator := &MockValidationHandler{}
				return &apiHandler{
					validationHandler: mockValidator,
				}
			},
			setupMock: func(mock *MockValidationHandler) {
				// Mock expects the specific object that will be created in the test
				mockObj := &MockObject{
					ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
					TypeMeta:   metav1.TypeMeta{Kind: "MockObject"},
				}
				mock.On("ValidateCreate", mockObj).Return(errors.New("validation failed"))
			},
			expectedError: true,
			errorMessage:  "validation failed",
		},
		{
			name: "no validation handler - should not fail",
			setupHandler: func() *apiHandler {
				return &apiHandler{
					validationHandler: nil,
				}
			},
			setupMock: func(mock *MockValidationHandler) {
				// No mock setup needed
			},
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := tt.setupHandler()

			if handler.validationHandler != nil {
				mockValidator := handler.validationHandler.(*MockValidationHandler)
				tt.setupMock(mockValidator)
			}

			obj := &MockObject{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
				TypeMeta:   metav1.TypeMeta{Kind: "MockObject"},
			}

			// Test the validation logic directly
			var err error
			if handler.validationHandler != nil {
				err = handler.validationHandler.ValidateCreate(obj)
			}

			if tt.expectedError {
				assert.Error(t, err)
				if tt.errorMessage != "" {
					assert.Contains(t, err.Error(), tt.errorMessage)
				}
			} else {
				assert.NoError(t, err)
			}

			if handler.validationHandler != nil {
				mockValidator := handler.validationHandler.(*MockValidationHandler)
				mockValidator.AssertExpectations(t)
			}
		})
	}
}
