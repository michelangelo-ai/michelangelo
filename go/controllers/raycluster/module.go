package raycluster

import (
	e "github.com/michelangelo-ai/michelangelo/go/base/env"

	"go.uber.org/fx"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/manager"
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
	mgr manager.Manager,
) error {
	clientset, _ := kubernetes.NewForConfig(mgr.GetConfig())
	client := clientset.RESTClient()
	return (&Reconciler{
		Client:          mgr.GetClient(),
		env:           e.New().Environment,
		k8sRestClient: client,
	}).Register(mgr)
}
