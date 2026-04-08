package api

import (
	"reflect"
	"sync"
)

// ExtValidator is a function that validates an object with extension rules.
// It receives the original object and returns an error if validation fails.
type ExtValidator func(obj interface{}) error

// extRegistry holds registered extension validators.
// Thread-safe for concurrent access.
type extRegistry struct {
	mu         sync.RWMutex
	validators map[string]ExtValidator
}

// Global registry instance
var globalExtRegistry = &extRegistry{
	validators: make(map[string]ExtValidator),
}

// RegisterExtValidator registers an extension validator for a specific type.
// This is typically called from an init() function in the ext package.
//
// Example usage in v2_ext package:
//
//	func init() {
//	    api.RegisterExtValidator("*v2.Model", validateModelExt)
//	}
//
// The typeName should match the result of reflect.TypeOf(obj).String()
// for the object being validated (e.g., "*v2.Model").
func RegisterExtValidator(typeName string, validator ExtValidator) {
	globalExtRegistry.mu.Lock()
	defer globalExtRegistry.mu.Unlock()
	globalExtRegistry.validators[typeName] = validator
}

// UnregisterExtValidator removes a registered extension validator.
// Useful for testing to reset state between tests.
func UnregisterExtValidator(typeName string) {
	globalExtRegistry.mu.Lock()
	defer globalExtRegistry.mu.Unlock()
	delete(globalExtRegistry.validators, typeName)
}

// ClearExtValidators removes all registered extension validators.
// Useful for testing to reset state between tests.
func ClearExtValidators() {
	globalExtRegistry.mu.Lock()
	defer globalExtRegistry.mu.Unlock()
	globalExtRegistry.validators = make(map[string]ExtValidator)
}

// GetRegisteredExtValidators returns a list of registered type names.
// Useful for debugging and testing.
func GetRegisteredExtValidators() []string {
	globalExtRegistry.mu.RLock()
	defer globalExtRegistry.mu.RUnlock()
	
	names := make([]string, 0, len(globalExtRegistry.validators))
	for name := range globalExtRegistry.validators {
		names = append(names, name)
	}
	return names
}

// ValidateExt runs the registered extension validator for the given object.
// If no validator is registered for the object's type, returns nil (no error).
//
// This function is called by the validation handler after base validation passes.
// Extension validators are registered when ext packages are imported.
func ValidateExt(obj interface{}) error {
	if obj == nil {
		return nil
	}

	typeName := reflect.TypeOf(obj).String()

	globalExtRegistry.mu.RLock()
	validator, ok := globalExtRegistry.validators[typeName]
	globalExtRegistry.mu.RUnlock()

	if !ok {
		// No extension validator registered for this type
		return nil
	}

	return validator(obj)
}

// HasExtValidator checks if an extension validator is registered for the given type.
func HasExtValidator(typeName string) bool {
	globalExtRegistry.mu.RLock()
	defer globalExtRegistry.mu.RUnlock()
	_, ok := globalExtRegistry.validators[typeName]
	return ok
}

