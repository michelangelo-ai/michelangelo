package ray

import (
	"context"
	"errors"

	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"

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

// activities struct encapsulates the YARPC clients for Ray cluster and job services.
type activities struct {
	rayClusterService v2pb.RayClusterServiceYARPCClient
	rayJobService     v2pb.RayJobServiceYARPCClient
}

// CreateRayJob creates a new Ray job.
func (r *activities) CreateRayJob(ctx context.Context, request v2pb.CreateRayJobRequest) (*v2pb.CreateRayJobResponse, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("activity-start", zap.Any("request", request))

	createRayJobResponse, err := r.rayJobService.CreateRayJob(ctx, &request)
	if err != nil || createRayJobResponse == nil || createRayJobResponse.RayJob == nil ||
		createRayJobResponse.RayJob.Name == "" {
		logger.Error("activity-error", zap.Error(err))
		return nil, temporal.NewNonRetryableApplicationError(err.Error(), "ray_job_creation_failed", err)
	}
	return createRayJobResponse, nil
}

// CreateRayCluster creates a new Ray cluster.
func (r *activities) CreateRayCluster(ctx context.Context, request v2pb.CreateRayClusterRequest) (*v2pb.CreateRayClusterResponse, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("activity-start", zap.Any("request", request))

	createRayClusterResponse, err := r.rayClusterService.CreateRayCluster(ctx, &request)
	if err != nil || createRayClusterResponse == nil || createRayClusterResponse.RayCluster == nil ||
		createRayClusterResponse.RayCluster.Name == "" {
		logger.Error("activity-error", zap.Error(err))
		return nil, temporal.NewNonRetryableApplicationError(err.Error(), "ray_cluster_creation_failed", err)
	}
	return createRayClusterResponse, nil
}

// GetRayCluster retrieves details of a Ray cluster.
func (r *activities) GetRayCluster(ctx context.Context, request v2pb.GetRayClusterRequest) (*v2pb.GetRayClusterResponse, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("activity-start", zap.Any("request", request))

	getRayClusterResponse, err := r.rayClusterService.GetRayCluster(ctx, &request)
	if err != nil {
		logger.Error("activity-error", zap.Error(err))
		return nil, temporal.NewNonRetryableApplicationError(err.Error(), "get_ray_cluster_failed", err)
	}
	return getRayClusterResponse, nil
}

// GetRayJob retrieves details of a Ray job.
func (r *activities) GetRayJob(ctx context.Context, request v2pb.GetRayJobRequest) (*v2pb.GetRayJobResponse, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("activity-start", zap.Any("request", request))

	getRayJobResponse, err := r.rayJobService.GetRayJob(ctx, &request)
	if err != nil {
		logger.Error("activity-error", zap.Error(err))
		return nil, temporal.NewNonRetryableApplicationError(err.Error(), "get_ray_job_failed", err)
	}
	return getRayJobResponse, nil
}

func (r *activities) SensorRayClusterReadiness(ctx context.Context, request v2pb.GetRayClusterRequest) (*SensorRayClusterReadinessResponse, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("activity-start", zap.Any("request", request))

	getRayClusterResponse, err := r.rayClusterService.GetRayCluster(ctx, &request)
	if err != nil || getRayClusterResponse.RayCluster == nil {
		logger.Error("activity-error", zap.Error(err))
		return nil, temporal.NewNonRetryableApplicationError("Failed to retrieve ray cluster", "ray_cluster_not_found", err)
	}

	rayCluster := getRayClusterResponse.RayCluster
	status := rayCluster.Status

	if hasClusterTerminalCondition(status.State) {
		return nil, temporal.NewNonRetryableApplicationError("Cluster is in terminal state", "ray_cluster_terminal", nil)
	}

	if status.State == v2pb.RAY_CLUSTER_STATE_READY {
		return &SensorRayClusterReadinessResponse{
			RayCluster: rayCluster,
			JobURL:     rayCluster.Status.JobUrl,
			Ready:      true,
		}, nil
	}

	return nil, temporal.NewApplicationError("Cluster is not ready yet", "ray_cluster_not_ready", nil)
}

// TerminateCluster terminates a Ray cluster.
func (r *activities) TerminateCluster(ctx context.Context, request TerminateClusterRequest) (*v2pb.UpdateRayClusterResponse, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("activity-start", zap.Any("request", request))

	var cluster *v2pb.RayCluster
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		getRayClusterResponse, err := r.rayClusterService.GetRayCluster(ctx, &v2pb.GetRayClusterRequest{
			Name:       request.Name,
			Namespace:  request.Namespace,
			GetOptions: &metav1.GetOptions{},
		})
		if err != nil {
			logger.Error("activity-error", zap.Error(err))
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
		updateRayClusterResponse, err := r.rayClusterService.UpdateRayCluster(ctx, &v2pb.UpdateRayClusterRequest{
			RayCluster:    cluster,
			UpdateOptions: &metav1.UpdateOptions{},
		})
		if err != nil {
			logger.Error("activity-error", zap.Error(err))
			return err
		}
		if updateRayClusterResponse.RayCluster == nil {
			return errors.New("failed to update ray cluster")
		}
		return nil
	})

	if err != nil {
		logger.Error("activity-error", zap.Error(err))
		return nil, temporal.NewNonRetryableApplicationError(err.Error(), "ray_cluster_termination_failed", err)
	}

	return &v2pb.UpdateRayClusterResponse{RayCluster: cluster}, nil
}

// SensorRayJob monitors the status of a Ray job.
func (r *activities) SensorRayJob(ctx context.Context, request v2pb.GetRayJobRequest) (*SensorRayJobResponse, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("activity-start", zap.Any("request", request))

	getRayJobResponse, err := r.rayJobService.GetRayJob(ctx, &request)
	if err != nil {
		logger.Error("activity-error", zap.Error(err))
		return nil, temporal.NewNonRetryableApplicationError(err.Error(), "ray_job_sensor_failed", err)
	}
	rayJob := getRayJobResponse.RayJob
	status := rayJob.Status

	terminal := status.State == v2pb.RAY_JOB_STATE_KILLED || status.State == v2pb.RAY_JOB_STATE_FAILED || status.State == v2pb.RAY_JOB_STATE_SUCCEEDED

	if terminal {
		return &SensorRayJobResponse{
			RayJob:   rayJob,
			Terminal: terminal,
		}, nil
	}

	return nil, temporal.NewApplicationError("Ray job is not in a terminal state", "ray_job_not_terminal", nil)
}

func hasClusterTerminalCondition(state v2pb.RayClusterState) bool {
	return state == v2pb.RAY_CLUSTER_STATE_TERMINATED || state == v2pb.RAY_CLUSTER_STATE_FAILED
}
