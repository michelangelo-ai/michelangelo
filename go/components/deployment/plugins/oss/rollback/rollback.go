package rollback

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	conditionsutil "github.com/michelangelo-ai/michelangelo/go/base/conditions/utils"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/gateways"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

var _ conditionInterfaces.ConditionActor[*v2pb.Deployment] = &RollbackActor{}

// RollbackActor restores deployment to the previous stable revision when rollout fails.
type RollbackActor struct {
	logger  *zap.Logger
	gateway gateways.Gateway
}

// GetType returns the condition type identifier for rollback.
func (a *RollbackActor) GetType() string {
	return common.ActorTypeRollback
}

// Retrieve checks if rollback is complete by verifying whether CandidateRevision still exists on each cluster.
func (a *RollbackActor) Retrieve(ctx context.Context, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	candidateModel := resource.Status.CandidateRevision.GetName()
	if candidateModel == "" {
		return conditionsutil.GenerateTrueCondition(condition), nil
	}

	metadata := common.GetClusterMetadata(condition)
	if metadata == nil {
		a.logger.Info("No rollback metadata found, triggering Run to initialize")
		return conditionsutil.GenerateFalseCondition(condition, "RollbackNotStarted",
			"Rollback has not started"), nil
	}

	inferenceServerName := resource.Spec.GetInferenceServer().GetName()
	a.logger.Info("Checking rollback status",
		zap.String("candidate_model", candidateModel),
		zap.String("inference_server", inferenceServerName))

	// Find current cluster to rollback (first non-CLEANED)
	currentIdx := -1
	for i, cluster := range metadata.Clusters {
		if cluster.State != common.ClusterStateRolledBack {
			currentIdx = i
			break
		}
	}

	if currentIdx == -1 {
		a.logger.Info("All clusters rolled back successfully",
			zap.Int("total_clusters", len(metadata.Clusters)))
		return conditionsutil.GenerateTrueCondition(condition), nil
	}

	// Update CurrentIndex so Run knows which cluster to rollback
	if metadata.CurrentIndex != currentIdx {
		metadata.CurrentIndex = currentIdx
		if err := common.SetClusterMetadata(condition, metadata); err != nil {
			return nil, fmt.Errorf("failed to update current index: %w", err)
		}
	}

	currentCluster := &metadata.Clusters[currentIdx]
	a.logger.Info("Checking rollback status for cluster",
		zap.String("cluster_id", currentCluster.ClusterId),
		zap.String("state", currentCluster.State),
		zap.Int("cluster_index", currentIdx),
		zap.Int("total_clusters", len(metadata.Clusters)))

	// If PENDING, trigger Run to start rollback
	if currentCluster.State == common.ClusterStatePending {
		return conditionsutil.GenerateFalseCondition(condition, "RollbackPending",
			fmt.Sprintf("Cluster %s is pending rollback", currentCluster.ClusterId)), nil
	}

	// If ROLLBACK_IN_PROGRESS, check if candidate model still exists
	if currentCluster.State == common.ClusterStateRollbackInProgress {
		clusterTarget := common.GetClusterTargetConnection(currentCluster)
		backendType := v2pb.BackendType(v2pb.BackendType_value[metadata.BackendType])

		exists, err := a.gateway.CheckModelExists(
			ctx, a.logger, candidateModel, inferenceServerName, resource.Namespace, clusterTarget, backendType,
		)
		if err != nil {
			a.logger.Warn("Failed to check model existence during rollback, will retry",
				zap.String("cluster_id", currentCluster.ClusterId),
				zap.String("model", candidateModel),
				zap.Error(err))
			return conditionsutil.GenerateUnknownCondition(condition, "RollbackStatusCheckFailed",
				fmt.Sprintf("Failed to check rollback status on cluster %s: %v", currentCluster.ClusterId, err)), nil
		}

		if exists {
			a.logger.Info("Candidate model still exists on cluster, waiting for rollback",
				zap.String("cluster_id", currentCluster.ClusterId),
				zap.String("model", candidateModel))
			return conditionsutil.GenerateUnknownCondition(condition, "RollbackInProgress",
				fmt.Sprintf("Candidate model %s still exists on cluster %s", candidateModel, currentCluster.ClusterId)), nil
		}

		// Candidate model removed
		a.logger.Info("Candidate model removed from cluster",
			zap.String("cluster_id", currentCluster.ClusterId),
			zap.String("model", candidateModel))

		metadata.Clusters[currentIdx].State = common.ClusterStateRolledBack
		metadata.CurrentIndex = currentIdx + 1

		if err := common.SetClusterMetadata(condition, metadata); err != nil {
			return nil, fmt.Errorf("failed to update rollback metadata: %w", err)
		}

		if currentIdx+1 < len(metadata.Clusters) {
			return conditionsutil.GenerateFalseCondition(condition, "NextClusterRollbackPending",
				fmt.Sprintf("Cluster %s rolled back, moving to next cluster", currentCluster.ClusterId)), nil
		}

		return conditionsutil.GenerateTrueCondition(condition), nil
	}

	return conditionsutil.GenerateUnknownCondition(condition, "UnexpectedState",
		fmt.Sprintf("Cluster %s in unexpected state: %s", currentCluster.ClusterId, currentCluster.State)), nil
}

// Run unloads the candidate model from the current cluster.
func (a *RollbackActor) Run(ctx context.Context, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Running rollback for deployment", zap.String("deployment", resource.Name))

	candidateModel := resource.Status.CandidateRevision.GetName()
	inferenceServerName := resource.Spec.GetInferenceServer().GetName()

	metadata := common.GetClusterMetadata(condition)

	// if metadata is nil, then initialize it from the inference server
	if metadata == nil {
		a.logger.Info("Initializing rollback metadata from inference server",
			zap.String("inference_server", inferenceServerName))

		targetInfo, err := a.gateway.GetDeploymentTargetInfo(ctx, a.logger, inferenceServerName, resource.Namespace)
		if err != nil {
			return conditionsutil.GenerateFalseCondition(condition, "GetTargetInfoFailed",
				fmt.Sprintf("Failed to get deployment target info: %v", err)), nil
		}

		if len(targetInfo.ClusterTargets) == 0 {
			return conditionsutil.GenerateFalseCondition(condition, "NoClustersFound",
				"No target clusters found for inference server"), nil
		}

		metadata = &common.ClusterMetadata{
			BackendType:  targetInfo.BackendType.String(),
			Clusters:     make([]common.ClusterEntry, len(targetInfo.ClusterTargets)),
			CurrentIndex: 0,
		}

		for i, ct := range targetInfo.ClusterTargets {
			metadata.Clusters[i] = common.ClusterEntry{
				ClusterId:             ct.ClusterId,
				Host:                  ct.Host,
				Port:                  ct.Port,
				TokenTag:              ct.TokenTag,
				CaDataTag:             ct.CaDataTag,
				State:                 common.ClusterStatePending,
				IsControlPlaneCluster: ct.IsControlPlaneCluster,
			}
		}

		if err := common.SetClusterMetadata(condition, metadata); err != nil {
			return nil, fmt.Errorf("failed to set initial metadata: %w", err)
		}

		a.logger.Info("Initialized rollback metadata, returning to let Retrieve start rollback",
			zap.Int("cluster_count", len(metadata.Clusters)),
			zap.String("backend_type", metadata.BackendType))

		return conditionsutil.GenerateUnknownCondition(condition, "MetadataInitialized",
			"Rollback metadata initialized, ready for rollback"), nil
	}

	if metadata.CurrentIndex >= len(metadata.Clusters) || metadata.CurrentIndex < 0 {
		a.logger.Info("All clusters already rolled back")
		return conditionsutil.GenerateTrueCondition(condition), nil
	}

	currentCluster := &metadata.Clusters[metadata.CurrentIndex]

	if currentCluster.State == common.ClusterStateRollbackInProgress {
		a.logger.Info("Rollback in progress, waiting for Retrieve to check status",
			zap.String("cluster_id", currentCluster.ClusterId))
		return conditionsutil.GenerateUnknownCondition(condition, "RollbackInProgress",
			fmt.Sprintf("Rollback in progress on cluster %s", currentCluster.ClusterId)), nil
	}

	// Unload candidate model from inference server
	a.logger.Info("Unloading candidate model from cluster",
		zap.String("candidate_model", candidateModel),
		zap.String("cluster_id", currentCluster.ClusterId),
		zap.String("inference_server", inferenceServerName))

	clusterTarget := common.GetClusterTargetConnection(currentCluster)
	if err := a.gateway.UnloadModel(ctx, a.logger, candidateModel, inferenceServerName, resource.Namespace, clusterTarget); err != nil {
		a.logger.Error("Failed to unload candidate model from cluster",
			zap.String("model", candidateModel),
			zap.String("cluster_id", currentCluster.ClusterId),
			zap.Error(err))
		return conditionsutil.GenerateFalseCondition(condition, "RollbackFailed",
			fmt.Sprintf("Failed to unload candidate model %s from cluster %s: %v", candidateModel, currentCluster.ClusterId, err)), nil
	}

	metadata.Clusters[metadata.CurrentIndex].State = common.ClusterStateRollbackInProgress
	if err := common.SetClusterMetadata(condition, metadata); err != nil {
		return nil, fmt.Errorf("failed to update rollback metadata: %w", err)
	}

	a.logger.Info("Successfully initiated rollback on cluster",
		zap.String("cluster_id", currentCluster.ClusterId),
		zap.String("candidate_model", candidateModel))

	return conditionsutil.GenerateUnknownCondition(condition, "RollbackStarted",
		fmt.Sprintf("Rollback started on cluster %s", currentCluster.ClusterId)), nil
}
