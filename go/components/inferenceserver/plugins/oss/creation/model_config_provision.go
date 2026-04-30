package creation

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/client"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	conditionsutil "github.com/michelangelo-ai/michelangelo/go/base/conditions/utils"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/clientfactory"
	modelconfig "github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/modelconfig"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/plugins/oss/common"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

var _ conditionInterfaces.ConditionActor[*v2pb.InferenceServer] = &ModelConfigProvisionActor{}

// ModelConfigProvisionActor provisions model configuration for inference servers.
type ModelConfigProvisionActor struct {
	clientFactory       clientfactory.ClientFactory
	modelConfigProvider modelconfig.ModelConfigProvider
	logger              *zap.Logger
}

func NewModelConfigProvisionActor(clientFactory clientfactory.ClientFactory, modelConfigProvider modelconfig.ModelConfigProvider, logger *zap.Logger) conditionInterfaces.ConditionActor[*v2pb.InferenceServer] {
	return &ModelConfigProvisionActor{
		clientFactory:       clientFactory,
		modelConfigProvider: modelConfigProvider,
		logger:              logger,
	}
}

func (a *ModelConfigProvisionActor) GetType() string {
	return common.ModelConfigProvisionConditionType
}

func (a *ModelConfigProvisionActor) Retrieve(ctx context.Context, resource *v2pb.InferenceServer, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Retrieving model config provisioning condition")

	var failures []string
	for _, target := range resource.Spec.ClusterTargets {
		kubeClient, err := a.clientFactory.GetClient(ctx, target)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: client error: %v", target.GetClusterId(), err))
			continue
		}

		exists, err := a.modelConfigProvider.CheckModelConfigExists(ctx, a.logger, kubeClient, resource.Name, resource.Namespace)
		if err != nil {
			a.logger.Error("Failed to check model config existence", zap.Error(err), zap.String("cluster_id", target.GetClusterId()))
			failures = append(failures, fmt.Sprintf("%s: %v", target.GetClusterId(), err))
			continue
		}

		if !exists {
			failures = append(failures, fmt.Sprintf("%s: model config not found", target.GetClusterId()))
		}
	}

	if len(failures) > 0 {
		return conditionsutil.GenerateFalseCondition(condition, "ModelConfigNotFound", strings.Join(failures, "; ")), nil
	}
	return conditionsutil.GenerateTrueCondition(condition), nil
}

func (a *ModelConfigProvisionActor) Run(ctx context.Context, resource *v2pb.InferenceServer, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Running model config provisioning")

	isDone := func(ctx context.Context, kubeClient client.Client, target *v2pb.ClusterTarget) (bool, error) {
		return a.modelConfigProvider.CheckModelConfigExists(ctx, a.logger, kubeClient, resource.Name, resource.Namespace)
	}
	doWork := func(ctx context.Context, kubeClient client.Client, target *v2pb.ClusterTarget) error {
		err := a.modelConfigProvider.CreateModelConfig(ctx, a.logger, kubeClient, resource.Name, resource.Namespace, map[string]string{}, map[string]string{})
		if err != nil {
			a.logger.Error("Failed to create model config",
				zap.Error(err),
				zap.String("operation", "create_modelconfig"),
				zap.String("namespace", resource.Namespace),
				zap.String("inferenceServer", resource.Name),
				zap.String("cluster_id", target.GetClusterId()))
		}
		return err
	}
	return common.RunRolling(ctx, a.clientFactory, resource.Spec.ClusterTargets, condition, isDone, doWork)
}
