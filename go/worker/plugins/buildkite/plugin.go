package buildkite

import (
	"errors"
	"fmt"
	"time"

	"github.com/cadence-workflow/starlark-worker/service"
	"github.com/cadence-workflow/starlark-worker/workflow"
	"github.com/michelangelo-ai/michelangelo/go/worker/activities/buildkite"
	"github.com/michelangelo-ai/michelangelo/go/worker/plugins/utils"
	"go.starlark.net/starlark"
	"go.temporal.io/sdk/temporal"
	temporalworkflow "go.temporal.io/sdk/workflow"
	"go.uber.org/cadence"
	"go.uber.org/yarpc/yarpcerrors"
	"go.uber.org/zap"
)

const (
	defaultTimeoutSeconds = 3600
	defaultPollInterval   = 30
	maxTimeoutSeconds     = 86400
	maxPollInterval       = 3600
)

type waitOptions struct {
	timeoutSeconds  int64
	pollInterval    int64
	cancelOnTimeout bool
}

// triggerBuild submits once and returns the current snapshot without waiting.
func triggerBuild(t *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	req, err := unpackTrigger("trigger_build", args, kwargs, nil)
	if err != nil {
		return nil, argumentError(service.GetContext(t), err)
	}
	return executeBuild(singleCallContext(service.GetContext(t)), buildkite.Activities.TriggerBuildkiteBuild, req)
}

func getBuild(t *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	req, err := unpackBuild("get_build", args, kwargs, nil)
	if err != nil {
		return nil, argumentError(service.GetContext(t), err)
	}
	return executeBuild(singleCallContext(service.GetContext(t)), buildkite.Activities.GetBuildkiteBuild, req)
}

func waitForBuild(t *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	options := waitOptions{defaultTimeoutSeconds, defaultPollInterval, true}
	req, err := unpackBuild("wait_for_build", args, kwargs, &options)
	if err != nil {
		return nil, argumentError(service.GetContext(t), err)
	}
	return waitForBuildRequest(service.GetContext(t), req, options)
}

// invokeBuild submits once, then waits for passed within the timeout/poll budget.
// Unlike triggerBuild, terminal failure returns an error; timeout or workflow
// cancellation also requests best-effort cancellation of the submitted build.
func invokeBuild(t *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	options := waitOptions{defaultTimeoutSeconds, defaultPollInterval, true}
	req, err := unpackTrigger("invoke_build", args, kwargs, &options)
	if err != nil {
		return nil, argumentError(service.GetContext(t), err)
	}
	ctx := service.GetContext(t)
	// A canceled parent must not discard an in-flight submission's build
	// number. Finish the bounded, non-retried POST, then let the canceled
	// wait context request cleanup for the acknowledged build.
	triggerCtx, cancel := workflow.NewDisconnectedContext(ctx)
	defer cancel()
	callCtx := singleCallContext(triggerCtx)
	var build buildkite.BuildResponse
	if err := workflow.ExecuteActivity(callCtx, buildkite.Activities.TriggerBuildkiteBuild, req).Get(callCtx, &build); err != nil {
		return nil, err
	}
	return waitForBuildRequest(ctx, buildkite.GetBuildRequest{
		Pipeline: req.Pipeline, BuildNumber: build.Number,
	}, options)
}

func cancelBuild(t *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	req, err := unpackBuild("cancel_build", args, kwargs, nil)
	if err != nil {
		return nil, argumentError(service.GetContext(t), err)
	}
	return executeBuild(singleCallContext(service.GetContext(t)), buildkite.Activities.CancelBuildkiteBuild, buildkite.CancelBuildRequest(req))
}

func unpackTrigger(name string, args starlark.Tuple, kwargs []starlark.Tuple, options *waitOptions) (buildkite.TriggerBuildRequest, error) {
	req := buildkite.TriggerBuildRequest{Branch: "main", Commit: "HEAD"}
	var env, metadata starlark.Value = starlark.None, starlark.None
	parameters := []interface{}{
		"pipeline", &req.Pipeline,
		"branch?", &req.Branch,
		"commit?", &req.Commit,
		"message?", &req.Message,
		"env?", &env,
		"metadata?", &metadata,
	}
	if options != nil {
		parameters = append(parameters,
			"timeout_seconds?", &options.timeoutSeconds, "poll_interval?", &options.pollInterval)
	}
	if err := starlark.UnpackArgs(name, args, kwargs, parameters...); err != nil {
		return req, err
	}
	for _, field := range []struct {
		name  string
		value starlark.Value
		out   *map[string]string
	}{
		{"env", env, &req.Env},
		{"metadata", metadata, &req.Metadata},
	} {
		if field.value == starlark.None {
			continue
		}
		dictionary, ok := field.value.(*starlark.Dict)
		if !ok {
			return req, fmt.Errorf("%s must be a dict of strings or None", field.name)
		}
		for _, item := range dictionary.Items() {
			if _, ok := item[0].(starlark.String); !ok {
				return req, fmt.Errorf("%s keys must be strings", field.name)
			}
			if _, ok := item[1].(starlark.String); !ok {
				return req, fmt.Errorf("%s values must be strings", field.name)
			}
		}
		if err := utils.AsGo(field.value, field.out); err != nil {
			return req, fmt.Errorf("%s must be a dict of strings: %w", field.name, err)
		}
	}
	if err := req.Validate(); err != nil {
		return req, err
	}
	return req, options.validate()
}

func unpackBuild(name string, args starlark.Tuple, kwargs []starlark.Tuple, options *waitOptions) (buildkite.GetBuildRequest, error) {
	var req buildkite.GetBuildRequest
	parameters := []interface{}{"pipeline", &req.Pipeline, "build_number", &req.BuildNumber}
	if options != nil {
		parameters = append(parameters,
			"timeout_seconds?", &options.timeoutSeconds, "poll_interval?", &options.pollInterval,
			"cancel_on_timeout?", &options.cancelOnTimeout)
	}
	if err := starlark.UnpackArgs(name, args, kwargs, parameters...); err != nil {
		return req, err
	}
	if err := req.Validate(); err != nil {
		return req, err
	}
	return req, options.validate()
}

func (o *waitOptions) validate() error {
	if o == nil {
		return nil
	}
	if o.timeoutSeconds <= 0 || o.timeoutSeconds > maxTimeoutSeconds {
		return fmt.Errorf("timeout_seconds must be between 1 and %d", maxTimeoutSeconds)
	}
	if o.pollInterval <= 0 || o.pollInterval > maxPollInterval {
		return fmt.Errorf("poll_interval must be between 1 and %d", maxPollInterval)
	}
	return nil
}

func singleCallContext(ctx workflow.Context) workflow.Context {
	options := workflow.ActivityOptions{
		ScheduleToStartTimeout: time.Minute,
		StartToCloseTimeout:    35 * time.Second,
		ScheduleToCloseTimeout: 95 * time.Second,
	}
	// Cadence disables retries with nil; Temporal defaults to retrying. Their
	// SDK contexts are distinct, so select the native no-retry representation.
	if _, temporalBackend := ctx.(temporalworkflow.Context); temporalBackend {
		options.RetryPolicy = &workflow.RetryPolicy{
			InitialInterval: time.Second,
			MaximumAttempts: 1,
		}
	}
	return workflow.WithActivityOptions(ctx, options)
}

func argumentError(ctx workflow.Context, err error) error {
	return newWorkflowError(ctx, yarpcerrors.CodeInvalidArgument.String(), err.Error())
}

func newWorkflowError(ctx workflow.Context, reason, message string) error {
	if _, temporalBackend := ctx.(temporalworkflow.Context); temporalBackend {
		return temporal.NewApplicationError(message, reason)
	}
	return cadence.NewCustomError(reason, message)
}

func executeBuild(ctx workflow.Context, activity interface{}, request interface{}) (starlark.Value, error) {
	var response buildkite.BuildResponse
	if err := workflow.ExecuteActivity(ctx, activity, request).Get(ctx, &response); err != nil {
		return nil, err
	}
	var result starlark.Value
	if err := utils.AsStar(response, &result); err != nil {
		return nil, fmt.Errorf("convert build response: %w", err)
	}
	return result, nil
}

func waitForBuildRequest(ctx workflow.Context, req buildkite.GetBuildRequest, options waitOptions) (starlark.Value, error) {
	// Both native SDK contexts expose Err. In particular, an invoke whose
	// disconnected submission just completed must clean up immediately if
	// its parent was canceled, without scheduling another status read.
	if parent, ok := ctx.(interface{ Err() error }); ok && parent.Err() != nil {
		return cancelWait(ctx, req, options, parent.Err())
	}
	timeout := time.Duration(options.timeoutSeconds) * time.Second
	deadline := workflow.Now(ctx).Add(timeout)
	poll := time.Duration(options.pollInterval) * time.Second
	for remaining := timeout; remaining > 0; remaining = deadline.Sub(workflow.Now(ctx)) {
		policy := utils.DefaultSensorRetryPolicy
		policy.InitialInterval = poll
		policy.MaximumInterval = poll
		policy.ExpirationInterval = remaining
		sensorCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
			// Queue availability consumes the overall wait budget. A separate,
			// shorter queue timeout can be non-retryable on Temporal.
			ScheduleToStartTimeout: remaining,
			StartToCloseTimeout:    min(35*time.Second, remaining),
			ScheduleToCloseTimeout: remaining,
			RetryPolicy:            &policy,
		})
		result, err := executeBuild(sensorCtx, buildkite.Activities.SensorBuildkiteBuild, req)
		if err == nil {
			// ScheduleToCloseTimeout bounds completion on the server. A slow
			// workflow task can observe that success after our local deadline.
			return result, nil
		}
		if workflow.IsCanceledError(ctx, err) {
			return cancelWait(ctx, req, options, err)
		}
		if !retryableSensorError(err) {
			return nil, err
		}
		remaining = deadline.Sub(workflow.Now(ctx))
		if remaining > 0 {
			// A retry policy may stop when its next backoff would cross the
			// expiration time, before our actual deadline. Keep that remainder
			// open using a durable timer rather than cancelling early.
			if err := workflow.Sleep(ctx, min(poll, remaining)); err != nil {
				return cancelWait(ctx, req, options, err)
			}
		}
	}
	return cancelWait(ctx, req, options, newWorkflowError(ctx, yarpcerrors.CodeDeadlineExceeded.String(),
		fmt.Sprintf("build %s/%d did not pass within %d seconds",
			req.Pipeline, req.BuildNumber, options.timeoutSeconds)))
}

func cancelWait(ctx workflow.Context, req buildkite.GetBuildRequest, options waitOptions, original error) (starlark.Value, error) {
	if !options.cancelOnTimeout {
		return nil, original
	}
	// Parent cancellation must not suppress cleanup or be replaced by its error.
	cleanupCtx, cancel := workflow.NewDisconnectedContext(ctx)
	defer cancel()
	cleanupCtx = singleCallContext(cleanupCtx)
	if _, err := executeBuild(cleanupCtx, buildkite.Activities.CancelBuildkiteBuild, buildkite.CancelBuildRequest(req)); err != nil {
		workflow.GetLogger(ctx).Error("buildkite-cleanup-failed",
			zap.String("pipeline", req.Pipeline), zap.Int64("build_number", req.BuildNumber), zap.Error(err))
	}
	return nil, original
}

func retryableSensorError(err error) bool {
	if cadence.IsTimeoutError(err) || temporal.IsTimeoutError(err) {
		return true
	}
	switch errorReason(err) {
	case buildkite.NotReadyReason, yarpcerrors.CodeUnavailable.String(), yarpcerrors.CodeDeadlineExceeded.String():
		return true
	}
	return false
}

func errorReason(err error) string {
	var customError workflow.CustomError
	if errors.As(err, &customError) {
		return customError.Reason()
	}
	var applicationError *temporal.ApplicationError
	if errors.As(err, &applicationError) {
		return applicationError.Type()
	}
	return ""
}
