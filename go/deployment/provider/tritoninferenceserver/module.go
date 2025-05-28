package tritoninferenceserver

import (
	"go.uber.org/fx"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/michelangelo-ai/michelangelo/go/base/env"
	"github.com/michelangelo-ai/michelangelo/go/deployment/provider"
)

var (
	// Module FX
	Module = fx.Options(
		fx.Provide(register),
	)
)

func register(
	env env.Context,
	mgr manager.Manager,
) provider.Provider {

	config := mgr.GetConfig()

	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		panic("failed to create dynamic client: " + err.Error())
	}

	return &TritonProvider{
		DynamicClient: dynamicClient,
	}
}
