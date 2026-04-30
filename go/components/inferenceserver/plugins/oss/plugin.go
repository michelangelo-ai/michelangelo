package oss

import (
	"context"

	"go.uber.org/zap"
	"k8s.io/client-go/tools/record"

	corev1 "k8s.io/api/core/v1"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/clientfactory"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/endpoints"
	modelconfig "github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/modelconfig"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/plugins"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/plugins/oss/creation"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/plugins/oss/deletion"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

var _ plugins.Plugin = &Plugin{}

// Plugin is the OSS plugin implementation.
// It manages lifecycle workflows for open-source inference server backends.
type Plugin struct {
	creationPlugin conditionInterfaces.Plugin[*v2pb.InferenceServer]
	deletionPlugin conditionInterfaces.Plugin[*v2pb.InferenceServer]

	registry      *backends.Registry
	clientFactory clientfactory.ClientFactory
	Recorder      record.EventRecorder
	logger        *zap.Logger
}

// NewPlugin creates a plugin with creation and deletion workflows.
func NewOSSPlugin(clientFactory clientfactory.ClientFactory, registry *backends.Registry, modelConfigProvider modelconfig.ModelConfigProvider, endpointPublisher endpoints.EndpointPublisher, endpointSource endpoints.EndpointSource, recorder record.EventRecorder, logger *zap.Logger) plugins.Plugin {
	return &Plugin{
		creationPlugin: creation.NewCreationPlugin(clientFactory, registry, modelConfigProvider, endpointPublisher, endpointSource, logger),
		deletionPlugin: deletion.NewDeletionPlugin(clientFactory, registry, modelConfigProvider, logger),

		clientFactory: clientFactory,
		registry:      registry,
		Recorder:      recorder,
		logger:        logger,
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

	// Get backend from registry based on resource spec
	backend, err := p.registry.GetBackend(resource.Spec.BackendType)
	if err != nil {
		p.logger.Error("Failed to get backend for inference server",
			zap.Error(err),
			zap.String("operation", "get_backend"),
			zap.String("namespace", resource.Namespace),
			zap.String("inferenceServer", resource.Name),
			zap.String("backendType", resource.Spec.BackendType.String()))
		return nil
	}

	// Get current status from backend, aggregated across cluster targets
	aggregateState, ok := p.aggregateBackendState(ctx, backend, resource)
	if !ok {
		// No conclusive aggregate this reconcile — keep the existing state.
		return nil
	}

	// Update status based on external state
	if aggregateState != resource.Status.State {
		p.logger.Info("External state change detected",
			zap.String("currentState", resource.Status.State.String()),
			zap.String("externalState", aggregateState.String()))

		resource.Status.State = aggregateState

		// Record state transition events
		switch aggregateState {
		case v2pb.INFERENCE_SERVER_STATE_SERVING:
			p.Recorder.Event(resource, corev1.EventTypeNormal, "CreationCompleted", "InferenceServer creation completed successfully")
		case v2pb.INFERENCE_SERVER_STATE_FAILED:
			p.Recorder.Event(resource, corev1.EventTypeWarning, "CreationFailed", "InferenceServer creation failed")
		}
	}
	return nil
}

// aggregateBackendState polls the backend on every target cluster and reduces the
// per-cluster states into a single InferenceServerState. The boolean return is false
// when no conclusive aggregate exists (e.g., all status fetches errored).
//
// FAILED on any cluster wins; otherwise SERVING requires every reachable cluster to
// be SERVING.
func (p *Plugin) aggregateBackendState(ctx context.Context, backend backends.Backend, resource *v2pb.InferenceServer) (v2pb.InferenceServerState, bool) {
	servingCount := 0
	totalCount := 0
	hasFailure := false

	for _, target := range resource.Spec.ClusterTargets {
		kubeClient, err := p.clientFactory.GetClient(ctx, target)
		if err != nil {
			p.logger.Error("Failed to resolve client",
				zap.Error(err),
				zap.String("operation", "resolve_client"),
				zap.String("inferenceServer", resource.Name),
				zap.String("cluster_id", target.GetClusterId()))
			continue
		}
		totalCount++
		status, err := backend.GetServerStatus(ctx, p.logger, kubeClient, resource.Name, resource.Namespace)
		if err != nil {
			// Don't fail reconciliation for status check errors
			p.logger.Error("Failed to get server status",
				zap.Error(err),
				zap.String("operation", "get_server_status"),
				zap.String("namespace", resource.Namespace),
				zap.String("inferenceServer", resource.Name),
				zap.String("cluster_id", target.GetClusterId()))
			continue
		}
		switch status.State {
		case v2pb.INFERENCE_SERVER_STATE_SERVING:
			servingCount++
		case v2pb.INFERENCE_SERVER_STATE_FAILED:
			hasFailure = true
		}
	}

	if hasFailure {
		return v2pb.INFERENCE_SERVER_STATE_FAILED, true
	}
	if totalCount > 0 && servingCount == totalCount {
		return v2pb.INFERENCE_SERVER_STATE_SERVING, true
	}
	return v2pb.INFERENCE_SERVER_STATE_INVALID, false
}

// UpdateConditions filters the resource conditions to only those relevant to the current plugin workflow.
func (p *Plugin) UpdateConditions(resource *v2pb.InferenceServer, conditionPlugin conditionInterfaces.Plugin[*v2pb.InferenceServer]) {
	actors := conditionPlugin.GetActors()
	resource.Status.Conditions = p.getRelevantConditions(actors, resource.Status.Conditions)
}

// getRelevantConditions gets the list of Conditions for a given conditional plugin.
func (p Plugin) getRelevantConditions(actors []conditionInterfaces.ConditionActor[*v2pb.InferenceServer], allConditons []*apipb.Condition) []*apipb.Condition {
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
