package cachedoutput

import (
	"context"

	"github.com/cadence-workflow/starlark-worker/activity"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

var Activities = (*activities)(nil)

// ShouldOverrideCacheForRetryRequest identifies the task asking whether its
// declared-false cache_enabled default should be overridden.
type ShouldOverrideCacheForRetryRequest struct {
	Namespace string `json:"namespace,omitempty"`
	TaskPath  string `json:"task_path,omitempty"`
	TaskName  string `json:"task_name,omitempty"`
}

// ShouldOverrideCacheForRetryResponse reports the backend's live decision.
//
// HasOverride is false when there's no active manual retry on this pipeline
// run, in which case the caller should fall back to its own env-based default.
//
// ActivityID is this decision activity's own ID. Tasks report it as their
// first activity so a manual retry resets the workflow to before the cache
// decision, making the retried task re-decide live (cache off) instead of
// replaying whatever it decided last time.
type ShouldOverrideCacheForRetryResponse struct {
	HasOverride bool   `json:"has_override"`
	UseCache    bool   `json:"use_cache"`
	ActivityID  string `json:"activity_id"`
}

// TerminateClusterRequest defines the request parameters for terminating a Spark cluster.
type TerminateClusterRequest struct {
	Name      string `json:"name,omitempty"`      // name of the spark job
	Namespace string `json:"namespace,omitempty"` // namespace of the spark job
	Type      string `json:"type,omitempty"`      // termination code
	Reason    string `json:"reason,omitempty"`    // termination reason
}

// TerminateSparkJobRequest defines the request parameters for terminating a Spark job.
type TerminateSparkJobRequest struct {
	Name      string               `json:"name,omitempty"`      // name of the spark job
	Namespace string               `json:"namespace,omitempty"` // namespace of the spark job
	Type      v2pb.TerminationType `json:"type,omitempty"`      // termination code
	Reason    string
}

// activities struct encapsulates the YARPC clients for Spark cluster and job services.
type activities struct {
	cachedOutput       v2pb.CachedOutputServiceYARPCClient
	pipelineRunService v2pb.PipelineRunServiceYARPCClient
}

func (r *activities) GetCachedOutput(ctx context.Context, request v2pb.GetCachedOutputRequest) (*v2pb.GetCachedOutputResponse, error) {
	return r.cachedOutput.GetCachedOutput(ctx, &request)
}

func (r *activities) ListCachedOutput(ctx context.Context, request v2pb.ListCachedOutputRequest) (*v2pb.ListCachedOutputResponse, error) {
	return r.cachedOutput.ListCachedOutput(ctx, &request)
}

func (r *activities) CreateCachedOutput(ctx context.Context, request v2pb.CreateCachedOutputRequest) (*v2pb.CreateCachedOutputResponse, error) {
	return r.cachedOutput.CreateCachedOutput(ctx, &request)
}

// ShouldOverrideCacheForRetry decides, at the activity boundary, whether a task's
// declared-false cache_enabled default should be overridden for a manual retry.
//
// This exists because Temporal Reset replays a workflow forward using the SAME
// input/env baked in at the original StartWorkflow call - a retry cannot change
// CACHE_ENABLED for the run it resets. Every task swept up by the reset boundary
// (not just the one being retried) would otherwise be forced to genuinely
// re-execute. An activity, unlike the frozen env, re-executes with live state
// after a Reset, so it can read the pipeline run's current RetryInfo and decide
// per task: the exact retry target keeps caching off (real re-execution); every
// other task gets caching turned on (replay from its own prior CachedOutput).
//
// Errors are treated as "no override" (falls back to the caller's env-based
// default) rather than failing the activity - a lookup failure here should never
// block the retried task from running.
func (r *activities) ShouldOverrideCacheForRetry(ctx context.Context, request ShouldOverrideCacheForRetryRequest) (*ShouldOverrideCacheForRetryResponse, error) {
	info := activity.GetInfo(ctx)
	noOverride := &ShouldOverrideCacheForRetryResponse{ActivityID: info.ActivityID}

	response, err := r.pipelineRunService.GetPipelineRun(ctx, &v2pb.GetPipelineRunRequest{
		Namespace: request.Namespace,
		Name:      info.WorkflowExecution.ID,
	})
	if err != nil || response == nil || response.PipelineRun == nil {
		return noOverride, nil
	}

	// RetryInfo is never cleared, so its presence alone doesn't mean a retry is in
	// effect. The controller (processManualRetrySpec) treats a request as pending
	// while retryInfo.workflowRunId still names the live run, and processing it
	// Resets that run into a new one. So a retry is in effect exactly when the run
	// it named is no longer the one executing this activity. The live RunID is
	// used rather than status.workflowRunId because the reset run starts (and
	// makes these decisions) before the controller has written the new ID back.
	retryInfo := response.PipelineRun.Spec.RetryInfo
	if retryInfo == nil || retryInfo.ActivityId == "" || retryInfo.WorkflowRunId == "" {
		return noOverride, nil
	}
	if retryInfo.WorkflowRunId == info.WorkflowExecution.RunID {
		return noOverride, nil
	}

	target := findStepByActivityID(response.PipelineRun.Status.Steps, retryInfo.ActivityId)
	isRetryTarget := target != nil && target.Name == request.TaskPath && target.DisplayName == request.TaskName

	return &ShouldOverrideCacheForRetryResponse{
		HasOverride: true,
		UseCache:    !isRetryTarget,
		ActivityID:  info.ActivityID,
	}, nil
}

// findStepByActivityID recursively searches a pipeline run's step tree for the
// step whose ActivityId matches, returning nil when not found.
func findStepByActivityID(steps []*v2pb.PipelineRunStepInfo, activityID string) *v2pb.PipelineRunStepInfo {
	for _, step := range steps {
		if step == nil {
			continue
		}
		if step.ActivityId == activityID {
			return step
		}
		if found := findStepByActivityID(step.SubSteps, activityID); found != nil {
			return found
		}
	}
	return nil
}
