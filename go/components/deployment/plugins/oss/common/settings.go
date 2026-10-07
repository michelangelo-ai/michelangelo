package common

import (
	"time"

	maconfig "github.com/michelangelo-ai/michelangelo/go/base/config"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

const (
	// DefaultModelLoadTimeout bounds a cluster's model load when the config sets no budget.
	DefaultModelLoadTimeout = 15 * time.Minute
	// DefaultRollbackTimeout bounds a rollback's wait for the previous model when the config
	// sets no budget.
	DefaultRollbackTimeout = 15 * time.Minute
)

// RolloutSettings are the knobs the per-cluster actor chain is built with, resolved from the
// controller config and the deployment's strategy.
type RolloutSettings struct {
	// Canary loads and validates the new model on one replica per cluster before the rest
	// of the cluster loads it.
	Canary bool
	// ModelLoadTimeout bounds how long a cluster may take to load the model on every replica.
	ModelLoadTimeout time.Duration
	// SoakPeriod is how long a cluster keeps serving the new model, with the health and
	// metric gates active, before the rollout moves to the next cluster. Zero skips soaking.
	SoakPeriod time.Duration
	// RollbackTimeout bounds how long a rollback waits for the previous model to be loaded
	// again before restoring traffic to it.
	RollbackTimeout time.Duration
}

// ResolveRolloutSettings applies the deployment's strategy on top of the controller config.
// Clusters play the role zones play in the Zonal strategy, so its rollout period becomes
// the per-cluster soak. Blast is the emergency path: no canary and no soak.
func ResolveRolloutSettings(cfg maconfig.DeploymentConfig, deployment *v2pb.Deployment) RolloutSettings {
	settings := RolloutSettings{
		Canary:           !cfg.Rollout.SkipCanary,
		ModelLoadTimeout: cfg.Rollout.ModelLoadTimeout,
		SoakPeriod:       cfg.Rollout.SoakPeriod,
		RollbackTimeout:  cfg.Rollback.ModelLoadTimeout,
	}
	if settings.ModelLoadTimeout <= 0 {
		settings.ModelLoadTimeout = DefaultModelLoadTimeout
	}
	if settings.RollbackTimeout <= 0 {
		settings.RollbackTimeout = DefaultRollbackTimeout
	}

	switch strategy := deployment.Spec.GetStrategy().GetRolloutStrategy().(type) {
	case *v2pb.DeploymentStrategy_Zonal:
		if period := strategy.Zonal.GetRolloutPeriodInSeconds(); period > 0 {
			settings.SoakPeriod = time.Duration(period * float64(time.Second))
		}
	case *v2pb.DeploymentStrategy_Blast:
		settings.Canary = false
		settings.SoakPeriod = 0
	}
	return settings
}
