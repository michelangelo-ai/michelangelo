package spark

import (
	"context"

	"github.com/cadence-workflow/starlark-worker/activity"
	"github.com/cadence-workflow/starlark-worker/ext"
	"github.com/gogo/protobuf/proto"

	"github.com/cadence-workflow/starlark-worker/workflow"
	"go.uber.org/yarpc"
	"go.uber.org/yarpc/yarpcerrors"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/michelangelo-ai/michelangelo/go/api/utils"
	pluginutils "github.com/michelangelo-ai/michelangelo/go/worker/plugins/utils"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
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
	Name      string               `json:"name,omitempty"`      // name of the spark job
	Namespace string               `json:"namespace,omitempty"` // namespace of the spark job
	Type      v2pb.TerminationType `json:"type,omitempty"`      // termination code
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
	*v2pb.CreateSparkJobResponse, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("activity-start", zap.Any("request", request))
	createSparkJobResponse, err := r.sparkJobService.CreateSparkJob(ctx, &request)
	if err != nil || createSparkJobResponse == nil || createSparkJobResponse.SparkJob == nil ||
		createSparkJobResponse.SparkJob.Name == "" {
		logger.Error("activity-error", zap.Any("error", err.Error()))
		return nil, workflow.NewCustomError(ctx, yarpcerrors.CodeUnavailable.String(), err.Error())
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
func (r *activities) GetSparkJob(ctx context.Context, request v2pb.GetSparkJobRequest) (*v2pb.GetSparkJobResponse, error) {
	logger := log.FromContext(ctx)
	logger.Info("activity-start", zap.Any("request", request))
	getSparkJobResponse, err := r.sparkJobService.GetSparkJob(ctx, &request)
	if err != nil {
		logger.Error(err, "activity-error")
		return nil, workflow.NewCustomError(ctx, err.Error())
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
func (r *activities) SensorSparkJob(ctx context.Context, request v2pb.GetSparkJobRequest) (*SensorSparkJobResponse, error) {
	logger := log.FromContext(ctx)
	logger.Info("activity-start", zap.Any("request", request))

	getSparkJobResponse, err := r.sparkJobService.GetSparkJob(ctx, &request)
	if err != nil {
		logger.Error(err, "activity-error")
		return nil, workflow.NewCustomError(ctx, err.Error())
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
	return nil, workflow.NewCustomError(ctx, yarpcerrors.CodeFailedPrecondition.String(), status)
}

func hasSparkJobTerminalCondition(state v2pb.SparkJobStatus) bool {
	// Check if the job has a terminal condition (Succeeded or Killed)
	succeeded := GetCondition(pluginutils.SucceededCondition, state.GetStatusConditions())
	if succeeded != nil && succeeded.Status != apipb.CONDITION_STATUS_UNKNOWN {
		return true
	}

	killed := GetCondition(pluginutils.KilledCondition, state.GetStatusConditions())
	if killed != nil && killed.Status == apipb.CONDITION_STATUS_TRUE {
		return true
	}

	return false
}

// TerminateSparkJob kills a spark job
func (r *activities) TerminateSparkJob(ctx context.Context, request TerminateSparkJobRequest) (*v2pb.UpdateSparkJobResponse, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("activity-start", zap.String("namespace", request.Namespace), zap.String("name", request.Name))

	getRequest := v2pb.GetSparkJobRequest{
		Namespace: request.Namespace,
		Name:      request.Name,
	}
	response, err := r.sparkJobService.GetSparkJob(ctx, &getRequest)
	if err != nil {
		logger.Error("activity-error", ext.ZapError(err)...)
		if utils.IsNotFoundError(err) {
			// If it is not find, no need to kill it
			logger.Error("Spark Job Not Found", zap.String("error", err.Error()))
			return nil, nil
		}
		return nil, workflow.NewCustomError(ctx, err.Error())
	}
	sparkJob := response.SparkJob
	succeeded := GetCondition(pluginutils.SucceededCondition, sparkJob.Status.GetStatusConditions())
	if succeeded != nil && succeeded.Status != apipb.CONDITION_STATUS_UNKNOWN {
		// If the job is already succeeded, no need to kill it
		logger.Info("Skip Killing. Spark Job Already Terminated.", zap.String("namespace", request.Namespace), zap.String("name", request.Name))
		return &v2pb.UpdateSparkJobResponse{SparkJob: sparkJob}, nil
	}
	sparkJob.Spec.Termination = &v2pb.TerminationSpec{
		Type:   request.Type,
		Reason: request.Reason,
	}
	updateResp, err := r.sparkJobService.UpdateSparkJob(ctx, &v2pb.UpdateSparkJobRequest{SparkJob: sparkJob})
	if err != nil {
		logger.Error("activity-error", ext.ZapError(err)...)
		return nil, workflow.NewCustomError(ctx, err.Error())
	}
	return updateResp, nil
}

// GetCondition provides a utility method for retrieving a particular condition from a condition list.
// If there is no such condition that exists, nil is returned.
func GetCondition(t string, conditions []*apipb.Condition) *apipb.Condition {
	if conditions == nil {
		return nil
	}
	for _, condition := range conditions {
		if condition != nil && condition.Type == t {
			return condition
		}
	}
	return nil
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
