package uuid

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.starlark.net/starlark"
	"go.uber.org/cadence/testsuite"
	"go.uber.org/cadence/workflow"
)

// Mock UUID object for testing
func generateMockUUID(uuidStr string) *UUID {
	return &UUID{
		StringUUID: starlark.String(uuidStr),
	}
}

// Test the uuid4 function
func TestUUID4(t *testing.T) {
	// Setup Cadence test environment
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()

	env.ExecuteWorkflow(func(ctx workflow.Context) error {
		// Create a new thread for starlark execution
		thread := &starlark.Thread{Name: "test-thread"}

		// Call the uuid4 function
		result, err := uuid4(thread, nil, nil, nil)

		// Using testify/require to ensure the result and error are as expected
		require.NoError(t, err, "Expected no error from uuid4")

		// Check if the result is of type *UUID and not empty string
		uuidObj, ok := result.(*UUID)
		require.True(t, ok, "Expected result to be of type *UUID")
		require.NotEmpty(t, string(uuidObj.StringUUID), "UUID should not be empty")

		return nil
	})

	// Assert that all expectations are met
	env.AssertExpectations(t)
}
