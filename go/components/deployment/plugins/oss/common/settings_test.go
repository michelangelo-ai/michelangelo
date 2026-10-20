package common

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	maconfig "github.com/michelangelo-ai/michelangelo/go/base/config"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

func TestResolveRolloutSettings(t *testing.T) {
	configured := maconfig.DeploymentConfig{
		Rollout:  maconfig.RolloutConfig{ModelLoadTimeout: 5 * time.Minute, SoakPeriod: 2 * time.Minute},
		Rollback: maconfig.RollbackConfig{ModelLoadTimeout: 3 * time.Minute},
	}
	withStrategy := func(strategy *v2pb.DeploymentStrategy) *v2pb.Deployment {
		return &v2pb.Deployment{Spec: v2pb.DeploymentSpec{Strategy: strategy}}
	}

	tests := []struct {
		name       string
		cfg        maconfig.DeploymentConfig
		deployment *v2pb.Deployment
		want       RolloutSettings
	}{
		{
			name:       "empty config falls back to the safe defaults",
			cfg:        maconfig.DeploymentConfig{},
			deployment: &v2pb.Deployment{},
			want: RolloutSettings{
				Canary:           true,
				ModelLoadTimeout: DefaultModelLoadTimeout,
				SoakPeriod:       0,
				RollbackTimeout:  DefaultRollbackTimeout,
			},
		},
		{
			name:       "configured values are used as is",
			cfg:        configured,
			deployment: &v2pb.Deployment{},
			want: RolloutSettings{
				Canary:           true,
				ModelLoadTimeout: 5 * time.Minute,
				SoakPeriod:       2 * time.Minute,
				RollbackTimeout:  3 * time.Minute,
			},
		},
		{
			name:       "skipCanary turns the canary off",
			cfg:        maconfig.DeploymentConfig{Rollout: maconfig.RolloutConfig{SkipCanary: true}},
			deployment: &v2pb.Deployment{},
			want: RolloutSettings{
				Canary:           false,
				ModelLoadTimeout: DefaultModelLoadTimeout,
				RollbackTimeout:  DefaultRollbackTimeout,
			},
		},
		{
			name: "zonal rollout period becomes the per-cluster soak",
			cfg:  configured,
			deployment: withStrategy(&v2pb.DeploymentStrategy{
				RolloutStrategy: &v2pb.DeploymentStrategy_Zonal{Zonal: &v2pb.ZonalUpdate{RolloutPeriodInSeconds: 600}},
			}),
			want: RolloutSettings{
				Canary:           true,
				ModelLoadTimeout: 5 * time.Minute,
				SoakPeriod:       10 * time.Minute,
				RollbackTimeout:  3 * time.Minute,
			},
		},
		{
			name: "zonal without a period keeps the configured soak",
			cfg:  configured,
			deployment: withStrategy(&v2pb.DeploymentStrategy{
				RolloutStrategy: &v2pb.DeploymentStrategy_Zonal{Zonal: &v2pb.ZonalUpdate{}},
			}),
			want: RolloutSettings{
				Canary:           true,
				ModelLoadTimeout: 5 * time.Minute,
				SoakPeriod:       2 * time.Minute,
				RollbackTimeout:  3 * time.Minute,
			},
		},
		{
			name: "blast skips the canary and the soak but keeps the load budgets",
			cfg:  configured,
			deployment: withStrategy(&v2pb.DeploymentStrategy{
				RolloutStrategy: &v2pb.DeploymentStrategy_Blast{Blast: &v2pb.BlastUpdate{}},
			}),
			want: RolloutSettings{
				Canary:           false,
				ModelLoadTimeout: 5 * time.Minute,
				SoakPeriod:       0,
				RollbackTimeout:  3 * time.Minute,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ResolveRolloutSettings(tt.cfg, tt.deployment))
		})
	}
}
