package configmap

import (
	"go.uber.org/fx"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Module provides the configmap module
var Module = fx.Options(
	fx.Provide(newModelConfigMapProvider),
)

// newModelConfigMapProvider creates a new model config map provider
func newModelConfigMapProvider(client client.Client, logger *zap.Logger) ModelConfigMapProvider {
	return NewDefaultModelConfigMapProvider(client, logger)
}
