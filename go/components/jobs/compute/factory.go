//go:generate mamockgen Factory
package compute

import (
	"context"
	"fmt"
	"sync"
	"time"

	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/secrets"
	"github.com/michelangelo-ai/michelangelo/go/components/ray/kuberay"
)

const (
	_serviceName = "michelangelo-controllermgr"

	// _credentialReadTimeout bounds each SecretProvider read. A read served from an informer
	// cache waits for the informer to sync, which never happens if the controller cannot list
	// Secrets. The second read in GetClientSetForCluster runs while the cluster's lock is held, so
	// an unbounded read would block every lookup of that cluster that misses the cache.
	_credentialReadTimeout = 10 * time.Second
)

// Factory is the interface to get a REST client
// to a cluster. Consumers are expected to write
// utils with REST interface for their usage.
type Factory interface {
	GetClientSetForCluster(*v2pb.Cluster) (*ClientSet, error)
}

// ClientSet keeps rest clients
// corresponding to different schemas.
type ClientSet struct {
	Ray       rest.Interface
	Spark     rest.Interface
	CoreV1    rest.Interface
	ComputeV1 rest.Interface
}

// factory caches one ClientSet per cluster and reuses it only while the SecretProvider returns
// the credentials it was built from. A SecretProvider error fails the lookup rather than falling
// back to a cached ClientSet whose credentials may have been revoked.
type factory struct {
	clients               sync.Map // cache key string → cachedClientSet
	locks                 sync.Map // cache key string → *sync.Mutex
	secretProvider        secrets.SecretProvider
	credentialReadTimeout time.Duration
}

// cachedClientSet is a ClientSet together with the credentials it was built from.
type cachedClientSet struct {
	auth      secrets.ClientAuth
	clientSet *ClientSet
}

// NewClientSetFactory provides constructor for the factory
func NewClientSetFactory(secretProvider secrets.SecretProvider) Factory {
	return &factory{
		secretProvider:        secretProvider,
		credentialReadTimeout: _credentialReadTimeout,
	}
}

func (f *factory) GetClientSetForCluster(c *v2pb.Cluster) (*ClientSet, error) {
	key := getCacheKey(c)
	auth, err := f.getClusterClientAuth(c)
	if err != nil {
		return nil, err
	}

	// optimize the steady state read path by using a sync map as opposed
	// to taking a lock
	if cs, ok := f.load(key, auth); ok {
		return cs, nil
	}

	mu := f.lockFor(key)
	mu.Lock()
	defer mu.Unlock()

	// Re-read under the lock so a caller holding pre-rotation credentials cannot replace a
	// ClientSet built from newer ones.
	auth, err = f.getClusterClientAuth(c)
	if err != nil {
		return nil, err
	}
	if cs, ok := f.load(key, auth); ok {
		return cs, nil
	}

	cs, err := newClientSet(getServerAddress(c), auth)
	if err != nil {
		return nil, fmt.Errorf(
			"build client set for cluster %s/%s: %w", c.GetNamespace(), c.GetName(), err)
	}
	// TODO(#2415): close idle connections of the replaced ClientSet and evict entries for deleted clusters.
	f.clients.Store(key, cachedClientSet{auth: auth, clientSet: cs})

	return cs, nil
}

// lockFor returns the mutex that serializes ClientSet builds for one cache key, so a blocked
// credential read for one cluster does not stall lookups for the others.
func (f *factory) lockFor(key string) *sync.Mutex {
	mu, _ := f.locks.LoadOrStore(key, &sync.Mutex{})
	return mu.(*sync.Mutex)
}

// load returns the ClientSet cached under key if it was built from auth.
func (f *factory) load(key string, auth secrets.ClientAuth) (*ClientSet, bool) {
	entry, ok := f.clients.Load(key)
	if !ok {
		return nil, false
	}
	cached, ok := entry.(cachedClientSet)
	if !ok || cached.auth != auth {
		return nil, false
	}
	return cached.clientSet, true
}

func (f *factory) getClusterClientAuth(c *v2pb.Cluster) (secrets.ClientAuth, error) {
	ctx, cancel := context.WithTimeout(context.Background(), f.credentialReadTimeout)
	defer cancel()

	auth, err := f.secretProvider.GetClusterClientAuth(ctx, c)
	// TODO(#2415): wrap with a sentinel (e.g. ErrCredentialsUnavailable) so callers can requeue instead of failing the job.
	if err != nil {
		return secrets.ClientAuth{}, fmt.Errorf(
			"get client auth for cluster %s/%s: %w", c.GetNamespace(), c.GetName(), err)
	}
	return auth, nil
}

func newClientSet(server string, auth secrets.ClientAuth) (*ClientSet, error) {
	cfg, err := GetKubeClientConfigFromConfiguration(
		_serviceName, server, metav1.NamespaceAll, &auth, true)
	if err != nil {
		return nil, fmt.Errorf("build rest config: %w", err)
	}

	ray, err := kuberay.NewRestClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("build kuberay client: %w", err)
	}

	core, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("build core client: %w", err)
	}

	return &ClientSet{
		Ray:    ray,
		CoreV1: core.CoreV1().RESTClient(),
	}, nil
}

// getCacheKey identifies a cluster by its namespace, name and server address. Two Cluster objects
// that point at the same API server may carry different credentials, so they need separate
// entries. The address is in the key so that a Cluster moved to another server gets a new
// ClientSet.
func getCacheKey(c *v2pb.Cluster) string {
	return fmt.Sprintf("%s/%s@%s", c.GetNamespace(), c.GetName(), getServerAddress(c))
}

func getServerAddress(c *v2pb.Cluster) string {
	conn := c.Spec.GetKubernetes().GetRest()
	return fmt.Sprintf("%s:%s", conn.GetHost(), conn.GetPort())
}
