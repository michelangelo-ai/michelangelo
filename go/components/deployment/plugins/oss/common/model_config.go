package common

import (
	"context"

	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/modelconfig"
)

// CheckModelExists reports whether the named deployment has an entry for a model.
// Entries belonging to other deployments are ignored.
func CheckModelExists(ctx context.Context, logger *zap.Logger, modelConfigProvider modelconfig.ModelConfigProvider, kubeclient client.Client, deploymentName string, modelName string, inferenceServerName string, namespace string) (bool, error) {
	models, err := modelConfigProvider.GetModelsFromConfig(ctx, logger, kubeclient, inferenceServerName, namespace)
	if err != nil {
		return false, err
	}
	for _, model := range models {
		if model.Name == modelName && model.DeploymentName == deploymentName {
			return true, nil
		}
	}
	return false, nil
}
