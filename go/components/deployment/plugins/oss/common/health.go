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

// GetModelStatusInCluster asks the backend for the model's per-replica load state in one
// cluster, resolving the cluster's clients from the factory.
func GetModelStatusInCluster(
	ctx context.Context,
	logger *zap.Logger,
	clientFactory clientfactory.ClientFactory,
	backend backends.Backend,
	target *v2pb.ClusterTarget,
	namespace string,
	inferenceServerName string,
	modelName string,
) (*backends.ModelStatus, error) {
	clusterID := target.GetClusterId()
	kubeClient, err := clientFactory.GetClient(ctx, target)
	if err != nil {
		return nil, fmt.Errorf("get client for cluster %s: %w", clusterID, err)
	}
	httpClient, err := clientFactory.GetHTTPClient(ctx, target)
	if err != nil {
		return nil, fmt.Errorf("get http client for cluster %s: %w", clusterID, err)
	}
	status, err := backend.GetModelStatus(ctx, logger, kubeClient, httpClient, APIServerURLFromTarget(target), inferenceServerName, namespace, modelName)
	if err != nil {
		return nil, fmt.Errorf("check model status in cluster %s: %w", clusterID, err)
	}
	return status, nil
}

// CheckModelStatusAllClusters reports whether the model is loaded and ready on every
// replica in every cluster listed in the deployment's target-clusters snapshot.
// Aggregation is strict: a single replica that is not ready makes the whole result false.
//
// Returns:
//   - (true, "", nil) when every replica in every cluster reports ready.
//   - (false, summary, nil) when one or more clusters are reachable but not yet ready;
//     summary lists the failing clusters with their replica states.
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
		status, err := GetModelStatusInCluster(ctx, logger, clientFactory, backend, target, deployment.Namespace, inferenceServerName, modelName)
		if err != nil {
			return false, "", err
		}
		if !status.Ready() {
			unhealthy = append(unhealthy, fmt.Sprintf("%s: %s", target.GetClusterId(), status.Summary()))
		}
	}

	if len(unhealthy) > 0 {
		return false, fmt.Sprintf("model %s not ready in clusters: %v", modelName, unhealthy), nil
	}
	return true, "", nil
}
