package kuberay

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
)

// k8sClient wraps a Kubernetes REST client configured for KubeRay resources.
//
// This internal type holds the configured REST client that can communicate with
// the ray.io/v1 API group in a Kubernetes cluster.
type k8sClient struct {
	RestClient rest.Interface // REST client for ray.io/v1 API operations
}

// newClient creates a new k8sClient configured for the KubeRay API group.
//
// The client is configured with:
//   - GroupVersion: ray.io/v1
//   - APIPath: /apis (standard for custom resource APIs)
//   - ContentType: JSON
//   - Serializer: Codec factory without conversion
//
// Returns an error if REST client construction fails.
func newClient(cfg *rest.Config) (*k8sClient, error) {
	config := *cfg
	config.GroupVersion = &SchemeGroupVersion
	config.APIPath = "/apis"
	config.ContentType = runtime.ContentTypeJSON
	config.NegotiatedSerializer = Codecs.WithoutConversion()
	client, err := rest.RESTClientFor(&config)
	if err != nil {
		return nil, err
	}

	return &k8sClient{RestClient: client}, nil
}

// NewRestClient creates a REST client for interacting with KubeRay resources.
//
// This function constructs a Kubernetes REST client configured to communicate
// with the ray.io/v1 API group. The returned client can perform CRUD operations
// on RayCluster and other KubeRay custom resources.
//
// The client is typically injected via FX dependency injection through the
// kuberay Module.
//
// Returns an error if client construction fails (e.g., invalid config).
func NewRestClient(config *rest.Config) (rest.Interface, error) {
	client, err := newClient(config)
	if err != nil {
		return nil, err
	}

	return client.RestClient, nil
}
