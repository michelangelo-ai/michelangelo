package common

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

const (
	// TargetClustersAnnotation is written by PlacementPrepActor and lists, as JSON, the clusters a
	// rollout placed the Deployment in. Its Secret names go stale when credentials are rotated into
	// a new Secret, so take connections from ResolveTargetClusters.
	TargetClustersAnnotation = "deployment.michelangelo.ai/target-clusters"
	// BackendTypeAnnotation records the InferenceServer's backend type, snapshotted by
	// PlacementPrepActor next to the cluster set so every later actor talks to the same backend.
	BackendTypeAnnotation = "deployment.michelangelo.ai/backend-type"
)

// clusterTargetAnnotation is the wire representation stored in the annotation.
type clusterTargetAnnotation struct {
	ClusterID string `json:"clusterId"`
	Host      string `json:"host,omitempty"`
	Port      string `json:"port,omitempty"`
	TokenTag  string `json:"tokenTag,omitempty"`
	CADataTag string `json:"caDataTag,omitempty"`
}

// FetchInferenceServer loads the InferenceServer referenced by the Deployment.
func FetchInferenceServer(ctx context.Context, kubeClient client.Reader, deployment *v2pb.Deployment) (*v2pb.InferenceServer, error) {
	isRef := deployment.Spec.GetInferenceServer()
	if isRef == nil {
		return nil, fmt.Errorf("deployment %s/%s has no inference server reference", deployment.Namespace, deployment.Name)
	}
	inferenceServer := &v2pb.InferenceServer{}
	if err := kubeClient.Get(ctx, k8stypes.NamespacedName{Name: isRef.GetName(), Namespace: deployment.Namespace}, inferenceServer); err != nil {
		return nil, fmt.Errorf("get inference server %s/%s: %w", deployment.Namespace, isRef.GetName(), err)
	}
	return inferenceServer, nil
}

// WriteBackendTypeAnnotation records the InferenceServer's backend type on the Deployment.
// The caller is responsible for issuing the Update.
func WriteBackendTypeAnnotation(deployment *v2pb.Deployment, backendType v2pb.BackendType) {
	if deployment.Annotations == nil {
		deployment.Annotations = make(map[string]string)
	}
	deployment.Annotations[BackendTypeAnnotation] = backendType.String()
}

// BackendTypeOf returns the backend type snapshotted on the Deployment. Deployments placed
// before the snapshot existed, or whose annotation is unreadable, are served by Triton, the
// only backend they could have been rolled out on.
func BackendTypeOf(deployment *v2pb.Deployment) v2pb.BackendType {
	if value, ok := v2pb.BackendType_value[deployment.GetAnnotations()[BackendTypeAnnotation]]; ok && value != int32(v2pb.BACKEND_TYPE_INVALID) {
		return v2pb.BackendType(value)
	}
	return v2pb.BACKEND_TYPE_TRITON
}

// ReadTargetClustersAnnotation deserializes the cluster snapshot from the Deployment annotation.
// Returns (nil, nil) if the annotation is absent.
func ReadTargetClustersAnnotation(deployment *v2pb.Deployment) ([]*v2pb.ClusterTarget, error) {
	if deployment.Annotations == nil {
		return nil, nil
	}
	raw, ok := deployment.Annotations[TargetClustersAnnotation]
	if !ok {
		return nil, nil
	}

	var items []clusterTargetAnnotation
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil, fmt.Errorf("unmarshal %s annotation: %w", TargetClustersAnnotation, err)
	}

	targets := make([]*v2pb.ClusterTarget, 0, len(items))
	for _, item := range items {
		targets = append(targets, &v2pb.ClusterTarget{
			ClusterId: item.ClusterID,
			Connection: &v2pb.ClusterTarget_Kubernetes{
				Kubernetes: &v2pb.ConnectionSpec{
					Host:      item.Host,
					Port:      item.Port,
					TokenTag:  item.TokenTag,
					CaDataTag: item.CADataTag,
				},
			},
		})
	}
	return targets, nil
}

// ResolveTargetClusters returns the Deployment's cluster snapshot with each cluster's connection
// replaced by the InferenceServer's current one, matched by cluster ID. The snapshot decides which
// clusters to visit; connections come from the InferenceServer because the snapshot's Secret names
// go stale once credentials are rotated into a new Secret and the old one is deleted. A cluster
// the InferenceServer cannot supply (not listed with a Kubernetes connection, InferenceServer
// missing or not referenced, or a nil kubeClient) keeps its snapshot connection. Any other
// InferenceServer read error is returned.
func ResolveTargetClusters(ctx context.Context, kubeClient client.Reader, deployment *v2pb.Deployment) ([]*v2pb.ClusterTarget, error) {
	snapshot, err := ReadTargetClustersAnnotation(deployment)
	if err != nil {
		return nil, err
	}
	if len(snapshot) == 0 || deployment.Spec.GetInferenceServer() == nil || kubeClient == nil {
		return snapshot, nil
	}

	inferenceServer, err := FetchInferenceServer(ctx, kubeClient, deployment)
	if apierrors.IsNotFound(err) {
		return snapshot, nil
	}
	if err != nil {
		return nil, err
	}

	return overlayLiveConnections(snapshot, inferenceServer.Spec.GetClusterTargets()), nil
}

// overlayLiveConnections replaces each snapshot target with the live target of the same cluster
// ID, keeping the snapshot's membership and order. Live targets with no Kubernetes connection are
// ignored, and of repeated cluster IDs the first entry with a Kubernetes connection wins.
func overlayLiveConnections(snapshot, live []*v2pb.ClusterTarget) []*v2pb.ClusterTarget {
	byClusterID := make(map[string]*v2pb.ClusterTarget, len(live))
	for _, target := range live {
		if _, seen := byClusterID[target.GetClusterId()]; seen || target.GetKubernetes() == nil {
			continue
		}
		byClusterID[target.GetClusterId()] = target
	}
	resolved := make([]*v2pb.ClusterTarget, 0, len(snapshot))
	for _, target := range snapshot {
		if current, ok := byClusterID[target.GetClusterId()]; ok {
			target = current
		}
		resolved = append(resolved, target)
	}
	return resolved
}

// WriteTargetClustersAnnotation serializes the cluster snapshot onto the Deployment.
// The caller is responsible for issuing the Update.
func WriteTargetClustersAnnotation(deployment *v2pb.Deployment, targets []*v2pb.ClusterTarget) error {
	items := make([]clusterTargetAnnotation, 0, len(targets))
	for _, target := range targets {
		item := clusterTargetAnnotation{
			ClusterID: target.GetClusterId(),
		}
		if kube := target.GetKubernetes(); kube != nil {
			item.Host = kube.GetHost()
			item.Port = kube.GetPort()
			item.TokenTag = kube.GetTokenTag()
			item.CADataTag = kube.GetCaDataTag()
		}
		items = append(items, item)
	}

	raw, err := json.Marshal(items)
	if err != nil {
		return fmt.Errorf("marshal %s annotation: %w", TargetClustersAnnotation, err)
	}

	if deployment.Annotations == nil {
		deployment.Annotations = make(map[string]string)
	}
	deployment.Annotations[TargetClustersAnnotation] = string(raw)
	return nil
}

// ClusterTargetsEqual reports whether two ClusterTarget slices contain the same set of cluster
// IDs, regardless of order.
func ClusterTargetsEqual(left, right []*v2pb.ClusterTarget) bool {
	if len(left) != len(right) {
		return false
	}
	ids := func(targets []*v2pb.ClusterTarget) []string {
		out := make([]string, len(targets))
		for i, target := range targets {
			out[i] = target.GetClusterId()
		}
		sort.Strings(out)
		return out
	}
	leftIDs := ids(left)
	rightIDs := ids(right)
	for i := range leftIDs {
		if leftIDs[i] != rightIDs[i] {
			return false
		}
	}
	return true
}
