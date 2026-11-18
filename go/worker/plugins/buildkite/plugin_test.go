package buildkite

import (
	"fmt"
	"testing"
	"time"

	"github.com/cadence-workflow/starlark-worker/service"
	"github.com/michelangelo-ai/michelangelo/go/worker/activities/buildkite"
	"github.com/michelangelo-ai/michelangelo/go/worker/plugins/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.starlark.net/starlark"
	"go.temporal.io/sdk/temporal"
	"go.uber.org/cadence"
	"go.uber.org/yarpc/yarpcerrors"
)

func newEnvironment(t *testing.T) *service.TestEnvironment {
	t.Helper()
	var suite service.TestSuite
	env := suite.NewTestEnvironment(t, &service.TestEnvironmentParams{
		RootDirectory: "testdata", Plugins: map[string]service.IPlugin{pluginID: Plugin},
	})
	t.Cleanup(func() { env.Cadence.AssertExpectations(t) })
	t.Cleanup(func() { env.Temporal.AssertExpectations(t) })
	return env
}

func call(t *testing.T, env *service.TestEnvironment, operation string, parameters map[string]interface{}) (buildkite.BuildResponse, error) {
	t.Helper()
	var values starlark.Value
	require.NoError(t, utils.AsStar(parameters, &values))
	env.Cadence.ExecuteFunction("/test.star", "call", starlark.Tuple{starlark.String(operation), values}, nil, nil)
	var response buildkite.BuildResponse
	err := env.Cadence.GetResult(&response)
	return response, err
}

func response(state string) *buildkite.BuildResponse {
	return &buildkite.BuildResponse{
		ID: "build-uuid", Number: 17, Pipeline: "smoke-test", State: state,
		WebURL: "https://buildkite.com/example/smoke-test/builds/17", Branch: "main", Commit: "HEAD",
	}
}

func errorDetails(t *testing.T, err error, reason string) string {
	t.Helper()
	require.Error(t, err)
	var customError *cadence.CustomError
	require.ErrorAs(t, err, &customError)
	assert.Equal(t, reason, customError.Reason())
	var details map[string]interface{}
	require.NoError(t, customError.Details(&details))
	return fmt.Sprint(details)
}

func TestTriggerDefaultsAndNullableMaps(t *testing.T) {
	for _, maps := range []map[string]interface{}{
		{}, {"env": nil, "metadata": nil}, {"env": map[string]string{}, "metadata": map[string]string{}},
	} {
		t.Run("defaults", func(t *testing.T) {
			env := newEnvironment(t)
			cad := env.Cadence.GetTestWorkflowEnvironment()
			cad.RegisterActivity(buildkite.Activities.TriggerBuild)
			cad.OnActivity(buildkite.Activities.TriggerBuild, mock.Anything,
				mock.MatchedBy(func(req buildkite.TriggerBuildRequest) bool {
					return req.Pipeline == "smoke-test" && req.Branch == "main" &&
						req.Commit == "HEAD" && req.Message == "" && len(req.Env) == 0 && len(req.Metadata) == 0
				})).Once().Return(response("scheduled"), nil)
			maps["pipeline"] = "smoke-test"
			got, err := call(t, env, "trigger_build", maps)
			require.NoError(t, err)
			assert.Equal(t, *response("scheduled"), got)
		})
	}
}

func TestInvokeForwardsAllArgumentsAndPolls(t *testing.T) {
	env := newEnvironment(t)
	cad := env.Cadence.GetTestWorkflowEnvironment()
	cad.RegisterActivity(buildkite.Activities.TriggerBuild)
	cad.RegisterActivity(buildkite.Activities.SensorGetBuild)
	cad.OnActivity(buildkite.Activities.TriggerBuild, mock.Anything, buildkite.TriggerBuildRequest{
		Pipeline: "smoke-test", Branch: "feature", Commit: "sha", Message: "test",
		Env: map[string]string{"CONFIG": "smoke.yaml"}, Metadata: map[string]string{"release": "model"},
	}).Once().Return(response("scheduled"), nil)
	req := buildkite.GetBuildRequest{Pipeline: "smoke-test", BuildNumber: 17}
	cad.OnActivity(buildkite.Activities.SensorGetBuild, mock.Anything, req).Once().
		Return(nil, cadence.NewCustomError(buildkite.NotReadyReason, "running"))
	cad.OnActivity(buildkite.Activities.SensorGetBuild, mock.Anything, req).Once().
		Return(response("passed"), nil)
	got, err := call(t, env, "invoke_build", map[string]interface{}{
		"pipeline": "smoke-test", "branch": "feature", "commit": "sha", "message": "test",
		"env": map[string]string{"CONFIG": "smoke.yaml"}, "metadata": map[string]string{"release": "model"},
		"timeout_seconds": 60, "poll_interval": 1,
	})
	require.NoError(t, err)
	assert.Equal(t, *response("passed"), got)
}

func TestGetAndCancelSnapshots(t *testing.T) {
	for _, operation := range []string{"get_build", "cancel_build"} {
		t.Run(operation, func(t *testing.T) {
			env := newEnvironment(t)
			cad := env.Cadence.GetTestWorkflowEnvironment()
			var method interface{} = buildkite.Activities.GetBuild
			var req interface{} = buildkite.GetBuildRequest{Pipeline: "smoke-test", BuildNumber: 17}
			state := "failed"
			if operation == "cancel_build" {
				method = buildkite.Activities.CancelBuild
				req = buildkite.CancelBuildRequest{Pipeline: "smoke-test", BuildNumber: 17}
				state = "canceling"
			}
			cad.RegisterActivity(method)
			cad.OnActivity(method, mock.Anything, req).
				Once().Return(response(state), nil)
			got, err := call(t, env, operation, map[string]interface{}{"pipeline": "smoke-test", "build_number": 17})
			require.NoError(t, err)
			assert.Equal(t, *response(state), got)
		})
	}
}

func TestSubmitIsNotRetried(t *testing.T) {
	for _, operation := range []string{"trigger_build", "invoke_build", "cancel_build"} {
		t.Run(operation, func(t *testing.T) {
			env := newEnvironment(t)
			cad := env.Cadence.GetTestWorkflowEnvironment()
			var method any = buildkite.Activities.TriggerBuild
			parameters := map[string]any{"pipeline": "smoke-test"}
			if operation == "cancel_build" {
				method = buildkite.Activities.CancelBuild
				parameters["build_number"] = 17
			}
			cad.RegisterActivity(method)
			cad.OnActivity(method, mock.Anything, mock.Anything).Once().
				Return(nil, cadence.NewCustomError(yarpcerrors.CodeUnavailable.String(), "ambiguous mutation"))
			_, err := call(t, env, operation, parameters)
			assert.Contains(t, errorDetails(t, err, yarpcerrors.CodeUnavailable.String()), "ambiguous mutation")
		})
	}
}

func TestWaitRetriesTransientRead(t *testing.T) {
	env := newEnvironment(t)
	cad := env.Cadence.GetTestWorkflowEnvironment()
	cad.RegisterActivity(buildkite.Activities.SensorGetBuild)
	cad.OnActivity(buildkite.Activities.SensorGetBuild, mock.Anything, mock.Anything).Once().
		Return(nil, cadence.NewCustomError(yarpcerrors.CodeUnavailable.String(), "retryable read"))
	cad.OnActivity(buildkite.Activities.SensorGetBuild, mock.Anything, mock.Anything).Once().
		Return(response("passed"), nil)
	got, err := call(t, env, "wait_for_build", map[string]interface{}{"pipeline": "smoke-test", "build_number": 17})
	require.NoError(t, err)
	assert.Equal(t, *response("passed"), got)
}

func TestTerminalAndPermissionFailuresDoNotCancel(t *testing.T) {
	for _, reason := range []string{
		yarpcerrors.CodeInternal.String(), yarpcerrors.CodePermissionDenied.String(),
		yarpcerrors.CodeNotFound.String(), yarpcerrors.CodeInvalidArgument.String(),
	} {
		t.Run(reason, func(t *testing.T) {
			env := newEnvironment(t)
			cad := env.Cadence.GetTestWorkflowEnvironment()
			cad.RegisterActivity(buildkite.Activities.SensorGetBuild)
			cad.OnActivity(buildkite.Activities.SensorGetBuild, mock.Anything, mock.Anything).Once().
				Return(nil, cadence.NewCustomError(reason, "permanent failure"))
			_, err := call(t, env, "wait_for_build", map[string]interface{}{"pipeline": "smoke-test", "build_number": 17})
			assert.Contains(t, errorDetails(t, err, reason), "permanent failure")
		})
	}
}

func TestWaitCancellationCleanupPreservesOriginalError(t *testing.T) {
	env := newEnvironment(t)
	cad := env.Cadence.GetTestWorkflowEnvironment()
	cad.RegisterActivity(buildkite.Activities.SensorGetBuild)
	cad.RegisterActivity(buildkite.Activities.CancelBuild)
	primary := cadence.NewCanceledError("workflow cancelled")
	cad.OnActivity(buildkite.Activities.SensorGetBuild, mock.Anything, mock.Anything).Once().
		Return(nil, primary)
	cad.OnActivity(buildkite.Activities.CancelBuild, mock.Anything,
		buildkite.CancelBuildRequest{Pipeline: "smoke-test", BuildNumber: 17}).Once().
		Return(nil, cadence.NewCustomError(yarpcerrors.CodePermissionDenied.String(), "cleanup failed"))
	_, err := call(t, env, "wait_for_build", map[string]interface{}{"pipeline": "smoke-test", "build_number": 17})
	details := errorDetails(t, err, yarpcerrors.CodeUnknown.String())
	assert.Contains(t, details, primary.Error())
	assert.NotContains(t, details, "cleanup failed")
}

func TestWaitBudgetExhaustionCancels(t *testing.T) {
	env := newEnvironment(t)
	cad := env.Cadence.GetTestWorkflowEnvironment()
	cad.RegisterActivity(buildkite.Activities.SensorGetBuild)
	cad.RegisterActivity(buildkite.Activities.CancelBuild)
	cad.OnActivity(buildkite.Activities.SensorGetBuild, mock.Anything, mock.Anything).
		Return(nil, cadence.NewCustomError(buildkite.NotReadyReason, "running"))
	cad.OnActivity(buildkite.Activities.CancelBuild, mock.Anything,
		buildkite.CancelBuildRequest{Pipeline: "smoke-test", BuildNumber: 17}).Once().
		Return(response("canceling"), nil)
	_, err := call(t, env, "wait_for_build", map[string]interface{}{
		"pipeline": "smoke-test", "build_number": 17, "timeout_seconds": 2, "poll_interval": 1,
	})
	require.Error(t, err)
	var customError *cadence.CustomError
	require.ErrorAs(t, err, &customError)
	assert.Equal(t, yarpcerrors.CodeDeadlineExceeded.String(), customError.Reason())
	assert.Contains(t, errorDetails(t, err, yarpcerrors.CodeDeadlineExceeded.String()), "did not pass within 2 seconds")
}

func TestParentCancellationStillSchedulesCleanup(t *testing.T) {
	env := newEnvironment(t)
	cad := env.Cadence.GetTestWorkflowEnvironment()
	cad.RegisterActivity(buildkite.Activities.SensorGetBuild)
	cad.RegisterActivity(buildkite.Activities.CancelBuild)
	cad.OnActivity(buildkite.Activities.SensorGetBuild, mock.Anything, mock.Anything).
		After(time.Hour).Return(response("passed"), nil)
	cad.OnActivity(buildkite.Activities.CancelBuild, mock.Anything,
		buildkite.CancelBuildRequest{Pipeline: "smoke-test", BuildNumber: 17}).Once().
		Return(response("canceling"), nil)
	cad.RegisterDelayedCallback(func() { cad.CancelWorkflow() }, time.Second)
	_, err := call(t, env, "wait_for_build", map[string]interface{}{
		"pipeline": "smoke-test", "build_number": 17, "timeout_seconds": 60,
	})
	require.Error(t, err)
}

func TestInvalidArgumentsDoNotSubmit(t *testing.T) {
	tests := []struct {
		operation  string
		parameters map[string]interface{}
		message    string
	}{
		{"trigger_build", map[string]interface{}{}, "pipeline"},
		{"trigger_build", map[string]interface{}{"pipeline": "../other"}, "pipeline"},
		{"trigger_build", map[string]interface{}{"pipeline": "smoke-test", "branch": ""}, "branch"},
		{"trigger_build", map[string]interface{}{"pipeline": "smoke-test", "commit": ""}, "commit"},
		{"trigger_build", map[string]interface{}{"pipeline": "smoke-test", "env": []string{"bad"}}, "env"},
		{"trigger_build", map[string]interface{}{"pipeline": "smoke-test", "env": map[string]int{"X": 1}}, "values must be strings"},
		{"trigger_build", map[string]interface{}{"pipeline": "smoke-test", "env": map[string]interface{}{"X": nil}}, "values must be strings"},
		{"get_build", map[string]interface{}{"pipeline": "smoke-test", "build_number": 0}, "positive"},
		{"cancel_build", map[string]interface{}{"pipeline": "smoke-test"}, "build_number"},
		{"wait_for_build", map[string]interface{}{"pipeline": "smoke-test", "build_number": 17, "timeout_seconds": 0}, "timeout_seconds"},
		{"wait_for_build", map[string]interface{}{"pipeline": "smoke-test", "build_number": 17, "poll_interval": -1}, "poll_interval"},
		{"invoke_build", map[string]interface{}{"pipeline": "smoke-test", "timeout_seconds": 86401}, "timeout_seconds"},
		{"invoke_build", map[string]interface{}{"pipeline": "smoke-test", "poll_interval": 3601}, "poll_interval"},
		{"invoke_build", map[string]interface{}{"pipeline": "smoke-test", "unexpected": 1}, "unexpected"},
	}
	for _, test := range tests {
		t.Run(test.operation+" "+test.message, func(t *testing.T) {
			env := newEnvironment(t)
			_, err := call(t, env, test.operation, test.parameters)
			assert.Contains(t, errorDetails(t, err, yarpcerrors.CodeInvalidArgument.String()), test.message)
		})
	}
}

func TestRejectsNonStringMapKeys(t *testing.T) {
	env := newEnvironment(t)
	env.Cadence.ExecuteFunction("/test.star", "invalid_metadata_key", nil, nil, nil)
	var result interface{}
	err := env.Cadence.GetResult(&result)
	assert.Contains(t, errorDetails(t, err, yarpcerrors.CodeInvalidArgument.String()), "keys must be strings")
}

func TestModuleContract(t *testing.T) {
	module := Plugin.Create(service.RunInfo{})
	assert.Equal(t, "buildkite", Plugin.ID())
	assert.Equal(t, "buildkite", module.String())
	assert.Equal(t, "buildkite", module.Type())
	assert.True(t, bool(module.Truth()))
	module.Freeze()
	_, err := module.Hash()
	require.Error(t, err)
	attributes := module.(starlark.HasAttrs)
	assert.ElementsMatch(t, []string{"trigger_build", "get_build", "wait_for_build", "invoke_build", "cancel_build"}, attributes.AttrNames())
	missing, err := attributes.Attr("missing")
	require.NoError(t, err)
	assert.Nil(t, missing)
}

func callTemporal(t *testing.T, env *service.TestEnvironment, operation string, parameters map[string]any) (buildkite.BuildResponse, error) {
	t.Helper()
	var values starlark.Value
	require.NoError(t, utils.AsStar(parameters, &values))
	env.Temporal.ExecuteFunction("/test.star", "call",
		starlark.Tuple{starlark.String(operation), values}, nil, nil)
	var result buildkite.BuildResponse
	err := env.Temporal.GetResult(&result)
	return result, err
}

func TestTemporalMutationsAreNotRetried(t *testing.T) {
	for _, operation := range []string{"trigger_build", "cancel_build"} {
		t.Run(operation, func(t *testing.T) {
			env := newEnvironment(t)
			temp := env.Temporal.GetTestWorkflowEnvironment()
			var method any = buildkite.Activities.TriggerBuild
			parameters := map[string]any{"pipeline": "smoke-test"}
			if operation == "cancel_build" {
				method = buildkite.Activities.CancelBuild
				parameters["build_number"] = 17
			}
			temp.RegisterActivity(method)
			temp.OnActivity(method, mock.Anything, mock.Anything).Once().
				Return(nil, temporal.NewApplicationError("ambiguous mutation", yarpcerrors.CodeUnavailable.String()))
			_, err := callTemporal(t, env, operation, parameters)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "ambiguous mutation")
		})
	}
}

func TestTemporalInvokeRetriesOnlySensor(t *testing.T) {
	env := newEnvironment(t)
	temp := env.Temporal.GetTestWorkflowEnvironment()
	temp.RegisterActivity(buildkite.Activities.TriggerBuild)
	temp.RegisterActivity(buildkite.Activities.SensorGetBuild)
	temp.OnActivity(buildkite.Activities.TriggerBuild, mock.Anything, mock.Anything).
		Once().Return(response("scheduled"), nil)
	temp.OnActivity(buildkite.Activities.SensorGetBuild, mock.Anything, mock.Anything).
		Once().Return(nil, temporal.NewApplicationError("running", buildkite.NotReadyReason))
	temp.OnActivity(buildkite.Activities.SensorGetBuild, mock.Anything, mock.Anything).
		Once().Return(response("passed"), nil)
	got, err := callTemporal(t, env, "invoke_build", map[string]any{
		"pipeline": "smoke-test", "timeout_seconds": 60, "poll_interval": 1,
	})
	require.NoError(t, err)
	assert.Equal(t, *response("passed"), got)
}

func TestTemporalPermanentErrorsAreNotRetried(t *testing.T) {
	env := newEnvironment(t)
	temp := env.Temporal.GetTestWorkflowEnvironment()
	temp.RegisterActivity(buildkite.Activities.SensorGetBuild)
	temp.OnActivity(buildkite.Activities.SensorGetBuild, mock.Anything, mock.Anything).
		Once().Return(nil, temporal.NewApplicationError("denied", yarpcerrors.CodePermissionDenied.String()))
	_, err := callTemporal(t, env, "wait_for_build", map[string]any{
		"pipeline": "smoke-test", "build_number": 17,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "permission-denied")
}

func TestTemporalTimeoutAndCancellationRequestCleanup(t *testing.T) {
	for _, cancel := range []bool{false, true} {
		t.Run(fmt.Sprint("cancel=", cancel), func(t *testing.T) {
			env := newEnvironment(t)
			temp := env.Temporal.GetTestWorkflowEnvironment()
			temp.RegisterActivity(buildkite.Activities.SensorGetBuild)
			temp.RegisterActivity(buildkite.Activities.CancelBuild)
			temp.OnActivity(buildkite.Activities.SensorGetBuild, mock.Anything, mock.Anything).
				After(time.Hour).Return(response("passed"), nil)
			temp.OnActivity(buildkite.Activities.CancelBuild, mock.Anything,
				buildkite.CancelBuildRequest{Pipeline: "smoke-test", BuildNumber: 17}).
				Once().Return(response("canceling"), nil)
			if cancel {
				temp.RegisterDelayedCallback(temp.CancelWorkflow, time.Second)
			}
			_, err := callTemporal(t, env, "wait_for_build", map[string]any{
				"pipeline": "smoke-test", "build_number": 17, "timeout_seconds": 2,
			})
			require.Error(t, err)
		})
	}
}
