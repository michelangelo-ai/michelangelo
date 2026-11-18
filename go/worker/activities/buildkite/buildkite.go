// Package buildkite implements bounded Buildkite REST API activities.
package buildkite

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/cadence-workflow/starlark-worker/activity"
	temporalactivity "go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"
	"go.uber.org/cadence"
	"go.uber.org/yarpc/yarpcerrors"
	"go.uber.org/zap"
)

// NotReadyReason identifies a pending build, which the workflow may poll again.
const NotReadyReason = "buildkite-not-ready"

// Activities is the sentinel used to schedule registered activity methods.
var Activities = (*activities)(nil)

// TriggerBuildRequest describes a submission. Env and Metadata must not contain
// secrets: activity inputs are persisted in workflow history.
type TriggerBuildRequest struct {
	Pipeline string            `json:"pipeline"`
	Branch   string            `json:"branch"`
	Commit   string            `json:"commit"`
	Message  string            `json:"message"`
	Env      map[string]string `json:"env,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// GetBuildRequest identifies a build to read or poll by its number, not its UUID.
type GetBuildRequest struct {
	Pipeline    string `json:"pipeline"`
	BuildNumber int64  `json:"build_number"`
}

// CancelBuildRequest identifies a build to cancel by its number, not its UUID.
type CancelBuildRequest struct {
	Pipeline    string `json:"pipeline"`
	BuildNumber int64  `json:"build_number"`
}

// BuildResponse is the shared Python/Starlark snapshot. Raw builds can contain
// secrets and large job lists, so only these fields cross the activity boundary.
type BuildResponse struct {
	// ID is Buildkite's globally unique build UUID.
	ID string `json:"id"`
	// Number is pipeline-scoped; use it for subsequent get, wait, and cancel calls.
	Number   int64  `json:"number"`
	Pipeline string `json:"pipeline"`
	State    string `json:"state"`
	WebURL   string `json:"web_url"`
	Branch   string `json:"branch"`
	Commit   string `json:"commit"`
}

type activities struct {
	client *client
}

// Validate checks a submission before any external effect.
func (r TriggerBuildRequest) Validate() error {
	if err := validateSlug(r.Pipeline); err != nil {
		return fmt.Errorf("pipeline %w", err)
	}
	if r.Branch == "" || r.Commit == "" {
		return fmt.Errorf("branch and commit must be non-empty")
	}
	return nil
}

// Validate checks the pipeline slug and positive build number.
func (r GetBuildRequest) Validate() error {
	if err := validateSlug(r.Pipeline); err != nil {
		return fmt.Errorf("pipeline %w", err)
	}
	if r.BuildNumber <= 0 {
		return fmt.Errorf("build_number must be positive")
	}
	return nil
}

// Validate checks the pipeline slug and positive build number.
func (r CancelBuildRequest) Validate() error {
	return GetBuildRequest(r).Validate()
}

// TriggerBuild submits once. Retrying an ambiguous submission can create another
// build, so the plugin disables mutation retries on both workflow backends.
func (a *activities) TriggerBuild(ctx context.Context, req TriggerBuildRequest) (*BuildResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, activityError(ctx, yarpcerrors.CodeInvalidArgument.String(), err.Error())
	}
	payload := struct {
		Branch   string            `json:"branch"`
		Commit   string            `json:"commit"`
		Message  string            `json:"message"`
		Env      map[string]string `json:"env,omitempty"`
		Metadata map[string]string `json:"meta_data,omitempty"`
	}{req.Branch, req.Commit, req.Message, req.Env, req.Metadata}
	snapshot, err := a.client.request(ctx, http.MethodPost, req.Pipeline, "", payload)
	return publicBuild(snapshot, err)
}

// GetBuild returns a snapshot, including terminal failure states.
func (a *activities) GetBuild(ctx context.Context, req GetBuildRequest) (*BuildResponse, error) {
	snapshot, err := a.getBuild(ctx, req)
	return publicBuild(snapshot, err)
}

func (a *activities) getBuild(ctx context.Context, req GetBuildRequest) (*buildSnapshot, error) {
	if err := req.Validate(); err != nil {
		return nil, activityError(ctx, yarpcerrors.CodeInvalidArgument.String(), err.Error())
	}
	snapshot, err := a.client.request(ctx, http.MethodGet, req.Pipeline,
		"/"+strconv.FormatInt(req.BuildNumber, 10)+"?exclude_jobs=true", nil)
	if err != nil {
		return nil, err
	}
	if snapshot.build.Number != req.BuildNumber {
		return nil, activityError(ctx, yarpcerrors.CodeInternal.String(), "Buildkite returned a different build number")
	}
	return snapshot, nil
}

// SensorGetBuild makes one status check. The workflow owns durable polling.
func (a *activities) SensorGetBuild(ctx context.Context, req GetBuildRequest) (*BuildResponse, error) {
	snapshot, err := a.getBuild(ctx, req)
	if err != nil {
		return nil, err
	}
	build := snapshot.build
	// A block step can report passed or failed while approval is outstanding.
	// Do not treat either as terminal until the independent blocked flag clears.
	if snapshot.blocked {
		return nil, newActivityError(ctx, NotReadyReason,
			fmt.Sprintf("build %s/%d is blocked", req.Pipeline, req.BuildNumber))
	}
	switch build.State {
	case "passed":
		return build, nil
	case "failed", "canceled", "skipped", "not_run":
		return nil, activityError(ctx, yarpcerrors.CodeInternal.String(),
			fmt.Sprintf("build %s/%d ended with state %s: %s",
				req.Pipeline, req.BuildNumber, build.State, build.WebURL))
	case "creating", "scheduled", "waiting", "running", "failing", "blocked", "canceling":
		return nil, newActivityError(ctx, NotReadyReason,
			fmt.Sprintf("build %s/%d is %s", req.Pipeline, req.BuildNumber, build.State))
	default:
		return nil, activityError(ctx, yarpcerrors.CodeInternal.String(),
			fmt.Sprintf("build %s/%d has unknown state %s", req.Pipeline, req.BuildNumber, build.State))
	}
}

// CancelBuild requests cancellation once; its snapshot may still be canceling.
func (a *activities) CancelBuild(ctx context.Context, req CancelBuildRequest) (*BuildResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, activityError(ctx, yarpcerrors.CodeInvalidArgument.String(), err.Error())
	}
	snapshot, err := a.client.request(ctx, http.MethodPut, req.Pipeline,
		"/"+strconv.FormatInt(req.BuildNumber, 10)+"/cancel", nil)
	if err != nil {
		return nil, err
	}
	if snapshot.build.Number != req.BuildNumber {
		return nil, activityError(ctx, yarpcerrors.CodeInternal.String(), "Buildkite returned a different build number")
	}
	return snapshot.build, nil
}

func publicBuild(snapshot *buildSnapshot, err error) (*BuildResponse, error) {
	if err != nil {
		return nil, err
	}
	return snapshot.build, nil
}

func validateSlug(value string) error {
	if value != "" {
		valid := true
		for index, character := range value {
			if character >= 'a' && character <= 'z' || character >= '0' && character <= '9' ||
				index > 0 && (character == '-' || character == '_') {
				continue
			}
			valid = false
			break
		}
		if valid {
			return nil
		}
	}
	return fmt.Errorf("must be a slug matching [a-z0-9][a-z0-9_-]*")
}

func activityError(ctx context.Context, reason, message string) error {
	if logger := activity.GetLogger(ctx); logger != nil {
		logger.Error("buildkite-activity-error", zap.String("reason", reason), zap.String("message", message))
	}
	return newActivityError(ctx, reason, message)
}

func newActivityError(ctx context.Context, reason, message string) error {
	// starlark-worker v1.0.15 wraps Temporal application errors in a type the
	// Temporal failure converter cannot recognize. Return the native error so
	// sensor retry policies see the actual error type, not "TemporalCustomError".
	if temporalactivity.IsActivity(ctx) {
		return temporal.NewApplicationError(message, reason)
	}
	return cadence.NewCustomError(reason, message)
}
