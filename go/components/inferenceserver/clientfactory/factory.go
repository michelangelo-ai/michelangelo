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

	// httpIdleConnTimeout closes, once idle, connections that a caller still holding a replaced
	// client opens after closeIdleConnections has run.
	httpIdleConnTimeout = 90 * time.Second

	// secretReadTimeout bounds each SecretProvider read, which may run while the cluster's build
	// lock is held.
	secretReadTimeout = 10 * time.Second
)

var _ ClientFactory = &remoteClientFactory{}

type idleConnectionsCloser interface {
	CloseIdleConnections()
}

// remoteClientFactory builds and caches Kubernetes clients for ClusterTargets.
//
// Clients are cached per cacheKey, so a steady-state reconcile does not rebuild a TLS transport on
// every lookup. A target whose Secret names change is cached under a new key, and the entry and
// build lock for the old names stay until the controller restarts.
//
// Each lookup re-reads the cluster's credentials and reuses the cached client only if they are
// unchanged. A SecretProvider error fails the lookup rather than serving a client built from
// credentials that may have been revoked.
type remoteClientFactory struct {
	secretProvider secrets.SecretProvider
	scheme         *runtime.Scheme
	logger         *zap.Logger

	kubeClients    clientCache[client.Client]
	httpClients    clientCache[*http.Client]
	dynamicClients clientCache[dynamic.Interface]
}

// cachedClient is a client together with the credentials it was built from.
type cachedClient[T any] struct {
	auth   secrets.ClientAuth
	client T
}

type clientCache[T any] struct {
	entries sync.Map
	locks   sync.Map // cache key -> *sync.Mutex
}

// buildLock returns the mutex that serializes builds of the client cached under key.
func (c *clientCache[T]) buildLock(key string) *sync.Mutex {
	lock, _ := c.locks.LoadOrStore(key, &sync.Mutex{})
	return lock.(*sync.Mutex)
}

// lookup returns the client cached under key if it was built from auth.
func (c *clientCache[T]) lookup(key string, auth secrets.ClientAuth) (T, bool) {
	var zero T
	entry, ok := c.entries.Load(key)
	if !ok {
		return zero, false
	}
	cached := entry.(cachedClient[T])
	if cached.auth != auth {
		return zero, false
	}
	return cached.client, true
}

func (c *clientCache[T]) swap(key string, entry cachedClient[T]) (cachedClient[T], bool) {
	previous, replaced := c.entries.Swap(key, entry)
	if !replaced {
		return cachedClient[T]{}, false
	}
	return previous.(cachedClient[T]), true
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
	return getOrBuild(ctx, f, &f.httpClients, cluster, "http", buildHTTPClient)
}

// GetDynamicClient returns a dynamic.Interface for the given ClusterTarget. Use this for
// resources addressed by GVR (e.g., Gateway API HTTPRoute).
func (f *remoteClientFactory) GetDynamicClient(ctx context.Context, cluster *v2pb.ClusterTarget) (dynamic.Interface, error) {
	return getOrBuild(ctx, f, &f.dynamicClients, cluster, "dynamic", buildDynamicClient)
}

// getOrBuild returns the client cached for the cluster if it was built from the cluster's
// current credentials. Otherwise it builds and caches a new client and closes the idle
// connections of the client it replaces under the same key.
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

	auth, err := f.currentAuth(ctx, cluster)
	if err != nil {
		return zero, fmt.Errorf("get client auth for cluster %q: %w", cluster.GetClusterId(), err)
	}
	key := cacheKey(cluster)
	if cached, ok := cache.lookup(key, auth); ok {
		return cached, nil
	}

	// Builds are serialized per client kind and cluster, so concurrent reconciles don't duplicate
	// TLS setup and a stalled Secret read for one cluster doesn't block the others.
	lock := cache.buildLock(key)
	lock.Lock()
	defer lock.Unlock()

	// Re-read under the lock so credentials read before a rotation can't replace a newer client.
	auth, err = f.currentAuth(ctx, cluster)
	if err != nil {
		return zero, fmt.Errorf("re-read client auth for cluster %q: %w", cluster.GetClusterId(), err)
	}
	if cached, ok := cache.lookup(key, auth); ok {
		return cached, nil
	}

	built, err := build(cluster, auth)
	if err != nil {
		return zero, fmt.Errorf("create %s client for cluster %q: %w", kind, cluster.GetClusterId(), err)
	}
	previous, replaced := cache.swap(key, cachedClient[T]{auth: auth, client: built})
	if !replaced {
		f.logBuild(kind, cluster, nil, auth)
		return built, nil
	}
	closeIdleConnections(previous.client)
	f.logBuild(kind, cluster, &previous.auth, auth)
	return built, nil
}

// logBuild records a client build. A rebuild, which has the previous credentials, also reports
// which of them changed. Secret values are never logged.
func (f *remoteClientFactory) logBuild(
	kind string,
	cluster *v2pb.ClusterTarget,
	previous *secrets.ClientAuth,
	auth secrets.ClientAuth,
) {
	fields := []zap.Field{
		zap.String("client", kind),
		zap.String("cluster_id", cluster.GetClusterId()),
		zap.String("host", cluster.GetKubernetes().GetHost()),
		zap.String("port", cluster.GetKubernetes().GetPort()),
		zap.String("ca_secret", cluster.GetKubernetes().GetCaDataTag()),
		zap.String("token_secret", cluster.GetKubernetes().GetTokenTag()),
	}
	if previous == nil {
		f.logger.Info("Built client for cluster", fields...)
		return
	}
	fields = append(fields,
		zap.Bool("token_changed", previous.ClientTokenData != auth.ClientTokenData),
		zap.Bool("ca_changed", previous.CertificateAuthorityData != auth.CertificateAuthorityData),
	)
	f.logger.Info("Rebuilt client for cluster after its credentials changed", fields...)
}

// currentAuth returns the cluster's credentials, rejecting an empty token or CA. A client built
// from an empty token would send no token, one built from an empty CA would trust the system
// roots, and either would replace a cached client that still works.
func (f *remoteClientFactory) currentAuth(
	ctx context.Context,
	cluster *v2pb.ClusterTarget,
) (secrets.ClientAuth, error) {
	ctx, cancel := context.WithTimeout(ctx, secretReadTimeout)
	defer cancel()
	auth, err := f.secretProvider.GetClientAuth(ctx, cluster)
	if err != nil {
		return secrets.ClientAuth{}, err
	}
	if auth.ClientTokenData == "" {
		return secrets.ClientAuth{}, fmt.Errorf("token from secret %q is empty",
			cluster.GetKubernetes().GetTokenTag())
	}
	if auth.CertificateAuthorityData == "" {
		return secrets.ClientAuth{}, fmt.Errorf("CA certificate from secret %q is empty",
			cluster.GetKubernetes().GetCaDataTag())
	}
	return auth, nil
}

// closeIdleConnections closes a replaced *http.Client's idle connections. Kube and dynamic
// clients have no such method: their transport comes from client-go's shared TLS cache, which
// other clients still use.
func closeIdleConnections(replaced any) {
	if closer, ok := replaced.(idleConnectionsCloser); ok {
		closer.CloseIdleConnections()
	}
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

func buildHTTPClient(cluster *v2pb.ClusterTarget, auth secrets.ClientAuth) (*http.Client, error) {
	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM([]byte(auth.CertificateAuthorityData)) {
		return nil, fmt.Errorf("parse CA certificate from secret %q: invalid PEM",
			cluster.GetKubernetes().GetCaDataTag())
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

// cacheKey produces a stable cache key for a ClusterTarget. It includes the Secret names so that
// two targets for one endpoint that reference different Secrets keep separate clients, instead of
// evicting each other on every lookup.
func cacheKey(cluster *v2pb.ClusterTarget) string {
	return fmt.Sprintf("%s|%s:%s|%s|%s",
		cluster.GetClusterId(),
		cluster.GetKubernetes().GetHost(),
		cluster.GetKubernetes().GetPort(),
		cluster.GetKubernetes().GetCaDataTag(),
		cluster.GetKubernetes().GetTokenTag(),
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

// CloseIdleConnections lets http.Client.CloseIdleConnections reach the wrapped transport.
func (rt *bearerTokenRoundTripper) CloseIdleConnections() {
	if closer, ok := rt.rt.(idleConnectionsCloser); ok {
		closer.CloseIdleConnections()
	}
}
