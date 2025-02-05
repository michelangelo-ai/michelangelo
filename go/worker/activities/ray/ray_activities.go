package ray

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gogo/protobuf/proto"
	"go.uber.org/cadence"
	"go.uber.org/yarpc"
	"go.uber.org/yarpc/yarpcerrors"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"os"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"k8s.io/apimachinery/pkg/types"

	"go.uber.org/zap"

	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

var Activities = (*activities)(nil)

// TerminateRayJobRequest request
type TerminateRayJobRequest struct {
	Name      string               `json:"name,omitempty"`      // name of the ray job
	Namespace string               `json:"namespace,omitempty"` // namespace of the ray job
	Type      v2pb.TerminationType `json:"type,omitempty"`      // termination code
	Reason    string               `json:"reason,omitempty"`    // termination reason
}

// TerminateSparkJobRequest request
type TerminateSparkJobRequest struct {
	Name      string               `json:"name,omitempty"`      // name of the spark job
	Namespace string               `json:"namespace,omitempty"` // namespace of the spark job
	Type      v2pb.TerminationType `json:"type,omitempty"`      // termination code
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
	println("======sensoring=======")
	println(nn.Name)
	rayCluster := &v2pb.RayCluster{}
	err := r.k8sClient.Get(ctx, nn, rayCluster)
	if err != nil {
		println("=============error sensoring==========")
		println(err.Error())
		logger.Error(err, "activity-error")
		return nil, cadence.NewCustomError(err.Error())
	}
	println("=============get status==========")
	status := rayCluster.Status

	if hasClusterTerminalCondition(status.State) {
		println("=============get terminated condition==========")
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

func (r *activities) TerminateRayJob(ctx context.Context, request TerminateRayJobRequest) (*v2pb.UpdateRayClusterResponse, *cadence.CustomError) {
	logger := log.FromContext(ctx)
	logger.Info("activity-start", zap.Any("request", request))

	nn := types.NamespacedName{
		Name:      request.Name,
		Namespace: request.Namespace,
	}
	rayCluster := &v2pb.RayCluster{}
	err := r.k8sClient.Get(ctx, nn, rayCluster)
	if err != nil {
		logger.Error(err, "activity-error")
		return nil, cadence.NewCustomError(err.Error())
	}

	job := rayCluster
	job.Spec.Termination = &v2pb.TerminationSpec{
		Type:   request.Type,
		Reason: request.Reason,
	}

	err = r.k8sClient.Update(ctx, job)
	if err != nil {
		logger.Error(err, "activity-error")
		return nil, cadence.NewCustomError(err.Error())
	}
	return &v2pb.UpdateRayClusterResponse{
		RayCluster: rayCluster,
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

// SensorRayJobSubmissionRequest DTO
type SensorRayJobSubmissionRequest struct {
	SubmissionURL   string `json:"submission_url,omitempty"`
	RayJobNamespace string `json:"ray_job_namespace,omitempty"`
	RayJobName      string `json:"ray_job_name,omitempty"`
}

// SensorRayJobSubmission is a sensor activity used to monitor completeness of the Ray job submission.
func (r *activities) SensorRayJobSubmission(ctx context.Context, request SensorRayJobSubmissionRequest) (map[string]any, error) {
	logger := log.FromContext(ctx)
	logger.Info("activity-start", zap.Any("request", request))
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		kubeconfig = os.ExpandEnv("/Users/weric/.kube/config")
	}
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		logger.Error(err, "activity-error")
		return nil, cadence.NewCustomError("500", err)
	}

	// Create a REST client
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		logger.Error(err, "activity-error")
		return nil, cadence.NewCustomError("500", err)
	} // Define the runtime environment as a JSON string

	// Define the runtime environment JSON
	//runtimeEnv := `{
	//	"working_dir": "/workspace",
	//	"container": {
	//		"image": "your-docker-repo/model-training:latest"
	//	}
	//}`

	// Encode the runtime environment in Base64
	//encodedRuntimeEnv := base64.StdEncoding.EncodeToString([]byte(runtimeEnv))

	// Fetch the status of the RayJob
	statusRequest := clientset.RESTClient().Get().
		AbsPath(fmt.Sprintf("/apis/ray.io/v1alpha1/namespaces/default/rayjobs/%s", request.RayJobName))

	statusResponse := statusRequest.Do(context.TODO())
	if statusResponse.Error() != nil {
		logger.Error(err, "activity-error", zap.Any("err", statusResponse.Error()), zap.Any("statusResponse", statusResponse))
		return nil, cadence.NewCustomError("500", statusResponse.Error())
	}

	// Read the status response
	statusRaw, err := statusResponse.Raw()
	if err != nil {
		logger.Error(err, "activity-error")
		return nil, cadence.NewCustomError("400", err)
	}

	// Parse the status JSON
	var statusMap map[string]interface{}
	err = json.Unmarshal(statusRaw, &statusMap)
	if err != nil {
		logger.Error(err, "activity-error", zap.Any("err", "Parse the status JSON"), zap.Any("statusMap", statusMap))
		return nil, cadence.NewCustomError("400", err)
	}

	// Extract and print the RayJob status
	status, ok := statusMap["status"].(map[string]interface{})
	if !ok {
		logger.Error(err, "activity-error", zap.Any("failed to extract rayjob status", "Extract and print the RayJob status"), zap.Any("statusMap", statusMap))
		return nil, cadence.NewCustomError(yarpcerrors.CodeFailedPrecondition.String(), nil)
	}

	//jobId := status["jobId"]
	jobStatus := status["jobStatus"]
	logger.Info("activity-running", zap.Any("jobStatus", jobStatus))
	//message := status["message"]
	//rayClusterStatus := status["rayClusterStatus"]
	//dashboardURL := status["dashboardURL"]
	//jobDeploymentStatus := status["jobDeploymentStatus"]
	//rayClusterName := status["rayClusterName"]
	if jobStatus != "RUNNING" {
		// Return OK. The job submission has reached a terminal status.
		return map[string]any{
			"status":  jobStatus,
			"message": status["message"],
		}, nil
	}
	// Return retry-able error.
	return nil, cadence.NewCustomError(yarpcerrors.CodeFailedPrecondition.String(), nil)
}

//
//// CreateSparkJob creates a SparkJob CRD
//func (r *activities) CreateSparkJob(ctx context.Context, request v2pb.CreateSparkJobRequest) (*v2pb.CreateSparkJobResponse, *cadence.CustomError) {
//	logger := log.FromContext(ctx)
//	logger.Info("activity-start", zap.Any("request", request))
//	response, err := r.sparkJob.CreateSparkJob(ctx, &request)
//	if err != nil {
//		logger.Error(err, "activity-error")
//		return nil, cadence.NewCustomError(err.Error())
//	}
//	return response, nil
//}
//
//// SensorSparkJob sensors a SparkJob CRD
//// It will return if the status of the sparkJob changes.
//func (r *activities) SensorSparkJob(ctx context.Context, request v2pb.GetSparkJobRequest, originalStatus *v2pb.SparkJobStatus) (*v2pb.GetSparkJobResponse, *cadence.CustomError) {
//	logger := log.FromContext(ctx)
//	logger.Info("activity-start", zap.Any("request", request))
//	response, err := r.sparkJob.GetSparkJob(ctx, &request)
//	if err != nil {
//		logger.Error(err, "activity-error")
//		return nil, cadence.NewCustomError(err.Error())
//	}
//
//	status := &response.SparkJob.Status
//	// If job status change, return to update result
//	if !reflect.DeepEqual(status, originalStatus) {
//		return response, nil
//	}
//	return nil, cadence.NewCustomError(yarpcerrors.CodeFailedPrecondition.String(), status)
//}
//
//// TerminateSparkJob kills a spark job
//func (r *activities) TerminateSparkJob(ctx context.Context, request TerminateSparkJobRequest) (*v2pb.UpdateSparkJobResponse, *cadence.CustomError) {
//	logger := log.FromContext(ctx)
//	logger.Info("activity-start", zap.String("namespace", request.Namespace), zap.String("name", request.Name))
//
//	getRequest := v2pb.GetSparkJobRequest{
//		Namespace: request.Namespace,
//		Name:      request.Name,
//	}
//	response, err := r.sparkJob.GetSparkJob(ctx, &getRequest)
//	if err != nil {
//		logger.Error(err, "activity-error")
//		if utils.IsNotFoundError(err) {
//			// If it is not find, no need to kill it
//			logger.Error(err, "Spark Job Not Found")
//			return nil, nil
//		}
//		return nil, cadence.NewCustomError(err.Error())
//	}
//	sparkJob := response.SparkJob
//	if sparkJob.Status.GetStatusConditions() != nil {
//		// If the job is already succeeded, no need to kill it
//		logger.Info("Skip Killing. Spark Job Already Terminated.", zap.String("namespace", request.Namespace), zap.String("name", request.Name))
//		return &v2pb.UpdateSparkJobResponse{SparkJob: sparkJob}, nil
//	}
//	sparkJob.Spec.Termination = &v2pb.TerminationSpec{
//		Type:   request.Type,
//		Reason: request.Reason,
//	}
//	updateResp, err := r.sparkJob.UpdateSparkJob(ctx, &v2pb.UpdateSparkJobRequest{SparkJob: sparkJob})
//	if err != nil {
//		logger.Error(err, "activity-error")
//		return nil, cadence.NewCustomError(err.Error())
//	}
//	return updateResp, nil
//}

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
