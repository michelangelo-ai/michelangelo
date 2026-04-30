package creation

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/client"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	conditionUtils "github.com/michelangelo-ai/michelangelo/go/base/conditions/utils"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/clientfactory"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/plugins/oss/common"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

var _ conditionInterfaces.ConditionActor[*v2pb.InferenceServer] = &BackendProvisionActor{}

// BackendProvisioningActor provisions Kubernetes resources for inference servers.
type BackendProvisionActor struct {
	clientFactory clientfactory.ClientFactory
	registry      *backends.Registry
	logger        *zap.Logger
}

// NewBackendProvisionActor creates a condition actor for inference server provisioning.
func NewBackendProvisionActor(clientFactory clientfactory.ClientFactory, registry *backends.Registry, logger *zap.Logger) conditionInterfaces.ConditionActor[*v2pb.InferenceServer] {
	return &BackendProvisionActor{
		clientFactory: clientFactory,
		registry:      registry,
		logger:        logger,
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

	var failures []string
	for _, target := range resource.Spec.ClusterTargets {
		kubeClient, err := a.clientFactory.GetClient(ctx, target)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: client error: %v", target.GetClusterId(), err))
			continue
		}

		// Check if inference server resources exist
		status, err := backend.GetServerStatus(ctx, a.logger, kubeClient, resource.Name, resource.Namespace)
		if err != nil {
			a.logger.Error("Failed to check backend provisioning status",
				zap.Error(err),
				zap.String("operation", "get_backend_provisioning_status"),
				zap.String("namespace", resource.Namespace),
				zap.String("backend", resource.Name),
				zap.String("cluster_id", target.GetClusterId()))
			failures = append(failures, fmt.Sprintf("%s: %v", target.GetClusterId(), err))
			continue
		}

		if status.State != v2pb.INFERENCE_SERVER_STATE_SERVING {
			failures = append(failures, fmt.Sprintf("%s: state %s", target.GetClusterId(), status.State))
		}
	}

	if len(failures) > 0 {
		return conditionUtils.GenerateFalseCondition(condition, "BackendProvisioningFailed", strings.Join(failures, "; ")), nil
	}
	return conditionUtils.GenerateTrueCondition(condition), nil
}

// Run creates the Kubernetes deployment, service, and related resources for inference servers.
func (a *BackendProvisionActor) Run(ctx context.Context, resource *v2pb.InferenceServer, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Running backend provisioning")

	backend, err := a.registry.GetBackend(resource.Spec.BackendType)
	if err != nil {
		return conditionUtils.GenerateFalseCondition(condition, "BackendNotFound", fmt.Sprintf("Failed to get backend: %v", err)), nil
	}

	isDone := func(ctx context.Context, kubeClient client.Client, target *v2pb.ClusterTarget) (bool, error) {
		status, err := backend.GetServerStatus(ctx, a.logger, kubeClient, resource.Name, resource.Namespace)
		if err != nil {
			return false, err
		}
		return status.State == v2pb.INFERENCE_SERVER_STATE_SERVING, nil
	}
	doWork := func(ctx context.Context, kubeClient client.Client, target *v2pb.ClusterTarget) error {
		_, err := backend.CreateServer(ctx, a.logger, kubeClient, resource)
		if err != nil {
			a.logger.Error("Failed to create backend",
				zap.Error(err),
				zap.String("operation", "create_backend"),
				zap.String("namespace", resource.Namespace),
				zap.String("inferenceServer", resource.Name),
				zap.String("cluster_id", target.GetClusterId()))
		}
		return err
	}
	return common.RunRolling(ctx, a.clientFactory, resource.Spec.ClusterTargets, condition, isDone, doWork)
}
