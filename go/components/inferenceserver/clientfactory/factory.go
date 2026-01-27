package clientfactory

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"sync"
	"time"

	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/client-go/util/flowcontrol"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/secrets"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

const serviceName = "michelangelo-inferenceserver"

var _ ClientFactory = &defaultClientFactory{}

// defaultClientFactory implements the ClientFactory interface.
type defaultClientFactory struct {
	defaultClient  client.Client
	secretProvider secrets.SecretProvider
	scheme         *runtime.Scheme
	logger         *zap.Logger

	// controlPlaneClusterId is the cluster ID that represents the control plane cluster.
	controlPlaneClusterId string

	// Cache for remote cluster clients
	clients sync.Map
	mu      sync.Mutex
}

// NewClientFactory creates a new ClientFactory instance.
// defaultClient is the in-cluster client to use for the control plane cluster returned when cluster ID matches the control plane cluster ID.
// secretProvider retrieves credentials for remote clusters.
// scheme is the runtime scheme to use for the clients.
func NewClientFactory(
	defaultClient client.Client,
	secretProvider secrets.SecretProvider,
	scheme *runtime.Scheme,
	logger *zap.Logger,
	controlPlaneClusterId string,
) ClientFactory {
	return &defaultClientFactory{
		defaultClient:         defaultClient,
		secretProvider:        secretProvider,
		scheme:                scheme,
		logger:                logger,
		controlPlaneClusterId: controlPlaneClusterId,
	}
}

// GetClient returns a controller-runtime client for the given cluster target.
func (f *defaultClientFactory) GetClient(ctx context.Context, cluster *v2pb.ClusterTarget) (client.Client, error) {
	// If the cluster ID matches the control plane cluster ID, returns the default client.
	if f.controlPlaneClusterId != "" && cluster.ClusterId == f.controlPlaneClusterId {
		f.logger.Debug("Using in-cluster client for control plane cluster",
			zap.String("clusterId", cluster.ClusterId))
		return f.defaultClient, nil
	}

	// For remote clusters, validate kubernetes config with connection details is provided
	k8sConfig := cluster.GetKubernetes()
	if k8sConfig == nil || k8sConfig.Host == "" || k8sConfig.Port == "" {
		return nil, fmt.Errorf("missing kubernetes connection details for cluster %s (host and port required)", cluster.ClusterId)
	}

	// For remote clusters, creates a client using the provided connection configuration.
	// Create a cache key from the connection spec
	key := f.getClientKey(cluster)

	// check if client already exists
	if cachedClient, ok := f.clients.Load(key); ok {
		return cachedClient.(client.Client), nil
	}

	// create new client with mutex protection
	f.mu.Lock()
	defer f.mu.Unlock()

	// Double-check after acquiring lock
	if cachedClient, ok := f.clients.Load(key); ok {
		return cachedClient.(client.Client), nil
	}

	// Get authentication credentials from secret provider
	auth, err := f.secretProvider.GetClientAuth(ctx, cluster)
	if err != nil {
		return nil, fmt.Errorf("failed to get client auth for %s: %w",
			cluster.ClusterId, err)
	}

	// Build REST config
	// Note: host field should already include the scheme (e.g., "https://host.docker.internal")
	server := fmt.Sprintf("%s:%s", cluster.GetKubernetes().GetHost(), cluster.GetKubernetes().GetPort())
	cfg, err := f.getKubeClientConfig(server, &auth)
	if err != nil {
		return nil, fmt.Errorf("failed to create kube config for %s: %w", server, err)
	}

	// Create a new controller-runtime client
	newClient, err := client.New(cfg, client.Options{
		Scheme: f.scheme,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create client for %s: %w", server, err)
	}
	f.clients.Store(key, newClient)

	return newClient, nil
}

// GetHTTPClient returns an HTTP client configured for the given cluster target.
func (f *defaultClientFactory) GetHTTPClient(ctx context.Context, cluster *v2pb.ClusterTarget) (*http.Client, error) {
	// If the cluster ID matches the configured control plane cluster ID, returns a simple HTTP client.
	if f.controlPlaneClusterId != "" && cluster.ClusterId == f.controlPlaneClusterId {
		f.logger.Debug("Using in-cluster HTTP client",
			zap.String("clusterId", cluster.ClusterId))
		return &http.Client{
			Timeout: 10 * time.Second,
		}, nil
	}

	// For remote clusters, validate kubernetes config with connection details is provided
	k8sConfig := cluster.GetKubernetes()
	if k8sConfig == nil || k8sConfig.Host == "" || k8sConfig.Port == "" {
		return nil, fmt.Errorf("missing kubernetes connection details for cluster %s (host and port required)", cluster.ClusterId)
	}

	// For remote clusters, returns a client configured with TLS using credentials from the secret provider.
	// Get authentication credentials from secret provider
	auth, err := f.secretProvider.GetClientAuth(ctx, cluster)
	if err != nil {
		return nil, fmt.Errorf("failed to get client auth for %s: %w",
			cluster.ClusterId, err)
	}

	// Create a certificate pool with the CA certificate
	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM([]byte(auth.CertificateAuthorityData)) {
		return nil, fmt.Errorf("failed to parse CA certificate for %s",
			cluster.ClusterId)
	}

	// Create TLS config with the CA certificate
	tlsConfig := &tls.Config{
		RootCAs:    caCertPool,
		MinVersion: tls.VersionTLS12,
	}

	// Create transport with TLS config and bearer token
	transport := &bearerTokenRoundTripper{
		token: auth.ClientTokenData,
		rt: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
	}

	return &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}, nil
}

// bearerTokenRoundTripper adds a bearer token to each request.
type bearerTokenRoundTripper struct {
	token string
	rt    http.RoundTripper
}

func (rt *bearerTokenRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("Authorization", "Bearer "+rt.token)
	return rt.rt.RoundTrip(req)
}

// getClientKey generates a unique key for caching clients based on connection spec.
func (f *defaultClientFactory) getClientKey(cluster *v2pb.ClusterTarget) string {
	return fmt.Sprintf("%s:%s:%s", cluster.ClusterId, cluster.GetKubernetes().GetHost(), cluster.GetKubernetes().GetPort())
}

// getKubeClientConfig builds a REST config from connection details and auth.
func (f *defaultClientFactory) getKubeClientConfig(server string, auth *secrets.ClientAuth) (*rest.Config, error) {
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
func (f *defaultClientFactory) getKubeConfigStruct(server string, auth *secrets.ClientAuth) *api.Config {
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
