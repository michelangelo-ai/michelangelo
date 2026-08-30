package job

import (
	"time"

	"github.com/go-logr/logr"
	"go.uber.org/fx"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/michelangelo-ai/michelangelo/go/base/env"
	jobsclient "github.com/michelangelo-ai/michelangelo/go/components/jobs/client"
	jobsCluster "github.com/michelangelo-ai/michelangelo/go/components/jobs/cluster"
	"github.com/michelangelo-ai/michelangelo/go/components/ray/cluster"
)

// Module FX
var Module = fx.Options(
	fx.Provide(newConfig),
	fx.Invoke(register),
)

func register(
	conf cluster.Config,
	jobConf Config,
	logger logr.Logger,
	env env.Context,
	mgr manager.Manager,
	federatedClient jobsclient.FederatedClient,
	clusterCache jobsCluster.RegisteredClustersCache,
) error {
	restConfig := mgr.GetConfig()
	restConfig.QPS = conf.QPS
	restConfig.Burst = conf.Burst

	// A non-positive duration (unset config) falls back to the controller
	// default inside the reconciler (effectiveFinishedJobTTL).
	return NewReconciler(
		logger,
		mgr.GetClient(),
		env,
		federatedClient,
		clusterCache,
		time.Duration(jobConf.FinishedJobTtlSeconds)*time.Second,
	).Register(mgr)
}
