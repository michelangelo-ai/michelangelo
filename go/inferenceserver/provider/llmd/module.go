package llmd

import (
	"github.com/michelangelo-ai/michelangelo/go/base/env"
	"github.com/michelangelo-ai/michelangelo/go/inferenceserver/provider/serving"
	"go.uber.org/fx"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// Module provides the LLM-D inference server provider
var Module = fx.Module("llmd",
	fx.Provide(fx.Annotate(NewLLMDProvider, fx.ResultTags(`name:"llmd"`))),
)

// NewLLMDProvider creates a new LLMDProvider
func NewLLMDProvider(
	env env.Context,
	mgr manager.Manager,
) serving.Provider {

	config := mgr.GetConfig()

	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		panic("failed to create dynamic client: " + err.Error())
	}
	return &LLMDProvider{
		DynamicClient: dynamicClient,
	}
}