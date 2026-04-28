package cleanup

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/api/errors"

	"sigs.k8s.io/controller-runtime/pkg/client"

	conditionUtils "github.com/michelangelo-ai/michelangelo/go/base/conditions/utils"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/route"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/clientfactory"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/modelconfig"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// CleanupActor removes models from ConfigMap and deletes deployment HTTPRoutes during deletion.
type CleanupActor struct {
	defaultClient       client.Client
	clientFactory       clientfactory.ClientFactory
	routeProvider       route.RouteProvider
	logger              *zap.Logger
	modelConfigProvider modelconfig.ModelConfigProvider
}

// GetType returns the condition type identifier for cleanup.
func (a *CleanupActor) GetType() string {
	return common.ActorTypeCleanup
}

// Retrieve checks if model is still loaded in inference server and deployment HTTPRoute still exist.
func (a *CleanupActor) Retrieve(ctx context.Context, deployment *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	currentModel := deployment.Status.GetCurrentRevision().GetName()
	inferenceServerName := deployment.Spec.GetInferenceServer().GetName()

	metadata := common.GetClusterMetadata(condition)
	if metadata == nil {
		a.logger.Info("No cleanup metadata found, triggering Run to initialize")
		return conditionUtils.GenerateFalseCondition(condition, "CleanupNotStarted",
			"Cleanup has not started"), nil
	}

	a.logger.Info("Checking deletion cleanup status",
		zap.String("current_model", currentModel),
		zap.String("inference_server", inferenceServerName))

	// Find current cluster to clean (first non-DELETED)
	currentIdx := -1
	for i, cluster := range metadata.Clusters {
		if cluster.State != common.ClusterStateCleaned {
			currentIdx = i
			break
		}
	}

	// If all clusters are clean, then check if HTTPRoute still exists
	if currentIdx == -1 {
		a.logger.Info("All clusters cleaned, checking HTTPRoute",
			zap.Int("total_clusters", len(metadata.Clusters)))

		// todo: ghosharitra: interface broke, need to update
		exists, err := a.routeProvider.DeploymentRouteExists(ctx, a.logger, deployment.Name, deployment.Namespace)
		if err != nil {
			return conditionUtils.GenerateFalseCondition(condition, "UnableToCheckHTTPRouteExists",
				fmt.Sprintf("Unable to check if HTTPRoute %s exists: %v", fmt.Sprintf("%s-httproute", deployment.Name), err)), nil
		}
		if exists {
			return conditionUtils.GenerateFalseCondition(condition, "HTTPRouteStillExists",
				fmt.Sprintf("Cleanup required: HTTPRoute %s still exists", fmt.Sprintf("%s-httproute", deployment.Name))), nil
		}

		return conditionUtils.GenerateTrueCondition(condition), nil
	}

	// Update CurrentIndex so Run() knows which cluster to clean
	if metadata.CurrentIndex != currentIdx {
		metadata.CurrentIndex = currentIdx
		if err := common.SetClusterMetadata(condition, metadata); err != nil {
			return nil, fmt.Errorf("failed to update current index: %w", err)
		}
	}

	currentCluster := &metadata.Clusters[currentIdx]
	a.logger.Info("Checking deletion cleanup status for cluster",
		zap.String("cluster_id", currentCluster.ClusterId),
		zap.String("state", currentCluster.State),
		zap.Int("cluster_index", currentIdx),
		zap.Int("total_clusters", len(metadata.Clusters)))

	// If PENDING, trigger Run to start cleanup
	if currentCluster.State == common.ClusterStatePending {
		return conditionUtils.GenerateFalseCondition(condition, "DeletionCleanupPending",
			fmt.Sprintf("Cluster %s is pending deletion cleanup", currentCluster.ClusterId)), nil
	}

	// If DELETION_IN_PROGRESS, check if model still exists
	if currentCluster.State == common.ClusterStateCleanupInProgress {

		// todo: ghosharitra: pass along these information in the form of annotations, these annotations should be cleaned up in the cleanup actor
		var targetClusterClient client.Client
		if currentCluster.IsControlPlaneCluster {
			targetClusterClient = a.defaultClient
		} else {
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
				return conditionUtils.GenerateFalseCondition(condition, "GetClientFailed",
					fmt.Sprintf("Failed to get client for cluster %s: %v", currentCluster.ClusterId, err)), nil
			}
			targetClusterClient = client
		}

		exists, err := common.CheckModelExists(
			ctx, a.logger, a.modelConfigProvider, targetClusterClient, currentModel, inferenceServerName, deployment.Namespace,
		)
		if err != nil {
			a.logger.Warn("Failed to check model existence during deletion cleanup, will retry",
				zap.String("cluster_id", currentCluster.ClusterId),
				zap.String("model", currentModel),
				zap.Error(err))
			return conditionUtils.GenerateUnknownCondition(condition, "DeletionStatusCheckFailed",
				fmt.Sprintf("Failed to check deletion status on cluster %s: %v", currentCluster.ClusterId, err)), nil
		}

		if exists {
			a.logger.Info("Model still exists on cluster, waiting for deletion",
				zap.String("cluster_id", currentCluster.ClusterId),
				zap.String("model", currentModel))
			return conditionUtils.GenerateUnknownCondition(condition, "DeletionInProgress",
				fmt.Sprintf("Model %s still exists on cluster %s", currentModel, currentCluster.ClusterId)), nil
		}

		// Model deleted from cluster
		a.logger.Info("Model deleted from cluster",
			zap.String("cluster_id", currentCluster.ClusterId),
			zap.String("model", currentModel))

		metadata.Clusters[currentIdx].State = common.ClusterStateCleaned
		metadata.CurrentIndex = currentIdx + 1

		if err := common.SetClusterMetadata(condition, metadata); err != nil {
			return nil, fmt.Errorf("failed to update deletion metadata: %w", err)
		}

		if currentIdx+1 < len(metadata.Clusters) {
			return conditionUtils.GenerateFalseCondition(condition, "NextClusterDeletionPending",
				fmt.Sprintf("Cluster %s cleaned, moving to next cluster", currentCluster.ClusterId)), nil
		}

		// All clusters done, but still need to delete HTTPRoute
		return conditionUtils.GenerateFalseCondition(condition, "HTTPRouteDeletionPending",
			"All clusters cleaned, HTTPRoute deletion pending"), nil
	}

	return conditionUtils.GenerateUnknownCondition(condition, "UnexpectedState",
		fmt.Sprintf("Cluster %s in unexpected state: %s", currentCluster.ClusterId, currentCluster.State)), nil
}

// Run removes model from ConfigMap and deletes the deployment HTTPRoute.
func (a *CleanupActor) Run(ctx context.Context, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Running cleanup for deployment", zap.String("deployment", resource.Name))

	currentModel := resource.Status.CurrentRevision.Name
	inferenceServerName := resource.Spec.GetInferenceServer().Name

	metadata := common.GetClusterMetadata(condition)

	// if metadata is nil, then initialize it from the inference server
	if metadata == nil {
		a.logger.Info("Initializing deletion cleanup metadata from inference server",
			zap.String("inference_server", inferenceServerName))

		// todo: ghosharitra: cannot use GetDeploymentTargetInfo, we need to pass in information via annotations
		// this should be handled by some annotation that is passed through
		targetClusters := common.GetInferenceServerTargetClusters(ctx, a.defaultClient, resource)
		metadata = &common.ClusterMetadata{
			Clusters:     make([]common.ClusterEntry, len(targetClusters)),
			CurrentIndex: 0,
		}

		// if target clusters are found, then add them to the metadata along with connection information
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

		a.logger.Info("Initialized deletion cleanup metadata, returning to let Retrieve start cleanup",
			zap.Int("cluster_count", len(metadata.Clusters)))

		return conditionUtils.GenerateUnknownCondition(condition, "MetadataInitialized",
			"Deletion cleanup metadata initialized, ready for cleanup"), nil
	}

	// Check if all clusters are cleaned; if so, delete HTTPRoute
	allCleaned := true
	for _, cluster := range metadata.Clusters {
		if cluster.State != common.ClusterStateCleaned {
			allCleaned = false
			break
		}
	}

	if allCleaned {
		a.logger.Info("All clusters cleaned, deleting HTTPRoute",
			zap.String("deploymentRoute", fmt.Sprintf("%s-httproute", resource.Name)))

		// todo: ghosharitra: interface broke, need to update
		if err := a.routeProvider.DeleteDeploymentRoute(ctx, a.logger, resource.Name, resource.Namespace); err != nil {
			if errors.IsNotFound(err) {
				a.logger.Info("HTTPRoute not found, already deleted",
					zap.String("httpRoute", fmt.Sprintf("%s-httproute", resource.Name)))
			} else {
				a.logger.Error("Failed to delete HTTPRoute", zap.Error(err))
				return conditionUtils.GenerateFalseCondition(condition, "HTTPRouteCleanupFailed",
					fmt.Sprintf("Failed to delete HTTPRoute %s: %v", fmt.Sprintf("%s-httproute", resource.Name), err)), nil
			}
		}

		a.logger.Info("Deletion cleanup completed successfully", zap.String("current_model", currentModel))
		return conditionUtils.GenerateTrueCondition(condition), nil
	}

	if metadata.CurrentIndex >= len(metadata.Clusters) || metadata.CurrentIndex < 0 {
		a.logger.Info("All clusters already cleaned")
		return conditionUtils.GenerateTrueCondition(condition), nil
	}

	currentCluster := &metadata.Clusters[metadata.CurrentIndex]

	if currentCluster.State == common.ClusterStateCleanupInProgress {
		a.logger.Info("Deletion in progress, waiting for Retrieve to check status",
			zap.String("cluster_id", currentCluster.ClusterId))
		return conditionUtils.GenerateUnknownCondition(condition, "DeletionInProgress",
			fmt.Sprintf("Deletion in progress on cluster %s", currentCluster.ClusterId)), nil
	}

	// Unload model from cluster
	a.logger.Info("Unloading model from cluster",
		zap.String("model", currentModel),
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
			return conditionUtils.GenerateFalseCondition(condition, "GetClientFailed",
				fmt.Sprintf("Failed to get client for cluster %s: %v", currentCluster.ClusterId, err)), nil
		}
		targetClusterClient = client
	}
	if err := a.modelConfigProvider.RemoveModelFromConfig(ctx, a.logger, targetClusterClient, currentModel, inferenceServerName, resource.Namespace); err != nil {
		a.logger.Error("Failed to remove model from config",
			zap.String("model", currentModel),
			zap.String("cluster_id", currentCluster.ClusterId),
			zap.Error(err))
		return conditionUtils.GenerateFalseCondition(condition, "ModelRemovalFailed",
			fmt.Sprintf("Failed to unload model %s from cluster %s: %v", currentModel, currentCluster.ClusterId, err)), nil
	}

	metadata.Clusters[metadata.CurrentIndex].State = common.ClusterStateCleanupInProgress
	if err := common.SetClusterMetadata(condition, metadata); err != nil {
		return nil, fmt.Errorf("failed to update deletion metadata: %w", err)
	}

	a.logger.Info("Successfully initiated model deletion on cluster",
		zap.String("cluster_id", currentCluster.ClusterId),
		zap.String("model", currentModel))

	return conditionUtils.GenerateUnknownCondition(condition, "DeletionStarted",
		fmt.Sprintf("Deletion started on cluster %s", currentCluster.ClusterId)), nil
}
