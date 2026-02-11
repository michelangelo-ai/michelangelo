package creation

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	conditionsUtils "github.com/michelangelo-ai/michelangelo/go/base/conditions/utils"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/endpointregistry"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/plugins/oss/common"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

var _ conditionInterfaces.ConditionActor[*v2pb.InferenceServer] = &ControlPlaneDiscoveryActor{}

// ControlPlaneDiscoveryActor ensures that control-plane discovery resources (ServiceEntry, bridge Service)
// are registered for all target clusters and pruned for clusters no longer in the spec.
type ControlPlaneDiscoveryActor struct {
	endpointRegistry endpointregistry.EndpointRegistry
	logger           *zap.Logger
}

func NewControlPlaneDiscoveryActor(endpointRegistry endpointregistry.EndpointRegistry, logger *zap.Logger) conditionInterfaces.ConditionActor[*v2pb.InferenceServer] {
	return &ControlPlaneDiscoveryActor{
		endpointRegistry: endpointRegistry,
		logger:           logger,
	}
}

func (a *ControlPlaneDiscoveryActor) GetType() string {
	return common.TritonControlPlaneDiscoveryConditionType
}

// Retrieve checks if all ClusterTargets are registered and no stale endpoints exist.
func (a *ControlPlaneDiscoveryActor) Retrieve(ctx context.Context, resource *v2pb.InferenceServer, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Retrieving control plane discovery state",
		zap.String("inferenceServer", resource.Name),
		zap.String("namespace", resource.Namespace),
	)

	// if the inference server is deployed to control plane cluster, skip discovery
	if resource.Spec.GetDeploymentStrategy() == nil || resource.Spec.GetDeploymentStrategy().GetControlPlaneClusterDeployment() != nil {
		a.logger.Info("Inference server is deployed to control plane cluster, skipping discovery",
			zap.String("inferenceServer", resource.Name),
		)
		return conditionsUtils.GenerateTrueCondition(condition), nil
	}

	// Filter to only clusters that need endpoint registration (remote clusters with kubernetes config).
	// Control plane clusters (no kubernetes config) don't need cross-cluster discovery.
	remoteClusters := filterRemoteClusters(resource.Spec.GetDeploymentStrategy().GetRemoteClusterDeployment().GetClusterTargets())
	if len(remoteClusters) == 0 {
		a.logger.Info("No remote clusters requiring endpoint registration, skipping discovery",
			zap.String("inferenceServer", resource.Name),
		)
		return conditionsUtils.GenerateTrueCondition(condition), nil
	}

	registered, err := a.endpointRegistry.ListRegisteredEndpoints(ctx, a.logger, resource.Name, resource.Namespace)
	if err != nil {
		a.logger.Error("Failed to list registered endpoints",
			zap.Error(err),
			zap.String("inferenceServer", resource.Name),
		)
		return conditionsUtils.GenerateFalseCondition(condition, "ListEndpointsFailed", fmt.Sprintf("Failed to list registered endpoints: %v", err)), nil
	}

	missing, stale := findEndpointDiff(remoteClusters, registered)

	if len(missing) > 0 || len(stale) > 0 {
		missingIDs := make([]string, 0, len(missing))
		for id := range missing {
			missingIDs = append(missingIDs, id)
		}
		msg := fmt.Sprintf("Discovery out of sync: missing=%v, stale=%v", missingIDs, stale)
		a.logger.Info(msg,
			zap.String("inferenceServer", resource.Name),
		)
		return conditionsUtils.GenerateUnknownCondition(condition, "DiscoveryOutOfSync", msg), nil
	}

	a.logger.Info("Control plane discovery is in sync",
		zap.String("inferenceServer", resource.Name),
		zap.Int("registeredClusters", len(registered)),
	)
	return conditionsUtils.GenerateTrueCondition(condition), nil
}

// Run reconciles registered endpoints to match ClusterTargets: registers missing, deletes stale.
func (a *ControlPlaneDiscoveryActor) Run(ctx context.Context, resource *v2pb.InferenceServer, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Running control plane discovery reconciliation",
		zap.String("inferenceServer", resource.Name),
		zap.String("namespace", resource.Namespace),
	)

	registered, err := a.endpointRegistry.ListRegisteredEndpoints(ctx, a.logger, resource.Name, resource.Namespace)
	if err != nil {
		return conditionsUtils.GenerateFalseCondition(condition, "ListEndpointsFailed", fmt.Sprintf("Failed to list registered endpoints: %v", err)), nil
	}

	missing, stale := findEndpointDiff(resource.Spec.GetDeploymentStrategy().GetRemoteClusterDeployment().GetClusterTargets(), registered)

	// Register missing endpoints.
	for clusterID, clusterTarget := range missing {
		a.logger.Info("Registering endpoint for cluster",
			zap.String("inferenceServer", resource.Name),
			zap.String("clusterID", clusterID),
		)

		if err := a.endpointRegistry.EnsureRegisteredEndpoint(ctx, a.logger, endpointregistry.ClusterEndpoint{
			ClusterID:           clusterID,
			InferenceServerName: resource.Name,
			Namespace:           resource.Namespace,
		}, clusterTarget); err != nil {
			a.logger.Error("Failed to register endpoint",
				zap.Error(err),
				zap.String("clusterID", clusterID),
			)
			return conditionsUtils.GenerateFalseCondition(condition, "RegisterEndpointFailed", fmt.Sprintf("Failed to register endpoint for cluster %s: %v", clusterID, err)), nil
		}
	}

	// Delete stale endpoints.
	for _, clusterID := range stale {
		a.logger.Info("Deleting stale endpoint for cluster",
			zap.String("inferenceServer", resource.Name),
			zap.String("clusterID", clusterID),
		)

		if err := a.endpointRegistry.DeleteRegisteredEndpoint(ctx, a.logger, resource.Name, resource.Namespace, clusterID); err != nil {
			a.logger.Error("Failed to delete stale endpoint",
				zap.Error(err),
				zap.String("clusterID", clusterID),
			)
			return conditionsUtils.GenerateFalseCondition(condition, "DeleteEndpointFailed", fmt.Sprintf("Failed to delete stale endpoint for cluster %s: %v", clusterID, err)), nil
		}
	}

	a.logger.Info("Control plane discovery reconciliation completed",
		zap.String("inferenceServer", resource.Name),
	)

	// Return Unknown to trigger re-retrieve on next reconcile to confirm sync.
	return conditionsUtils.GenerateUnknownCondition(condition, "DiscoveryReconciled", "Endpoints registered/pruned, awaiting verification"), nil
}

// findEndpointDiff compares the desired ClusterTargets against currently registered endpoints.
func findEndpointDiff(clusterTargets []*v2pb.ClusterTarget, registered []endpointregistry.ClusterEndpoint) (map[string]*v2pb.ClusterTarget, []string) {
	// Build set of desired cluster IDs.
	desired := make(map[string]*v2pb.ClusterTarget)
	for _, ct := range clusterTargets {
		desired[ct.ClusterId] = ct
	}

	// Build set of registered cluster IDs.
	registeredSet := make(map[string]bool)
	for _, ep := range registered {
		registeredSet[ep.ClusterID] = true
	}

	// Find missing (desired but not registered).
	missing := make(map[string]*v2pb.ClusterTarget)
	for clusterID, ct := range desired {
		if !registeredSet[clusterID] {
			missing[clusterID] = ct
		}
	}

	// Find stale (registered but not desired).
	var stale []string
	for clusterID := range registeredSet {
		if _, ok := desired[clusterID]; !ok {
			stale = append(stale, clusterID)
		}
	}

	return missing, stale
}

// filterRemoteClusters returns only remote clusters that have kubernetes config.
func filterRemoteClusters(clusterTargets []*v2pb.ClusterTarget) []*v2pb.ClusterTarget {
	var remote []*v2pb.ClusterTarget
	for _, ct := range clusterTargets {
		if ct.GetKubernetes() != nil {
			remote = append(remote, ct)
		}
	}
	return remote
}
