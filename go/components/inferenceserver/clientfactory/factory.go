package clientfactory

import (
	"context"
	"fmt"
	"sync"

	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/client-go/util/flowcontrol"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

const serviceName = "michelangelo-inferenceserver"

// ClientAuth contains the credentials needed to authenticate to a Kubernetes cluster.
type ClientAuth struct {
	// CertificateAuthorityData contains PEM-encoded certificate authority certificates.
	CertificateAuthorityData string
	// ClientTokenData contains the bearer token for the client.
	ClientTokenData string
}

// SecretProvider retrieves cluster authentication credentials from a secret store.
type SecretProvider interface {
	// GetClientAuth retrieves the authentication credentials for a given connection spec.
	GetClientAuth(ctx context.Context, connectionSpec *v2pb.ConnectionSpec) (ClientAuth, error)
}

var _ ClientFactory = &defaultClientFactory{} // ensure implementation satisfies interface

// defaultClientFactory implements the ClientFactory interface.
type defaultClientFactory struct {
	defaultClient  client.Client
	secretProvider SecretProvider
	scheme         *runtime.Scheme
	logger         *zap.Logger

	// Cache for remote cluster clients
	clients sync.Map
	mu      sync.Mutex
}

// NewClientFactory creates a new ClientFactory instance.
// defaultClient is the in-cluster client to use when connectionSpec is nil.
// secretProvider retrieves credentials for remote clusters.
// scheme is the runtime scheme to use for the clients.
func NewClientFactory(
	defaultClient client.Client,
	secretProvider SecretProvider,
	scheme *runtime.Scheme,
	logger *zap.Logger,
) ClientFactory {
	return &defaultClientFactory{
		defaultClient:  defaultClient,
		secretProvider: secretProvider,
		scheme:         scheme,
		logger:         logger,
	}
}

// GetClient returns a controller-runtime client for the given connection spec.
func (f *defaultClientFactory) GetClient(ctx context.Context, connectionSpec *v2pb.ConnectionSpec) (client.Client, error) {
	// If no connectionSpec provided, use the default in-cluster client
	if connectionSpec == nil {
		return f.defaultClient, nil
	}

	// Create a cache key from the connection spec
	key := f.getClientKey(connectionSpec)

	// Fast path: check if client already exists
	if cachedClient, ok := f.clients.Load(key); ok {
		return cachedClient.(client.Client), nil
	}

	// Slow path: create new client with mutex protection
	f.mu.Lock()
	defer f.mu.Unlock()

	// Double-check after acquiring lock
	if cachedClient, ok := f.clients.Load(key); ok {
		return cachedClient.(client.Client), nil
	}

	// Get authentication credentials from secret provider
	auth, err := f.secretProvider.GetClientAuth(ctx, connectionSpec)
	if err != nil {
		return nil, fmt.Errorf("failed to get client auth for %s:%s: %w",
			connectionSpec.Host, connectionSpec.Port, err)
	}

	// Build REST config
	server := fmt.Sprintf("https://%s:%s", connectionSpec.Host, connectionSpec.Port)
	cfg, err := f.getKubeClientConfig(server, &auth)
	if err != nil {
		return nil, fmt.Errorf("failed to create kube config for %s: %w", server, err)
	}

	// Create controller-runtime client
	newClient, err := client.New(cfg, client.Options{
		Scheme: f.scheme,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create client for %s: %w", server, err)
	}

	// Cache and return
	f.clients.Store(key, newClient)
	f.logger.Info("Created new client for remote cluster",
		zap.String("host", connectionSpec.Host),
		zap.String("port", connectionSpec.Port))

	return newClient, nil
}

// getClientKey generates a unique key for caching clients based on connection spec.
func (f *defaultClientFactory) getClientKey(connectionSpec *v2pb.ConnectionSpec) string {
	return fmt.Sprintf("%s:%s", connectionSpec.Host, connectionSpec.Port)
}

// getKubeClientConfig builds a REST config from connection details and auth.
func (f *defaultClientFactory) getKubeClientConfig(server string, auth *ClientAuth) (*rest.Config, error) {
	clientCmdConfig := f.getKubeConfigStruct(server, auth)

	config, err := clientcmd.NewDefaultClientConfig(
		*clientCmdConfig,
		&clientcmd.ConfigOverrides{},
	).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("create kube config: %w", err)
	}

	// Disable client-side rate limiting, rely on API Priority and Fairness
	config.RateLimiter = flowcontrol.NewFakeAlwaysRateLimiter()

	// Use JSON for content type
	config.ContentType = runtime.ContentTypeJSON

	return rest.AddUserAgent(config, serviceName), nil
}

// getKubeConfigStruct builds a kubeconfig struct from connection details.
func (f *defaultClientFactory) getKubeConfigStruct(server string, auth *ClientAuth) *api.Config {
	clusterName := "remote-cluster"
	contextName := fmt.Sprintf("%s@%s", serviceName, clusterName)

	return &api.Config{
		Kind:       "Config",
		APIVersion: "v1",
		Clusters: map[string]*api.Cluster{
			clusterName: {
				Server:                   server,
				CertificateAuthorityData: []byte(auth.CertificateAuthorityData),
			},
		},
		AuthInfos: map[string]*api.AuthInfo{
			serviceName: {
				Token: auth.ClientTokenData,
			},
		},
		Contexts: map[string]*api.Context{
			contextName: {
				Cluster:  clusterName,
				AuthInfo: serviceName,
			},
		},
		CurrentContext: contextName,
	}
}
