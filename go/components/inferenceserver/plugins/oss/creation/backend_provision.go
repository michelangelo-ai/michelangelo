package creation

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"sigs.k8s.io/controller-runtime/pkg/client"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	conditionUtils "github.com/michelangelo-ai/michelangelo/go/base/conditions/utils"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/plugins/oss/common"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

var _ conditionInterfaces.ConditionActor[*v2pb.InferenceServer] = &BackendProvisionActor{}

// BackendProvisioningActor provisions Kubernetes resources for inference servers.
type BackendProvisionActor struct {
	client   client.Client
	registry *backends.Registry
	logger   *zap.Logger
}

// NewBackendProvisionActor creates a condition actor for inference server provisioning.
func NewBackendProvisionActor(client client.Client, registry *backends.Registry, logger *zap.Logger) conditionInterfaces.ConditionActor[*v2pb.InferenceServer] {
	return &BackendProvisionActor{
		client:   client,
		registry: registry,
		logger:   logger,
	}
}

// GetType returns the condition type identifier for backend provisioning.
func (a *BackendProvisionActor) GetType() string {
	return common.BackendProvisionConditionType
}

// Retrieve checks if Kubernetes infrastructure exists (deployment and service).
func (a *BackendProvisionActor) Retrieve(ctx context.Context, resource *v2pb.InferenceServer, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Retrieving backend provisioning condition")

	backend, err := a.registry.GetBackend(resource.Spec.BackendType)
	if err != nil {
		return conditionUtils.GenerateFalseCondition(condition, "BackendNotFound", fmt.Sprintf("Failed to get backend: %v", err)), nil
	}

	// Check if inference server resources exist
	status, err := backend.GetServerStatus(ctx, a.logger, a.client, resource.Name, resource.Namespace)
	if err != nil {
		a.logger.Error("Failed to check backend provisioning status",
			zap.Error(err),
			zap.String("operation", "get_backend_provisioning_status"),
			zap.String("namespace", resource.Namespace),
			zap.String("backend", resource.Name))
		return conditionUtils.GenerateFalseCondition(condition, "BackendProvisioningCheckFailed", fmt.Sprintf("Failed to check backend status: %v", err)), nil
	}

	switch status.State {
	case v2pb.INFERENCE_SERVER_STATE_SERVING:
		return conditionUtils.GenerateTrueCondition(condition), nil
	default:
		return conditionUtils.GenerateFalseCondition(condition, "BackendProvisioningFailed", fmt.Sprintf("Backend state is not serving: %v", status.State)), nil
	}
}

// Run creates the Kubernetes deployment, service, and related resources for inference servers.
func (a *BackendProvisionActor) Run(ctx context.Context, resource *v2pb.InferenceServer, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Running backend provisioning")

	backend, err := a.registry.GetBackend(resource.Spec.BackendType)
	if err != nil {
		return conditionUtils.GenerateFalseCondition(condition, "BackendNotFound", fmt.Sprintf("Failed to get backend: %v", err)), nil
	}

	_, err = backend.CreateServer(ctx, a.logger, a.client, resource)
	if err != nil {
		a.logger.Error("Failed to create backend",
			zap.Error(err),
			zap.String("operation", "create_backend"),
			zap.String("namespace", resource.Namespace),
			zap.String("inferenceServer", resource.Name))
		return conditionUtils.GenerateFalseCondition(condition, "BackendProvisionFailed", fmt.Sprintf("Failed to provision backend: %v", err)), err
	}

	return conditionUtils.GenerateTrueCondition(condition), nil
}
