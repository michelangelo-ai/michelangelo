package common

import (
	"context"

	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/modelconfig"
)

func CheckModelExists(ctx context.Context, logger *zap.Logger, modelConfigProvider modelconfig.ModelConfigProvider, kubeclient client.Client, modelName string, inferenceServerName string, namespace string) (bool, error) {
	models, err := modelConfigProvider.GetModelsFromConfig(ctx, logger, kubeclient, inferenceServerName, namespace)
	if err != nil {
		return false, err
	}
	for _, model := range models {
		if model.Name == modelName {
			return true, nil
		}
	}
	return false, nil
}
