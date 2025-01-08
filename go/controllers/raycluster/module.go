package raycluster

import (
	"go.uber.org/fx"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/michelangelo-ai/michelangelo/go/base/env"
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
	restClient restclient.Config,
) error {
	clientset, _ := kubernetes.NewForConfig(&restClient)
	client := clientset.RESTClient()
	return (&Reconciler{
		env:           env.New().Environment,
		k8sRestClient: client,
	}).Register(mgr)
}
