package sparkjob

import (
	"github.com/michelangelo-ai/michelangelo/go/controllermgr"
	rayv1 "github.com/ray-project/kuberay/ray-operator/pkg/client/clientset/versioned/typed/ray/v1"
	"go.uber.org/fx"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/michelangelo-ai/michelangelo/go/base/env"
)

var (
	// Module FX
	Module = fx.Options(
		fx.Invoke(register),
	)
)

func register(
	conf controllermgr.Config,
	env env.Context,
	mgr manager.Manager,
) error {
	restConfig := mgr.GetConfig()
	restConfig.QPS = conf.QPS
	restConfig.Burst = conf.Burst
	rayClient, err := rayv1.NewForConfig(restConfig)
	if err != nil {
		return err
	}
	return (&Reconciler{
		Client:         mgr.GetClient(),
		env:            env,
		RayV1Interface: rayClient,
	}).Register(mgr)
}
