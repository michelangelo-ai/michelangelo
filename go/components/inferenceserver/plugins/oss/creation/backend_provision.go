package creation

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"sigs.k8s.io/controller-runtime/pkg/client"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/plugins/oss/common"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

var _ conditionInterfaces.ConditionActor[*v2pb.InferenceServer] = &BackendProvisionActor{}

// BackendProvisioningActor provisions Kubernetes resources for inference servers.
type BackendProvisionActor struct {
	client  client.Client
	backend backends.Backend
	logger  *zap.Logger
}

// NewBackendProvisionActor creates a condition actor for inference server provisioning.
func NewBackendProvisionActor(client client.Client, backend backends.Backend, logger *zap.Logger) conditionInterfaces.ConditionActor[*v2pb.InferenceServer] {
	return &BackendProvisionActor{
		client:  client,
		backend: backend,
		logger:  logger,
	}
}

// GetType returns the condition type identifier for backend provisioning.
func (a *BackendProvisionActor) GetType() string {
	return common.BackendProvisionConditionType
}

// Retrieve checks if Kubernetes infrastructure exists and is ready.
func (a *BackendProvisionActor) Retrieve(ctx context.Context, resource *v2pb.InferenceServer, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Retrieving backend provisioning condition")

	// Check if inference server exists
	status, err := a.backend.GetServerStatus(ctx, a.logger, a.client, resource.Name, resource.Namespace)
	if err != nil {
		a.logger.Error("Failed to check backend provisioning status",
			zap.Error(err),
			zap.String("operation", "get_backend_provisioning_status"),
			zap.String("namespace", resource.Namespace),
			zap.String("backend", resource.Name))
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_FALSE,
			Reason:  "BackendProvisioningCheckFailed",
			Message: fmt.Sprintf("Failed to check backend status: %v", err),
		}, nil
	}

	if status.State == v2pb.INFERENCE_SERVER_STATE_SERVING {
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_TRUE,
			Reason:  "ServerReady",
			Message: "Server is ready",
		}, nil
	} else if status.State == v2pb.INFERENCE_SERVER_STATE_CREATING {
		// Server doesn't exist or is incomplete, needs to be created
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_FALSE,
			Reason:  "ServerNotFound",
			Message: "Server needs to be created",
		}, nil
	}

	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_FALSE,
		Reason:  "ServerCreating",
		Message: "Server is being created",
	}, nil
}

// Run creates the Kubernetes deployment, service, and related resources for inference servers.
func (a *BackendProvisionActor) Run(ctx context.Context, resource *v2pb.InferenceServer, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Running backend provisioning")

	_, err := a.backend.CreateServer(ctx, a.logger, a.client, resource)
	if err != nil {
		a.logger.Error("Failed to create backend",
			zap.Error(err),
			zap.String("operation", "create_backend"),
			zap.String("namespace", resource.Namespace),
			zap.String("inferenceServer", resource.Name))
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_FALSE,
			Reason:  "BackendProvisionFailed",
			Message: fmt.Sprintf("Failed to provision backend: %v", err),
		}, err
	}

	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_TRUE,
		Reason:  "BackendProvisionSucceeded",
		Message: "Backend provisioned successfully",
	}, nil
}
