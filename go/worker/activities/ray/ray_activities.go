package ray

import (
	"context"
	"github.com/gogo/protobuf/proto"
	"go.uber.org/cadence"
	"go.uber.org/yarpc"
	"go.uber.org/yarpc/yarpcerrors"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"k8s.io/apimachinery/pkg/types"

	"go.uber.org/zap"

	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

var Activities = (*activities)(nil)

// TerminateClusterRequest request
type TerminateClusterRequest struct {
	Name      string `json:"name,omitempty"`      // name of the ray job
	Namespace string `json:"namespace,omitempty"` // namespace of the ray job
	Type      string `json:"type,omitempty"`      // termination code
	Reason    string `json:"reason,omitempty"`    // termination reason
}

// TerminateSparkJobRequest request
type TerminateSparkJobRequest struct {
	Name      string `json:"name,omitempty"`      // name of the spark job
	Namespace string `json:"namespace,omitempty"` // namespace of the spark job
	Type      string `json:"type,omitempty"`      // termination code
	Reason    string
}

type activities struct {
	//project    v2pb.ProjectServiceYARPCClient
	//rayJob     v2pb.RayJobServiceYARPCClient
	//rayCluster v2pb.RayClusterServiceYARPCClient
	//sparkJob   v2pb.SparkJobServiceYARPCClient

	k8sClient client.Client
}

func (r *activities) GetProject(ctx context.Context, request v2pb.GetProjectRequest) (*v2pb.GetProjectResponse, *cadence.CustomError) {
	logger := log.FromContext(ctx)
	logger.Info("activity-start", zap.Any("request", request))
	//response, err := r.project.GetProject(ctx, &request)
	nn := types.NamespacedName{
		Namespace: request.Namespace,
		Name:      request.Name,
	}
	project := &v2pb.Project{}
	err := r.k8sClient.Get(ctx, nn, project)
	if err != nil {
		logger.Error(err, "activity-error")
		return nil, cadence.NewCustomError(err.Error())
	}
	return &v2pb.GetProjectResponse{
		Project: project,
	}, nil
}

func (r *activities) CreateRayJob(ctx context.Context, request v2pb.CreateRayJobRequest) (*v2pb.CreateRayJobResponse, *cadence.CustomError) {
	logger := log.FromContext(ctx)
	logger.Info("activity-start", zap.Any("request", request))
	err := r.k8sClient.Create(ctx, request.RayJob)
	if err != nil {
		logger.Error(err, "activity-error")
		return nil, cadence.NewCustomError(yarpcerrors.CodeUnavailable.String(), err.Error())
	}
	return &v2pb.CreateRayJobResponse{
		RayJob: request.RayJob,
	}, nil
}

func (r *activities) CreateRayCluster(ctx context.Context, request v2pb.RayCluster) (*v2pb.CreateRayClusterResponse, *cadence.CustomError) {
	logger := log.FromContext(ctx)
	logger.Info("activity-start", zap.Any("request", request))
	err := r.k8sClient.Create(ctx, &request)
	if err != nil || request.ObjectMeta.Name == "" {
		return nil, cadence.NewCustomError(yarpcerrors.CodeUnavailable.String(), err.Error())
	}
	return &v2pb.CreateRayClusterResponse{
		RayCluster: &request,
	}, nil
}

func (r *activities) GetRayCluster(ctx context.Context, request v2pb.GetRayClusterRequest) (*v2pb.GetRayClusterResponse, *cadence.CustomError) {
	logger := log.FromContext(ctx)
	logger.Info("activity-start", zap.Any("request", request))
	nn := types.NamespacedName{
		Namespace: request.Namespace,
		Name:      request.Name,
	}
	rayCluster := &v2pb.RayCluster{}
	err := r.k8sClient.Get(ctx, nn, rayCluster)
	if err != nil {
		logger.Error(err, "activity-error")
		return nil, cadence.NewCustomError(err.Error())
	}
	return &v2pb.GetRayClusterResponse{
		RayCluster: rayCluster,
	}, nil
}

func (r *activities) GetRayJob(ctx context.Context, request v2pb.GetRayJobRequest) (*v2pb.GetRayJobResponse, *cadence.CustomError) {
	logger := log.FromContext(ctx)
	logger.Info("activity-start", zap.Any("request", request))
	nn := types.NamespacedName{
		Namespace: request.Namespace,
		Name:      request.Name,
	}
	rayJob := &v2pb.RayJob{}
	err := r.k8sClient.Get(ctx, nn, rayJob)
	if err != nil {
		logger.Error(err, "activity-error")
		return nil, cadence.NewCustomError(err.Error())
	}
	return &v2pb.GetRayJobResponse{
		RayJob: rayJob,
	}, nil
}

// SensorRayClusterReadinessRequest DTO
type SensorRayClusterReadinessRequest struct {
	// Request is the request object containing the namespace and name of the ray job to run a sensor on.
	Request v2pb.GetRayClusterRequest `json:"request,omitempty"`
	// ReturnJobURL is an early-return flag. It indicates whether the sensor activity should return as soon as the
	// RayJob's URL becomes available, even if the RayJob itself isn't ready to accept a job request.
	ReturnJobURL bool `json:"return_job_url,omitempty"`
}

// SensorRayJobReadinessRequest DTO
type SensorRayJobReadinessRequest struct {
	// Request is the request object containing the namespace and name of the ray job to run a sensor on.
	Request v2pb.GetRayJobRequest `json:"request,omitempty"`
	// ReturnJobURL is an early-return flag. It indicates whether the sensor activity should return as soon as the
	// RayJob's URL becomes available, even if the RayJob itself isn't ready to accept a job request.
	ReturnJobURL bool `json:"return_job_url,omitempty"`
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

// SensorRayClusterReadiness is a sensor activity used to monitor the RayJob readiness to accept a job submission request.
func (r *activities) SensorRayClusterReadiness(ctx context.Context, request SensorRayClusterReadinessRequest) (*SensorRayClusterReadinessResponse, error) {
	logger := log.FromContext(ctx)
	logger.Info("activity-start", zap.Any("request", request))
	nn := types.NamespacedName{
		Namespace: request.Request.Namespace,
		Name:      request.Request.Name,
	}
	rayCluster := &v2pb.RayCluster{}
	err := r.k8sClient.Get(ctx, nn, rayCluster)
	if err != nil {
		logger.Error(err, "activity-error")
		return nil, cadence.NewCustomError(err.Error())
	}
	status := rayCluster.Status

	if hasClusterTerminalCondition(status.State) {
		// Return non-retry-able error. RayCluster is in the terminal state - it'll never be ready to accept a job request.
		return nil, cadence.NewCustomError(yarpcerrors.CodeInternal.String(), status)
	}

	if status.State == v2pb.RAY_CLUSTER_STATE_READY {
		println("==========Cluster is ready!!!!!!!==============")
		return &SensorRayClusterReadinessResponse{
			RayCluster: rayCluster,
			Ready:      true,
		}, nil
	}

	println("==========Cluster is not ready==============")

	// Return retry-able error.
	return nil, cadence.NewCustomError(yarpcerrors.CodeFailedPrecondition.String(), status)
}

// SensorRayJobReadiness is a sensor activity used to monitor the RayJob readiness to accept a job submission request.
func (r *activities) SensorRayJobReadiness(ctx context.Context, request SensorRayJobReadinessRequest) (*SensorRayJobReadinessResponse, error) {
	logger := log.FromContext(ctx)
	logger.Info("activity-start", zap.Any("request", request))

	nn := types.NamespacedName{
		Namespace: request.Request.Namespace,
		Name:      request.Request.Name,
	}
	rayJob := &v2pb.RayJob{}
	err := r.k8sClient.Get(ctx, nn, rayJob)
	if err != nil {
		logger.Error(err, "activity-error")
		return nil, cadence.NewCustomError(err.Error())
	}

	status := rayJob.Status

	if hasJobTerminalCondition(status.State) {
		// Return non-retry-able error. RayJob is in the terminal state - it'll never be ready to accept a job request.
		return nil, cadence.NewCustomError(yarpcerrors.CodeInternal.String(), status)
	}

	ready := status.State == v2pb.RAY_JOB_STATE_SUCCEEDED

	if ready {
		return &SensorRayJobReadinessResponse{
			RayJob: rayJob,
			Ready:  ready,
		}, nil
	}

	// Return retry-able error.
	return nil, cadence.NewCustomError(yarpcerrors.CodeFailedPrecondition.String(), status)
}

func (r *activities) TerminateCluster(ctx context.Context, request TerminateClusterRequest) (*v2pb.UpdateRayClusterResponse, *cadence.CustomError) {
	logger := log.FromContext(ctx)
	logger.Info("activity-start", zap.Any("request", request))

	nn := types.NamespacedName{
		Name:      request.Name,
		Namespace: request.Namespace,
	}
	var cluster *v2pb.RayCluster
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		rayCluster := &v2pb.RayCluster{}
		err := r.k8sClient.Get(ctx, nn, rayCluster)
		if err != nil {
			logger.Error(err, "activity-error")
			return err
		}

		cluster = rayCluster
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

		err = r.k8sClient.Update(ctx, cluster)
		if err != nil {
			logger.Error(err, "activity-error")
			return err
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

// SensorRayClusterRequest is the request object for SensorRayJob activity
type SensorRayClusterRequest struct {
	// Request is the request object containing the namespace and name of the ray job to run a sensor on.
	Request v2pb.GetRayClusterRequest `json:"request,omitempty"`
	// ReturnJobURL indicates whether sensor should return early, as soon as the job's URL becomes available, even if the job has not reached a terminal state.
	// If this is set to true, the sensor might return the SensorRayJobResponse with Terminal field set to false.
	ReturnJobURL bool `json:"return_job_url,omitempty"`
}

// SensorRayJobRequest is the request object for SensorRayJob activity
type SensorRayJobRequest struct {
	// Request is the request object containing the namespace and name of the ray job to run a sensor on.
	Request v2pb.GetRayJobRequest `json:"request,omitempty"`
	// ReturnJobURL indicates whether sensor should return early, as soon as the job's URL becomes available, even if the job has not reached a terminal state.
	// If this is set to true, the sensor might return the SensorRayJobResponse with Terminal field set to false.
	ReturnJobURL bool `json:"return_job_url,omitempty"`
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

// SensorRayCluster is a sensor-like activity that is used to monitor the status of a RayJob.
func (r *activities) SensorRayCluster(ctx context.Context, request SensorRayClusterRequest) (*SensorRayClusterResponse, error) {
	logger := log.FromContext(ctx)
	logger.Info("activity-start", zap.Any("request", request))
	nn := types.NamespacedName{
		Name:      request.Request.Name,
		Namespace: request.Request.Namespace,
	}
	rayCluster := &v2pb.RayCluster{}
	err := r.k8sClient.Get(ctx, nn, rayCluster)
	if err != nil {
		logger.Error(err, "activity-error")
		return nil, cadence.NewCustomError(err.Error())
	}

	status := rayCluster.Status

	// Check if the job has reached a terminal state
	terminal := (status.State == v2pb.RAY_CLUSTER_STATE_TERMINATED || status.State == v2pb.RAY_CLUSTER_STATE_FAILED || status.State == v2pb.RAY_CLUSTER_STATE_UNKNOWN)

	if terminal || request.ReturnJobURL {
		return &SensorRayClusterResponse{
			RayCluster: rayCluster,
			Terminal:   terminal,
		}, nil
	}
	return nil, cadence.NewCustomError(yarpcerrors.CodeFailedPrecondition.String(), status)
}

// SensorRayJob is a sensor-like activity that is used to monitor the status of a RayJob.
func (r *activities) SensorRayJob(ctx context.Context, request SensorRayJobRequest) (*SensorRayJobResponse, error) {
	logger := log.FromContext(ctx)
	logger.Info("activity-start", zap.Any("request", request))
	nn := types.NamespacedName{
		Namespace: request.Request.Namespace,
		Name:      request.Request.Name,
	}
	rayJob := &v2pb.RayJob{}
	err := r.k8sClient.Get(ctx, nn, rayJob)
	if err != nil {
		logger.Error(err, "activity-error")
		return nil, cadence.NewCustomError(err.Error())
	}

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

func hasJobTerminalCondition(state v2pb.RayJobState) bool {
	return state == v2pb.RAY_JOB_STATE_SUCCEEDED || state == v2pb.RAY_JOB_STATE_KILLED || state == v2pb.RAY_JOB_STATE_FAILED
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
