package endpoints

import (
	"go.uber.org/config"
	"go.uber.org/fx"
	"sigs.k8s.io/controller-runtime/pkg/client"

	maconfig "github.com/michelangelo-ai/michelangelo/go/base/config"
)

// Module wires the EndpointPublisher with the in-cluster Kubernetes client and
// the InferenceServerConfig from the typed config provider. The
// EndpointSource is provided separately by an environment-specific module
// (for example, source.Module for k3d-style clusters).
var Module = fx.Options(
	fx.Provide(newDefaultPublisher),
	fx.Provide(newInferenceServerConfig),
)

func newDefaultPublisher(kubeClient client.Client) EndpointPublisher {
	return NewDefaultPublisher(kubeClient, kubeClient.Scheme())
}

func newInferenceServerConfig(provider config.Provider) (maconfig.InferenceServerConfig, error) {
	return maconfig.GetInferenceServerConfig(provider)
}
