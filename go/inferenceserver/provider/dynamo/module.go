package dynamo

import (
	"github.com/michelangelo-ai/michelangelo/go/inferenceserver/provider/serving"
	"go.uber.org/fx"
	"k8s.io/client-go/dynamic"
)

// Module provides the Dynamo inference server provider
var Module = fx.Module("dynamo",
	fx.Provide(
		NewDynamoInferenceServerProvider,
	),
)

// NewDynamoInferenceServerProvider creates a new instance of DynamoInferenceServerProvider
func NewDynamoInferenceServerProvider(dynamicClient dynamic.Interface) serving.Provider {
	config := &DynamoConfig{
		Backend:         "vllm",           // Default to vLLM backend
		ImageRegistry:   "docker.io",      // Default registry
		DefaultReplicas: 1,                // Default replicas for workers
		ComponentTag:    "latest",         // Default component tag
	}

	return &DynamoInferenceServerProvider{
		DynamicClient: dynamicClient,
		Config:        config,
	}
}

// NewDynamoInferenceServerProviderWithConfig creates a new instance with custom config
func NewDynamoInferenceServerProviderWithConfig(dynamicClient dynamic.Interface, config *DynamoConfig) serving.Provider {
	if config == nil {
		config = &DynamoConfig{
			Backend:         "vllm",
			ImageRegistry:   "docker.io",
			DefaultReplicas: 1,
			ComponentTag:    "latest",
		}
	}

	return &DynamoInferenceServerProvider{
		DynamicClient: dynamicClient,
		Config:        config,
	}
}