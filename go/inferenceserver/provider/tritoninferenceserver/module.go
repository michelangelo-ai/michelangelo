package tritoninferenceserver

import (
	"github.com/michelangelo-ai/michelangelo/go/base/env"
	"github.com/michelangelo-ai/michelangelo/go/inferenceserver/provider/serving"
	"go.uber.org/fx"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// Module provides the Triton inference server provider
var Module = fx.Module("tritoninferenceserver",
	fx.Provide(NewTritonInferenceServerProvider),
)

// NewTritonInferenceServerProvider creates a new TritonInferenceServerProvider
func NewTritonInferenceServerProvider(
	env env.Context,
	mgr manager.Manager,
) serving.Provider {

	config := mgr.GetConfig()

	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		panic("failed to create dynamic client: " + err.Error())
	}
	return &TritonInferenceServerProvider{
		DynamicClient: dynamicClient,
	}
}
