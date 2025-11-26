package triton

import (
	"context"

	"go.uber.org/zap"
	"k8s.io/client-go/tools/record"

	corev1 "k8s.io/api/core/v1"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/configmap"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/gateways"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/plugins"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/plugins/oss/triton/creation"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/plugins/oss/triton/deletion"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/proxy"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

var _ plugins.InferenceServerPlugin = &TritonPlugin{}

// TritonPlugin implements InferenceServerPlugin for Triton backend
type TritonPlugin struct {
	creationPlugin conditionInterfaces.Plugin[*v2pb.InferenceServer]
	deletionPlugin conditionInterfaces.Plugin[*v2pb.InferenceServer]

	gateway                gateways.Gateway
	modelConfigMapProvider configmap.ModelConfigMapProvider
	proxyProvider          proxy.ProxyProvider
	Recorder               record.EventRecorder
	logger                 *zap.Logger
}

// NewPlugin creates a new Triton plugin
func NewPlugin(gateway gateways.Gateway, modelConfigMapProvider configmap.ModelConfigMapProvider, proxyProvider proxy.ProxyProvider, recorder record.EventRecorder, logger *zap.Logger) plugins.InferenceServerPlugin {
	return &TritonPlugin{
		creationPlugin: creation.NewTritonCreationPlugin(gateway, proxyProvider, logger),
		deletionPlugin: deletion.NewTritonDeletionPlugin(gateway, proxyProvider, modelConfigMapProvider, logger),

		gateway:                gateway,
		modelConfigMapProvider: modelConfigMapProvider,
		proxyProvider:          proxyProvider,
		Recorder:               recorder,
		logger:                 logger,
	}
}

// GetCreationPlugin returns the plugin for infrastructure creation
func (p *TritonPlugin) GetCreationPlugin() conditionInterfaces.Plugin[*v2pb.InferenceServer] {
	return p.creationPlugin
}

// GetDeletionPlugin returns the plugin for infrastructure cleanup
func (p *TritonPlugin) GetDeletionPlugin(resource *v2pb.InferenceServer) conditionInterfaces.Plugin[*v2pb.InferenceServer] {
	return p.deletionPlugin
}

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

// UpdateDetails will retrieve and set the status that is relevant to the inference server plugin.
func (p *TritonPlugin) UpdateDetails(ctx context.Context, resource *v2pb.InferenceServer) error {
	// Skip if resource is being deleted
	if !resource.GetDeletionTimestamp().IsZero() {
		return nil
	}

	// Skip if we haven't attempted creation yet
	if resource.Status.ObservedGeneration == 0 || resource.Status.State == v2pb.INFERENCE_SERVER_STATE_CREATING {
		return nil
	}

	// Get current status from gateway
	statusResp, err := p.gateway.GetInfrastructureStatus(ctx, p.logger, gateways.GetInfrastructureStatusRequest{
		InferenceServer: resource.Name,
		Namespace:       resource.Namespace,
		BackendType:     resource.Spec.BackendType,
	})
	if err != nil {
		// Don't fail reconciliation for status check errors
		p.logger.Info("Failed to get infrastructure status", zap.Error(err))
		return nil
	}

	// Update status based on external state
	if statusResp.Status.State != resource.Status.State {
		p.logger.Info("External state change detected",
			zap.String("currentState", resource.Status.State.String()),
			zap.String("externalState", statusResp.Status.State.String()))

		resource.Status.State = statusResp.Status.State
		resource.Status.ProviderMetadata = statusResp.Status.Message

		// Record state transition events
		switch statusResp.Status.State {
		case v2pb.INFERENCE_SERVER_STATE_SERVING:
			p.Recorder.Event(resource, corev1.EventTypeNormal, "CreationCompleted", "InferenceServer creation completed successfully")
		case v2pb.INFERENCE_SERVER_STATE_FAILED:
			p.Recorder.Event(resource, corev1.EventTypeWarning, "CreationFailed", "InferenceServer creation failed")
		}
	}
	return nil
}

// UpdateConditions gets the list of conditions pertaining to a particular plugin
// and for a particular resource instance.
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
