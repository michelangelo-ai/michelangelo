package common

import (
	"context"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// GetInferenceServerTargetClusters returns the target clusters for the inference server.
// If the Inference Server deployment strategy is set to control plane only, then it returns nil.
// todo: ghosharitra: see if there's a better way to do this rather than just returning nil.
// in stead of returning nil, we should return backendtype, DeploymentStrategy(controlplane,remote),remote clusterTargets.
func GetInferenceServerTargetClusters(ctx context.Context, client client.Client, deployment *v2pb.Deployment) []*v2pb.ClusterTarget {
	targetInferenceServer := deployment.Spec.GetInferenceServer()
	if targetInferenceServer == nil {
		return nil
	}
	inferenceServer := &v2pb.InferenceServer{}
	if err := client.Get(ctx, types.NamespacedName{
		Namespace: targetInferenceServer.Namespace,
		Name:      targetInferenceServer.Name,
	}, inferenceServer); err != nil {
		return nil
	}
	if inferenceServer.Spec.GetDeploymentStrategy().GetControlPlaneClusterDeployment() != nil {
		return nil
	}
	return inferenceServer.Spec.GetDeploymentStrategy().GetRemoteClusterDeployment().GetClusterTargets()
}
