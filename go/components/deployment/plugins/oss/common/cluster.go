package common

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

const (
	// TargetClustersAnnotation is written by PlacementPrepActor and read by all downstream actors.
	// The value is a JSON-encoded list of cluster targets from the InferenceServer.
	TargetClustersAnnotation = "deployment.michelangelo.ai/target-clusters"
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
func FetchInferenceServer(ctx context.Context, kubeClient client.Client, deployment *v2pb.Deployment) (*v2pb.InferenceServer, error) {
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
