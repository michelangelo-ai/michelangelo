package handler

import (
	"github.com/michelangelo-ai/michelangelo/go/api"
	ctrlRTClient "sigs.k8s.io/controller-runtime/pkg/client"
)

// ValidationHandlerImpl wraps existing api.Validate functionality with NO new logic
type ValidationHandlerImpl struct{}

func NewValidationHandler() ValidationHandler {
	return &ValidationHandlerImpl{}
}

// ValidateCreate directly delegates to existing api.Validate - NO new logic
func (v *ValidationHandlerImpl) ValidateCreate(obj ctrlRTClient.Object) error {
	return api.Validate(obj)
}

// ValidateUpdate directly delegates to existing api.Validate - NO new logic
func (v *ValidationHandlerImpl) ValidateUpdate(obj ctrlRTClient.Object) error {
	return api.Validate(obj)
}

// ValidateDelete has no validation in original code - NO new logic
func (v *ValidationHandlerImpl) ValidateDelete(obj ctrlRTClient.Object) error {
	return nil
}