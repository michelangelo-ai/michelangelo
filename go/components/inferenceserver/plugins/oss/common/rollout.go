package common

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"

	conditionsutil "github.com/michelangelo-ai/michelangelo/go/base/conditions/utils"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/clientfactory"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

const (
	// ClusterRolloutStrategyAnnotation is the annotation key for specifying the per-cluster rollout strategy.
	ClusterRolloutStrategyAnnotation = "michelangelo.ai/cluster-rollout-strategy"
	rollingStrategy                  = "rolling"
)

// GetRolloutStrategy reads the rollout strategy annotation. Defaults to "rolling" when absent.
func GetRolloutStrategy(resource *v2pb.InferenceServer) string {
	if anns := resource.GetMetadata().GetAnnotations(); anns != nil {
		if v, ok := anns[ClusterRolloutStrategyAnnotation]; ok {
			return v
		}
	}
	return rollingStrategy
}

// IsKnownRolloutStrategy reports whether strategy is a recognized rollout strategy value.
func IsKnownRolloutStrategy(strategy string) bool {
	return strategy == rollingStrategy
}

// RunRolling iterates cluster_targets in spec order and calls doWork on the first cluster
// that isDone returns false for. Returns TRUE once all clusters are done.
func RunRolling(
	ctx context.Context,
	factory clientfactory.ClientFactory,
	targets []*v2pb.ClusterTarget,
	condition *apipb.Condition,
	isDone func(ctx context.Context, kubeClient client.Client, target *v2pb.ClusterTarget) (bool, error),
	doWork func(ctx context.Context, kubeClient client.Client, target *v2pb.ClusterTarget) error,
) (*apipb.Condition, error) {
	for _, target := range targets {
		kubeClient, err := factory.GetClient(ctx, target)
		if err != nil {
			return conditionsutil.GenerateFalseCondition(condition, "ClientError",
				fmt.Sprintf("%s: %v", target.GetClusterId(), err)), nil
		}
		done, err := isDone(ctx, kubeClient, target)
		if err != nil {
			return conditionsutil.GenerateFalseCondition(condition, "StatusCheckFailed",
				fmt.Sprintf("%s: %v", target.GetClusterId(), err)), nil
		}
		if done {
			continue
		}
		if err := doWork(ctx, kubeClient, target); err != nil {
			return conditionsutil.GenerateFalseCondition(condition, "ProvisionFailed",
				fmt.Sprintf("%s: %v", target.GetClusterId(), err)), nil
		}
		return conditionsutil.GenerateFalseCondition(condition, "RollingInProgress",
			fmt.Sprintf("provisioning cluster %s", target.GetClusterId())), nil
	}
	return conditionsutil.GenerateTrueCondition(condition), nil
}
