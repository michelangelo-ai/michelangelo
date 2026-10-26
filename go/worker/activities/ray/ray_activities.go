package ray

import (
	"context"
	"errors"

	"github.com/cadence-workflow/starlark-worker/activity"
	"github.com/cadence-workflow/starlark-worker/workflow"
	"github.com/gogo/protobuf/proto"
	tmpactivity "go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"
	"go.uber.org/yarpc"
	"go.uber.org/yarpc/yarpcerrors"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/michelangelo-ai/michelangelo/go/worker/runnertoken"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

var Activities = (*activities)(nil)

// TerminateClusterRequest defines the request parameters for terminating a Ray cluster.
type TerminateClusterRequest struct {
	Name      string `json:"name,omitempty"`      // name of the ray job
	Namespace string `json:"namespace,omitempty"` // namespace of the ray job
	Type      string `json:"type,omitempty"`      // termination code
	Reason    string `json:"reason,omitempty"`    // termination reason
}

// TerminateSparkJobRequest defines the request parameters for terminating a Spark job.
type TerminateSparkJobRequest struct {
	Name      string `json:"name,omitempty"`      // name of the spark job
	Namespace string `json:"namespace,omitempty"` // namespace of the spark job
	Type      string `json:"type,omitempty"`      // termination code
	Reason    string
}

// activities struct encapsulates the YARPC clients for Ray cluster and job services.
type activities struct {
	rayClusterService v2pb.RayClusterServiceYARPCClient
	rayJobService     v2pb.RayJobServiceYARPCClient
}

// CreateRayClusterActivityResponse includes the activity ID for progress reporting
type CreateRayClusterActivityResponse struct {
	RayCluster *v2pb.RayCluster
	ActivityID string `json:"activity_id"` // The actual activity ID from workflow engine
}

// CreateRayJob creates a new Ray job using the provided request parameters and returns activity ID.
//
// This method is executed as part of a Starlark worker activity.
//
// Params:
// - ctx: The context for the operation.
// - request: The request containing details of the Ray job to create.
//
// Returns:
// - *CreateRayJobActivityResponse: Response containing the created Ray job details and activity ID.
// - *workflow.CustomError: Error information if the operation fails.
func (r *activities) CreateRayJob(ctx context.Context, request v2pb.CreateRayJobRequest) (
	*v2pb.CreateRayJobResponse, error) {
	logger := log.FromContext(ctx)
	logger.Info("activity-start", zap.Any("request", request))

	// Get activity ID from Temporal or Cadence context
	activityInfo := activity.GetInfo(ctx)
	activityID := activityInfo.ActivityID
	logger.Info("activity-id-extracted", zap.String("activityID", activityID))

	// Execute the original job creation logic
	if request.RayJob != nil {
		ctx = runnertoken.WithNamespace(ctx, request.RayJob.Namespace)
	}
	createRayJobResponse, err := r.rayJobService.CreateRayJob(ctx, &request)
	if err != nil || createRayJobResponse == nil || createRayJobResponse.RayJob == nil ||
		createRayJobResponse.RayJob.Name == "" {
		logger.Error(err, "activity-error")
		return nil, workflow.NewCustomError(ctx, yarpcerrors.CodeUnavailable.String(), err.Error())
	}

	return createRayJobResponse, nil
}

// CreateRayCluster creates a new Ray cluster and returns it with activity ID for retry tracking.
//
// This method is executed as part of a Starlark worker activity.
//
// Params:
// - ctx: The context for the operation.
// - request: The details of the Ray cluster to create.
//
// Returns:
// - *CreateRayClusterActivityResponse: Response containing the created cluster details and activity ID.
// - *workflow.CustomError: Error information if the operation fails.
func (r *activities) CreateRayCluster(ctx context.Context, request v2pb.CreateRayClusterRequest) (
	*CreateRayClusterActivityResponse, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("activity-start", zap.Any("request", request))

	// Get activity ID from Temporal or Cadence context
	activityInfo := activity.GetInfo(ctx)
	activityID := activityInfo.ActivityID
	logger.Info("activity-id-extracted", zap.String("activityID", activityID))

	// Execute the original cluster creation logic
	if request.RayCluster != nil {
		ctx = runnertoken.WithNamespace(ctx, request.RayCluster.Namespace)
	}
	createRayClusterResponse, err := r.rayClusterService.CreateRayCluster(ctx, &request)
	if err != nil {
		logger.Error("activity-error: failed to create ray cluster", zap.Error(err))
		return &CreateRayClusterActivityResponse{ActivityID: activityID}, nil
	}
	if createRayClusterResponse == nil || createRayClusterResponse.RayCluster == nil ||
		createRayClusterResponse.RayCluster.Name == "" {
		logger.Error("activity-error: empty or invalid response from CreateRayCluster")
		return &CreateRayClusterActivityResponse{ActivityID: activityID}, nil
	}

	return &CreateRayClusterActivityResponse{
		RayCluster: createRayClusterResponse.RayCluster,
		ActivityID: activityID,
	}, nil
}

// GetRayCluster retrieves details of a Ray cluster.
//
// This method is executed as part of a Starlark worker activity.
//
// Params:
// - ctx: The context for the operation.
// - request: Request containing the cluster name and namespace.
//
// Returns:
// - *v2pb.GetRayClusterResponse: Response containing the cluster details.
// - *workflow.CustomError: Error information if the operation fails.
func (r *activities) GetRayCluster(ctx context.Context, request v2pb.GetRayClusterRequest) (*v2pb.GetRayClusterResponse, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("activity-start", zap.Any("request", request))
	getRayClusterRequest := &v2pb.GetRayClusterRequest{
		Name:       request.Name,
		Namespace:  request.Namespace,
		GetOptions: &metav1.GetOptions{},
	}
	ctx = runnertoken.WithNamespace(ctx, request.Namespace)
	getRayClusterResponse, err := r.rayClusterService.GetRayCluster(ctx, getRayClusterRequest)
	if err != nil {
		logger.Error("activity-error", zap.Any("error", err.Error()))
		return nil, workflow.NewCustomError(ctx, err.Error())
	}
	return getRayClusterResponse, nil
}

// GetRayJob retrieves details of a Ray job.
//
// This method is executed as part of a Starlark worker activity.
//
// Params:
// - ctx: The context for the operation.
// - request: Request containing the job name and namespace.
//
// Returns:
// - *v2pb.GetRayJobResponse: Response containing the job details.
// - *workflow.CustomError: Error information if the operation fails.
func (r *activities) GetRayJob(ctx context.Context, request v2pb.GetRayJobRequest) (*v2pb.GetRayJobResponse, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("activity-start", zap.Any("request", request))
	getRayJobRequest := &v2pb.GetRayJobRequest{
		Name:       request.Name,
		Namespace:  request.Namespace,
		GetOptions: &metav1.GetOptions{},
	}
	ctx = runnertoken.WithNamespace(ctx, request.Namespace)
	getRayJobResponse, err := r.rayJobService.GetRayJob(ctx, getRayJobRequest)
	if err != nil {
		logger.Error("activity-error", zap.Any("error", err.Error()))
		return nil, workflow.NewCustomError(ctx, err.Error())
	}
	return getRayJobResponse, nil
}

// SensorRayClusterReadinessResponse DTO
type SensorRayClusterReadinessResponse struct {
	RayCluster *v2pb.RayCluster `json:"ray_job,omitempty"`
	// JobURL is the RayJob's URL.
	JobURL string `json:"job_url,omitempty"`
	// Ready indicates whether the RayJob is ready to accept a job request.
	// Ready can be false if the sensor activity request contained an early return flag, such as SensorRayClusterRequest.ReturnJobURL.
	Ready bool `json:"ready,omitempty"`
}

// SensorRayJobReadinessResponse DTO
type SensorRayJobReadinessResponse struct {
	RayJob *v2pb.RayJob `json:"ray_job,omitempty"`
	// JobURL is the RayJob's URL.
	JobURL string `json:"job_url,omitempty"`
	// Ready indicates whether the RayJob is ready to accept a job request.
	// Ready can be false if the sensor activity request contained an early return flag, such as SensorRayJobRequest.ReturnJobURL.
	Ready bool `json:"ready,omitempty"`
}

// SensorRayClusterReadiness monitors the readiness of a Ray cluster.
//
// Params:
// - ctx: The context for the operation.
// - request: Request containing the cluster name and namespace.
//
// Returns:
// - *SensorRayClusterReadinessResponse: Response indicating the readiness of the cluster.
// - error: Error information if the operation fails.
func (r *activities) SensorRayClusterReadiness(ctx context.Context, request v2pb.GetRayClusterRequest) (*SensorRayClusterReadinessResponse, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("activity-start", zap.Any("request", request))
	ctx = runnertoken.WithNamespace(ctx, request.Namespace)
	getRayClusterResponse, err := r.rayClusterService.GetRayCluster(ctx, &request)
	if err != nil {
		logger.Error("activity-error", zap.Any("error", err.Error()))
		return nil, workflow.NewCustomError(ctx, err.Error())
	}
	if getRayClusterResponse.RayCluster == nil {
		logger.Error("activity-error", zap.Any("error", err))
		return nil, workflow.NewCustomError(ctx, "Failed to retrieve ray cluster from ray.io")
	}
	rayCluster := getRayClusterResponse.RayCluster
	status := rayCluster.Status

	if hasClusterTerminalCondition(status.State) {
		// Return non-retry-able error. RayCluster is in the terminal state - it'll never be ready to accept a job request.
		return nil, workflow.NewCustomError(ctx, yarpcerrors.CodeInternal.String(), status)
	}

	if status.State == v2pb.RAY_CLUSTER_STATE_READY {
		return &SensorRayClusterReadinessResponse{
			RayCluster: rayCluster,
			JobURL:     rayCluster.Status.JobUrl,
			Ready:      true,
		}, nil
	}

	// Return retry-able error.
	return nil, workflow.NewCustomError(ctx, yarpcerrors.CodeFailedPrecondition.String(), status)
}

// SensorRayClusterProvisioningResponse DTO
type SensorRayClusterProvisioningResponse struct {
	RayCluster *v2pb.RayCluster `json:"ray_cluster,omitempty"`
	// JobURL is the RayCluster's URL.
	JobURL string `json:"job_url,omitempty"`
	// Ready is true when the RayCluster is READY and false when it is SUSPENDED.
	Ready bool `json:"ready,omitempty"`
}

// SensorRayClusterProvisioning monitors a Ray cluster until it is READY or SUSPENDED.
//
// It is the budgeted half of create_cluster's provisioning wait: the workflow retries it within the remaining
// provisioning budget and hands a SUSPENDED cluster to SensorRayClusterAdmission, which spends no budget.
//
// Params:
// - ctx: The context for the operation.
// - request: Request containing the cluster name and namespace.
//
// Returns:
// - *SensorRayClusterProvisioningResponse: The cluster, when it is READY or SUSPENDED.
// - error: Non-retryable when the cluster is FAILED or TERMINATED, retryable in any other state.
func (r *activities) SensorRayClusterProvisioning(ctx context.Context, request v2pb.GetRayClusterRequest) (*SensorRayClusterProvisioningResponse, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("activity-start", zap.Any("request", request))
	rayCluster, err := r.senseRayCluster(ctx, request, provisioningOutcome)
	if err != nil {
		return nil, err
	}
	return &SensorRayClusterProvisioningResponse{
		RayCluster: rayCluster,
		JobURL:     rayCluster.Status.JobUrl,
		Ready:      rayCluster.Status.State == v2pb.RAY_CLUSTER_STATE_READY,
	}, nil
}

// SensorRayClusterAdmission monitors a SUSPENDED Ray cluster until Kueue admits it.
//
// Params:
// - ctx: The context for the operation.
// - request: Request containing the cluster name and namespace.
//
// Returns:
// - *v2pb.GetRayClusterResponse: The cluster, once it is in a state other than SUSPENDED, FAILED or TERMINATED.
// - error: Non-retryable when the cluster is FAILED or TERMINATED, retryable while it is SUSPENDED.
func (r *activities) SensorRayClusterAdmission(ctx context.Context, request v2pb.GetRayClusterRequest) (*v2pb.GetRayClusterResponse, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("activity-start", zap.Any("request", request))
	rayCluster, err := r.senseRayCluster(ctx, request, admissionOutcome)
	if err != nil {
		return nil, err
	}
	return &v2pb.GetRayClusterResponse{RayCluster: rayCluster}, nil
}

// sensorOutcome is what a RayCluster sensor activity reports for the observed cluster state.
type sensorOutcome int

const (
	// sensorPending: the cluster may still reach the awaited state. The activity fails retryably, so its retry
	// policy polls again.
	sensorPending sensorOutcome = iota
	// sensorDone: the cluster is in the awaited state. The activity succeeds.
	sensorDone
	// sensorTerminal: the cluster is FAILED or TERMINATED and will never reach the awaited state. The activity
	// fails non-retryably.
	sensorTerminal
)

// provisioningOutcome maps a state for SensorRayClusterProvisioning: READY ends provisioning and SUSPENDED
// ends its budgeted part; every other live state is still provisioning.
func provisioningOutcome(state v2pb.RayClusterState) sensorOutcome {
	switch {
	case hasClusterTerminalCondition(state):
		return sensorTerminal
	case state == v2pb.RAY_CLUSTER_STATE_READY, state == v2pb.RAY_CLUSTER_STATE_SUSPENDED:
		return sensorDone
	default:
		return sensorPending
	}
}

// admissionOutcome maps a state for SensorRayClusterAdmission: any live state other than SUSPENDED ends the
// wait for admission.
func admissionOutcome(state v2pb.RayClusterState) sensorOutcome {
	switch {
	case hasClusterTerminalCondition(state):
		return sensorTerminal
	case state == v2pb.RAY_CLUSTER_STATE_SUSPENDED:
		return sensorPending
	default:
		return sensorDone
	}
}

// senseRayCluster reads a Ray cluster and maps its state to the result of a sensor activity: the cluster when
// the outcome is sensorDone, a retryable error when it is sensorPending or the read fails, and a non-retryable
// error when it is sensorTerminal.
func (r *activities) senseRayCluster(ctx context.Context, request v2pb.GetRayClusterRequest, outcome func(v2pb.RayClusterState) sensorOutcome) (*v2pb.RayCluster, error) {
	logger := activity.GetLogger(ctx)
	ctx = runnertoken.WithNamespace(ctx, request.Namespace)
	getRayClusterResponse, err := r.rayClusterService.GetRayCluster(ctx, &request)
	if err != nil {
		logger.Error("activity-error", zap.Any("error", err.Error()))
		return nil, workflow.NewCustomError(ctx, err.Error())
	}
	if getRayClusterResponse.RayCluster == nil {
		logger.Error("activity-error", zap.Any("error", "ray cluster not found in response"))
		return nil, workflow.NewCustomError(ctx, "Failed to retrieve ray cluster from ray.io")
	}
	rayCluster := getRayClusterResponse.RayCluster
	status := rayCluster.Status

	switch outcome(status.State) {
	case sensorTerminal:
		return nil, nonRetryableError(ctx, yarpcerrors.CodeInternal.String(), status)
	case sensorPending:
		return nil, workflow.NewCustomError(ctx, yarpcerrors.CodeFailedPrecondition.String(), status)
	default:
		return rayCluster, nil
	}
}

// nonRetryableError returns an activity error that the activity's retry policy does not retry, on either
// workflow engine. On Cadence it is a CustomError whose reason is one of the policy's non-retriable reasons. On
// Temporal a starlark-worker CustomError reaches the server as a retryable failure of type
// "TemporalCustomError", whatever its reason, so the error is a non-retryable ApplicationError instead, with the
// reason as its type and message.
func nonRetryableError(ctx context.Context, reason string, details ...interface{}) error {
	if tmpactivity.IsActivity(ctx) {
		return temporal.NewNonRetryableApplicationError(reason, reason, nil, details...)
	}
	return workflow.NewCustomError(ctx, reason, details...)
}

// TerminateCluster terminates a Ray cluster with the provided parameters.
//
// Params:
// - ctx: The context for the operation.
// - request: Request containing the cluster name, namespace, termination type, and reason.
//
// Returns:
// - *v2pb.UpdateRayClusterResponse: Response containing the updated cluster details.
// - *workflow.CustomError: Error information if the operation fails.
func (r *activities) TerminateCluster(ctx context.Context, request TerminateClusterRequest) (*v2pb.UpdateRayClusterResponse, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("activity-start", zap.Any("request", request))

	var cluster *v2pb.RayCluster
	ctx = runnertoken.WithNamespace(ctx, request.Namespace)
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		getRayClusterRequest := &v2pb.GetRayClusterRequest{
			Name:       request.Name,
			Namespace:  request.Namespace,
			GetOptions: &metav1.GetOptions{},
		}
		getRayClusterResponse, err := r.rayClusterService.GetRayCluster(ctx, getRayClusterRequest)
		if err != nil {
			logger.Error("activity-error", zap.Any("error", err.Error()))
			return err
		}

		cluster = getRayClusterResponse.RayCluster
		var terminateType v2pb.TerminationType
		if request.Type == v2pb.TERMINATION_TYPE_SUCCEEDED.String() {
			terminateType = v2pb.TERMINATION_TYPE_SUCCEEDED
		} else if request.Type == v2pb.TERMINATION_TYPE_FAILED.String() {
			terminateType = v2pb.TERMINATION_TYPE_FAILED
		}
		cluster.Spec.Termination = &v2pb.TerminationSpec{
			Type:   terminateType,
			Reason: request.Reason,
		}
		updateRayClusterRequest := &v2pb.UpdateRayClusterRequest{
			RayCluster:    cluster,
			UpdateOptions: &metav1.UpdateOptions{},
		}
		updateRayClusterResponse, err := r.rayClusterService.UpdateRayCluster(ctx, updateRayClusterRequest)
		if err != nil {
			logger.Error("activity-error", zap.Any("error", err.Error()))
			return err
		}
		if updateRayClusterResponse.RayCluster == nil {
			return errors.New("failed to update ray cluster")
		}
		return nil
	})

	if err != nil {
		logger.Error("activity-error", zap.Any("error", err.Error()))
		return nil, workflow.NewCustomError(ctx, err.Error())
	}

	return &v2pb.UpdateRayClusterResponse{
		RayCluster: cluster,
	}, nil
}

// SensorRayClusterResponse is the response object for SensorRayJob activity
type SensorRayClusterResponse struct {
	RayCluster *v2pb.RayCluster `json:"ray_job,omitempty"`
	// JobURL is the URL of the Ray cluster as reported by the RayJob status.
	JobURL string `json:"job_url,omitempty"`
	// Terminal indicates whether the job has reached a terminal state. This might be false if SensorRayJobRequest has early return flags, such as ReturnJobURL, set to true.
	Terminal bool `json:"terminal,omitempty"`
}

// SensorRayJobResponse is the response object for SensorRayJob activity
type SensorRayJobResponse struct {
	RayJob *v2pb.RayJob `json:"ray_job,omitempty"`
	// JobURL is the URL of the Ray cluster as reported by the RayJob status.
	JobURL string `json:"job_url,omitempty"`
	// Terminal indicates whether the job has reached a terminal state. This might be false if SensorRayJobRequest has early return flags, such as ReturnJobURL, set to true.
	Terminal bool `json:"terminal,omitempty"`
}

// SensorRayJob is a sensor-like activity that is used to monitor the status of a RayJob.
func (r *activities) SensorRayJob(ctx context.Context, request v2pb.GetRayJobRequest) (*SensorRayJobResponse, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("activity-start", zap.Any("request", request))

	ctx = runnertoken.WithNamespace(ctx, request.Namespace)
	getRayJobResponse, err := r.rayJobService.GetRayJob(ctx, &request)
	if err != nil {
		logger.Error("activity-error", zap.Any("error", err.Error()))
		return nil, workflow.NewCustomError(ctx, err.Error())
	}
	rayJob := getRayJobResponse.RayJob
	status := rayJob.Status

	// Check if the job has reached a terminal state
	terminal := status.State == v2pb.RAY_JOB_STATE_KILLED || status.State == v2pb.RAY_JOB_STATE_FAILED || status.State == v2pb.RAY_JOB_STATE_SUCCEEDED

	if terminal {
		return &SensorRayJobResponse{
			RayJob:   rayJob,
			Terminal: terminal,
		}, nil
	}

	return nil, workflow.NewCustomError(ctx, yarpcerrors.CodeFailedPrecondition.String(), status)
}

func hasClusterTerminalCondition(state v2pb.RayClusterState) bool {
	return state == v2pb.RAY_CLUSTER_STATE_TERMINATED ||
		state == v2pb.RAY_CLUSTER_STATE_FAILED
}

func _activity[REQ proto.Message, RES proto.Message](
	ctx context.Context,
	request REQ,
	delegate func(context.Context, REQ, ...yarpc.CallOption) (RES, error),
) (
	RES,
	error,
) {
	logger := log.FromContext(ctx)
	logger.Info("activity-start", zap.Any("request", request))
	response, err := delegate(ctx, request)
	if err != nil {
		logger.Error(err, "activity-error")
		return *new(RES), workflow.NewCustomError(ctx, err.Error())
	}
	return response, nil
}
