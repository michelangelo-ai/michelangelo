//go:generate mamockgen ClientFactory

package clientfactory

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"

	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// ClientFactory provides Kubernetes clients for connecting to clusters.
// When connectionSpec is nil, it returns the default in-cluster client.
// When connectionSpec is provided, it creates a client for the specified remote cluster.
type ClientFactory interface {
	// GetClient returns a controller-runtime client for the given connection spec.
	// If connectionSpec is nil, returns the default in-cluster client.
	GetClient(ctx context.Context, connectionSpec *v2pb.ConnectionSpec) (client.Client, error)
}
