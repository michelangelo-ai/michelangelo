package triton

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
	"k8s.io/client-go/tools/record"

	corev1 "k8s.io/api/core/v1"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/configmap"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/endpointregistry"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/plugins"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/plugins/oss/triton/creation"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/plugins/oss/triton/deletion"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

var _ plugins.InferenceServerPlugin = &TritonPlugin{}

// TritonPlugin manages the full lifecycle of Triton inference servers including creation and deletion.
type TritonPlugin struct {
	creationPlugin conditionInterfaces.Plugin[*v2pb.InferenceServer]
	deletionPlugin conditionInterfaces.Plugin[*v2pb.InferenceServer]

	backend  backends.Backend
	Recorder record.EventRecorder
	logger   *zap.Logger
}

// NewPlugin creates a Triton plugin with creation and deletion workflows.
func NewPlugin(backend backends.Backend, endpointRegistry endpointregistry.EndpointRegistry, modelConfigMapProvider configmap.ModelConfigMapProvider, recorder record.EventRecorder, logger *zap.Logger) plugins.InferenceServerPlugin {
	return &TritonPlugin{
		creationPlugin: creation.NewTritonCreationPlugin(backend, endpointRegistry, logger),
		deletionPlugin: deletion.NewTritonDeletionPlugin(backend, modelConfigMapProvider, logger),

		backend:  backend,
		Recorder: recorder,
		logger:   logger,
	}
}

// GetCreationPlugin returns the plugin for provisioning new inference servers.
func (p *TritonPlugin) GetCreationPlugin() conditionInterfaces.Plugin[*v2pb.InferenceServer] {
	return p.creationPlugin
}

// GetDeletionPlugin returns the plugin for removing inference server resources.
func (p *TritonPlugin) GetDeletionPlugin(resource *v2pb.InferenceServer) conditionInterfaces.Plugin[*v2pb.InferenceServer] {
	return p.deletionPlugin
}

// ParseState derives the inference server state from conditions and deletion status.
func (p *TritonPlugin) ParseState(inferenceServer *v2pb.InferenceServer) v2pb.InferenceServerState {
	if !inferenceServer.GetDeletionTimestamp().IsZero() {
		// Resource is being deleted
		return v2pb.INFERENCE_SERVER_STATE_DELETING
	}

	if len(inferenceServer.Status.Conditions) == 0 {
		// No conditions yet, starting creation
		return v2pb.INFERENCE_SERVER_STATE_CREATING
	}

	// Check if all conditions are healthy
	allHealthy := true
	hasFailure := false

	for _, condition := range inferenceServer.Status.Conditions {
		if condition == nil {
			continue
		}
		switch condition.Status {
		case apipb.CONDITION_STATUS_FALSE:
			hasFailure = true
			allHealthy = false
		case apipb.CONDITION_STATUS_UNKNOWN:
			allHealthy = false
		}
	}

	if hasFailure {
		return v2pb.INFERENCE_SERVER_STATE_FAILED
	}

	if allHealthy {
		return v2pb.INFERENCE_SERVER_STATE_SERVING
	}

	// Still in progress
	return v2pb.INFERENCE_SERVER_STATE_CREATING
}

// UpdateDetails updates status, annotations, and labels with backend-specific information from the backend.
func (p *TritonPlugin) UpdateDetails(ctx context.Context, resource *v2pb.InferenceServer) error {
	// Skip if resource is being deleted
	if !resource.GetDeletionTimestamp().IsZero() {
		return nil
	}

	// Skip if we haven't attempted creation yet
	if resource.Status.ObservedGeneration == 0 || resource.Status.State == v2pb.INFERENCE_SERVER_STATE_CREATING {
		return nil
	}

	// Handle based on deployment strategy
	if resource.Spec.GetDeploymentStrategy().GetRemoteClusterDeployment() != nil {
		return p.updateRemoteClustersDetails(ctx, resource)
	}
	return p.updateControlPlaneClusterDetails(ctx, resource)
}

// updateControlPlaneDetails updates status for control plane cluster deployment.
// RemoteClusterStatuses is left empty for this strategy.
func (p *TritonPlugin) updateControlPlaneClusterDetails(ctx context.Context, resource *v2pb.InferenceServer) error {
	status, err := p.backend.GetServerStatus(ctx, resource.Name, resource.Namespace, nil)
	if err != nil {
		p.logger.Error("Failed to get server status",
			zap.Error(err),
			zap.String("operation", "get_server_status"),
			zap.String("namespace", resource.Namespace),
			zap.String("inferenceServer", resource.Name))
		return nil
	}

	// Map cluster state to inference server state
	switch status.ClusterState {
	case v2pb.CLUSTER_STATE_READY:
		if resource.Status.State != v2pb.INFERENCE_SERVER_STATE_SERVING {
			p.Recorder.Event(resource, corev1.EventTypeNormal, "ServerReady", "Inference server is ready")
		}
		resource.Status.State = v2pb.INFERENCE_SERVER_STATE_SERVING
	case v2pb.CLUSTER_STATE_CREATING:
		resource.Status.State = v2pb.INFERENCE_SERVER_STATE_CREATING
	case v2pb.CLUSTER_STATE_FAILED:
		if resource.Status.State != v2pb.INFERENCE_SERVER_STATE_FAILED {
			p.Recorder.Event(resource, corev1.EventTypeWarning, "ServerFailed", "Inference server failed")
		}
		resource.Status.State = v2pb.INFERENCE_SERVER_STATE_FAILED
	case v2pb.CLUSTER_STATE_DELETING:
		resource.Status.State = v2pb.INFERENCE_SERVER_STATE_DELETING
	}

	return nil
}

// updateRemoteClustersDetails updates status for remote clusters deployment.
// Populates RemoteClusterStatuses with per-cluster state.
func (p *TritonPlugin) updateRemoteClustersDetails(ctx context.Context, resource *v2pb.InferenceServer) error {
	clusterTargets := resource.Spec.GetDeploymentStrategy().GetRemoteClusterDeployment().GetClusterTargets()

	// Initialize remote cluster statuses if needed
	if len(resource.Status.GetRemoteClusterStatuses()) == 0 {
		resource.Status.RemoteClusterStatuses = make([]*v2pb.RemoteClusterStatus, len(clusterTargets))
		for i := range resource.Status.RemoteClusterStatuses {
			resource.Status.RemoteClusterStatuses[i] = &v2pb.RemoteClusterStatus{}
		}
	}

	numReadyClusters := 0
	numFailedClusters := 0

	for i, clusterTarget := range clusterTargets {
		status, err := p.backend.GetServerStatus(ctx, resource.Name, resource.Namespace, clusterTarget)
		if err != nil {
			p.logger.Error("Failed to get server status",
				zap.Error(err),
				zap.String("operation", "get_server_status"),
				zap.String("namespace", resource.Namespace),
				zap.String("inferenceServer", resource.Name),
				zap.String("cluster", clusterTarget.ClusterId))
			continue
		}

		// Check for state change
		if status.ClusterState != resource.Status.RemoteClusterStatuses[i].State {
			p.logger.Info("External state change detected",
				zap.String("cluster", clusterTarget.ClusterId),
				zap.String("currentState", resource.Status.RemoteClusterStatuses[i].State.String()),
				zap.String("externalState", status.ClusterState.String()))

			// Record state transition events
			var message string
			switch status.ClusterState {
			case v2pb.CLUSTER_STATE_READY:
				message = fmt.Sprintf("Cluster %s is ready", clusterTarget.ClusterId)
				p.Recorder.Event(resource, corev1.EventTypeNormal, "ClusterReady", message)
			case v2pb.CLUSTER_STATE_CREATING:
				message = fmt.Sprintf("Cluster %s is creating", clusterTarget.ClusterId)
				p.Recorder.Event(resource, corev1.EventTypeNormal, "ClusterCreating", message)
			case v2pb.CLUSTER_STATE_DELETING:
				message = fmt.Sprintf("Cluster %s is deleting", clusterTarget.ClusterId)
				p.Recorder.Event(resource, corev1.EventTypeNormal, "ClusterDeleting", message)
			case v2pb.CLUSTER_STATE_FAILED:
				message = fmt.Sprintf("Cluster %s failed", clusterTarget.ClusterId)
				p.Recorder.Event(resource, corev1.EventTypeWarning, "ClusterFailed", message)
			}

			// Update cluster status
			resource.Status.RemoteClusterStatuses[i].State = status.ClusterState
			resource.Status.RemoteClusterStatuses[i].ClusterId = clusterTarget.ClusterId
			resource.Status.RemoteClusterStatuses[i].Message = message
			resource.Status.RemoteClusterStatuses[i].Endpoint = status.Endpoint
			resource.Status.RemoteClusterStatuses[i].LastUpdated = time.Now().Format(time.RFC3339)
		}

		// Count cluster states for overall status
		switch resource.Status.RemoteClusterStatuses[i].State {
		case v2pb.CLUSTER_STATE_READY:
			numReadyClusters++
		case v2pb.CLUSTER_STATE_FAILED:
			numFailedClusters++
		}
	}

	// Infer server state based on cluster states
	if numReadyClusters == len(clusterTargets) {
		resource.Status.State = v2pb.INFERENCE_SERVER_STATE_SERVING
	} else if numFailedClusters > 0 {
		resource.Status.State = v2pb.INFERENCE_SERVER_STATE_FAILED
	} else if numReadyClusters > 0 {
		resource.Status.State = v2pb.INFERENCE_SERVER_STATE_CREATING
	}

	return nil
}

// UpdateConditions filters the resource conditions to only those relevant to the current plugin workflow.
func (p *TritonPlugin) UpdateConditions(resource *v2pb.InferenceServer, conditionPlugin conditionInterfaces.Plugin[*v2pb.InferenceServer]) {
	actors := conditionPlugin.GetActors()
	resource.Status.Conditions = p.getRelevantConditions(actors, resource.Status.Conditions)
}

// getRelevantConditions gets the list of Conditions for a given conditional plugin.
func (p TritonPlugin) getRelevantConditions(actors []conditionInterfaces.ConditionActor[*v2pb.InferenceServer], allConditons []*apipb.Condition) []*apipb.Condition {
	relevantConditions := make([]*apipb.Condition, 0)
	conditionTypesMap := getConditionsMap(allConditons)

	for _, actor := range actors {
		if condition, wasFound := conditionTypesMap[actor.GetType()]; wasFound {
			relevantConditions = append(relevantConditions, condition)
		}
	}
	return relevantConditions
}

// getConditionsMap gets the object mapping condition types to conditions
func getConditionsMap(conditions []*apipb.Condition) map[string]*apipb.Condition {
	conditionTypesMap := make(map[string]*apipb.Condition)
	for _, condition := range conditions {
		conditionTypesMap[condition.GetType()] = condition
	}
	return conditionTypesMap
}
