package raycluster

import (
	"github.com/michelangelo-ai/michelangelo/go/api"
	"go.uber.org/fx"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/michelangelo-ai/michelangelo/go/base/env"
	rayv1 "github.com/ray-project/kuberay/ray-operator/pkg/client/clientset/versioned/typed/ray/v1"
)

var (
	// Module FX
	Module = fx.Options(
		fx.Provide(newConfig),
		fx.Invoke(register),
	)
)

func register(
	conf Config,
	env env.Context,
	mgr manager.Manager,
	handler api.Handler,
) error {
	restConfig := mgr.GetConfig()
	restConfig.QPS = conf.QPS
	restConfig.Burst = conf.Burst
	rayClient, err := rayv1.NewForConfig(restConfig)
	if err != nil {
		return err
	}
	return (&Reconciler{
		Handler:        handler,
		env:            env,
		RayV1Interface: rayClient,
	}).Register(mgr)
}
