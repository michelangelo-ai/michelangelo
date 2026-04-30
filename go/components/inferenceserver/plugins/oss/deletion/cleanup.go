package deletion

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/client"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	conditionsUtil "github.com/michelangelo-ai/michelangelo/go/base/conditions/utils"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/clientfactory"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/modelconfig"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/plugins/oss/common"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

var _ conditionInterfaces.ConditionActor[*v2pb.InferenceServer] = &CleanupActor{}

// CleanupActor removes all Kubernetes resources associated with an inference server.
type CleanupActor struct {
	clientFactory       clientfactory.ClientFactory
	registry            *backends.Registry
	modelConfigProvider modelconfig.ModelConfigProvider
	logger              *zap.Logger
}

// NewCleanupActor creates a condition actor for inference server cleanup during deletion.
func NewCleanupActor(clientFactory clientfactory.ClientFactory, registry *backends.Registry, modelConfigProvider modelconfig.ModelConfigProvider, logger *zap.Logger) conditionInterfaces.ConditionActor[*v2pb.InferenceServer] {
	return &CleanupActor{
		clientFactory:       clientFactory,
		registry:            registry,
		modelConfigProvider: modelConfigProvider,
		logger:              logger,
	}
}

// GetType returns the condition type identifier for cleanup.
func (a *CleanupActor) GetType() string {
	return common.CleanupConditionType
}

// Retrieve checks if all inference server has been successfully deleted.
func (a *CleanupActor) Retrieve(ctx context.Context, resource *v2pb.InferenceServer, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Retrieving inference server cleanup condition")

	backend, err := a.registry.GetBackend(resource.Spec.BackendType)
	if err != nil {
		// If backend is not found, then consider cleanup is complete
		return conditionsUtil.GenerateTrueCondition(condition), nil
	}

	var failures []string
	for _, target := range resource.Spec.ClusterTargets {
		kubeClient, err := a.clientFactory.GetClient(ctx, target)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: client error: %v", target.GetClusterId(), err))
			continue
		}

		// Check if inference server still exists
		_, err = backend.GetServerStatus(ctx, a.logger, kubeClient, resource.Name, resource.Namespace)
		if err == nil {
			failures = append(failures, fmt.Sprintf("%s: still present", target.GetClusterId()))
		}
	}

	if len(failures) > 0 {
		return conditionsUtil.GenerateFalseCondition(condition, "CleanupInProgress", strings.Join(failures, "; ")), nil
	}
	return conditionsUtil.GenerateTrueCondition(condition), nil
}

// Run deletes the deployment, service, ConfigMaps for the inference server.
func (a *CleanupActor) Run(ctx context.Context, resource *v2pb.InferenceServer, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Running inference server cleanup with ConfigMap cleanup")

	// Get backend from registry
	backend, err := a.registry.GetBackend(resource.Spec.BackendType)
	if err != nil {
		// If backend is not found, then consider cleanup is complete
		return conditionsUtil.GenerateTrueCondition(condition), nil
	}

	isDone := func(ctx context.Context, kubeClient client.Client, target *v2pb.ClusterTarget) (bool, error) {
		_, err := backend.GetServerStatus(ctx, a.logger, kubeClient, resource.Name, resource.Namespace)
		return err != nil, nil // resource gone (error) means done
	}
	doWork := func(ctx context.Context, kubeClient client.Client, target *v2pb.ClusterTarget) error {
		// Delete Model Config first (preserving existing ordering)
		a.logger.Info("Cleaning up Model Config for inference server",
			zap.String("namespace", resource.Namespace),
			zap.String("inferenceServer", resource.Name),
			zap.String("cluster_id", target.GetClusterId()))
		if mcErr := a.modelConfigProvider.DeleteModelConfig(ctx, a.logger, kubeClient, resource.Name, resource.Namespace); mcErr != nil {
			a.logger.Error("Failed to delete Model Config",
				zap.Error(mcErr),
				zap.String("operation", "delete_modelconfig"),
				zap.String("namespace", resource.Namespace),
				zap.String("inferenceServer", resource.Name),
				zap.String("cluster_id", target.GetClusterId()),
			)
		} else {
			a.logger.Info("Successfully deleted Model Config for inference server",
				zap.String("namespace", resource.Namespace),
				zap.String("inferenceServer", resource.Name),
				zap.String("cluster_id", target.GetClusterId()))
		}
		a.logger.Info("Cleaning up inference server",
			zap.String("namespace", resource.Namespace),
			zap.String("inferenceServer", resource.Name),
			zap.String("cluster_id", target.GetClusterId()))
		if err := backend.DeleteServer(ctx, a.logger, kubeClient, resource.Name, resource.Namespace); err != nil {
			return err
		}
		a.logger.Info("Inference server cleanup completed successfully",
			zap.String("namespace", resource.Namespace),
			zap.String("inferenceServer", resource.Name),
			zap.String("cluster_id", target.GetClusterId()))
		return nil
	}
	return common.RunRolling(ctx, a.clientFactory, resource.Spec.ClusterTargets, condition, isDone, doWork)
}
