package handler

import (
	"github.com/michelangelo-ai/michelangelo/go/api"
	ctrlRTClient "sigs.k8s.io/controller-runtime/pkg/client"
)

// ValidationHandlerImpl implements the ValidationHandler interface by delegating to
// the existing validation functions. This provides a consistent abstraction layer
// for validation operations while maintaining compatibility with existing validation logic.
type ValidationHandlerImpl struct{}

// NewValidationHandler creates a new ValidationHandler implementation.
// The implementation is stateless and safe for concurrent use.
func NewValidationHandler() ValidationHandler {
	return &ValidationHandlerImpl{}
}

// ValidateCreate implements ValidationHandler.ValidateCreate by delegating to the api.Validate function.
func (v *ValidationHandlerImpl) ValidateCreate(obj ctrlRTClient.Object) error {
	return api.Validate(obj)
}

// ValidateUpdate implements ValidationHandler.ValidateUpdate by delegating to the api.Validate function.
func (v *ValidationHandlerImpl) ValidateUpdate(obj ctrlRTClient.Object) error {
	return api.Validate(obj)
}

// ValidateDelete implements ValidationHandler.ValidateDelete as a no-op.
// Currently no specific delete validation is required, but this provides a hook
// for future validation requirements.
func (v *ValidationHandlerImpl) ValidateDelete(obj ctrlRTClient.Object) error {
	return nil
}