package rollback

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"sigs.k8s.io/controller-runtime/pkg/client"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	conditionsutil "github.com/michelangelo-ai/michelangelo/go/base/conditions/utils"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/clientfactory"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/modelconfig"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

var _ conditionInterfaces.ConditionActor[*v2pb.Deployment] = &RollbackActor{}

// RollbackActor restores deployment to the previous stable revision when rollout fails.
type RollbackActor struct {
	defaultClient       client.Client
	logger              *zap.Logger
	clientFactory       clientfactory.ClientFactory
	modelConfigProvider modelconfig.ModelConfigProvider
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
		exists, err := common.CheckModelExists(
			ctx, a.logger, a.modelConfigProvider, a.defaultClient, candidateModel, inferenceServerName, resource.Namespace,
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

		targetClusters := common.GetInferenceServerTargetClusters(ctx, a.defaultClient, resource)
		if len(targetClusters) == 0 {
			return conditionsutil.GenerateFalseCondition(condition, "NoClustersFound",
				"No target clusters found for inference server"), nil
		}

		metadata = &common.ClusterMetadata{
			Clusters:     make([]common.ClusterEntry, len(targetClusters)),
			CurrentIndex: 0,
		}

		for i, ct := range targetClusters {
			metadata.Clusters[i] = common.ClusterEntry{
				ClusterId: ct.GetClusterId(),
				Host:      ct.GetKubernetes().GetHost(),
				Port:      ct.GetKubernetes().GetPort(),
				TokenTag:  ct.GetKubernetes().GetTokenTag(),
				CaDataTag: ct.GetKubernetes().GetCaDataTag(),
				State:     common.ClusterStatePending,
			}
		}
		// if no target clusters are found, then add the control plane cluster
		if len(targetClusters) == 0 {
			metadata.Clusters = []common.ClusterEntry{
				{
					State:                 common.ClusterStatePending,
					IsControlPlaneCluster: true,
				},
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

	targetClusterClient := a.defaultClient
	if !currentCluster.IsControlPlaneCluster {
		client, err := a.clientFactory.GetClient(ctx, &v2pb.ClusterTarget{
			ClusterId: currentCluster.ClusterId,
			Config: &v2pb.ClusterTarget_Kubernetes{
				Kubernetes: &v2pb.ConnectionSpec{
					Host:      currentCluster.Host,
					Port:      currentCluster.Port,
					CaDataTag: currentCluster.CaDataTag,
					TokenTag:  currentCluster.TokenTag,
				},
			},
		})
		if err != nil {
			// todo: ghosharitra: in case of error, we should just log error and continue with the next cluster, the logic needs to be updated for this.
			return nil, fmt.Errorf("failed to get client for cluster %s: %w", currentCluster.ClusterId, err)
		}
		targetClusterClient = client
	}

	if err := a.modelConfigProvider.RemoveModelFromConfig(ctx, a.logger, targetClusterClient, candidateModel, inferenceServerName, resource.Namespace); err != nil {
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
