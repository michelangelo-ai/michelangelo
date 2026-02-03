package strategies

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	conditionUtils "github.com/michelangelo-ai/michelangelo/go/base/conditions/utils"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	actorCommon "github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	strategiesCommon "github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/rollout/strategies/common"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/gateways"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// getRollingActors returns actors for rolling rollout strategy
func getRollingActors(params Params, deployment *v2pb.Deployment) []conditionInterfaces.ConditionActor[*v2pb.Deployment] {
	return []conditionInterfaces.ConditionActor[*v2pb.Deployment]{
		&RollingRolloutActor{
			gateway: params.Gateway,
			logger:  params.Logger,
		},
		&strategiesCommon.TrafficRoutingActor{
			ProxyProvider: params.ProxyProvider,
			Gateway:       params.Gateway,
			Logger:        params.Logger,
		},
		&ModelCleanupActor{
			Gateway: params.Gateway,
			Logger:  params.Logger,
		},
	}
}

var _ conditionInterfaces.ConditionActor[*v2pb.Deployment] = &RollingRolloutActor{}

// RollingRolloutActor loads models into the inference servers via a rolling rollout strategy.
// The strategy involves loading the model into one target cluster at a time and verifying it is ready.
type RollingRolloutActor struct {
	gateway gateways.Gateway
	logger  *zap.Logger
}

// GetType returns the condition type identifier for rolling rollout.
func (a *RollingRolloutActor) GetType() string {
	return common.ActorTypeRollingRollout
}

// Retrieve checks the deployment status of the current cluster and updates state accordingly.
func (a *RollingRolloutActor) Retrieve(ctx context.Context, deployment *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	metadata := actorCommon.GetClusterMetadata(condition)

	// No metadata means Run hasn't been called yet
	if metadata == nil {
		a.logger.Info("No rollout metadata found, triggering Run to initialize")
		return conditionUtils.GenerateFalseCondition(condition, "RollingRolloutNotStarted", "Rolling rollout has not started"), nil
	}

	// Find current cluster (first non-DEPLOYED)
	currentIdx := -1
	for i, cluster := range metadata.Clusters {
		if cluster.State != actorCommon.ClusterStateDeployed {
			currentIdx = i
			break
		}
	}

	if currentIdx == -1 {
		a.logger.Info("All clusters have been deployed successfully",
			zap.Int("total_clusters", len(metadata.Clusters)))
		return conditionUtils.GenerateTrueCondition(condition), nil
	}

	// Update CurrentIndex so Run knows which cluster to deploy
	if metadata.CurrentIndex != currentIdx {
		metadata.CurrentIndex = currentIdx
		if err := actorCommon.SetClusterMetadata(condition, metadata); err != nil {
			return nil, fmt.Errorf("failed to update current index: %w", err)
		}
	}

	currentCluster := &metadata.Clusters[currentIdx]
	modelName := deployment.Spec.DesiredRevision.Name
	inferenceServerName := deployment.Spec.GetInferenceServer().Name

	a.logger.Info("Checking deployment status for cluster",
		zap.String("cluster_id", currentCluster.ClusterId),
		zap.String("state", currentCluster.State),
		zap.Int("cluster_index", currentIdx),
		zap.Int("total_clusters", len(metadata.Clusters)))

	// If in PENDING state, update CurrentIndex and trigger Run to deploy
	if currentCluster.State == actorCommon.ClusterStatePending {
		return conditionUtils.GenerateFalseCondition(condition, "ClusterPendingDeployment",
			fmt.Sprintf("Cluster %s is pending deployment", currentCluster.ClusterId)), nil
	}

	// If IN_PROGRESS state, then check model status
	if currentCluster.State == actorCommon.ClusterStateDeploymentInProgress {
		clusterTarget := actorCommon.GetClusterTargetConnection(currentCluster)
		backendType := v2pb.BackendType(v2pb.BackendType_value[metadata.BackendType])

		modelReady, err := a.gateway.CheckModelStatus(ctx, a.logger, modelName, inferenceServerName, deployment.Namespace, clusterTarget, backendType)
		if err != nil {
			a.logger.Warn("Failed to check model status, will retry",
				zap.String("cluster_id", currentCluster.ClusterId),
				zap.String("model", modelName),
				zap.Error(err))
			return conditionUtils.GenerateUnknownCondition(condition, "ModelStatusCheckFailed",
				fmt.Sprintf("Failed to check model status on cluster %s: %v", currentCluster.ClusterId, err)), nil
		}

		if modelReady {
			// Mark as DEPLOYED
			a.logger.Info("Model deployed successfully on cluster",
				zap.String("cluster_id", currentCluster.ClusterId),
				zap.String("model", modelName))

			metadata.Clusters[currentIdx].State = actorCommon.ClusterStateDeployed
			metadata.CurrentIndex = currentIdx + 1

			if err := actorCommon.SetClusterMetadata(condition, metadata); err != nil {
				return nil, fmt.Errorf("failed to update metadata: %w", err)
			}

			// If more clusters remain, return false to trigger next cluster deployment
			if currentIdx+1 < len(metadata.Clusters) {
				return conditionUtils.GenerateFalseCondition(condition, "NextClusterPending",
					fmt.Sprintf("Cluster %s deployed, moving to next cluster", currentCluster.ClusterId)), nil
			}
			return conditionUtils.GenerateTrueCondition(condition), nil
		}

		// Model not ready yet
		a.logger.Info("Model not yet ready on cluster, continuing to wait",
			zap.String("cluster_id", currentCluster.ClusterId),
			zap.String("model", modelName))
		return conditionUtils.GenerateUnknownCondition(condition, "ModelLoading",
			fmt.Sprintf("Model %s is loading on cluster %s", modelName, currentCluster.ClusterId)), nil
	}

	return conditionUtils.GenerateUnknownCondition(condition, "UnexpectedState",
		fmt.Sprintf("Cluster %s in unexpected state: %s", currentCluster.ClusterId, currentCluster.State)), nil
}

// Run initiates model deployment on the current cluster.
func (a *RollingRolloutActor) Run(ctx context.Context, deployment *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Running rolling rollout for deployment", zap.String("deployment", deployment.Name))

	if deployment.Spec.DesiredRevision == nil {
		return conditionUtils.GenerateFalseCondition(condition, "NoDesiredRevision", "No desired revision specified"), nil
	}

	modelName := deployment.Spec.DesiredRevision.Name
	inferenceServerName := deployment.Spec.GetInferenceServer().Name

	metadata := actorCommon.GetClusterMetadata(condition)

	// if metadata is nil, then initialize it from the inference server
	if metadata == nil {
		a.logger.Info("Initializing rollout metadata from inference server",
			zap.String("inference_server", inferenceServerName))

		targetInfo, err := a.gateway.GetDeploymentTargetInfo(ctx, a.logger, inferenceServerName, deployment.Namespace)
		if err != nil {
			return conditionUtils.GenerateFalseCondition(condition, "GetTargetInfoFailed",
				fmt.Sprintf("Failed to get deployment target info: %v", err)), nil
		}

		if len(targetInfo.ClusterTargets) == 0 {
			return conditionUtils.GenerateFalseCondition(condition, "NoClustersFound",
				"No target clusters found for inference server"), nil
		}

		// Build metadata with all clusters in PENDING state
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

		a.logger.Info("Initialized rollout metadata, returning to let Retrieve start deployment",
			zap.Int("cluster_count", len(metadata.Clusters)),
			zap.String("backend_type", metadata.BackendType))

		return conditionUtils.GenerateUnknownCondition(condition, "MetadataInitialized",
			"Rollout metadata initialized, ready for deployment"), nil
	}

	if metadata.CurrentIndex >= len(metadata.Clusters) || metadata.CurrentIndex < 0 {
		a.logger.Info("All clusters already deployed")
		return conditionUtils.GenerateTrueCondition(condition), nil
	}

	currentCluster := &metadata.Clusters[metadata.CurrentIndex]
	if currentCluster.State == actorCommon.ClusterStateDeploymentInProgress {
		a.logger.Info("Cluster deployment in progress, waiting for Retrieve to check status",
			zap.String("cluster_id", currentCluster.ClusterId))
		return conditionUtils.GenerateUnknownCondition(condition, "DeploymentInProgress",
			fmt.Sprintf("Model deployment in progress on cluster %s", currentCluster.ClusterId)), nil
	}

	// Deploy to this cluster
	a.logger.Info("Starting model deployment on cluster",
		zap.String("cluster_id", currentCluster.ClusterId),
		zap.String("model", modelName),
		zap.Int("cluster_index", metadata.CurrentIndex),
		zap.Int("total_clusters", len(metadata.Clusters)))

	clusterTarget := actorCommon.GetClusterTargetConnection(currentCluster)
	// TODO(#696): make the storage path configurable w.r.t storage client and storage location
	storagePath := fmt.Sprintf("s3://deploy-models/%s/", modelName)

	if err := a.gateway.LoadModel(ctx, a.logger, modelName, storagePath, inferenceServerName, deployment.Namespace, clusterTarget); err != nil {
		a.logger.Error("Failed to initiate model loading",
			zap.Error(err),
			zap.String("cluster_id", currentCluster.ClusterId),
			zap.String("model", modelName))
		return conditionUtils.GenerateFalseCondition(condition, "ModelLoadingFailed",
			fmt.Sprintf("Failed to load model on cluster %s: %v", currentCluster.ClusterId, err)), nil
	}

	// Mark as IN_PROGRESS
	metadata.Clusters[metadata.CurrentIndex].State = actorCommon.ClusterStateDeploymentInProgress
	if err := actorCommon.SetClusterMetadata(condition, metadata); err != nil {
		return nil, fmt.Errorf("failed to update metadata: %w", err)
	}

	a.logger.Info("Successfully initiated model loading on cluster",
		zap.String("cluster_id", currentCluster.ClusterId),
		zap.String("model", modelName))

	return conditionUtils.GenerateUnknownCondition(condition, "DeploymentStarted",
		fmt.Sprintf("Model deployment started on cluster %s", currentCluster.ClusterId)), nil
}
