package kuberay

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
)

/*
TODO Move this to cluster-specific implementation
This rest client is written in a manner similar to how the rest client id
generated in MA 2.0 framework
*/

type k8sClient struct {
	RestClient rest.Interface
}

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

// NewRestClient provide a kuberay client
func NewRestClient(config *rest.Config) (rest.Interface, error) {
	client, err := newClient(config)
	if err != nil {
		return nil, err
	}

	return client.RestClient, nil
}
