package cluster

import (
	"go.uber.org/fx"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	rayv1 "github.com/ray-project/kuberay/ray-operator/pkg/client/clientset/versioned/typed/ray/v1"

	"github.com/michelangelo-ai/michelangelo/go/base/env"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/scheduler"
)

// Module FX
var Module = fx.Options(
	fx.Provide(newConfig),
	fx.Invoke(register),
)

func register(
	conf Config,
	env env.Context,
	mgr manager.Manager,
	jobQueue scheduler.JobQueue,
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
		jobQueue:       jobQueue,
	}).Register(mgr)
}
