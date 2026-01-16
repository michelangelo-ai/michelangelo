package triton

import (
	"context"

	"go.uber.org/zap"
	"k8s.io/client-go/tools/record"

	corev1 "k8s.io/api/core/v1"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/configmap"
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
func NewPlugin(backend backends.Backend, modelConfigMapProvider configmap.ModelConfigMapProvider, recorder record.EventRecorder, logger *zap.Logger) plugins.InferenceServerPlugin {
	return &TritonPlugin{
		creationPlugin: creation.NewTritonCreationPlugin(backend, logger),
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

	// Get current status from backend
	numReadyClusters := 0
	numFailedClusters := 0

	// todo: ghosharitra: revise this
	if len(resource.Status.TargetClusterStatuses) == 0 {
		resource.Status.TargetClusterStatuses = make([]*v2pb.TargetClusterStatus, len(resource.Spec.ClusterTargets))
		for i := range resource.Status.TargetClusterStatuses {
			resource.Status.TargetClusterStatuses[i] = &v2pb.TargetClusterStatus{}
		}
	}

	for i, clusterTarget := range resource.Spec.ClusterTargets {
		status, err := p.backend.GetServerStatus(ctx, resource.Name, resource.Namespace, clusterTarget)
		if err != nil {
			// Don't fail reconciliation for status check errors
			p.logger.Error("Failed to get server status",
				zap.Error(err),
				zap.String("operation", "get_server_status"),
				zap.String("namespace", resource.Namespace),
				zap.String("inferenceServer", resource.Name))
			continue
		}

		// Update status based on external state
		if status.ClusterState != resource.Status.TargetClusterStatuses[i].State {
			p.logger.Info("External state change detected",
				zap.String("currentState", resource.Status.TargetClusterStatuses[i].State.String()),
				zap.String("externalState", status.ClusterState.String()))

			resource.Status.TargetClusterStatuses[i].State = status.ClusterState

			// Record state transition events
			switch status.ClusterState {
			case v2pb.CLUSTER_STATE_READY:
				p.Recorder.Event(resource, corev1.EventTypeNormal, "ClusterReady", "Cluster is ready")
				numReadyClusters++
			case v2pb.CLUSTER_STATE_CREATING:
				p.Recorder.Event(resource, corev1.EventTypeNormal, "ClusterCreating", "Cluster is creating")
			case v2pb.CLUSTER_STATE_DELETING:
				p.Recorder.Event(resource, corev1.EventTypeNormal, "ClusterDeleting", "Cluster is deleting")
			case v2pb.CLUSTER_STATE_FAILED:
				numFailedClusters++
				p.Recorder.Event(resource, corev1.EventTypeWarning, "ClusterFailed", "Cluster failed")
			}
		}
	}

	// todo: ghosharitra: revise this
	// infer server state based on the number of ready and failed target cluster deployments
	if numReadyClusters == len(resource.Spec.ClusterTargets) {
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
