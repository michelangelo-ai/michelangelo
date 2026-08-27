package strategies

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	osscommon "github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// deploymentWithTargets builds a Deployment carrying the cluster snapshot annotation that
// PlacementPrepActor writes, since that annotation is what the actor chain is built from.
func deploymentWithTargets(t *testing.T, clusterIDs ...string) *v2pb.Deployment {
	t.Helper()
	deployment := &v2pb.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "dep", Namespace: "default"}}
	if len(clusterIDs) == 0 {
		return deployment
	}

	targets := make([]*v2pb.ClusterTarget, 0, len(clusterIDs))
	for _, id := range clusterIDs {
		targets = append(targets, &v2pb.ClusterTarget{ClusterId: id})
	}
	require.NoError(t, osscommon.WriteTargetClustersAnnotation(deployment, targets))
	return deployment
}

func actorTypes(actors []conditionInterfaces.ConditionActor[*v2pb.Deployment]) []string {
	types := make([]string, 0, len(actors))
	for _, actor := range actors {
		types = append(types, actor.GetType())
	}
	return types
}

func TestGetActorsForStrategy(t *testing.T) {
	tests := []struct {
		name       string
		deployment *v2pb.Deployment
		want       []string
	}{
		{
			// PlacementPrep has not written the snapshot yet, so no cluster is known and
			// the per-cluster actors only appear on a later reconcile.
			name:       "no cluster snapshot yet",
			deployment: deploymentWithTargets(t),
			want:       []string{},
		},
		{
			name:       "rollout and traffic are interleaved per cluster",
			deployment: deploymentWithTargets(t, "c1"),
			want: []string{
				"RollingRolloutComplete-c1",
				"TrafficRoutingConfigured-c1",
				"DiscoveryRoutingConfigured",
				"ModelCleanupComplete-c1",
			},
		},
		{
			// Cleanup trails every cluster's rollout so the old model stays loaded until
			// the last cluster has flipped to the new one.
			name:       "cleanup for all clusters follows the last rollout",
			deployment: deploymentWithTargets(t, "c1", "c2"),
			want: []string{
				"RollingRolloutComplete-c1",
				"TrafficRoutingConfigured-c1",
				"RollingRolloutComplete-c2",
				"TrafficRoutingConfigured-c2",
				"DiscoveryRoutingConfigured",
				"ModelCleanupComplete-c1",
				"ModelCleanupComplete-c2",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actors, err := GetActorsForStrategy(context.Background(), Params{Logger: zap.NewNop()}, tt.deployment)

			require.NoError(t, err)
			assert.Equal(t, tt.want, actorTypes(actors))
		})
	}
}

func TestGetActorsForStrategyUnreadableSnapshot(t *testing.T) {
	deployment := &v2pb.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dep",
			Namespace:   "default",
			Annotations: map[string]string{osscommon.TargetClustersAnnotation: "{not json}"},
		},
	}

	_, err := GetActorsForStrategy(context.Background(), Params{Logger: zap.NewNop()}, deployment)

	require.Error(t, err)
}
