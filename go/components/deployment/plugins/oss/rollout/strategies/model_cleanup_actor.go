package strategies

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	conditionUtils "github.com/michelangelo-ai/michelangelo/go/base/conditions/utils"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	actorCommon "github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/gateways"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

var _ conditionInterfaces.ConditionActor[*v2pb.Deployment] = &ModelCleanupActor{}

// ModelCleanupActor unloads previous model versions from inference servers after successful rollout.
type ModelCleanupActor struct {
	Gateway gateways.Gateway
	Logger  *zap.Logger
}

// GetType returns the condition type identifier for model cleanup.
func (a *ModelCleanupActor) GetType() string {
	return common.ActorTypeModelCleanup
}

// Retrieve checks if old models are still loaded and require cleanup.
func (a *ModelCleanupActor) Retrieve(ctx context.Context, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	currentModel := resource.Status.GetCurrentRevision().GetName()
	desiredModel := resource.Spec.GetDesiredRevision().GetName()
	// If models are the same, no cleanup needed
	if currentModel == "" || (currentModel == desiredModel) {
		return conditionUtils.GenerateTrueCondition(condition), nil
	}

	metadata := actorCommon.GetClusterMetadata(condition)
	if metadata == nil {
		a.Logger.Info("No cleanup metadata found, triggering Run to initialize")
		return conditionUtils.GenerateFalseCondition(condition, "CleanupNotStarted",
			"Cleanup has not started"), nil
	}

	inferenceServerName := resource.Spec.GetInferenceServer().GetName()
	a.Logger.Info("Checking if old model cleanup is needed",
		zap.String("current_model", currentModel),
		zap.String("desired_model", desiredModel),
		zap.String("inference_server", inferenceServerName))

	// Find current cluster to clean (first non-CLEANED)
	currentIdx := -1
	for i, cluster := range metadata.Clusters {
		if cluster.State != actorCommon.ClusterStateCleaned {
			currentIdx = i
			break
		}
	}

	if currentIdx == -1 {
		a.Logger.Info("All clusters cleaned up successfully",
			zap.Int("total_clusters", len(metadata.Clusters)))
		return conditionUtils.GenerateTrueCondition(condition), nil
	}

	// Update CurrentIndex so Run knows which cluster to clean
	if metadata.CurrentIndex != currentIdx {
		metadata.CurrentIndex = currentIdx
		if err := actorCommon.SetClusterMetadata(condition, metadata); err != nil {
			return nil, fmt.Errorf("failed to update current index: %w", err)
		}
	}

	currentCluster := &metadata.Clusters[currentIdx]
	a.Logger.Info("Checking cleanup status for cluster",
		zap.String("cluster_id", currentCluster.ClusterId),
		zap.String("state", currentCluster.State),
		zap.Int("cluster_index", currentIdx),
		zap.Int("total_clusters", len(metadata.Clusters)))

	// If PENDING, trigger Run to start cleanup
	if currentCluster.State == actorCommon.ClusterStatePending {
		return conditionUtils.GenerateFalseCondition(condition, "CleanupPending",
			fmt.Sprintf("Cluster %s is pending cleanup", currentCluster.ClusterId)), nil
	}

	// If IN_PROGRESS, check if old model still exists
	if currentCluster.State == actorCommon.ClusterStateCleanupInProgress {
		clusterTarget := actorCommon.GetClusterTargetConnection(currentCluster)
		backendType := v2pb.BackendType(v2pb.BackendType_value[metadata.BackendType])

		modelReady, err := a.Gateway.CheckModelStatus(
			ctx, a.Logger, currentModel, inferenceServerName, resource.Namespace, clusterTarget, backendType,
		)
		if err != nil {
			a.Logger.Warn("Failed to check model status during cleanup, will retry",
				zap.String("cluster_id", currentCluster.ClusterId),
				zap.String("model", currentModel),
				zap.Error(err))
			return conditionUtils.GenerateUnknownCondition(condition, "CleanupStatusCheckFailed",
				fmt.Sprintf("Failed to check cleanup status on cluster %s: %v", currentCluster.ClusterId, err)), nil
		}

		if modelReady {
			a.Logger.Info("Old model still loaded on cluster, waiting for cleanup",
				zap.String("cluster_id", currentCluster.ClusterId),
				zap.String("model", currentModel))
			return conditionUtils.GenerateUnknownCondition(condition, "CleanupInProgress",
				fmt.Sprintf("Old model %s still loaded on cluster %s", currentModel, currentCluster.ClusterId)), nil
		}

		// Old model cleaned up
		a.Logger.Info("Old model cleaned up on cluster",
			zap.String("cluster_id", currentCluster.ClusterId),
			zap.String("model", currentModel))

		metadata.Clusters[currentIdx].State = actorCommon.ClusterStateCleaned
		metadata.CurrentIndex = currentIdx + 1

		if err := actorCommon.SetClusterMetadata(condition, metadata); err != nil {
			return nil, fmt.Errorf("failed to update cleanup metadata: %w", err)
		}

		if currentIdx+1 < len(metadata.Clusters) {
			return conditionUtils.GenerateFalseCondition(condition, "NextClusterCleanupPending",
				fmt.Sprintf("Cluster %s cleaned, moving to next cluster", currentCluster.ClusterId)), nil
		}

		return conditionUtils.GenerateTrueCondition(condition), nil
	}

	return conditionUtils.GenerateUnknownCondition(condition, "UnexpectedState",
		fmt.Sprintf("Cluster %s in unexpected state: %s", currentCluster.ClusterId, currentCluster.State)), nil
}

// Run removes old model from ConfigMap and directly unloads it from Triton via API.
func (a *ModelCleanupActor) Run(ctx context.Context, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	a.Logger.Info("Running model cleanup for deployment", zap.String("deployment", resource.Name))

	currentModel := resource.Status.GetCurrentRevision().GetName()
	desiredModel := resource.Spec.GetDesiredRevision().GetName()
	inferenceServerName := resource.Spec.GetInferenceServer().GetName()

	metadata := actorCommon.GetClusterMetadata(condition)

	// if metadata is nil, then initialize it from the inference server
	if metadata == nil {
		a.Logger.Info("Initializing cleanup metadata from inference server",
			zap.String("inference_server", inferenceServerName))

		targetInfo, err := a.Gateway.GetDeploymentTargetInfo(ctx, a.Logger, inferenceServerName, resource.Namespace)
		if err != nil {
			return conditionUtils.GenerateFalseCondition(condition, "GetTargetInfoFailed",
				fmt.Sprintf("Failed to get deployment target info: %v", err)), nil
		}

		if len(targetInfo.ClusterTargets) == 0 {
			return conditionUtils.GenerateFalseCondition(condition, "NoClustersFound",
				"No target clusters found for inference server"), nil
		}

		metadata = &actorCommon.ClusterMetadata{
			BackendType:  targetInfo.BackendType.String(),
			Clusters:     make([]actorCommon.ClusterEntry, len(targetInfo.ClusterTargets)),
			CurrentIndex: 0,
		}

		for i, ct := range targetInfo.ClusterTargets {
			metadata.Clusters[i] = actorCommon.ClusterEntry{
				ClusterId:             ct.ClusterId,
				Host:                  ct.Host,
				Port:                  ct.Port,
				TokenTag:              ct.TokenTag,
				CaDataTag:             ct.CaDataTag,
				State:                 actorCommon.ClusterStatePending,
				IsControlPlaneCluster: ct.IsControlPlaneCluster,
			}
		}

		if err := actorCommon.SetClusterMetadata(condition, metadata); err != nil {
			return nil, fmt.Errorf("failed to set initial metadata: %w", err)
		}

		a.Logger.Info("Initialized cleanup metadata, returning to let Retrieve start cleanup",
			zap.Int("cluster_count", len(metadata.Clusters)),
			zap.String("backend_type", metadata.BackendType))

		return conditionUtils.GenerateUnknownCondition(condition, "MetadataInitialized",
			"Cleanup metadata initialized, ready for cleanup"), nil
	}

	if metadata.CurrentIndex >= len(metadata.Clusters) || metadata.CurrentIndex < 0 {
		a.Logger.Info("All clusters already cleaned")
		return conditionUtils.GenerateTrueCondition(condition), nil
	}

	a.Logger.Info("Starting old model cleanup",
		zap.String("old_model", currentModel),
		zap.String("new_model", desiredModel),
		zap.String("inference_server", inferenceServerName))

	currentCluster := &metadata.Clusters[metadata.CurrentIndex]

	if currentCluster.State == actorCommon.ClusterStateCleanupInProgress {
		a.Logger.Info("Cleanup in progress, waiting for Retrieve to check status",
			zap.String("cluster_id", currentCluster.ClusterId))
		return conditionUtils.GenerateUnknownCondition(condition, "CleanupInProgress",
			fmt.Sprintf("Cleanup in progress on cluster %s", currentCluster.ClusterId)), nil
	}

	// Unload old model from inference server
	a.Logger.Info("Unloading old model from inference server", zap.String("old_model", currentModel))
	clusterTarget := actorCommon.GetClusterTargetConnection(currentCluster)
	if err := a.Gateway.UnloadModel(ctx, a.Logger, currentModel, inferenceServerName, resource.Namespace, clusterTarget); err != nil {
		a.Logger.Error("Failed to unload old model from inference server", zap.String("model", currentModel), zap.Error(err))
		return conditionUtils.GenerateFalseCondition(condition, "ModelUnloadingFailed", fmt.Sprintf("Failed to unload old model %s from inference server: %v", currentModel, err)), nil
	}

	metadata.Clusters[metadata.CurrentIndex].State = actorCommon.ClusterStateCleanupInProgress
	if err := actorCommon.SetClusterMetadata(condition, metadata); err != nil {
		return nil, fmt.Errorf("failed to update cleanup metadata: %w", err)
	}

	a.Logger.Info("Successfully initiated old model cleanup on cluster",
		zap.String("cluster_id", currentCluster.ClusterId),
		zap.String("old_model", currentModel),
		zap.String("new_model", desiredModel))

	return conditionUtils.GenerateUnknownCondition(condition, "CleanupStarted",
		fmt.Sprintf("Cleanup started on cluster %s", currentCluster.ClusterId)), nil
}
