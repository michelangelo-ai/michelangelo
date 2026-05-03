package common

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/clientfactory"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// APIServerURLFromTarget joins a cluster target's connection host and port into the
// base URL of its Kubernetes API server.
func APIServerURLFromTarget(target *v2pb.ClusterTarget) string {
	k := target.GetKubernetes()
	return fmt.Sprintf("%s:%s", k.GetHost(), k.GetPort())
}

// CheckModelStatusAllClusters reports whether the desired model is loaded and ready in
// every cluster listed in the deployment's target-clusters snapshot. Aggregation is
// strict: a single cluster reporting "not ready" causes the whole result to be false.
//
// Returns:
//   - (true, "", nil) when every cluster reports ready.
//   - (false, summary, nil) when one or more clusters are reachable but not yet ready;
//     summary lists the failing cluster IDs.
//   - (false, "", err) when a per-cluster probe itself errored.
//   - (false, "no target clusters in snapshot", nil) when the snapshot is missing or empty.
func CheckModelStatusAllClusters(
	ctx context.Context,
	logger *zap.Logger,
	deployment *v2pb.Deployment,
	clientFactory clientfactory.ClientFactory,
	backend backends.Backend,
	inferenceServerName string,
	modelName string,
) (bool, string, error) {
	targets, err := ReadTargetClustersAnnotation(deployment)
	if err != nil {
		return false, "", fmt.Errorf("read target clusters annotation: %w", err)
	}
	if len(targets) == 0 {
		return false, "no target clusters in snapshot", nil
	}

	var unhealthy []string
	for _, target := range targets {
		clusterID := target.GetClusterId()

		kubeClient, err := clientFactory.GetClient(ctx, target)
		if err != nil {
			return false, "", fmt.Errorf("get client for cluster %s: %w", clusterID, err)
		}
		httpClient, err := clientFactory.GetHTTPClient(ctx, target)
		if err != nil {
			return false, "", fmt.Errorf("get http client for cluster %s: %w", clusterID, err)
		}

		ready, err := backend.CheckModelStatus(
			ctx, logger, kubeClient, httpClient,
			APIServerURLFromTarget(target),
			inferenceServerName, deployment.Namespace, modelName,
		)
		if err != nil {
			return false, "", fmt.Errorf("check model status in cluster %s: %w", clusterID, err)
		}
		if !ready {
			unhealthy = append(unhealthy, clusterID)
		}
	}

	if len(unhealthy) > 0 {
		return false, fmt.Sprintf("model %s not ready in clusters: %v", modelName, unhealthy), nil
	}
	return true, "", nil
}
