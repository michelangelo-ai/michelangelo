// Package clientfactory provides Kubernetes API clients for ClusterTargets that an
// InferenceServer is provisioned across.
//
//go:generate mamockgen ClientFactory
package clientfactory

import (
	"context"
	"net/http"

	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// ClientFactory returns Kubernetes API clients for a ClusterTarget. The clients are
// built using credentials retrieved from the SecretProvider.
type ClientFactory interface {
	// GetClient returns a controller-runtime client for the given cluster.
	GetClient(ctx context.Context, cluster *v2pb.ClusterTarget) (client.Client, error)

	// GetHTTPClient returns an HTTP client for talking to user-space services in the
	// given cluster. The client is configured with TLS using the cluster's CA bundle
	// and authenticates with the bearer token.
	GetHTTPClient(ctx context.Context, cluster *v2pb.ClusterTarget) (*http.Client, error)

	// GetDynamicClient returns a dynamic client for the given cluster. Use this for
	// resources addressed by GVR rather than typed Go structs (e.g., Gateway API HTTPRoute).
	GetDynamicClient(ctx context.Context, cluster *v2pb.ClusterTarget) (dynamic.Interface, error)
}
