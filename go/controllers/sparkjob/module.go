package sparkjob

import (
	sparkclientset "github.com/GoogleCloudPlatform/spark-on-k8s-operator/pkg/client/clientset/versioned"
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
	env env.Context,
	mgr manager.Manager,
) error {
	restConfig := mgr.GetConfig()
	// Create SparkApplication client
	sparkClient, err := sparkclientset.NewForConfig(restConfig)
	if err != nil {
		return err
	}
	return (&Reconciler{
		Client:      mgr.GetClient(),
		SparkClient: sparkClient,
		env:         env,
	}).Register(mgr)
}
