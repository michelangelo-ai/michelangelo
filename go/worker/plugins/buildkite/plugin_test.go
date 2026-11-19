package buildkite

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cadence-workflow/starlark-worker/service"
	"github.com/cadence-workflow/starlark-worker/workflow"
	"github.com/michelangelo-ai/michelangelo/go/worker/activities/buildkite"
	"github.com/michelangelo-ai/michelangelo/go/worker/plugins/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.starlark.net/starlark"
	enumspb "go.temporal.io/api/enums/v1"
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
			cad.RegisterActivity(buildkite.Activities.TriggerBuildkiteBuild)
			cad.OnActivity(buildkite.Activities.TriggerBuildkiteBuild, mock.Anything,
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
	cad.RegisterActivity(buildkite.Activities.TriggerBuildkiteBuild)
	cad.RegisterActivity(buildkite.Activities.SensorBuildkiteBuild)
	cad.OnActivity(buildkite.Activities.TriggerBuildkiteBuild, mock.Anything, buildkite.TriggerBuildRequest{
		Pipeline: "smoke-test", Branch: "feature", Commit: "sha", Message: "test",
		Env: map[string]string{"CONFIG": "smoke.yaml"}, Metadata: map[string]string{"release": "model"},
	}).Once().Return(response("scheduled"), nil)
	req := buildkite.GetBuildRequest{Pipeline: "smoke-test", BuildNumber: 17}
	cad.OnActivity(buildkite.Activities.SensorBuildkiteBuild, mock.Anything, req).Once().
		Return(nil, cadence.NewCustomError(buildkite.NotReadyReason, "running"))
	cad.OnActivity(buildkite.Activities.SensorBuildkiteBuild, mock.Anything, req).Once().
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
			var method interface{} = buildkite.Activities.GetBuildkiteBuild
			var req interface{} = buildkite.GetBuildRequest{Pipeline: "smoke-test", BuildNumber: 17}
			state := "failed"
			if operation == "cancel_build" {
				method = buildkite.Activities.CancelBuildkiteBuild
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
			var method any = buildkite.Activities.TriggerBuildkiteBuild
			parameters := map[string]any{"pipeline": "smoke-test"}
			if operation == "cancel_build" {
				method = buildkite.Activities.CancelBuildkiteBuild
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
	cad.RegisterActivity(buildkite.Activities.SensorBuildkiteBuild)
	cad.OnActivity(buildkite.Activities.SensorBuildkiteBuild, mock.Anything, mock.Anything).Once().
		Return(nil, cadence.NewCustomError(yarpcerrors.CodeUnavailable.String(), "retryable read"))
	cad.OnActivity(buildkite.Activities.SensorBuildkiteBuild, mock.Anything, mock.Anything).Once().
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
			cad.RegisterActivity(buildkite.Activities.SensorBuildkiteBuild)
			cad.OnActivity(buildkite.Activities.SensorBuildkiteBuild, mock.Anything, mock.Anything).Once().
				Return(nil, cadence.NewCustomError(reason, "permanent failure"))
			_, err := call(t, env, "wait_for_build", map[string]interface{}{"pipeline": "smoke-test", "build_number": 17})
			assert.Contains(t, errorDetails(t, err, reason), "permanent failure")
		})
	}
}

func TestWaitCancellationCleanupPreservesOriginalError(t *testing.T) {
	env := newEnvironment(t)
	cad := env.Cadence.GetTestWorkflowEnvironment()
	cad.RegisterActivity(buildkite.Activities.SensorBuildkiteBuild)
	cad.RegisterActivity(buildkite.Activities.CancelBuildkiteBuild)
	primary := cadence.NewCanceledError("workflow cancelled")
	cad.OnActivity(buildkite.Activities.SensorBuildkiteBuild, mock.Anything, mock.Anything).Once().
		Return(nil, primary)
	cad.OnActivity(buildkite.Activities.CancelBuildkiteBuild, mock.Anything,
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
	cad.RegisterActivity(buildkite.Activities.SensorBuildkiteBuild)
	cad.RegisterActivity(buildkite.Activities.CancelBuildkiteBuild)
	cad.OnActivity(buildkite.Activities.SensorBuildkiteBuild, mock.Anything, mock.Anything).
		Return(nil, cadence.NewCustomError(buildkite.NotReadyReason, "running"))
	cad.OnActivity(buildkite.Activities.CancelBuildkiteBuild, mock.Anything,
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
	cad.RegisterActivity(buildkite.Activities.SensorBuildkiteBuild)
	cad.RegisterActivity(buildkite.Activities.CancelBuildkiteBuild)
	cad.OnActivity(buildkite.Activities.SensorBuildkiteBuild, mock.Anything, mock.Anything).
		After(time.Hour).Return(response("passed"), nil)
	cad.OnActivity(buildkite.Activities.CancelBuildkiteBuild, mock.Anything,
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
		{"trigger_build", map[string]interface{}{"pipeline": "smoke-test", "env": map[string]string{"": "private-value"}}, "env keys"},
		{"trigger_build", map[string]interface{}{"pipeline": "smoke-test", "env": map[string]string{"BUILDKITE_REPO": "private-value"}}, "env keys"},
		{"get_build", map[string]interface{}{"pipeline": "smoke-test", "build_number": 0}, "positive"},
		{"cancel_build", map[string]interface{}{"pipeline": "smoke-test"}, "build_number"},
		{"wait_for_build", map[string]interface{}{"pipeline": "smoke-test", "build_number": 17, "timeout_seconds": 0}, "timeout_seconds"},
		{"wait_for_build", map[string]interface{}{"pipeline": "smoke-test", "build_number": 17, "poll_interval": -1}, "poll_interval"},
		{"wait_for_build", map[string]interface{}{"pipeline": "smoke-test", "build_number": 17, "cancel_on_timeout": "false"}, "cancel_on_timeout"},
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

func TestInvokeCancellationDuringSubmissionCleansUpAcknowledgedBuild(t *testing.T) {
	for _, backend := range []string{"cadence", "temporal"} {
		t.Run(backend, func(t *testing.T) {
			env := newEnvironment(t)
			var err error
			if backend == "cadence" {
				cad := env.Cadence.GetTestWorkflowEnvironment()
				cad.RegisterActivity(buildkite.Activities.TriggerBuildkiteBuild)
				cad.RegisterActivity(buildkite.Activities.SensorBuildkiteBuild)
				cad.RegisterActivity(buildkite.Activities.CancelBuildkiteBuild)
				cad.OnActivity(buildkite.Activities.TriggerBuildkiteBuild, mock.Anything, mock.Anything).
					Once().After(2*time.Second).Return(response("scheduled"), nil)
				cad.OnActivity(buildkite.Activities.CancelBuildkiteBuild, mock.Anything,
					buildkite.CancelBuildRequest{Pipeline: "smoke-test", BuildNumber: 17}).
					Once().Return(response("canceling"), nil)
				cad.RegisterDelayedCallback(cad.CancelWorkflow, time.Second)
				_, err = call(t, env, "invoke_build", map[string]any{"pipeline": "smoke-test"})
			} else {
				temp := env.Temporal.GetTestWorkflowEnvironment()
				temp.RegisterActivity(buildkite.Activities.TriggerBuildkiteBuild)
				temp.RegisterActivity(buildkite.Activities.SensorBuildkiteBuild)
				temp.RegisterActivity(buildkite.Activities.CancelBuildkiteBuild)
				temp.OnActivity(buildkite.Activities.TriggerBuildkiteBuild, mock.Anything, mock.Anything).
					Once().After(2*time.Second).Return(response("scheduled"), nil)
				temp.OnActivity(buildkite.Activities.CancelBuildkiteBuild, mock.Anything,
					buildkite.CancelBuildRequest{Pipeline: "smoke-test", BuildNumber: 17}).
					Once().Return(response("canceling"), nil)
				temp.RegisterDelayedCallback(temp.CancelWorkflow, time.Second)
				_, err = callTemporal(t, env, "invoke_build", map[string]any{"pipeline": "smoke-test"})
			}
			require.Error(t, err)
			if backend == "cadence" {
				assert.Contains(t, errorDetails(t, err, yarpcerrors.CodeUnknown.String()), "CanceledError")
			} else {
				assert.Contains(t, err.Error(), "canceled")
			}
		})
	}
}

func TestWaitCanObserveWithoutTakingOwnership(t *testing.T) {
	for _, backend := range []string{"cadence", "temporal"} {
		for _, cancel := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/cancel=%t", backend, cancel), func(t *testing.T) {
				env := newEnvironment(t)
				parameters := map[string]any{
					"pipeline": "smoke-test", "build_number": 17,
					"timeout_seconds": 2, "poll_interval": 1, "cancel_on_timeout": false,
				}
				var cleanupCalls atomic.Int32
				var err error
				if backend == "cadence" {
					cad := env.Cadence.GetTestWorkflowEnvironment()
					cad.RegisterActivity(buildkite.Activities.SensorBuildkiteBuild)
					cad.RegisterActivity(buildkite.Activities.CancelBuildkiteBuild)
					cad.OnActivity(buildkite.Activities.CancelBuildkiteBuild, mock.Anything, mock.Anything).
						Maybe().Run(func(mock.Arguments) { cleanupCalls.Add(1) }).
						Return(response("canceling"), nil)
					cad.OnActivity(buildkite.Activities.SensorBuildkiteBuild, mock.Anything, mock.Anything).
						After(2*time.Second).Return(nil, cadence.NewCustomError(yarpcerrors.CodeDeadlineExceeded.String(), "read deadline"))
					if cancel {
						cad.RegisterDelayedCallback(cad.CancelWorkflow, time.Second)
					}
					_, err = call(t, env, "wait_for_build", parameters)
				} else {
					temp := env.Temporal.GetTestWorkflowEnvironment()
					temp.RegisterActivity(buildkite.Activities.SensorBuildkiteBuild)
					temp.RegisterActivity(buildkite.Activities.CancelBuildkiteBuild)
					temp.OnActivity(buildkite.Activities.CancelBuildkiteBuild, mock.Anything, mock.Anything).
						Maybe().Run(func(mock.Arguments) { cleanupCalls.Add(1) }).
						Return(response("canceling"), nil)
					temp.OnActivity(buildkite.Activities.SensorBuildkiteBuild, mock.Anything, mock.Anything).
						After(2*time.Second).Return(nil, temporal.NewTimeoutError(enumspb.TIMEOUT_TYPE_SCHEDULE_TO_CLOSE, nil))
					if cancel {
						temp.RegisterDelayedCallback(temp.CancelWorkflow, time.Second)
					}
					_, err = callTemporal(t, env, "wait_for_build", parameters)
				}
				require.Error(t, err)
				assert.Zero(t, cleanupCalls.Load(), "observer waits must never cancel a shared build")
				if cancel {
					if backend == "cadence" {
						assert.Contains(t, errorDetails(t, err, yarpcerrors.CodeUnknown.String()), "CanceledError")
					} else {
						assert.Contains(t, err.Error(), "canceled")
					}
				} else {
					if backend == "cadence" {
						assert.Contains(t, errorDetails(t, err, yarpcerrors.CodeDeadlineExceeded.String()), "did not pass within 2 seconds")
					} else {
						assert.Contains(t, err.Error(), "did not pass within 2 seconds")
					}
				}
			})
		}
	}
}

type lateObservationWorkflow struct {
	workflow.Workflow
	now time.Time
	t   *testing.T
}

func (w *lateObservationWorkflow) Now(workflow.Context) time.Time { return w.now }

func (w *lateObservationWorkflow) WithActivityOptions(ctx workflow.Context, options workflow.ActivityOptions) workflow.Context {
	assert.Equal(w.t, time.Minute, options.ScheduleToCloseTimeout)
	return ctx
}

func (w *lateObservationWorkflow) ExecuteActivity(_ workflow.Context, _ any, _ ...any) workflow.Future {
	return &lateSuccessFuture{workflow: w}
}

type lateSuccessFuture struct {
	workflow.Future
	workflow *lateObservationWorkflow
}

func (f *lateSuccessFuture) Get(_ workflow.Context, result any) error {
	// The server completed successfully inside the schedule-to-close budget,
	// but the workflow task does not resume until after its own deadline.
	f.workflow.now = f.workflow.now.Add(2 * time.Minute)
	*result.(*buildkite.BuildResponse) = *response("passed")
	return nil
}

func TestSuccessfulSensorIsNotDiscardedByLateWorkflowTask(t *testing.T) {
	backend := &lateObservationWorkflow{now: time.Date(2026, 11, 19, 0, 0, 0, 0, time.UTC), t: t}
	ctx := context.WithValue(context.Background(), workflow.BackendContextKey, backend)
	result, err := waitForBuildRequest(ctx,
		buildkite.GetBuildRequest{Pipeline: "smoke-test", BuildNumber: 17},
		waitOptions{timeoutSeconds: 60, pollInterval: 1, cancelOnTimeout: true})
	require.NoError(t, err)
	var build buildkite.BuildResponse
	require.NoError(t, utils.AsGo(result, &build))
	assert.Equal(t, *response("passed"), build)
}

func TestCompiledDocumentationExampleRunsOnRemoteRuntime(t *testing.T) {
	env := newEnvironment(t)
	cad := env.Cadence.GetTestWorkflowEnvironment()
	for _, method := range []any{
		buildkite.Activities.TriggerBuildkiteBuild, buildkite.Activities.GetBuildkiteBuild, buildkite.Activities.SensorBuildkiteBuild,
	} {
		cad.RegisterActivity(method)
		cad.OnActivity(method, mock.Anything, mock.Anything).Once().Return(response("passed"), nil)
	}
	env.Cadence.ExecuteFunction("/documented.star", "release",
		starlark.Tuple{starlark.String("smoke-test"), starlark.String("main"), starlark.String("HEAD")}, nil, nil)
	var result map[string]buildkite.BuildResponse
	require.NoError(t, env.Cadence.GetResult(&result))
	assert.Equal(t, map[string]buildkite.BuildResponse{
		"snapshot": *response("passed"), "completed": *response("passed"),
	}, result)
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
	for _, operation := range []string{"trigger_build", "invoke_build", "cancel_build"} {
		t.Run(operation, func(t *testing.T) {
			env := newEnvironment(t)
			temp := env.Temporal.GetTestWorkflowEnvironment()
			var method any = buildkite.Activities.TriggerBuildkiteBuild
			parameters := map[string]any{"pipeline": "smoke-test"}
			if operation == "cancel_build" {
				method = buildkite.Activities.CancelBuildkiteBuild
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
	temp.RegisterActivity(buildkite.Activities.TriggerBuildkiteBuild)
	temp.RegisterActivity(buildkite.Activities.SensorBuildkiteBuild)
	temp.OnActivity(buildkite.Activities.TriggerBuildkiteBuild, mock.Anything, mock.Anything).
		Once().Return(response("scheduled"), nil)
	temp.OnActivity(buildkite.Activities.SensorBuildkiteBuild, mock.Anything, mock.Anything).
		Once().Return(nil, temporal.NewApplicationError("running", buildkite.NotReadyReason))
	temp.OnActivity(buildkite.Activities.SensorBuildkiteBuild, mock.Anything, mock.Anything).
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
	temp.RegisterActivity(buildkite.Activities.SensorBuildkiteBuild)
	temp.OnActivity(buildkite.Activities.SensorBuildkiteBuild, mock.Anything, mock.Anything).
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
			temp.RegisterActivity(buildkite.Activities.SensorBuildkiteBuild)
			temp.RegisterActivity(buildkite.Activities.CancelBuildkiteBuild)
			temp.OnActivity(buildkite.Activities.SensorBuildkiteBuild, mock.Anything, mock.Anything).
				After(2*time.Second).Return(nil, temporal.NewTimeoutError(enumspb.TIMEOUT_TYPE_SCHEDULE_TO_CLOSE, nil))
			temp.OnActivity(buildkite.Activities.CancelBuildkiteBuild, mock.Anything,
				buildkite.CancelBuildRequest{Pipeline: "smoke-test", BuildNumber: 17}).
				Once().Return(response("canceling"), nil)
			if cancel {
				temp.RegisterDelayedCallback(temp.CancelWorkflow, time.Second)
			}
			_, err := callTemporal(t, env, "wait_for_build", map[string]any{
				"pipeline": "smoke-test", "build_number": 17, "timeout_seconds": 2,
			})
			require.Error(t, err)
			if cancel {
				assert.Contains(t, err.Error(), "canceled")
				assert.NotContains(t, err.Error(), "deadline")
			} else {
				assert.Contains(t, err.Error(), "did not pass within 2 seconds")
			}
		})
	}
}

func TestCadenceWaitDoesNotCancelBeforeDeadline(t *testing.T) {
	for _, interval := range []int{7, 30} {
		t.Run(fmt.Sprint(interval), func(t *testing.T) {
			env := newEnvironment(t)
			cad := env.Cadence.GetTestWorkflowEnvironment()
			cad.RegisterActivity(buildkite.Activities.SensorBuildkiteBuild)
			cad.RegisterActivity(buildkite.Activities.CancelBuildkiteBuild)
			cad.OnActivity(buildkite.Activities.SensorBuildkiteBuild, mock.Anything, mock.Anything).
				Return(nil, cadence.NewCustomError(buildkite.NotReadyReason, "pending"))
			cad.OnActivity(buildkite.Activities.CancelBuildkiteBuild, mock.Anything, mock.Anything).
				Once().Return(response("canceling"), nil)
			started := cad.Now()
			_, err := call(t, env, "wait_for_build", map[string]any{
				"pipeline": "smoke-test", "build_number": 17,
				"timeout_seconds": 20, "poll_interval": interval,
			})
			require.Error(t, err)
			assert.GreaterOrEqual(t, cad.Now().Sub(started), 20*time.Second)
			assert.Contains(t, errorDetails(t, err, yarpcerrors.CodeDeadlineExceeded.String()), "20 seconds")
		})
	}
}

func TestTemporalWaitDoesNotCancelBeforeDeadline(t *testing.T) {
	for _, interval := range []int{7, 30} {
		t.Run(fmt.Sprint(interval), func(t *testing.T) {
			env := newEnvironment(t)
			temp := env.Temporal.GetTestWorkflowEnvironment()
			temp.RegisterActivity(buildkite.Activities.SensorBuildkiteBuild)
			temp.RegisterActivity(buildkite.Activities.CancelBuildkiteBuild)
			temp.OnActivity(buildkite.Activities.SensorBuildkiteBuild, mock.Anything, mock.Anything).
				Return(nil, temporal.NewApplicationError("pending", buildkite.NotReadyReason))
			temp.OnActivity(buildkite.Activities.CancelBuildkiteBuild, mock.Anything, mock.Anything).
				Once().Return(response("canceling"), nil)
			started := temp.Now()
			_, err := callTemporal(t, env, "wait_for_build", map[string]any{
				"pipeline": "smoke-test", "build_number": 17,
				"timeout_seconds": 20, "poll_interval": interval,
			})
			require.Error(t, err)
			assert.GreaterOrEqual(t, temp.Now().Sub(started), 20*time.Second)
			assert.Contains(t, err.Error(), "did not pass within 20 seconds")
		})
	}
}

func TestTemporalQueueTimeoutCanRecoverWithinWaitBudget(t *testing.T) {
	env := newEnvironment(t)
	temp := env.Temporal.GetTestWorkflowEnvironment()
	temp.RegisterActivity(buildkite.Activities.SensorBuildkiteBuild)
	temp.OnActivity(buildkite.Activities.SensorBuildkiteBuild, mock.Anything, mock.Anything).
		Once().After(70*time.Second).
		Return(nil, temporal.NewTimeoutError(enumspb.TIMEOUT_TYPE_SCHEDULE_TO_START, nil))
	temp.OnActivity(buildkite.Activities.SensorBuildkiteBuild, mock.Anything, mock.Anything).
		Once().Return(response("passed"), nil)
	started := temp.Now()
	got, err := callTemporal(t, env, "wait_for_build", map[string]any{
		"pipeline": "smoke-test", "build_number": 17,
		"timeout_seconds": 300, "poll_interval": 1,
	})
	require.NoError(t, err)
	assert.Equal(t, *response("passed"), got)
	assert.GreaterOrEqual(t, temp.Now().Sub(started), 70*time.Second)
	assert.Less(t, temp.Now().Sub(started), 300*time.Second)
}
