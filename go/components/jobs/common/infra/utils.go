package infra

import (
	"fmt"

	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/secrets"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/client-go/util/flowcontrol"
)

func NewClientConfigFromConfiguration(
	serviceName string,
	dnsPath string,
	namespace string,
	auth *secrets.ClientAuth,
	useJSON bool,
) (*rest.Config, error) {
	clientCmdConfig := getKubeConfigStruct(serviceName, dnsPath, namespace, auth)

	config, err := clientcmd.NewDefaultClientConfig(
		*clientCmdConfig,
		&clientcmd.ConfigOverrides{},
	).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("create kube config: %w", err)
	}

	// We want to rely on API Priority and Fairness for rate-limiting (i.e. server side rate-limit).
	// Hence, disabling the default client-side ratelimiter(instantiate an always accept rate limiter).
	config.RateLimiter = flowcontrol.NewFakeAlwaysRateLimiter()

	// Use json to connect if requested otherwise use protobuf
	config.ContentType = runtime.ContentTypeProtobuf
	if useJSON {
		config.ContentType = runtime.ContentTypeJSON
	}

	return rest.AddUserAgent(config, serviceName), nil
}

// Populates and returns the kubeconfig struct.
func getKubeConfigStruct(
	serviceName string,
	dnsPath string,
	namespace string,
	auth *secrets.ClientAuth,
) *api.Config {
	contextName := fmt.Sprintf("%s@%s", serviceName, namespace)
	return &api.Config{
		Kind:       "Config",
		APIVersion: "v1",
		Clusters: map[string]*api.Cluster{
			namespace: {
				Server:                   dnsPath,
				CertificateAuthorityData: []byte(auth.CertificateAuthorityData),
			},
		},
		AuthInfos: map[string]*api.AuthInfo{
			serviceName: {
				Token: auth.ClientTokenData,
			},
		},
		Contexts: map[string]*api.Context{
			contextName: {
				Cluster:  namespace,
				AuthInfo: serviceName,
			},
		},
		CurrentContext: contextName,
	}
}
