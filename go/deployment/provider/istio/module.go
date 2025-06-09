package istio

import (
	"github.com/michelangelo-ai/michelangelo/go/base/env"
	"github.com/michelangelo-ai/michelangelo/go/deployment/provider/proxy"
	"go.uber.org/fx"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// Module provides the Istio proxy provider
var Module = fx.Module("istio",
	fx.Provide(NewIstioProvider),
)

func NewIstioProvider(
	env env.Context,
	mgr manager.Manager,
) proxy.ProxyProvider {

	config := mgr.GetConfig()

	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		panic("failed to create dynamic client: " + err.Error())
	}
	return &IstioProvider{
		DynamicClient: dynamicClient,
	}
}
