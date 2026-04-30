// Package source provides EndpointSource implementations for specific
// cluster environments.
package source

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	maconfig "github.com/michelangelo-ai/michelangelo/go/base/config"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/clientfactory"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/endpoints"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

var _ endpoints.EndpointSource = &K3dSource{}

// K3dSource resolves a cluster's ingress address using the k3d networking
// model: the gateway Service is read for its NodePort, and a Node's InternalIP
// is read as a routable address. Both reads target the cluster identified by
// the ClusterTarget via the supplied ClientFactory.
type K3dSource struct {
	clientFactory clientfactory.ClientFactory
	config        maconfig.InferenceServerConfig
	logger        *zap.Logger
}

// NewK3dSource returns an EndpointSource for k3d-style clusters where the
// gateway Service is exposed via NodePort and addressable from peers on the
// same docker network at any node's InternalIP.
func NewK3dSource(clientFactory clientfactory.ClientFactory, config maconfig.InferenceServerConfig, logger *zap.Logger) *K3dSource {
	return &K3dSource{
		clientFactory: clientFactory,
		config:        config,
		logger:        logger.With(zap.String("component", "k3d-endpoint-source")),
	}
}

// Resolve returns the Endpoint at which the target cluster's ingress gateway
// admits traffic. Returns an error when the gateway Service is missing, has
// no NodePort on the configured named port, or no node has an InternalIP.
func (s *K3dSource) Resolve(ctx context.Context, target *v2pb.ClusterTarget) (endpoints.Endpoint, error) {
	kubeClient, err := s.clientFactory.GetClient(ctx, target)
	if err != nil {
		return endpoints.Endpoint{}, fmt.Errorf("get client for cluster %q: %w", target.GetClusterId(), err)
	}

	nodePort, err := s.gatewayNodePort(ctx, kubeClient, target.GetClusterId())
	if err != nil {
		return endpoints.Endpoint{}, err
	}
	nodeAddr, err := s.firstNodeInternalIP(ctx, kubeClient, target.GetClusterId())
	if err != nil {
		return endpoints.Endpoint{}, err
	}

	return endpoints.Endpoint{
		Host:   nodeAddr,
		Port:   nodePort,
		Scheme: "http",
	}, nil
}

// gatewayNodePort fetches the gateway Service identified by config and returns
// the NodePort assigned to the named port.
func (s *K3dSource) gatewayNodePort(ctx context.Context, kubeClient client.Client, clusterID string) (int32, error) {
	gw := s.config.Gateway
	svc := &corev1.Service{}
	key := types.NamespacedName{Name: gw.ServiceName, Namespace: gw.ServiceNamespace}
	if err := kubeClient.Get(ctx, key, svc); err != nil {
		return 0, fmt.Errorf("get gateway service %s/%s on cluster %q: %w",
			gw.ServiceNamespace, gw.ServiceName, clusterID, err)
	}
	for _, port := range svc.Spec.Ports {
		if port.Name != gw.PortName {
			continue
		}
		if port.NodePort == 0 {
			return 0, fmt.Errorf("gateway service %s/%s on cluster %q has no NodePort on port %q (Service type may not be NodePort)",
				gw.ServiceNamespace, gw.ServiceName, clusterID, gw.PortName)
		}
		return port.NodePort, nil
	}
	return 0, fmt.Errorf("gateway service %s/%s on cluster %q has no port named %q",
		gw.ServiceNamespace, gw.ServiceName, clusterID, gw.PortName)
}

// firstNodeInternalIP returns the InternalIP of the first Node listed by the
// target cluster's API. Any node's InternalIP suffices because the gateway is
// a NodePort exposed on every node.
func (s *K3dSource) firstNodeInternalIP(ctx context.Context, kubeClient client.Client, clusterID string) (string, error) {
	nodes := &corev1.NodeList{}
	if err := kubeClient.List(ctx, nodes); err != nil {
		return "", fmt.Errorf("list nodes on cluster %q: %w", clusterID, err)
	}
	for _, node := range nodes.Items {
		for _, addr := range node.Status.Addresses {
			if addr.Type == corev1.NodeInternalIP && addr.Address != "" {
				return addr.Address, nil
			}
		}
	}
	return "", fmt.Errorf("no node on cluster %q reported an InternalIP", clusterID)
}
