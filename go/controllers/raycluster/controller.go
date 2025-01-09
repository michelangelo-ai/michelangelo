package raycluster

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	e "github.com/michelangelo-ai/michelangelo/go/base/env"
	"reflect"
	"strings"
	"time"

	"github.com/go-logr/logr"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	restclient "k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	_requestTimeout      = 60
	_requeueAfterSeconds = 10
)

// Reconciler reconciles a Ray CRD object
type Reconciler struct {
	client.Client

	env     e.Context

	k8sRestClient      restclient.Interface
}

const _controllerName = "raycluster"
const _apiVersion = "ray.io/v1"


// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the RayCluster object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.7.2/pkg/reconcile
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	ctx, cancel := context.WithTimeout(ctx, _requestTimeout*time.Second)
	logger := log.FromContext(ctx)
	defer cancel()

	logger.Info(fmt.Sprintf("Reconciling ray cluster %s", req.NamespacedName))

	// retrieve the ray cluster
	var rayCluster v2pb.RayCluster
	if err := r.Get(ctx, req.NamespacedName, &rayCluster); err != nil {
		// TODO when the ray cluster is not found, means it has been deleted
		// we also need to delete the evaluation report as well
		if IsNotFoundError(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	originalRayCluster := rayCluster.DeepCopy()

	result, err := r.reconcile(ctx, logger, &rayCluster)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("unable to reconcile %w", err)
	}
	if !reflect.DeepEqual(originalRayCluster, rayCluster) {
		logger.Info("Updating status")
		err = r.Status().Update(ctx, &rayCluster)
		if err != nil {
			logger.Error(err, "failed to update status")
			return result, nil
		}
	} else {
		logger.Info("Nothing changed")
	}

	logger.Info("Reconcile finished")

	return result, nil
}

func (r *Reconciler) Register(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v2pb.RayCluster{}).
		Complete(r)
}

// reconcile launches the cluster if it is not already launched
func (r *Reconciler) reconcile(
	ctx context.Context,
	log logr.Logger,
	rayCluster *v2pb.RayCluster,
) (ctrl.Result, error) {

	// Here we check the state, take an action and then transition to next state.
	// Only terminal state SUCCEED, FAILED and KILLED remove request from queue by returning empty ctrl.Result{}
	// For non-terminal state, we should always requeue because we don't have a watch on the dashboard state.
StateMachine:
	switch rayCluster.Status.State {
	case v2pb.RAY_CLUSTER_STATE_INVALID:
		log.Info("RAY_CLUSTER_STATE_INVALID")
		rayCluster.Status.State = v2pb.RAY_CLUSTER_STATE_LAUNCHED

	case v2pb.RAY_CLUSTER_STATE_LAUNCHED:
		log.Info("RAY_CLUSTER_STATE_CREATING")

		err := r.createCluster(ctx, log, rayCluster)
		if err != nil {
			rayCluster.Status.State = v2pb.RAY_CLUSTER_STATE_FAILED
			return ctrl.Result{}, nil
		}
		rayCluster.Status.State = v2pb.RAY_CLUSTER_STATE_PROVISIONING
	case v2pb.RAY_CLUSTER_STATE_PROVISIONING:
		if rayCluster.Spec.Termination.Type != v2pb.TERMINATION_TYPE_INVALID {
			rayCluster.Status.State = v2pb.RAY_CLUSTER_STATE_TERMINATING
			return ctrl.Result{}, nil
		}
		status, reason, err := r.getClusterStatus(ctx, log, rayCluster)
		if err != nil {
			errMsg := err.Error()
			if strings.Contains(errMsg, "not found") {
				log.Info("Resource not found, marking as terminated.")
				rayCluster.Status.State = v2pb.RAY_CLUSTER_STATE_TERMINATED
				return ctrl.Result{}, nil
			}
			rayCluster.Status.State = v2pb.RAY_CLUSTER_STATE_FAILED
			if reason != nil {
				podError := &v2pb.PodErrors{
					ContainerName: rayCluster.Status.HeadNode.Name,
					ExitCode:      0,
					Reason:        *reason,
				}
				rayCluster.Status.PodErrors = append(rayCluster.Status.PodErrors, podError)
			}
			break StateMachine
		}
		if reason != nil {
			podError := &v2pb.PodErrors{
				ContainerName: rayCluster.Status.HeadNode.Name,
				ExitCode:      0,
				Reason:        *reason,
			}
			rayCluster.Status.PodErrors = append(rayCluster.Status.PodErrors, podError)
		}
		if status != nil && r.isTerminatedState(*status) {
			if *status == "failed" {
				rayCluster.Status.State = v2pb.RAY_CLUSTER_STATE_FAILED
			} else if *status == "terminated" {
				rayCluster.Status.State = v2pb.RAY_CLUSTER_STATE_TERMINATED
			} else if *status == "unknown" {
				rayCluster.Status.State = v2pb.RAY_CLUSTER_STATE_UNKNOWN
			}
			return ctrl.Result{}, nil
		} else if *status == "ready" {
			rayCluster.Status.State = v2pb.RAY_CLUSTER_STATE_READY
		}
	case v2pb.RAY_CLUSTER_STATE_TERMINATING:
		err := r.deleteClusterStatus(ctx, log, rayCluster)
		if err != nil {
			rayCluster.Status.State = v2pb.RAY_CLUSTER_STATE_FAILED
			break StateMachine
		}
		// we check it back to provioning for checking the status
		rayCluster.Status.State = v2pb.RAY_CLUSTER_STATE_PROVISIONING
	case v2pb.RAY_CLUSTER_STATE_READY:
		log.Info("cluster is ready, we do nothing but continue requeue until the job finishes and received termination signal")
		if rayCluster.Spec.Termination.Type != v2pb.TERMINATION_TYPE_INVALID {
			rayCluster.Status.State = v2pb.RAY_CLUSTER_STATE_TERMINATING
			return ctrl.Result{}, nil
		}
	default:
		if rayCluster.Spec.Termination != nil && rayCluster.Spec.Termination.Type != v2pb.TERMINATION_TYPE_INVALID {
			rayCluster.Status.State = v2pb.RAY_CLUSTER_STATE_TERMINATING
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, nil
	}

	return ctrl.Result{RequeueAfter: time.Second * _requeueAfterSeconds}, nil
}

func (r *Reconciler) createCluster(ctx context.Context, log logr.Logger, cluster *v2pb.RayCluster) error {
	// Define the RayCluster resource
	rayCluster := map[string]interface{}{
		"apiVersion": _apiVersion,
		"kind":       "RayCluster",
		"metadata": map[string]interface{}{
			"generateName": fmt.Sprintf("rc-%s-", cluster.Name),
			"namespace":    cluster.Namespace,
		},
		"spec": map[string]interface{}{
			"rayVersion": "2.3.1",
			"headGroupSpec": map[string]interface{}{
				"serviceType":    cluster.Spec.Head.ServiceType,
				"rayStartParams": cluster.Spec.Head.RayStartParams,
				"replicas":       1,
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"containers": []map[string]interface{}{
							convertPodSpecToJSON(cluster.Spec.Head.Pod),
						},
					},
				},
			},
			"workerGroupSpecs":        convertWorkerGroupSpecsToJSON(cluster.Name, cluster.Spec.Workers),
			"enableInTreeAutoscaling": false,
		},
	}

	// Convert the RayCluster to JSON
	rayClusterJSON, err := json.Marshal(rayCluster)
	if err != nil {
		log.Error(err, "Failed to marshal RayCluster")
	}
	log.Info(string(rayClusterJSON))

	// Submit the rayCluster using the REST client
	request := r.k8sRestClient.Post().
		AbsPath(fmt.Sprintf("/apis/%s/namespaces/%s/rayclusters", _apiVersion, cluster.Namespace)).
		Body(bytes.NewReader(rayClusterJSON))

	response := request.Do(ctx)
	if response.Error() != nil {
		log.Error(response.Error(), "Failed to submit RayCluster")
		return response.Error()
	}
	data, err := response.Raw()
	if err != nil {
		log.Error(err, "error getting raw response")
		return err
	}
	// Parse and print the response
	var createdRayCluster map[string]interface{}
	err = json.Unmarshal(data, &createdRayCluster)
	if err != nil {
		log.Error(err, "Error unmarshaling JSON")
		return err
	}
	cluster.Status.HeadNode.Name = createdRayCluster["metadata"].(map[string]interface{})["name"].(string)
	return nil
}

func (r *Reconciler) getClusterStatus(ctx context.Context, log logr.Logger, cluster *v2pb.RayCluster) (*string, *string, error) {
	// Fetch the status of the RayCluster
	statusRequest := r.k8sRestClient.Get().
		AbsPath(fmt.Sprintf("/apis/%s/namespaces/%s/rayclusters/%s", _apiVersion, cluster.Namespace,
			cluster.Status.HeadNode.Name))

	statusResponse := statusRequest.Do(ctx)
	if statusResponse.Error() != nil {
		log.Error(statusResponse.Error(), "Failed to get RayCluster status: %v")
		return nil, nil, statusResponse.Error()
	}

	// Read the status response
	statusRaw, err := statusResponse.Raw()
	if err != nil {
		log.Error(err, "Failed to read the status message")
		return nil, nil, err
	}

	// Parse the status JSON
	var statusMap map[string]interface{}
	err = json.Unmarshal(statusRaw, &statusMap)
	if err != nil {
		log.Error(err, "Failed to parse status JSON")
		return nil, nil, err
	}

	// Extract and print the cluster status
	status, ok := statusMap["status"].(map[string]interface{})
	if !ok {
		log.Error(err, fmt.Sprintf("Failed to parse status JSON [%+v]", statusMap))
		return nil, nil, nil
	}
	var state *string
	var reason *string
	if rawState, ok := status["state"].(string); ok {
		state = &rawState // Create a pointer to the string
	} else {
		log.Info("state is not in response")
	}
	if endpoints, ok := status["endpoints"].(map[string]interface{}); ok {
		if dashboardUrl, ok := endpoints["dashboard"].(string); ok {
			log.Info(fmt.Sprintf("dashboardUrl is [%s]", dashboardUrl))
		} else {
			log.Info("dashboardUrl is not in response")
			return nil, nil, nil
		}
	} else {
		log.Info("endpoints is not in response")
		return nil, nil, nil
	}
	if rawReason, ok := status["reason"].(string); ok {
		reason = &rawReason
	}

	return state, reason, nil
}

func (r *Reconciler) deleteClusterStatus(ctx context.Context, log logr.Logger, cluster *v2pb.RayCluster) error {
	// Fetch the status of the RayCluster
	deleteRequest := r.k8sRestClient.Delete().
		AbsPath(fmt.Sprintf("/apis/%s/namespaces/%s/rayclusters/%s", _apiVersion, cluster.Namespace,
			cluster.Status.HeadNode.Name))

	deleteResponse := deleteRequest.Do(ctx)
	if deleteResponse.Error() != nil {
		log.Error(deleteResponse.Error(), "Failed to get RayCluster status: %v")
		return deleteResponse.Error()
	}
	return nil
}

func (r *Reconciler) isTerminatedState(status string) bool {
	for _, state := range []string{"failed", "terminated"} {
		if status == state {
			return true
		}
	}
	return false
}

func convertResource(resource *v2pb.ResourceSpec) map[string]map[string]interface{} {
	resourceRequests := map[string]map[string]interface{}{
		"requests": {
			"cpu":              fmt.Sprintf("%d", resource.Cpu),
			"memory":           resource.Memory,
		},
		"limits": {
			"cpu":              fmt.Sprintf("%d", resource.Cpu + 1),
			"memory":           resource.Memory,
		},
	}
	if resource.Gpu > 0 {
		resourceRequests["requests"]["gpu"] = fmt.Sprintf("%d", resource.Gpu)
	}
	if resource.GpuSku != "" {
		resourceRequests["requests"]["gpu_sku"] = resource.GpuSku
	}
	if resource.FileDescriptors != 0 {
		resourceRequests["requests"]["file_descriptors"] = fmt.Sprintf("%d", resource.FileDescriptors)
	}
	if resource.DiskSize != "" && resource.DiskSize != "0" {
		resourceRequests["requests"]["disk_size"] = resource.DiskSize
	}
	return resourceRequests
}

func convertEnvVar(environments []*v2pb.Environment) []map[string]interface{} {
	envVars := make([]map[string]interface{}, 0)
	for _, env := range environments {
		newEnv := map[string]interface{}{
			"name": env.Name,
			"value": env.Value,
		}
		envVars = append(envVars, newEnv)
	}
	return envVars
}

func convertPodSpecToJSON(pod *v2pb.PodSpec) map[string]interface{} {
	containerMap := map[string]interface{}{
		"name":       pod.Name,
		"image":      pod.Image,
		"imagePullPolicy": "Never",
		"command":    pod.Command,
		"resources": convertResource(pod.Resource),
		"volumeMounts": []map[string]string{
			{
				"mountPath": "/tmp/ray",
				"name": "log-volume",
			},
		},
		"envFrom": []map[string]interface{}{
			{
				"configMapRef": map[string]string{
					"name": "michelangelo-config",
				},
			},
		},
	}
	if pod.Env != nil && len(pod.Env) > 0 {
		containerMap["env"] = convertEnvVar(pod.Env)
	}

	return containerMap
}

// Function to convert WorkerGroupSpecs to JSON
func convertWorkerGroupSpecsToJSON(clusterName string, workers []*v2pb.RayWorkerSpec) []map[string]interface{} {
	workerGroupSpecsJson := make([]map[string]interface{}, len(workers))
	for i, workerGroup := range workers {
		workerGroupMap := map[string]interface{}{
			"groupName": fmt.Sprintf("wg-%v", clusterName),
			"replicas":       workerGroup.MinInstances,
			"minReplicas":    workerGroup.MinInstances,
			"maxReplicas":    workerGroup.MaxInstances,
			"rayStartParams": workerGroup.RayStartParams,
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"containers": []map[string]interface{}{convertPodSpecToJSON(workerGroup.Pod)},
				},
			},
		}
		workerGroupSpecsJson[i] = workerGroupMap
	}
	return workerGroupSpecsJson
}

// IsNotFoundError checks if the error is not found error
func IsNotFoundError(err error) bool {
	if e, ok := status.FromError(err); ok {
		return e.Code() == codes.NotFound
	}
	return false
}
