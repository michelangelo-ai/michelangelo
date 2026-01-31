//go:generate mamockgen Factory
package compute

import (
	"context"
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
	clients        sync.Map
	secretProvider secrets.SecretProvider

	m sync.Mutex
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

	// optimize the steady state read path by using a sync map as opposed
	// to taking a lock
	if cs, ok := f.clients.Load(key); ok {
		return cs.(*ClientSet), nil
	}

	// protect creating new clients with a mutex because creating client is not
	// guaranteed to be thread-safe
	f.m.Lock()
	defer f.m.Unlock()

	// double check lock
	if cs, ok := f.clients.Load(key); ok {
		return cs.(*ClientSet), nil
	}

	cfg, err := f.getClientCfg(c, key)
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
	f.clients.Store(key, cs)

	return cs, nil
}

func (f *factory) getClientCfg(c *v2pb.Cluster, key string) (*rest.Config, error) {
	auth, err := f.secretProvider.GetClusterClientAuth(context.Background(), c)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster client auth: %w", err)
	}
	cfg, err := GetKubeClientConfigFromConfiguration(
		_serviceName, key, metav1.NamespaceAll, &auth, true)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

func (f *factory) getClusterKey(c *v2pb.Cluster) (string, error) {
	key := fmt.Sprintf(
		"%s:%s", c.Spec.GetKubernetes().GetRest().GetHost(), c.Spec.GetKubernetes().GetRest().GetPort())
	return key, nil
}
