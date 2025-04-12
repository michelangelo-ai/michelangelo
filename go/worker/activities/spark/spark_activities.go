package spark

import (
	"context"
	"github.com/gogo/protobuf/proto"
	"go.uber.org/cadence"
	"go.uber.org/yarpc"
	"go.uber.org/yarpc/yarpcerrors"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"

	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

var Activities = (*activities)(nil)

// TerminateClusterRequest defines the request parameters for terminating a Spark cluster.
type TerminateClusterRequest struct {
	Name      string `json:"name,omitempty"`      // name of the spark job
	Namespace string `json:"namespace,omitempty"` // namespace of the spark job
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

// activities struct encapsulates the YARPC clients for Spark cluster and job services.
type activities struct {
	sparkJobService v2pb.SparkJobServiceYARPCClient
}

// CreateSparkJob creates a new Spark job using the provided request parameters.
//
// This method is executed as part of a Starlark worker activity.
//
// Params:
// - ctx: The context for the operation.
// - request: The request containing details of the Spark job to create.
//
// Returns:
// - *v2pb.CreateSparkJobResponse: Response containing the created Spark job details.
// - *cadence.CustomError: Error information if the operation fails.
func (r *activities) CreateSparkJob(ctx context.Context, request v2pb.CreateSparkJobRequest) (
	*v2pb.CreateSparkJobResponse, *cadence.CustomError) {
	logger := log.FromContext(ctx)
	logger.Info("activity-start", zap.Any("request", request))
	createSparkJobResponse, err := r.sparkJobService.CreateSparkJob(ctx, &request)
	if err != nil || createSparkJobResponse == nil || createSparkJobResponse.SparkJob == nil ||
		createSparkJobResponse.SparkJob.Name == "" {
		logger.Error(err, "activity-error")
		return nil, cadence.NewCustomError(yarpcerrors.CodeUnavailable.String(), err.Error())
	}
	return &v2pb.CreateSparkJobResponse{
		SparkJob: createSparkJobResponse.SparkJob,
	}, nil
}

// GetSparkJob retrieves details of a Spark job.
//
// This method is executed as part of a Starlark worker activity.
//
// Params:
// - ctx: The context for the operation.
// - request: Request containing the job name and namespace.
//
// Returns:
// - *v2pb.GetSparkJobResponse: Response containing the job details.
// - *cadence.CustomError: Error information if the operation fails.
func (r *activities) GetSparkJob(ctx context.Context, request v2pb.GetSparkJobRequest) (*v2pb.GetSparkJobResponse, *cadence.CustomError) {
	logger := log.FromContext(ctx)
	logger.Info("activity-start", zap.Any("request", request))
	getSparkJobRequest := &v2pb.GetSparkJobRequest{
		Name:       request.Name,
		Namespace:  request.Namespace,
		GetOptions: &metav1.GetOptions{},
	}
	getSparkJobResponse, err := r.sparkJobService.GetSparkJob(ctx, getSparkJobRequest)
	if err != nil {
		logger.Error(err, "activity-error")
		return nil, cadence.NewCustomError(err.Error())
	}
	return getSparkJobResponse, nil
}

// SensorSparkJobReadinessResponse DTO
type SensorSparkJobReadinessResponse struct {
	SparkJob *v2pb.SparkJob `json:"spark_job,omitempty"`
	// JobURL is the SparkJob's URL.
	JobURL string `json:"job_url,omitempty"`
	// Ready indicates whether the SparkJob is ready to accept a job request.
	// Ready can be false if the sensor activity request contained an early return flag, such as SensorSparkJobRequest.ReturnJobURL.
	Ready bool `json:"ready,omitempty"`
}

// SensorSparkJobResponse is the response object for SensorSparkJob activity
type SensorSparkJobResponse struct {
	SparkJob *v2pb.SparkJob `json:"spark_job,omitempty"`
	// JobURL is the URL of the Spark cluster as reported by the SparkJob status.
	JobURL string `json:"job_url,omitempty"`
	// Terminal indicates whether the job has reached a terminal state. This might be false if SensorSparkJobRequest has early return flags, such as ReturnJobURL, set to true.
	Terminal bool `json:"terminal,omitempty"`
}

// SensorSparkJob is a sensor-like activity that is used to monitor the status of a SparkJob.
func (r *activities) SensorSparkJob(ctx context.Context, request v2pb.GetSparkJobRequest) (*SensorSparkJobResponse, *cadence.CustomError) {
	logger := log.FromContext(ctx)
	logger.Info("activity-start", zap.Any("request", request))

	getSparkJobResponse, err := r.sparkJobService.GetSparkJob(ctx, &request)
	if err != nil {
		logger.Error(err, "activity-error")
		return nil, cadence.NewCustomError(err.Error())
	}
	sparkJob := getSparkJobResponse.SparkJob
	status := sparkJob.Status

	// Check if the job has reached a terminal state
	terminal := hasSparkJobTerminalCondition(status)

	if terminal {
		return &SensorSparkJobResponse{
			SparkJob: sparkJob,
			Terminal: terminal,
		}, nil
	}

	return nil, cadence.NewCustomError(yarpcerrors.CodeFailedPrecondition.String(), status)
}

func hasSparkJobTerminalCondition(state v2pb.SparkJobStatus) bool {
	return state.ApplicationId == "FAILED" || state.ApplicationId == "COMPLETED"
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
