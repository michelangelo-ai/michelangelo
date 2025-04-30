package cluster

import (
	"go.uber.org/fx"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/michelangelo-ai/michelangelo/go/base/env"
	rayv1 "github.com/ray-project/kuberay/ray-operator/pkg/client/clientset/versioned/typed/ray/v1"
	"k8s.io/client-go/rest"

)

var (
	// Module FX
	Module = fx.Options(
		fx.Invoke(register),
	)
)

func register(
	restConfig *rest.Config,
	env env.Context,
	mgr manager.Manager,
) error {
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
