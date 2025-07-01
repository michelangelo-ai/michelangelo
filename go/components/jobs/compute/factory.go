// // We need an open source version of this file.
// package compute

// import (
// 	"fmt"
// 	"sync"

// 	v2beta1pb "michelangelo/api/v2beta1"

// 	infraAuth "code.uber.internal/infra/compute/k8s-auth"
// 	infraK8s "code.uber.internal/infra/compute/k8s-client"
// 	infraClient "code.uber.internal/infra/compute/k8s-crds/generated/client/clientset/versioned"
// 	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/ray/kuberay"
// 	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/spark/kubespark"
// 	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
// 	"k8s.io/client-go/kubernetes"
// 	"k8s.io/client-go/rest"
// )

// const _serviceName = "michelangelo-controllermgr"

// // Factory is the interface to get a REST client
// // to a cluster. Consumers are expected to write
// // utils with REST interface for their usage.
// type Factory interface {
// 	GetClientSetForCluster(*v2beta1pb.Cluster) (*ClientSet, error)
// }

// // ClientSet keeps rest clients
// // corresponding to different schemas.
// type ClientSet struct {
// 	Ray       rest.Interface
// 	Spark     rest.Interface
// 	CoreV1    rest.Interface
// 	ComputeV1 rest.Interface
// }

// type factory struct {
// 	zonalAuth map[string]*infraAuth.ClientAuth
// 	clients   sync.Map

// 	m sync.Mutex
// }

// // NewClientSetFactory provides constructor for the factory
// func NewClientSetFactory(zonalAuth map[string]*infraAuth.ClientAuth) Factory {
// 	return &factory{
// 		zonalAuth: zonalAuth,
// 	}
// }

// func (f *factory) GetClientSetForCluster(c *v2beta1pb.Cluster) (*ClientSet, error) {
// 	key, err := f.getClusterKey(c)
// 	if err != nil {
// 		return nil, err
// 	}

// 	// optimize the steady state read path by using a sync map as opposed
// 	// to taking a lock
// 	if cs, ok := f.clients.Load(key); ok {
// 		return cs.(*ClientSet), nil
// 	}

// 	// protect creating new clients with a mutex because creating client is not
// 	// guaranteed to be thread-safe
// 	f.m.Lock()
// 	defer f.m.Unlock()

// 	// double check lock
// 	if cs, ok := f.clients.Load(key); ok {
// 		return cs.(*ClientSet), nil
// 	}

// 	cfg, err := f.getClientCfg(c, key)
// 	if err != nil {
// 		return nil, fmt.Errorf("client cfg err:%v", err)
// 	}

// 	ray, err := kuberay.NewRestClient(cfg)
// 	if err != nil {
// 		return nil, fmt.Errorf("kuberay client err:%w", err)
// 	}

// 	spark, err := kubespark.NewRestClient(cfg)
// 	if err != nil {
// 		return nil, fmt.Errorf("kubespark client err:%w", err)
// 	}

// 	core, err := kubernetes.NewForConfig(cfg)
// 	if err != nil {
// 		return nil, fmt.Errorf("core client err:%w", err)
// 	}

// 	computev1, err := infraClient.NewForConfig(cfg)
// 	if err != nil {
// 		return nil, fmt.Errorf("infra client err:%w", err)
// 	}

// 	cs := &ClientSet{
// 		Ray:       ray,
// 		Spark:     spark,
// 		CoreV1:    core.CoreV1().RESTClient(),
// 		ComputeV1: computev1.ComputeV1beta1().RESTClient(),
// 	}
// 	f.clients.Store(key, cs)

// 	return cs, nil
// }

// func (f *factory) getClientCfg(c *v2beta1pb.Cluster, key string) (*rest.Config, error) {
// 	zone := c.Spec.Zone
// 	auth, ok := f.zonalAuth[zone]
// 	if !ok {
// 		return nil, fmt.Errorf("auth for zone %s not provided in the configuration", zone)
// 	}

// 	cfg, err := infraK8s.NewClientConfigFromConfiguration(
// 		_serviceName, key, metav1.NamespaceAll, auth, true)
// 	if err != nil {
// 		return nil, err
// 	}
// 	return cfg, nil
// }

// func (f *factory) getClusterKey(c *v2beta1pb.Cluster) (string, error) {
// 	key := fmt.Sprintf(
// 		"%s:%s", c.Spec.GetKubernetes().GetRest().GetHost(), c.Spec.GetKubernetes().GetRest().GetPort())
// 	return key, nil
// }
