package rollout

import (
	"context"

	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/client"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	conditionsutil "github.com/michelangelo-ai/michelangelo/go/base/conditions/utils"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

var _ conditionInterfaces.ConditionActor[*v2pb.Deployment] = &PlacementPrepActor{}

// PlacementPrepActor snapshots the InferenceServer's currently-serving cluster targets onto the
// Deployment annotation. Only clusters whose IS status is SERVING are included, so downstream
// actors never attempt placement in a cluster that is not yet ready. Retrieve detects changes
// to the healthy set and marks the condition false so the engine reruns Run to refresh the snapshot.
type PlacementPrepActor struct {
	kubeClient client.Client
	logger     *zap.Logger
}

// GetType returns the condition type identifier for placement preparation.
func (a *PlacementPrepActor) GetType() string {
	return common.ActorTypePlacementPrep
}

// Retrieve checks whether the set of serving clusters has changed since the last snapshot.
// Returns false if a cluster has recovered or degraded since Run last wrote the annotation,
// causing the engine to rerun Run and refresh the snapshot.
func (a *PlacementPrepActor) Retrieve(ctx context.Context, deployment *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	snapshot, err := common.ReadTargetClustersAnnotation(deployment)
	if err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "AnnotationReadFailed", err.Error()), nil
	}
	if snapshot == nil {
		return conditionsutil.GenerateFalseCondition(condition, "SnapshotMissing", "target-clusters annotation is absent"), nil
	}

	inferenceServer, err := common.FetchInferenceServer(ctx, a.kubeClient, deployment)
	if err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "InferenceServerNotFound", err.Error()), nil
	}

	healthy := healthyTargets(inferenceServer)
	if !common.ClusterTargetsEqual(healthy, snapshot) {
		return conditionsutil.GenerateFalseCondition(condition, "ClusterSetChanged", "serving cluster set has changed since last snapshot"), nil
	}

	return conditionsutil.GenerateTrueCondition(condition), nil
}

// Run fetches the InferenceServer, filters its ClusterTargets to those in SERVING state, and
// writes the result to the Deployment annotation. On the first write (annotation previously
// absent) it returns UNKNOWN so the engine requeues and the next reconcile rebuilds the
// per-cluster actor chain from the new annotation. On subsequent calls (drift refresh after a
// cluster set change) it returns TRUE because the actor chain is already correctly built.
// Returns UNKNOWN without writing when no cluster is serving yet.
func (a *PlacementPrepActor) Run(ctx context.Context, deployment *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	existing, _ := common.ReadTargetClustersAnnotation(deployment)

	inferenceServer, err := common.FetchInferenceServer(ctx, a.kubeClient, deployment)
	if err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "InferenceServerNotFound", err.Error()), err
	}

	healthy := healthyTargets(inferenceServer)
	if len(healthy) == 0 {
		return conditionsutil.GenerateUnknownCondition(condition, "NoClusterServing", "no cluster has reached serving state"), nil
	}

	if err := common.WriteTargetClustersAnnotation(deployment, healthy); err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "AnnotationWriteFailed", err.Error()), err
	}

	if existing == nil {
		return conditionsutil.GenerateUnknownCondition(condition, "AnnotationWritten", "cluster snapshot written; actor chain rebuilds on next reconcile"), nil
	}
	return conditionsutil.GenerateTrueCondition(condition), nil
}

// healthyTargets returns the ClusterTargets from the IS spec whose per-cluster status is SERVING.
// Connection info comes from the spec; readiness state comes from the status.
func healthyTargets(is *v2pb.InferenceServer) []*v2pb.ClusterTarget {
	serving := make(map[string]bool, len(is.Status.GetClusterStatuses()))
	for _, cs := range is.Status.GetClusterStatuses() {
		if cs.GetState() == v2pb.INFERENCE_SERVER_STATE_SERVING {
			serving[cs.GetClusterId()] = true
		}
	}

	var targets []*v2pb.ClusterTarget
	for _, ct := range is.Spec.GetClusterTargets() {
		if serving[ct.GetClusterId()] {
			targets = append(targets, ct)
		}
	}
	return targets
}
