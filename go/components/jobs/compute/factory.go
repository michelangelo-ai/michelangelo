//go:generate mamockgen Factory
package compute

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"

	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/secrets"
	"github.com/michelangelo-ai/michelangelo/go/components/ray/kuberay"
)

const _serviceName = "michelangelo-controllermgr"

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

type factory struct {
	// clients maps a cluster key to a cachedClientSet. A cache hit still requires the
	// current credentials to match the fingerprint recorded alongside the cached entry
	// (see authFingerprint): GetClusterClientAuth is consulted on every lookup, so a
	// rotated token or CA/client cert is picked up on the next call instead of lingering
	// until a restart, while unchanged credentials still skip rebuilding the clients.
	clients        sync.Map // key string -> cachedClientSet
	secretProvider secrets.SecretProvider

	m sync.Mutex
}

// cachedClientSet pairs a built *ClientSet with the fingerprint of the credentials used
// to build it, so a later lookup can tell whether those credentials are still current.
type cachedClientSet struct {
	fingerprint string
	clientSet   *ClientSet
}

// NewClientSetFactory provides constructor for the factory
func NewClientSetFactory(secretProvider secrets.SecretProvider) Factory {
	return &factory{
		secretProvider: secretProvider,
	}
}

func (f *factory) GetClientSetForCluster(c *v2pb.Cluster) (*ClientSet, error) {
	key, err := f.getClusterKey(c)
	if err != nil {
		return nil, err
	}

	auth, err := f.secretProvider.GetClusterClientAuth(context.Background(), c)
	if err != nil {
		return nil, fmt.Errorf("client cfg err:%v", fmt.Errorf("failed to get cluster client auth: %w", err))
	}
	fingerprint := authFingerprint(auth)

	// optimize the steady state read path by using a sync map as opposed
	// to taking a lock
	if cached, ok := f.clients.Load(key); ok {
		if entry := cached.(cachedClientSet); entry.fingerprint == fingerprint {
			return entry.clientSet, nil
		}
	}

	// protect creating new clients with a mutex because creating client is not
	// guaranteed to be thread-safe
	f.m.Lock()
	defer f.m.Unlock()

	// double check lock
	if cached, ok := f.clients.Load(key); ok {
		if entry := cached.(cachedClientSet); entry.fingerprint == fingerprint {
			return entry.clientSet, nil
		}
	}

	cfg, err := f.getClientCfg(&auth, key)
	if err != nil {
		return nil, fmt.Errorf("client cfg err:%v", err)
	}

	ray, err := kuberay.NewRestClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("kuberay client err:%w", err)
	}

	core, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("core client err:%w", err)
	}

	cs := &ClientSet{
		Ray:    ray,
		CoreV1: core.CoreV1().RESTClient(),
	}
	f.clients.Store(key, cachedClientSet{fingerprint: fingerprint, clientSet: cs})

	return cs, nil
}

func (f *factory) getClientCfg(auth *secrets.ClientAuth, key string) (*rest.Config, error) {
	cfg, err := GetKubeClientConfigFromConfiguration(
		_serviceName, key, metav1.NamespaceAll, auth, true)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

// authFingerprint hashes the credentials in auth so a cached ClientSet can be compared
// against the SecretProvider's current answer without holding the raw token/key material
// in the cache map or doing a byte-for-byte comparison on every lookup.
func authFingerprint(auth secrets.ClientAuth) string {
	sum := sha256.Sum256([]byte(
		auth.CertificateAuthorityData + "\x00" +
			auth.ClientCertificateData + "\x00" +
			auth.ClientKeyData + "\x00" +
			auth.ClientTokenData,
	))
	return hex.EncodeToString(sum[:])
}

func (f *factory) getClusterKey(c *v2pb.Cluster) (string, error) {
	key := fmt.Sprintf(
		"%s:%s", c.Spec.GetKubernetes().GetRest().GetHost(), c.Spec.GetKubernetes().GetRest().GetPort())
	return key, nil
}
