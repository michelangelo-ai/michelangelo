package utils

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	tmptestsuite "go.temporal.io/sdk/testsuite"
	tmpworkflow "go.temporal.io/sdk/workflow"
	cadtestsuite "go.uber.org/cadence/testsuite"
	cadworkflow "go.uber.org/cadence/workflow"
)

const testChangeID = "test-change"

func TestDefaultVersionMatchesBothEngines(t *testing.T) {
	require.EqualValues(t, cadworkflow.DefaultVersion, DefaultVersion)
	require.EqualValues(t, tmpworkflow.DefaultVersion, DefaultVersion)
}

// versionTestCases: an execution that reached the change before it was deployed (mocked as a DefaultVersion
// replay) gets DefaultVersion; any other execution gets the newest supported version.
var versionTestCases = []struct {
	name   string
	replay bool
	want   int
}{
	{name: "new execution", want: 1},
	{name: "execution that predates the change", replay: true, want: DefaultVersion},
}

func TestGetVersionCadence(t *testing.T) {
	for _, tc := range versionTestCases {
		t.Run(tc.name, func(t *testing.T) {
			var s cadtestsuite.WorkflowTestSuite
			env := s.NewTestWorkflowEnvironment()
			wf := func(ctx cadworkflow.Context) (int, error) {
				return GetVersion(ctx, testChangeID, DefaultVersion, 1)
			}
			env.RegisterWorkflow(wf)
			if tc.replay {
				env.OnGetVersion(testChangeID, cadworkflow.DefaultVersion, 1).Return(cadworkflow.DefaultVersion)
			}
			env.ExecuteWorkflow(wf)

			require.True(t, env.IsWorkflowCompleted())
			require.NoError(t, env.GetWorkflowError())
			var got int
			require.NoError(t, env.GetWorkflowResult(&got))
			require.Equal(t, tc.want, got)
		})
	}
}

func TestGetVersionTemporal(t *testing.T) {
	for _, tc := range versionTestCases {
		t.Run(tc.name, func(t *testing.T) {
			var s tmptestsuite.WorkflowTestSuite
			env := s.NewTestWorkflowEnvironment()
			wf := func(ctx tmpworkflow.Context) (int, error) {
				return GetVersion(ctx, testChangeID, DefaultVersion, 1)
			}
			env.RegisterWorkflow(wf)
			if tc.replay {
				env.OnGetVersion(testChangeID, tmpworkflow.DefaultVersion, 1).Return(tmpworkflow.DefaultVersion)
			}
			env.ExecuteWorkflow(wf)

			require.True(t, env.IsWorkflowCompleted())
			require.NoError(t, env.GetWorkflowError())
			var got int
			require.NoError(t, env.GetWorkflowResult(&got))
			require.Equal(t, tc.want, got)
		})
	}
}

func TestGetVersionUnsupportedContext(t *testing.T) {
	_, err := GetVersion(context.Background(), testChangeID, DefaultVersion, 1)
	require.ErrorContains(t, err, "unsupported workflow context")
}
