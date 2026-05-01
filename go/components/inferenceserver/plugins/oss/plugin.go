package oss

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"
	"k8s.io/client-go/tools/record"

	corev1 "k8s.io/api/core/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/clientfactory"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/endpointregistry"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/plugins"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/plugins/oss/creation"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/plugins/oss/deletion"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

var _ plugins.InferenceServerPlugin = &Plugin{}

// Plugin manages the full lifecycle of inference servers including creation and deletion.
type Plugin struct {
	client         client.Client
	clientFactory  clientfactory.ClientFactory
	creationPlugin conditionInterfaces.Plugin[*v2pb.InferenceServer]
	deletionPlugin conditionInterfaces.Plugin[*v2pb.InferenceServer]

	registry *backends.Registry
	Recorder record.EventRecorder
	logger   *zap.Logger
}

// NewPlugin creates a inference server plugin with creation and deletion workflows.
func NewOSSPlugin(registry *backends.Registry, client client.Client, clientFactory clientfactory.ClientFactory, endpointRegistry endpointregistry.EndpointRegistry, recorder record.EventRecorder, logger *zap.Logger) plugins.InferenceServerPlugin {
	return &Plugin{
		creationPlugin: creation.NewCreationPlugin(client, clientFactory, registry, endpointRegistry, logger),
		deletionPlugin: deletion.NewDeletionPlugin(client, clientFactory, registry, logger),

		registry: registry,
		client:   client,
		Recorder: recorder,
		logger:   logger,
	}
}

// GetCreationPlugin returns the plugin for provisioning new inference servers.
func (p *Plugin) GetCreationPlugin() conditionInterfaces.Plugin[*v2pb.InferenceServer] {
	return p.creationPlugin
}

// GetDeletionPlugin returns the plugin for removing inference server resources.
func (p *Plugin) GetDeletionPlugin(resource *v2pb.InferenceServer) conditionInterfaces.Plugin[*v2pb.InferenceServer] {
	return p.deletionPlugin
}

// ParseState derives the inference server state from conditions and deletion status.
func (p *Plugin) ParseState(inferenceServer *v2pb.InferenceServer) v2pb.InferenceServerState {
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
func (p *Plugin) UpdateDetails(ctx context.Context, resource *v2pb.InferenceServer) error {
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
func (p *Plugin) updateControlPlaneClusterDetails(ctx context.Context, resource *v2pb.InferenceServer) error {
	status, err := p.backend.GetServerStatus(ctx, p.logger, p.client, resource.Name, resource.Namespace)
	if err != nil {
		p.logger.Error("Failed to get server status",
			zap.Error(err),
			zap.String("operation", "get_server_status"),
			zap.String("namespace", resource.Namespace),
			zap.String("inferenceServer", resource.Name))
		return nil
	}

	switch status.State {
	case v2pb.INFERENCE_SERVER_STATE_SERVING:
		if resource.Status.State != v2pb.INFERENCE_SERVER_STATE_SERVING {
			p.Recorder.Event(resource, corev1.EventTypeNormal, "ServerReady", "Inference server is ready")
		}
		resource.Status.State = v2pb.INFERENCE_SERVER_STATE_SERVING
	case v2pb.INFERENCE_SERVER_STATE_FAILED:
		if resource.Status.State != v2pb.INFERENCE_SERVER_STATE_FAILED {
			p.Recorder.Event(resource, corev1.EventTypeWarning, "ServerFailed", "Inference server failed")
		}
	}
	resource.Status.State = status.State

	return nil
}

// updateRemoteClustersDetails updates status for remote clusters deployment.
// Populates RemoteClusterStatuses with per-cluster state.
func (p *Plugin) updateRemoteClustersDetails(ctx context.Context, resource *v2pb.InferenceServer) error {
	clusterTargets := resource.Spec.GetDeploymentStrategy().GetRemoteClusterDeployment().GetClusterTargets()

	// resolve missing remote cluster statuses by matching current cluster targets,
	// preserving existing statuses by cluster_id and creating new ones as needed.
	existingStatuses := make(map[string]*v2pb.RemoteClusterStatus, len(resource.Status.GetRemoteClusterStatuses()))
	for _, s := range resource.Status.GetRemoteClusterStatuses() {
		existingStatuses[s.GetClusterId()] = s
	}
	resource.Status.RemoteClusterStatuses = make([]*v2pb.RemoteClusterStatus, len(clusterTargets))
	for i, ct := range clusterTargets {
		if existing, ok := existingStatuses[ct.GetClusterId()]; ok {
			resource.Status.RemoteClusterStatuses[i] = existing
		} else {
			resource.Status.RemoteClusterStatuses[i] = &v2pb.RemoteClusterStatus{ClusterId: ct.GetClusterId()}
		}
	}

	clusterStatusByID := make(map[string]*v2pb.RemoteClusterStatus, len(resource.Status.RemoteClusterStatuses))
	for _, cs := range resource.Status.RemoteClusterStatuses {
		clusterStatusByID[cs.GetClusterId()] = cs
	}

	numReadyClusters := 0
	numFailedClusters := 0
	targetClusterClients := common.GetClusterClients(ctx, p.logger, resource, p.clientFactory, p.client)
	for clusterId, clusterClient := range targetClusterClients {
		clusterStatus, ok := clusterStatusByID[clusterId]
		if !ok {
			p.logger.Warn("No status entry for cluster, skipping",
				zap.String("cluster", clusterId))
			continue
		}

		status, err := p.backend.GetServerStatus(ctx, p.logger, clusterClient, resource.Name, resource.Namespace)
		if err != nil {
			p.logger.Error("Failed to get server status",
				zap.Error(err),
				zap.String("operation", "get_server_status"),
				zap.String("namespace", resource.Namespace),
				zap.String("inferenceServer", resource.Name),
				zap.String("cluster", clusterId))
			continue
		}

		if status.State != clusterStatus.State {
			p.logger.Info("External state change detected",
				zap.String("cluster", clusterId),
				zap.String("previousState", clusterStatus.State.String()),
				zap.String("currentState", status.State.String()))

			var message string
			switch status.State {
			case v2pb.INFERENCE_SERVER_STATE_SERVING:
				message = fmt.Sprintf("Cluster %s is serving", clusterId)
				p.Recorder.Event(resource, corev1.EventTypeNormal, "ClusterReady", message)
			case v2pb.INFERENCE_SERVER_STATE_CREATING:
				message = fmt.Sprintf("Cluster %s is creating", clusterId)
				p.Recorder.Event(resource, corev1.EventTypeNormal, "ClusterCreating", message)
			case v2pb.INFERENCE_SERVER_STATE_DELETING:
				message = fmt.Sprintf("Cluster %s is deleting", clusterId)
				p.Recorder.Event(resource, corev1.EventTypeNormal, "ClusterDeleting", message)
			case v2pb.INFERENCE_SERVER_STATE_FAILED:
				message = fmt.Sprintf("Cluster %s failed", clusterId)
				p.Recorder.Event(resource, corev1.EventTypeWarning, "ClusterFailed", message)
			}

			clusterStatus.State = status.State
			clusterStatus.ClusterId = clusterId
			clusterStatus.Message = message
			clusterStatus.Endpoint = strings.Join(status.Endpoints, ",")
			clusterStatus.LastUpdated = time.Now().Format(time.RFC3339)
		}

		switch clusterStatus.State {
		case v2pb.INFERENCE_SERVER_STATE_SERVING:
			numReadyClusters++
		case v2pb.INFERENCE_SERVER_STATE_FAILED:
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
func (p *Plugin) UpdateConditions(resource *v2pb.InferenceServer, conditionPlugin conditionInterfaces.Plugin[*v2pb.InferenceServer]) {
	actors := conditionPlugin.GetActors()
	resource.Status.Conditions = p.getRelevantConditions(actors, resource.Status.Conditions)
}

// getRelevantConditions gets the list of Conditions for a given conditional plugin.
func (p *Plugin) getRelevantConditions(actors []conditionInterfaces.ConditionActor[*v2pb.InferenceServer], allConditons []*apipb.Condition) []*apipb.Condition {
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
