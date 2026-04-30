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
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/client-go/util/flowcontrol"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/clientfactory/secrets"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

const (
	userAgent = "michelangelo-inferenceserver"

	// httpClientTimeout caps any single HTTP request issued through the factory.
	httpClientTimeout = 30 * time.Second
)

var _ ClientFactory = &remoteClientFactory{}

// remoteClientFactory builds and caches Kubernetes clients for ClusterTargets.
//
// Both controller-runtime clients and HTTP clients are cached keyed by the connection
// tuple (cluster_id + host + port) so a steady-state reconcile does not rebuild a TLS
// transport on every actor invocation.
type remoteClientFactory struct {
	secretProvider secrets.SecretProvider
	scheme         *runtime.Scheme
	logger         *zap.Logger

	kubeClients sync.Map // key string → client.Client
	httpClients sync.Map // key string → *http.Client
	mu          sync.Mutex
}

// NewRemoteClientFactory constructs a ClientFactory.
//
// Parameters:
//   - secretProvider: source of CA bundles and bearer tokens for remote clusters.
//   - scheme: the runtime.Scheme used to build remote clients (must include all CRDs
//     the controller will read/write on remote clusters).
//   - logger: structured logger.
func NewRemoteClientFactory(
	secretProvider secrets.SecretProvider,
	scheme *runtime.Scheme,
	logger *zap.Logger,
) ClientFactory {
	return &remoteClientFactory{
		secretProvider: secretProvider,
		scheme:         scheme,
		logger:         logger.With(zap.String("component", "clientfactory")),
	}
}

// GetClient returns a controller-runtime client for the given ClusterTarget. The
// client is built (and cached) using credentials retrieved from the SecretProvider.
func (f *remoteClientFactory) GetClient(ctx context.Context, cluster *v2pb.ClusterTarget) (client.Client, error) {
	if cluster.GetKubernetes() == nil {
		return nil, fmt.Errorf("cluster %q has no kubernetes connection spec", cluster.GetClusterId())
	}

	key := cacheKey(cluster)
	if cached, ok := f.kubeClients.Load(key); ok {
		return cached.(client.Client), nil
	}

	// Building a kube client requires hitting the SecretProvider and constructing TLS
	// state. Guard with a mutex so concurrent reconciles for the same cluster don't
	// duplicate work.
	f.mu.Lock()
	defer f.mu.Unlock()

	if cached, ok := f.kubeClients.Load(key); ok {
		return cached.(client.Client), nil
	}

	cfg, err := f.buildRESTConfig(ctx, cluster)
	if err != nil {
		return nil, fmt.Errorf("build REST config for cluster %q: %w", cluster.GetClusterId(), err)
	}

	kubeClient, err := client.New(cfg, client.Options{Scheme: f.scheme})
	if err != nil {
		return nil, fmt.Errorf("create kube client for cluster %q: %w", cluster.GetClusterId(), err)
	}

	f.kubeClients.Store(key, kubeClient)
	f.logger.Info("Built kube client for cluster",
		zap.String("cluster_id", cluster.GetClusterId()),
		zap.String("host", cluster.GetKubernetes().GetHost()))
	return kubeClient, nil
}

// GetHTTPClient returns an HTTP client whose transport authenticates with a bearer
// token over TLS validated against the cluster's CA.
func (f *remoteClientFactory) GetHTTPClient(ctx context.Context, cluster *v2pb.ClusterTarget) (*http.Client, error) {
	if cluster.GetKubernetes() == nil {
		return nil, fmt.Errorf("cluster %q has no kubernetes connection spec", cluster.GetClusterId())
	}

	key := cacheKey(cluster)
	if cached, ok := f.httpClients.Load(key); ok {
		return cached.(*http.Client), nil
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	if cached, ok := f.httpClients.Load(key); ok {
		return cached.(*http.Client), nil
	}

	auth, err := f.secretProvider.GetClientAuth(ctx, cluster)
	if err != nil {
		return nil, fmt.Errorf("get client auth for cluster %q: %w", cluster.GetClusterId(), err)
	}

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM([]byte(auth.CertificateAuthorityData)) {
		return nil, fmt.Errorf("parse CA certificate for cluster %q: invalid PEM", cluster.GetClusterId())
	}

	httpClient := &http.Client{
		Transport: &bearerTokenRoundTripper{
			token: auth.ClientTokenData,
			rt: &http.Transport{
				TLSClientConfig: &tls.Config{
					RootCAs:    caPool,
					MinVersion: tls.VersionTLS12,
				},
			},
		},
		Timeout: httpClientTimeout,
	}

	f.httpClients.Store(key, httpClient)
	f.logger.Info("Built HTTP client for cluster",
		zap.String("cluster_id", cluster.GetClusterId()),
		zap.String("host", cluster.GetKubernetes().GetHost()))
	return httpClient, nil
}

// buildRESTConfig assembles a *rest.Config for a cluster from the connection spec
// and credentials retrieved from the SecretProvider.
func (f *remoteClientFactory) buildRESTConfig(ctx context.Context, cluster *v2pb.ClusterTarget) (*rest.Config, error) {
	auth, err := f.secretProvider.GetClientAuth(ctx, cluster)
	if err != nil {
		return nil, fmt.Errorf("get client auth: %w", err)
	}

	server := fmt.Sprintf("%s:%s", cluster.GetKubernetes().GetHost(), cluster.GetKubernetes().GetPort())

	// Build a kubeconfig in-memory and resolve it through clientcmd, which handles
	// CA-data + bearer-token wiring via its established schema.
	apiCfg := &clientcmdapi.Config{
		Kind:       "Config",
		APIVersion: "v1",
		Clusters: map[string]*clientcmdapi.Cluster{
			"remote": {
				Server:                   server,
				CertificateAuthorityData: []byte(auth.CertificateAuthorityData),
			},
		},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{
			userAgent: {Token: auth.ClientTokenData},
		},
		Contexts: map[string]*clientcmdapi.Context{
			userAgent + "@remote": {
				Cluster:  "remote",
				AuthInfo: userAgent,
			},
		},
		CurrentContext: userAgent + "@remote",
	}

	cfg, err := clientcmd.NewDefaultClientConfig(*apiCfg, &clientcmd.ConfigOverrides{}).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("resolve client config: %w", err)
	}

	// Disable client-side rate limiting; rely on the API server's Priority and Fairness.
	cfg.RateLimiter = flowcontrol.NewFakeAlwaysRateLimiter()
	cfg.ContentType = runtime.ContentTypeJSON

	return rest.AddUserAgent(cfg, userAgent), nil
}

// cacheKey produces a stable cache key for a ClusterTarget.
func cacheKey(cluster *v2pb.ClusterTarget) string {
	return fmt.Sprintf("%s|%s:%s",
		cluster.GetClusterId(),
		cluster.GetKubernetes().GetHost(),
		cluster.GetKubernetes().GetPort(),
	)
}

// bearerTokenRoundTripper injects a bearer Authorization header on every request.
type bearerTokenRoundTripper struct {
	token string
	rt    http.RoundTripper
}

// RoundTrip clones the request to avoid mutating the caller's headers, then forwards
// to the underlying transport.
func (rt *bearerTokenRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.Header.Set("Authorization", "Bearer "+rt.token)
	return rt.rt.RoundTrip(clone)
}
