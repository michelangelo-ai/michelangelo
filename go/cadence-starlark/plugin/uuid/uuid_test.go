package uuid

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.starlark.net/starlark"
)

// Mock UUID object for testing
func mockUUID(uuidStr string) *UUID {
	return &UUID{
		StringUUID: starlark.String(uuidStr),
	}
}

// Test the _urn function
func TestUrn(t *testing.T) {
	uuid := mockUUID("123e4567-e89b-12d3-a456-426614174000")

	// Call _urn with the mock UUID object
	result, err := _urn(uuid)

	// Using require assertions (failures stop test execution)
	require.NoError(t, err, "Expected no error from _urn")
	expected := starlark.String("urn:uuid:123e4567-e89b-12d3-a456-426614174000")
	require.Equal(t, expected, result, "urn should match the expected value")
}

// Test the _hex function
func TestHex(t *testing.T) {
	uuid := mockUUID("123e4567-e89b-12d3-a456-426614174000")

	// Call _hex with the mock UUID object
	result, err := _hex(uuid)

	// Using require assertions (failures stop test execution)
	require.NoError(t, err, "Expected no error from _hex")
	expected := starlark.String("123e4567e89b12d3a456426614174000")
	require.Equal(t, expected, result, "hex string should match the expected value")
}

// Test Freeze method to ensure no panics or issues
func TestFreeze(t *testing.T) {
	uuid := mockUUID("123e4567-e89b-12d3-a456-426614174000")

	// Using require's NotPanics to ensure Freeze does not cause panic
	require.NotPanics(t, func() {
		uuid.Freeze()
	}, "Freeze should not panic")
}
