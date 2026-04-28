package creation

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"sigs.k8s.io/controller-runtime/pkg/client"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	conditionsutil "github.com/michelangelo-ai/michelangelo/go/base/conditions/utils"
	modelconfig "github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/modelconfig"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/plugins/oss/common"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

var _ conditionInterfaces.ConditionActor[*v2pb.InferenceServer] = &ModelConfigProvisionActor{}

// ModelConfigProvisionActor provisions model configuration for inference servers.
type ModelConfigProvisionActor struct {
	client              client.Client
	modelConfigProvider modelconfig.ModelConfigProvider
	logger              *zap.Logger
}

func NewModelConfigProvisionActor(client client.Client, modelConfigProvider modelconfig.ModelConfigProvider, logger *zap.Logger) conditionInterfaces.ConditionActor[*v2pb.InferenceServer] {
	return &ModelConfigProvisionActor{
		client:              client,
		modelConfigProvider: modelConfigProvider,
		logger:              logger,
	}
}

func (a *ModelConfigProvisionActor) GetType() string {
	return common.ModelConfigProvisionConditionType
}

func (a *ModelConfigProvisionActor) Retrieve(ctx context.Context, resource *v2pb.InferenceServer, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Retrieving model config provisioning condition")

	exists, err := a.modelConfigProvider.CheckModelConfigExists(ctx, a.logger, a.client, resource.Name, resource.Namespace)
	if err != nil {
		a.logger.Error("Failed to check model config existence", zap.Error(err))
		return conditionsutil.GenerateFalseCondition(condition, "ModelConfigProvisionFailed", fmt.Sprintf("Failed to check model config existence: %v", err)), err
	}

	if !exists {
		return conditionsutil.GenerateFalseCondition(condition, "ModelConfigNotFound", "Model config not found"), nil
	}
	return conditionsutil.GenerateTrueCondition(condition), nil
}

func (a *ModelConfigProvisionActor) Run(ctx context.Context, resource *v2pb.InferenceServer, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Running model config provisioning")

	err := a.modelConfigProvider.CreateModelConfig(ctx, a.logger, a.client, resource.Name, resource.Namespace, map[string]string{}, map[string]string{})
	if err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "ModelConfigProvisionFailed", fmt.Sprintf("Failed to create config map: %v", err)), err
	}
	return conditionsutil.GenerateTrueCondition(condition), nil
}
