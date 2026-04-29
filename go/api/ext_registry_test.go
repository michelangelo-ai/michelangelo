package api

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestType is a mock type for testing ext validation
type TestType struct {
	Name  string
	Value int
}

// TestTypeSpec is a mock "ext" type with stricter validation
type TestTypeSpec struct {
	Name  string
	Value int
}

// Validate simulates the generated validation method on ext types
func (t *TestTypeSpec) Validate(prefix string) error {
	if t.Name == "" {
		return errors.New(prefix + "name is required")
	}
	if len(t.Name) > 50 {
		return errors.New(prefix + "name must be at most 50 characters")
	}
	if t.Value < 0 {
		return errors.New(prefix + "value must be non-negative")
	}
	if t.Value > 100 {
		return errors.New(prefix + "value must be at most 100")
	}
	return nil
}

func TestRegisterExtValidator(t *testing.T) {
	// Clean up after test
	defer ClearExtValidators()

	// Register a validator
	RegisterExtValidator("*api.TestType", func(obj interface{}) error {
		tt := obj.(*TestType)
		ext := &TestTypeSpec{
			Name:  tt.Name,
			Value: tt.Value,
		}
		return ext.Validate("")
	})

	// Verify it's registered
	assert.True(t, HasExtValidator("*api.TestType"))
	assert.Contains(t, GetRegisteredExtValidators(), "*api.TestType")
}

func TestValidateExt_NoValidatorRegistered(t *testing.T) {
	defer ClearExtValidators()

	// When no validator is registered, ValidateExt returns nil
	obj := &TestType{Name: "test", Value: 50}
	err := ValidateExt(obj)
	assert.NoError(t, err)
}

func TestValidateExt_NilObject(t *testing.T) {
	defer ClearExtValidators()

	err := ValidateExt(nil)
	assert.NoError(t, err)
}

func TestValidateExt_ValidObject(t *testing.T) {
	defer ClearExtValidators()

	// Register ext validator
	RegisterExtValidator("*api.TestType", func(obj interface{}) error {
		tt := obj.(*TestType)
		ext := &TestTypeSpec{
			Name:  tt.Name,
			Value: tt.Value,
		}
		return ext.Validate("")
	})

	// Valid object should pass
	obj := &TestType{Name: "valid-name", Value: 50}
	err := ValidateExt(obj)
	assert.NoError(t, err)
}

func TestValidateExt_InvalidObject_EmptyName(t *testing.T) {
	defer ClearExtValidators()

	// Register ext validator
	RegisterExtValidator("*api.TestType", func(obj interface{}) error {
		tt := obj.(*TestType)
		ext := &TestTypeSpec{
			Name:  tt.Name,
			Value: tt.Value,
		}
		return ext.Validate("")
	})

	// Empty name should fail ext validation
	obj := &TestType{Name: "", Value: 50}
	err := ValidateExt(obj)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "name is required")
}

func TestValidateExt_InvalidObject_NameTooLong(t *testing.T) {
	defer ClearExtValidators()

	// Register ext validator
	RegisterExtValidator("*api.TestType", func(obj interface{}) error {
		tt := obj.(*TestType)
		ext := &TestTypeSpec{
			Name:  tt.Name,
			Value: tt.Value,
		}
		return ext.Validate("")
	})

	// Name too long should fail ext validation
	longName := "this-is-a-very-long-name-that-exceeds-fifty-characters-limit"
	obj := &TestType{Name: longName, Value: 50}
	err := ValidateExt(obj)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "name must be at most 50 characters")
}

func TestValidateExt_InvalidObject_NegativeValue(t *testing.T) {
	defer ClearExtValidators()

	// Register ext validator
	RegisterExtValidator("*api.TestType", func(obj interface{}) error {
		tt := obj.(*TestType)
		ext := &TestTypeSpec{
			Name:  tt.Name,
			Value: tt.Value,
		}
		return ext.Validate("")
	})

	// Negative value should fail ext validation
	obj := &TestType{Name: "valid", Value: -10}
	err := ValidateExt(obj)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "value must be non-negative")
}

func TestValidateExt_InvalidObject_ValueTooHigh(t *testing.T) {
	defer ClearExtValidators()

	// Register ext validator
	RegisterExtValidator("*api.TestType", func(obj interface{}) error {
		tt := obj.(*TestType)
		ext := &TestTypeSpec{
			Name:  tt.Name,
			Value: tt.Value,
		}
		return ext.Validate("")
	})

	// Value too high should fail ext validation
	obj := &TestType{Name: "valid", Value: 150}
	err := ValidateExt(obj)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "value must be at most 100")
}

func TestUnregisterExtValidator(t *testing.T) {
	defer ClearExtValidators()

	// Register a validator
	RegisterExtValidator("*api.TestType", func(obj interface{}) error {
		return errors.New("should not be called")
	})
	assert.True(t, HasExtValidator("*api.TestType"))

	// Unregister it
	UnregisterExtValidator("*api.TestType")
	assert.False(t, HasExtValidator("*api.TestType"))

	// ValidateExt should now return nil (no validator)
	obj := &TestType{Name: "test", Value: 50}
	err := ValidateExt(obj)
	assert.NoError(t, err)
}

func TestClearExtValidators(t *testing.T) {
	// Register multiple validators
	RegisterExtValidator("*api.TestType", func(obj interface{}) error { return nil })
	RegisterExtValidator("*api.AnotherType", func(obj interface{}) error { return nil })

	assert.Len(t, GetRegisteredExtValidators(), 2)

	// Clear all
	ClearExtValidators()

	assert.Len(t, GetRegisteredExtValidators(), 0)
}

// TestExtValidationFlow demonstrates the complete flow:
// 1. Base type exists (e.g., v2.Model)
// 2. Ext type exists with stricter validation (e.g., v2_ext.ModelSpec)
// 3. When ext package is imported, it registers a validator
// 4. ValidateExt is called and runs the ext validation
func TestExtValidationFlow(t *testing.T) {
	defer ClearExtValidators()

	// Simulate what happens in v2_ext/register.go init()
	// This runs automatically when the package is imported
	RegisterExtValidator("*api.TestType", func(obj interface{}) error {
		// This is what the registration function does:
		// 1. Cast to the original type
		original := obj.(*TestType)

		// 2. Create ext type with same values
		ext := &TestTypeSpec{
			Name:  original.Name,
			Value: original.Value,
		}

		// 3. Call the GENERATED Validate() method on ext type
		return ext.Validate("")
	})

	// Test cases showing the validation flow
	testCases := []struct {
		name        string
		obj         *TestType
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid object passes both base and ext validation",
			obj:         &TestType{Name: "my-model", Value: 50},
			expectError: false,
		},
		{
			name:        "empty name fails ext validation (stricter than base)",
			obj:         &TestType{Name: "", Value: 50},
			expectError: true,
			errorMsg:    "name is required",
		},
		{
			name:        "negative value fails ext validation",
			obj:         &TestType{Name: "valid", Value: -1},
			expectError: true,
			errorMsg:    "value must be non-negative",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateExt(tc.obj)
			if tc.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.errorMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

