package ray

import (
	"math"
	"time"

	"github.com/cadence-workflow/starlark-worker/ext"
	"github.com/cadence-workflow/starlark-worker/service"
	"github.com/cadence-workflow/starlark-worker/workflow"
	"go.starlark.net/starlark"
	"go.uber.org/yarpc/yarpcerrors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/michelangelo-ai/michelangelo/go/worker/activities/ray"
	"github.com/michelangelo-ai/michelangelo/go/worker/plugins/utils"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// create_cluster's wait for the cluster it created is versioned. Workflow executions that reached it before
// the provisioning budget was deployed keep the single readiness sensor bounded by timeout_seconds
// (utils.DefaultVersion); the others wait with the provisioning budget (provisioningBudgetVersion).
const (
	provisioningBudgetChangeID = "ray-provisioning-budget"
	provisioningBudgetVersion  = 1
)

// maxAdmissionWait caps one wait for Kueue to admit a SUSPENDED cluster at the Uniflow workflow timeout. The
// control plane's admission clock (jobs.scheduler.kueue.admissionTimeout, default 24h) fails the cluster
// first, which ends the wait.
const maxAdmissionWait = 7 * 24 * time.Hour

// cleanupTimeout bounds each activity that ends a failed provisioning, like the default retry policy's
// expiration.
const cleanupTimeout = 5 * time.Minute

// awaitProvisioning waits within the provisioning budget for the cluster create_cluster created to be READY.
// The budget runs from creation to READY, excluding the time the cluster is SUSPENDED (held by Kueue), and is
// cumulative across evictions; waits for placement count against it. The remaining budget is workflow state:
// a replay recomputes it from the same recorded activity times.
//
// A cluster that does not become READY (the budget runs out, or the cluster is FAILED or TERMINATED) is
// terminated with its latest condition, which is also the task error.
func awaitProvisioning(t *starlark.Thread, ctx workflow.Context, cluster v2pb.RayCluster, activityID string, budget time.Duration) (starlark.Value, error) {
	logger := workflow.GetLogger(ctx)
	interval := time.Second * time.Duration(poll)
	sensorRequest := v2pb.GetRayClusterRequest{
		Name:       cluster.Name,
		Namespace:  cluster.Namespace,
		GetOptions: &metav1.GetOptions{},
	}
	admissionCtx := workflow.WithActivityOptions(ctx, activityOptions(interval, maxAdmissionWait))

	remaining := budget
	for {
		if remaining <= 0 {
			return failProvisioning(ctx, cluster, workflow.NewCustomError(ctx, yarpcerrors.CodeDeadlineExceeded.String(), "provisioning budget exhausted"))
		}
		// Poll until the cluster is READY or SUSPENDED, for at most the remaining budget.
		sensorCtx := workflow.WithActivityOptions(ctx, activityOptions(interval, remaining))
		var sensorResponse ray.SensorRayClusterProvisioningResponse
		start := workflow.Now(ctx)
		err := workflow.ExecuteActivity(sensorCtx, ray.Activities.SensorRayClusterProvisioning, sensorRequest).Get(ctx, &sensorResponse)
		remaining -= workflow.Now(ctx).Sub(start)
		if err != nil {
			return failProvisioning(ctx, cluster, err)
		}

		if sensorResponse.Ready {
			if sensorResponse.JobURL != "" {
				t.Print(t, "ray | create cluster: url="+sensorResponse.JobURL)
			}
			res, err := clusterResponse(sensorResponse.RayCluster, activityID)
			if err != nil {
				logger.Error("builtin-error", ext.ZapError(err)...)
				return nil, err
			}
			return res, nil
		}

		// SUSPENDED: Kueue holds the cluster. Waiting for admission spends no budget.
		if err := workflow.ExecuteActivity(admissionCtx, ray.Activities.SensorRayClusterAdmission, sensorRequest).Get(ctx, nil); err != nil {
			return failProvisioning(ctx, cluster, err)
		}
	}
}

// activityOptions are the workflow's default activity options retried every interval for at most expiration.
// The expiration is both the retry policy's ExpirationInterval, which Cadence enforces, and the ScheduleToClose
// timeout, which Temporal enforces: starlark-worker drops ExpirationInterval on Temporal. It is also the
// ScheduleToStart timeout, as Cadence makes it, so that a queued attempt does not fail before the expiration on
// Temporal either. The retry policy's MaximumAttempts is maxAttempts, so the expiration always ends the retries.
func activityOptions(interval, expiration time.Duration) workflow.ActivityOptions {
	retryPolicy := utils.DefaultSensorRetryPolicy
	retryPolicy.InitialInterval = interval
	retryPolicy.ExpirationInterval = expiration
	retryPolicy.MaximumAttempts = maxAttempts(interval, expiration)

	options := service.DefaultActivityOptions
	options.ScheduleToStartTimeout = expiration
	options.ScheduleToCloseTimeout = expiration
	options.RetryPolicy = &retryPolicy
	return options
}

// maxAttempts bounds the attempts of an activity retried every interval for at most expiration at one more than
// fit in the expiration, so that the bound never ends the retries before the expiration does. It replaces an
// unlimited policy, which the Temporal test environment caps at 10 attempts. 0 (unlimited) for a non-positive
// interval.
func maxAttempts(interval, expiration time.Duration) int32 {
	if interval <= 0 {
		return 0
	}
	return int32(min(int64(expiration/interval)+2, math.MaxInt32))
}

// failProvisioning ends a provisioning wait that will not reach READY. It reads the cluster's latest condition
// before terminating the cluster, because the termination writes a Succeeded condition that would replace it,
// then terminates the cluster with that condition and returns it as the task error. A canceled wait terminates
// the cluster as "Canceled" and returns the cancellation, as the default version does.
func failProvisioning(ctx workflow.Context, cluster v2pb.RayCluster, cause error) (starlark.Value, error) {
	logger := workflow.GetLogger(ctx)
	logger.Error("builtin-error", ext.ZapError(cause)...)

	canceled := workflow.IsCanceledError(ctx, cause)
	if canceled {
		ctx, _ = workflow.NewDisconnectedContext(ctx)
	}
	ctx = workflow.WithActivityOptions(ctx, activityOptions(time.Second*time.Duration(poll), cleanupTimeout))
	if canceled {
		terminateFailedCluster(ctx, cluster, "Canceled")
		return nil, cause
	}

	var latest v2pb.GetRayClusterResponse
	if err := workflow.ExecuteActivity(ctx, ray.Activities.GetRayCluster, v2pb.GetRayClusterRequest{
		Name:       cluster.Name,
		Namespace:  cluster.Namespace,
		GetOptions: &metav1.GetOptions{},
	}).Get(ctx, &latest); err != nil {
		logger.Error("builtin-error", ext.ZapError(err)...)
	}
	reason := provisioningFailure(latest.RayCluster, cause.Error())
	terminateFailedCluster(ctx, cluster, reason)
	return nil, workflow.NewCustomError(ctx, reason)
}

func terminateFailedCluster(ctx workflow.Context, cluster v2pb.RayCluster, reason string) {
	if err := workflow.ExecuteActivity(ctx, ray.Activities.TerminateCluster, ray.TerminateClusterRequest{
		Name:      cluster.Name,
		Namespace: cluster.Namespace,
		Type:      v2pb.TERMINATION_TYPE_FAILED.String(),
		Reason:    reason,
	}).Get(ctx, nil); err != nil {
		workflow.GetLogger(ctx).Error("builtin-error", ext.ZapError(err)...)
	}
}

// provisioningFailure is the reason a failed provisioning reports: the cluster's latest condition as
// "<reason>: <message>" (just the reason when it has no message), or fallback when the cluster could not be
// read or has no condition with a reason.
func provisioningFailure(cluster *v2pb.RayCluster, fallback string) string {
	if cluster == nil {
		return fallback
	}
	condition := latestCondition(cluster.Status.StatusConditions)
	if condition == nil {
		return fallback
	}
	if condition.Message == "" {
		return condition.Reason
	}
	return condition.Reason + ": " + condition.Message
}

// latestCondition picks the condition that explains why a cluster did not become ready: Succeeded=False (it
// failed or was terminated, e.g. AdmissionTimeout), else Enqueued=False (it could not be placed), else the
// newest condition by timestamp, the later one in the list on a tie. Conditions without a reason explain
// nothing and are skipped.
func latestCondition(conditions []*apipb.Condition) *apipb.Condition {
	var enqueuedFalse, newest *apipb.Condition
	for _, condition := range conditions {
		if condition == nil || condition.Reason == "" {
			continue
		}
		if condition.Status == apipb.CONDITION_STATUS_FALSE {
			switch condition.Type {
			case utils.SucceededCondition:
				return condition
			case utils.EnqueuedCondition:
				enqueuedFalse = condition
			}
		}
		if newest == nil || condition.LastUpdatedTimestamp >= newest.LastUpdatedTimestamp {
			newest = condition
		}
	}
	if enqueuedFalse != nil {
		return enqueuedFalse
	}
	return newest
}

// clusterResponse is create_cluster's result for a ready cluster.
func clusterResponse(cluster *v2pb.RayCluster, activityID string) (starlark.Value, error) {
	var res starlark.Value
	if err := utils.AsStar(map[string]interface{}{
		"rayCluster": cluster,
		"activityId": activityID,
	}, &res); err != nil {
		return nil, err
	}
	return res, nil
}
