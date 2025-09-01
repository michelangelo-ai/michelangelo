package handler

import (
	"github.com/go-logr/logr"
	"github.com/michelangelo-ai/michelangelo/go/api"
	ctrlRTClient "sigs.k8s.io/controller-runtime/pkg/client"
)

// ValidationHandlerImpl implements ValidationHandler interface.
// Focuses only on validation operations, following Flyte's validation pattern separation.
type ValidationHandlerImpl struct {
	logger logr.Logger
}

// NewValidationHandler creates a new ValidationHandler implementation.
func NewValidationHandler(logger logr.Logger) ValidationHandler {
	return &ValidationHandlerImpl{
		logger: logger.WithName("validation-handler"),
	}
}

// ValidateCreate validates an object for creation.
func (v *ValidationHandlerImpl) ValidateCreate(obj ctrlRTClient.Object) error {
	v.logger.V(2).Info("Validating object for creation",
		"namespace", obj.GetNamespace(),
		"name", obj.GetName(),
		"kind", obj.GetObjectKind().GroupVersionKind().Kind,
	)

	err := api.Validate(obj)
	if err != nil {
		v.logger.Error(err, "Validation failed for create operation",
			"namespace", obj.GetNamespace(),
			"name", obj.GetName(),
		)
		return err
	}

	v.logger.V(2).Info("Successfully validated object for creation",
		"namespace", obj.GetNamespace(),
		"name", obj.GetName(),
	)
	return nil
}

// ValidateUpdate validates an object for update.
func (v *ValidationHandlerImpl) ValidateUpdate(obj ctrlRTClient.Object) error {
	v.logger.V(2).Info("Validating object for update",
		"namespace", obj.GetNamespace(),
		"name", obj.GetName(),
		"kind", obj.GetObjectKind().GroupVersionKind().Kind,
	)

	err := api.Validate(obj)
	if err != nil {
		v.logger.Error(err, "Validation failed for update operation",
			"namespace", obj.GetNamespace(),
			"name", obj.GetName(),
		)
		return err
	}

	v.logger.V(2).Info("Successfully validated object for update",
		"namespace", obj.GetNamespace(),
		"name", obj.GetName(),
	)
	return nil
}

// ValidateDelete validates an object for deletion.
func (v *ValidationHandlerImpl) ValidateDelete(obj ctrlRTClient.Object) error {
	v.logger.V(2).Info("Validating object for deletion",
		"namespace", obj.GetNamespace(),
		"name", obj.GetName(),
		"kind", obj.GetObjectKind().GroupVersionKind().Kind,
	)

	// For deletion, we mainly check if the object has proper metadata
	if obj.GetName() == "" {
		v.logger.Error(nil, "Object name is required for deletion",
			"namespace", obj.GetNamespace(),
		)
		return &ValidationError{
			Field:   "metadata.name",
			Message: "Object name is required for deletion",
		}
	}

	v.logger.V(2).Info("Successfully validated object for deletion",
		"namespace", obj.GetNamespace(),
		"name", obj.GetName(),
	)
	return nil
}

// ValidationError represents a validation error with specific field information.
type ValidationError struct {
	Field   string
	Message string
}

// Error implements the error interface.
func (e *ValidationError) Error() string {
	return e.Message
}

// NullValidationHandler is a no-op implementation for when validation is disabled.
type NullValidationHandler struct {
	logger logr.Logger
}

// NewNullValidationHandler creates a no-op validation handler.
func NewNullValidationHandler(logger logr.Logger) ValidationHandler {
	return &NullValidationHandler{
		logger: logger.WithName("null-validation-handler"),
	}
}

// ValidateCreate is a no-op for null handler.
func (n *NullValidationHandler) ValidateCreate(obj ctrlRTClient.Object) error {
	n.logger.V(2).Info("Validation disabled, skipping create validation",
		"namespace", obj.GetNamespace(),
		"name", obj.GetName(),
	)
	return nil
}

// ValidateUpdate is a no-op for null handler.
func (n *NullValidationHandler) ValidateUpdate(obj ctrlRTClient.Object) error {
	n.logger.V(2).Info("Validation disabled, skipping update validation",
		"namespace", obj.GetNamespace(),
		"name", obj.GetName(),
	)
	return nil
}

// ValidateDelete is a no-op for null handler.
func (n *NullValidationHandler) ValidateDelete(obj ctrlRTClient.Object) error {
	n.logger.V(2).Info("Validation disabled, skipping delete validation",
		"namespace", obj.GetNamespace(),
		"name", obj.GetName(),
	)
	return nil
}