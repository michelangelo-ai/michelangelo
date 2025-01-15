package rayjob

import (
	"go.uber.org/fx"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	e "github.com/michelangelo-ai/michelangelo/go/base/env"
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
	mgr manager.Manager,
) error {
	rayClient, _ := rayv1.NewForConfig(mgr.GetConfig())
	return (&Reconciler{
		Client:      mgr.GetClient(),
		env:         e.New().Environment,
		rayV1Client: rayClient,
	}).Register(mgr)
}
