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
	"k8s.io/client-go/dynamic"
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

	// httpIdleConnTimeout closes pooled connections that sit unused, including ones a
	// replaced HTTP client gets back from requests that were in flight when it was replaced.
	httpIdleConnTimeout = 90 * time.Second
)

var _ ClientFactory = &remoteClientFactory{}

// remoteClientFactory builds and caches Kubernetes clients for ClusterTargets.
//
// All three client types are cached keyed by the connection tuple (cluster_id + host + port)
// so a steady-state reconcile does not rebuild a TLS transport on every actor invocation.
//
// Every lookup asks the SecretProvider for the cluster's current credentials and reuses the
// cached client only if it was built from the same credentials. A rotated token or CA bundle
// therefore takes effect on the next lookup, without a controller restart. If the
// SecretProvider returns an error, the lookup fails instead of falling back to a client built
// from credentials that may have been revoked.
type remoteClientFactory struct {
	secretProvider secrets.SecretProvider
	scheme         *runtime.Scheme
	logger         *zap.Logger

	kubeClients    clientCache[client.Client]
	httpClients    clientCache[*http.Client]
	dynamicClients clientCache[dynamic.Interface]
	mu             sync.Mutex
}

// cachedClient is a client together with the credentials it was built from.
type cachedClient[T any] struct {
	auth   secrets.ClientAuth
	client T
}

// clientCache maps a cacheKey to the cachedClient built for it.
type clientCache[T any] struct {
	entries sync.Map
}

func (c *clientCache[T]) load(key string) (cachedClient[T], bool) {
	entry, ok := c.entries.Load(key)
	if !ok {
		return cachedClient[T]{}, false
	}
	return entry.(cachedClient[T]), true
}

func (c *clientCache[T]) store(key string, entry cachedClient[T]) {
	c.entries.Store(key, entry)
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
	return getOrBuild(ctx, f, &f.kubeClients, cluster, "kube", f.buildKubeClient)
}

// GetHTTPClient returns an HTTP client whose transport authenticates with a bearer
// token over TLS validated against the cluster's CA.
func (f *remoteClientFactory) GetHTTPClient(ctx context.Context, cluster *v2pb.ClusterTarget) (*http.Client, error) {
	return getOrBuild(ctx, f, &f.httpClients, cluster, "HTTP", buildHTTPClient)
}

// GetDynamicClient returns a dynamic.Interface for the given ClusterTarget. Use this for
// resources addressed by GVR (e.g., Gateway API HTTPRoute).
func (f *remoteClientFactory) GetDynamicClient(ctx context.Context, cluster *v2pb.ClusterTarget) (dynamic.Interface, error) {
	return getOrBuild(ctx, f, &f.dynamicClients, cluster, "dynamic", buildDynamicClient)
}

// getOrBuild returns the client cached for the cluster if it was built from the cluster's
// current credentials, and otherwise builds a new client and caches it in place of the old one.
func getOrBuild[T any](
	ctx context.Context,
	f *remoteClientFactory,
	cache *clientCache[T],
	cluster *v2pb.ClusterTarget,
	kind string,
	build func(cluster *v2pb.ClusterTarget, auth secrets.ClientAuth) (T, error),
) (T, error) {
	var zero T
	if cluster.GetKubernetes() == nil {
		return zero, fmt.Errorf("cluster %q has no kubernetes connection spec", cluster.GetClusterId())
	}

	auth, err := f.secretProvider.GetClientAuth(ctx, cluster)
	if err != nil {
		return zero, fmt.Errorf("get client auth for cluster %q: %w", cluster.GetClusterId(), err)
	}

	key := cacheKey(cluster)
	if cached, ok := cache.load(key); ok && cached.auth == auth {
		return cached.client, nil
	}

	// One mutex serializes every client build across all clusters and client kinds, so
	// concurrent reconciles don't duplicate TLS setup.
	f.mu.Lock()
	defer f.mu.Unlock()

	// Read the credentials again under the lock. A caller that read them just before a
	// rotation would otherwise replace the newer client with one built from older credentials.
	auth, err = f.secretProvider.GetClientAuth(ctx, cluster)
	if err != nil {
		return zero, fmt.Errorf("get client auth for cluster %q: %w", cluster.GetClusterId(), err)
	}

	previous, hadPrevious := cache.load(key)
	if hadPrevious && previous.auth == auth {
		return previous.client, nil
	}

	built, err := build(cluster, auth)
	if err != nil {
		return zero, fmt.Errorf("create %s client for cluster %q: %w", kind, cluster.GetClusterId(), err)
	}
	cache.store(key, cachedClient[T]{auth: auth, client: built})

	fields := []zap.Field{
		zap.String("client", kind),
		zap.String("cluster_id", cluster.GetClusterId()),
		zap.String("host", cluster.GetKubernetes().GetHost()),
	}
	if !hadPrevious {
		f.logger.Info("Built client for cluster", fields...)
		return built, nil
	}

	if replaced, ok := any(previous.client).(interface{ CloseIdleConnections() }); ok {
		replaced.CloseIdleConnections()
	}
	f.logger.Info("Rebuilt client for cluster after its credentials changed", fields...)
	return built, nil
}

func (f *remoteClientFactory) buildKubeClient(
	cluster *v2pb.ClusterTarget,
	auth secrets.ClientAuth,
) (client.Client, error) {
	cfg, err := buildRESTConfig(cluster, auth)
	if err != nil {
		return nil, fmt.Errorf("build REST config: %w", err)
	}
	return client.New(cfg, client.Options{Scheme: f.scheme})
}

func buildHTTPClient(_ *v2pb.ClusterTarget, auth secrets.ClientAuth) (*http.Client, error) {
	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM([]byte(auth.CertificateAuthorityData)) {
		return nil, fmt.Errorf("parse CA certificate: invalid PEM")
	}

	return &http.Client{
		Transport: &bearerTokenRoundTripper{
			token: auth.ClientTokenData,
			rt: &http.Transport{
				TLSClientConfig: &tls.Config{
					RootCAs:    caPool,
					MinVersion: tls.VersionTLS12,
				},
				IdleConnTimeout: httpIdleConnTimeout,
			},
		},
		Timeout: httpClientTimeout,
	}, nil
}

func buildDynamicClient(
	cluster *v2pb.ClusterTarget,
	auth secrets.ClientAuth,
) (dynamic.Interface, error) {
	cfg, err := buildRESTConfig(cluster, auth)
	if err != nil {
		return nil, fmt.Errorf("build REST config: %w", err)
	}
	return dynamic.NewForConfig(cfg)
}

// buildRESTConfig assembles a *rest.Config for a cluster from its connection spec and
// credentials.
func buildRESTConfig(cluster *v2pb.ClusterTarget, auth secrets.ClientAuth) (*rest.Config, error) {
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

// CloseIdleConnections forwards to the wrapped transport. http.Client.CloseIdleConnections
// only reaches a transport that has this method, so without it the call would do nothing.
func (rt *bearerTokenRoundTripper) CloseIdleConnections() {
	if closer, ok := rt.rt.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
}
