package job

import (
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
	sparkClient Client,
) error {
	// Create SparkApplication client
	return (&Reconciler{
		Client:      mgr.GetClient(),
		SparkClient: sparkClient,
		env:         env,
	}).Register(mgr)
}
