package ray

import (
	"context"
	"errors"

	"github.com/gogo/protobuf/proto"
	"go.uber.org/cadence"
	"go.uber.org/yarpc"
	"go.uber.org/yarpc/yarpcerrors"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/log"

	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
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

// CreateRayJob creates a new Ray job using the provided request parameters.
//
// This method is executed as part of a Starlark worker activity.
//
// Params:
// - ctx: The context for the operation.
// - request: The request containing details of the Ray job to create.
//
// Returns:
// - *v2pb.CreateRayJobResponse: Response containing the created Ray job details.
// - *cadence.CustomError: Error information if the operation fails.
func (r *activities) CreateRayJob(ctx context.Context, request v2pb.CreateRayJobRequest) (
	*v2pb.CreateRayJobResponse, *cadence.CustomError) {
	logger := log.FromContext(ctx)
	logger.Info("activity-start", zap.Any("request", request))
	createRayJobResponse, err := r.rayJobService.CreateRayJob(ctx, &request)
	if err != nil || createRayJobResponse == nil || createRayJobResponse.RayJob == nil ||
		createRayJobResponse.RayJob.Name == "" {
		logger.Error(err, "activity-error")
		return nil, cadence.NewCustomError(yarpcerrors.CodeUnavailable.String(), err.Error())
	}
	return &v2pb.CreateRayJobResponse{
		RayJob: createRayJobResponse.RayJob,
	}, nil
}

// CreateRayCluster creates a new Ray cluster.
//
// This method is executed as part of a Starlark worker activity.
//
// Params:
// - ctx: The context for the operation.
// - request: The details of the Ray cluster to create.
//
// Returns:
// - *v2pb.CreateRayClusterResponse: Response containing the created cluster details.
// - *cadence.CustomError: Error information if the operation fails.
func (r *activities) CreateRayCluster(ctx context.Context, request v2pb.CreateRayClusterRequest) (
	*v2pb.CreateRayClusterResponse, *cadence.CustomError) {
	//logger := log.FromContext(ctx)
	//logger.Info("activity-start", zap.Any("request", request))
	createRayClusterResponse, err := r.rayClusterService.CreateRayCluster(ctx, &request)
	if err != nil || createRayClusterResponse == nil || createRayClusterResponse.RayCluster == nil ||
		createRayClusterResponse.RayCluster.Name == "" {
		return nil, cadence.NewCustomError(yarpcerrors.CodeUnavailable.String(), err.Error())
	}
	return createRayClusterResponse, nil
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
// - *cadence.CustomError: Error information if the operation fails.
func (r *activities) GetRayCluster(ctx context.Context, request v2pb.GetRayClusterRequest) (*v2pb.GetRayClusterResponse, *cadence.CustomError) {
	logger := log.FromContext(ctx)
	logger.Info("activity-start", zap.Any("request", request))
	getRayClusterRequest := &v2pb.GetRayClusterRequest{
		Name:       request.Name,
		Namespace:  request.Namespace,
		GetOptions: &metav1.GetOptions{},
	}
	getRayClusterResponse, err := r.rayClusterService.GetRayCluster(ctx, getRayClusterRequest)
	if err != nil {
		logger.Error(err, "activity-error")
		return nil, cadence.NewCustomError(err.Error())
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
// - *cadence.CustomError: Error information if the operation fails.
func (r *activities) GetRayJob(ctx context.Context, request v2pb.GetRayJobRequest) (*v2pb.GetRayJobResponse, *cadence.CustomError) {
	logger := log.FromContext(ctx)
	logger.Info("activity-start", zap.Any("request", request))
	getRayJobRequest := &v2pb.GetRayJobRequest{
		Name:       request.Name,
		Namespace:  request.Namespace,
		GetOptions: &metav1.GetOptions{},
	}
	getRayJobResponse, err := r.rayJobService.GetRayJob(ctx, getRayJobRequest)
	if err != nil {
		logger.Error(err, "activity-error")
		return nil, cadence.NewCustomError(err.Error())
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
func (r *activities) SensorRayClusterReadiness(ctx context.Context, request v2pb.GetRayClusterRequest) (*SensorRayClusterReadinessResponse, *cadence.CustomError) {
	logger := log.FromContext(ctx)
	logger.Info("activity-start", zap.Any("request", request))
	getRayClusterResponse, err := r.rayClusterService.GetRayCluster(ctx, &request)
	if err != nil {
		logger.Error(err, "activity-error")
		return nil, cadence.NewCustomError(err.Error())
	}
	if getRayClusterResponse.RayCluster == nil {
		logger.Error(err, "activity-error")
		return nil, cadence.NewCustomError("Failed to retrieve ray cluster from ray.io")
	}
	rayCluster := getRayClusterResponse.RayCluster
	status := rayCluster.Status

	if hasClusterTerminalCondition(status.State) {
		// Return non-retry-able error. RayCluster is in the terminal state - it'll never be ready to accept a job request.
		return nil, cadence.NewCustomError(yarpcerrors.CodeInternal.String(), status)
	}

	if status.State == v2pb.RAY_CLUSTER_STATE_READY {
		return &SensorRayClusterReadinessResponse{
			RayCluster: rayCluster,
			JobURL:     rayCluster.Status.JobUrl,
			Ready:      true,
		}, nil
	}

	// Return retry-able error.
	return nil, cadence.NewCustomError(yarpcerrors.CodeFailedPrecondition.String(), status)
}

// TerminateCluster terminates a Ray cluster with the provided parameters.
//
// Params:
// - ctx: The context for the operation.
// - request: Request containing the cluster name, namespace, termination type, and reason.
//
// Returns:
// - *v2pb.UpdateRayClusterResponse: Response containing the updated cluster details.
// - *cadence.CustomError: Error information if the operation fails.
func (r *activities) TerminateCluster(ctx context.Context, request TerminateClusterRequest) (*v2pb.UpdateRayClusterResponse, *cadence.CustomError) {
	logger := log.FromContext(ctx)
	logger.Info("activity-start", zap.Any("request", request))

	var cluster *v2pb.RayCluster
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		getRayClusterRequest := &v2pb.GetRayClusterRequest{
			Name:       request.Name,
			Namespace:  request.Namespace,
			GetOptions: &metav1.GetOptions{},
		}
		getRayClusterResponse, err := r.rayClusterService.GetRayCluster(ctx, getRayClusterRequest)
		if err != nil {
			logger.Error(err, "activity-error")
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
			logger.Error(err, "activity-error")
			return err
		}
		if updateRayClusterResponse.RayCluster == nil {
			return errors.New("failed to update ray cluster")
		}
		return nil
	})

	if err != nil {
		logger.Error(err, "activity-error")
		return nil, cadence.NewCustomError(err.Error())
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
func (r *activities) SensorRayJob(ctx context.Context, request v2pb.GetRayJobRequest) (*SensorRayJobResponse, *cadence.CustomError) {
	logger := log.FromContext(ctx)
	logger.Info("activity-start", zap.Any("request", request))

	getRayJobResponse, err := r.rayJobService.GetRayJob(ctx, &request)
	if err != nil {
		logger.Error(err, "activity-error")
		return nil, cadence.NewCustomError(err.Error())
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

	return nil, cadence.NewCustomError(yarpcerrors.CodeFailedPrecondition.String(), status)
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
	*cadence.CustomError,
) {
	logger := log.FromContext(ctx)
	logger.Info("activity-start", zap.Any("request", request))
	response, err := delegate(ctx, request)
	if err != nil {
		logger.Error(err, "activity-error")
		return *new(RES), cadence.NewCustomError(err.Error())
	}
	return response, nil
}
