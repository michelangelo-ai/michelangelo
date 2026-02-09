package oss

import (
	"context"

	"go.uber.org/zap"
	"k8s.io/client-go/tools/record"

	corev1 "k8s.io/api/core/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends"
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

	backend  backends.Backend
	client   client.Client
	Recorder record.EventRecorder
	logger   *zap.Logger
}

// NewPlugin creates a plugin with creation and deletion workflows.
func NewOSSPlugin(client client.Client, backend backends.Backend, modelConfigProvider modelconfig.ModelConfigProvider, recorder record.EventRecorder, logger *zap.Logger) plugins.Plugin {
	return &Plugin{
		creationPlugin: creation.NewCreationPlugin(client, backend, modelConfigProvider, logger),
		deletionPlugin: deletion.NewDeletionPlugin(client, backend, modelConfigProvider, logger),

		client:   client,
		backend:  backend,
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

	// Get current status from backend
	status, err := p.backend.GetServerStatus(ctx, p.logger, p.client, resource.Name, resource.Namespace)
	if err != nil {
		// Don't fail reconciliation for status check errors
		p.logger.Error("Failed to get server status",
			zap.Error(err),
			zap.String("operation", "get_server_status"),
			zap.String("namespace", resource.Namespace),
			zap.String("inferenceServer", resource.Name))
		return nil
	}

	// Update status based on external state
	if status.State != resource.Status.State {
		p.logger.Info("External state change detected",
			zap.String("currentState", resource.Status.State.String()),
			zap.String("externalState", status.State.String()))

		resource.Status.State = status.State

		// Record state transition events
		switch status.State {
		case v2pb.INFERENCE_SERVER_STATE_SERVING:
			p.Recorder.Event(resource, corev1.EventTypeNormal, "CreationCompleted", "InferenceServer creation completed successfully")
		case v2pb.INFERENCE_SERVER_STATE_FAILED:
			p.Recorder.Event(resource, corev1.EventTypeWarning, "CreationFailed", "InferenceServer creation failed")
		}
	}
	return nil
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
