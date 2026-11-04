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
	// Secrets, and the second read in GetClientSetForCluster runs while f.m is held.
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

// factory caches one ClientSet per cluster. Every lookup asks the SecretProvider for the
// cluster's current credentials and reuses the cached ClientSet only if it was built from the
// same credentials, so a rotated token or CA bundle takes effect on the next lookup without a
// controller restart. If the SecretProvider returns an error, the lookup fails instead of
// falling back to a ClientSet built from credentials that may have been revoked.
type factory struct {
	clients               sync.Map // cache key string → cachedClientSet
	secretProvider        secrets.SecretProvider
	credentialReadTimeout time.Duration

	m sync.Mutex
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

	// protect creating new clients with a mutex because creating client is not
	// guaranteed to be thread-safe
	f.m.Lock()
	defer f.m.Unlock()

	// Read the credentials again under the lock. A caller that read them just before a
	// rotation would otherwise replace the newer ClientSet with one built from older ones.
	auth, err = f.getClusterClientAuth(c)
	if err != nil {
		return nil, err
	}
	if cs, ok := f.load(key, auth); ok {
		return cs, nil
	}

	cs, err := newClientSet(getServerAddress(c), auth)
	if err != nil {
		return nil, err
	}
	f.clients.Store(key, cachedClientSet{auth: auth, clientSet: cs})

	return cs, nil
}

// load returns the ClientSet cached under key if it was built from auth.
func (f *factory) load(key string, auth secrets.ClientAuth) (*ClientSet, bool) {
	entry, ok := f.clients.Load(key)
	if !ok {
		return nil, false
	}
	cached := entry.(cachedClientSet)
	if cached.auth != auth {
		return nil, false
	}
	return cached.clientSet, true
}

func (f *factory) getClusterClientAuth(c *v2pb.Cluster) (secrets.ClientAuth, error) {
	ctx, cancel := context.WithTimeout(context.Background(), f.credentialReadTimeout)
	defer cancel()

	auth, err := f.secretProvider.GetClusterClientAuth(ctx, c)
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
		return nil, fmt.Errorf("client cfg err:%w", err)
	}

	ray, err := kuberay.NewRestClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("kuberay client err:%w", err)
	}

	core, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("core client err:%w", err)
	}

	return &ClientSet{
		Ray:    ray,
		CoreV1: core.CoreV1().RESTClient(),
	}, nil
}

// getCacheKey identifies a cluster by its Cluster object as well as its server address.
// Two Cluster objects that point at the same API server may carry different credentials, so
// keying on the address alone would make them rebuild each other's ClientSet on every lookup.
func getCacheKey(c *v2pb.Cluster) string {
	return fmt.Sprintf("%s/%s@%s", c.GetNamespace(), c.GetName(), getServerAddress(c))
}

func getServerAddress(c *v2pb.Cluster) string {
	conn := c.Spec.GetKubernetes().GetRest()
	return fmt.Sprintf("%s:%s", conn.GetHost(), conn.GetPort())
}
