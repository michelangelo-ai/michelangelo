package common

import (
	"context"

	"go.uber.org/zap"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/clientfactory"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// GetControlPlaneClusterId returns the ID of the control plane cluster.
func GetControlPlaneClusterId() string {
	return "michelangelo/control-plane-cluster"
}

// GetClusterClients returns a map of cluster clients for the given inference server.
func GetClusterClients(ctx context.Context, logger *zap.Logger, resource *v2pb.InferenceServer, clientFactory clientfactory.ClientFactory, defaultClient client.Client) map[string]client.Client {
	targetClusterClients := make(map[string]client.Client)
	if resource.Spec.GetDeploymentStrategy().GetRemoteClusterDeployment() != nil {
		clusterTargets := resource.Spec.DeploymentStrategy.GetRemoteClusterDeployment().GetClusterTargets()
		for _, target := range clusterTargets {
			remoteClusterClient, err := clientFactory.GetClient(ctx, target)
			if err != nil {
				// in case of errors, only log the error and continue
				logger.Error("Failed to get remote cluster client",
					zap.Error(err),
					zap.String("operation", "get_remote_cluster_client"),
					zap.String("namespace", resource.Namespace),
					zap.String("inferenceServer", resource.Name),
					zap.String("cluster", target.ClusterId))
				continue
			}
			targetClusterClients[target.ClusterId] = remoteClusterClient
		}
	} else {
		targetClusterClients[GetControlPlaneClusterId()] = defaultClient
	}
	return targetClusterClients
}
