package creation

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/plugins/oss/common"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

var _ conditionInterfaces.ConditionActor[*v2pb.InferenceServer] = &ResourceCreationActor{}

// ResourceCreationActor provisions Kubernetes resources for Triton inference servers.
type ResourceCreationActor struct {
	backend backends.Backend
	logger  *zap.Logger
}

// NewResourceCreationActor creates a condition actor for Triton server provisioning.
func NewResourceCreationActor(backend backends.Backend, logger *zap.Logger) conditionInterfaces.ConditionActor[*v2pb.InferenceServer] {
	return &ResourceCreationActor{
		backend: backend,
		logger:  logger,
	}
}

// GetType returns the condition type identifier for resource creation.
func (a *ResourceCreationActor) GetType() string {
	return common.TritonResourceCreationConditionType
}

// Retrieve checks if Kubernetes infrastructure exists and is ready.
func (a *ResourceCreationActor) Retrieve(ctx context.Context, resource *v2pb.InferenceServer, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Retrieving Triton server condition")
	// todo: ghosharitra: update this so that it checks all the cluster targets
	connectionSpec := resource.Spec.ClusterTargets[0].GetKubernetes()
	// Check if inference server exists
	status, err := a.backend.GetServerStatus(ctx, a.logger, resource.Name, resource.Namespace, connectionSpec)
	if err != nil {
		a.logger.Error("Failed to check server status",
			zap.Error(err),
			zap.String("operation", "get_server_status"),
			zap.String("namespace", resource.Namespace),
			zap.String("inferenceServer", resource.Name))
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_FALSE,
			Reason:  "ServerCheckFailed",
			Message: fmt.Sprintf("Failed to check server status: %v", err),
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

// Run creates the Kubernetes deployment, service, and related resources for Triton.
func (a *ResourceCreationActor) Run(ctx context.Context, resource *v2pb.InferenceServer, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Running Triton server creation")
	for _, clusterTarget := range resource.Spec.ClusterTargets {
		fmt.Println("creating pod in cluster", clusterTarget.GetClusterId())
		_, err := a.backend.CreateServer(ctx, a.logger, resource, clusterTarget.GetKubernetes())
		if err != nil {
			a.logger.Error("Failed to create server",
				zap.Error(err),
				zap.String("operation", "create_server"),
				zap.String("namespace", resource.Namespace),
				zap.String("inferenceServer", resource.Name))
			return &apipb.Condition{
				Type:    a.GetType(),
				Status:  apipb.CONDITION_STATUS_FALSE,
				Reason:  "ServerCreationFailed",
				Message: fmt.Sprintf("Failed to create server: %v", err),
			}, err
		}
	}

	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_TRUE,
		Reason:  "ServerCreationInitiated",
		Message: "Server creation initiated successfully",
	}, nil
}
