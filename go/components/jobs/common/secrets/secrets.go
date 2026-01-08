//go:generate mamockgen SecretProvider
package secrets

import (
	"context"
	"fmt"

	"go.uber.org/fx"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"

	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// SecretProvider defines the interface for secret management
type SecretProvider interface {
	GetClusterClientAuth(ctx context.Context, cluster *v2pb.Cluster) (ClientAuth, error)
	GetSecretsForDataStore(ctx context.Context, jobObject runtime.Object, cluster *v2pb.Cluster) (map[string][]byte, error)
}

// Provider implements the SecretProvider interface.
//
// NOTE: This is a SAMPLE IMPLEMENTATION that stores secrets in the MA control plane K8s Cluster.
// This is NOT recommended for production use. For real deployments, use external secret
// management systems (e.g., HashiCorp Vault, AWS Secrets Manager) or explore utilities
// designed for sandbox/testing purposes.
type Provider struct {
	k8sClusterClient kubernetes.Interface
	logger           *zap.Logger
}

// ClientAuth contains the certs needed to be provided to the Kubernetes API server.
type ClientAuth struct {
	// CertificateAuthorityData contains PEM-encoded certificate authority certificates.
	CertificateAuthorityData string
	// ClientCertificateData contains PEM-encoded data from a client cert file for TLS.
	ClientCertificateData string
	// ClientKeyData contains PEM-encoded data from a client key file for TLS.
	ClientKeyData string
	// ClientTokenData contains the token for the client.
	ClientTokenData string
}

// Params has params for constructor
type Params struct {
	fx.In

	ClientSet kubernetes.Interface `name:"inClusterClientSet"`
	Logger    *zap.Logger
}

type InClusterClientSet struct {
	fx.Out

	ClientSet kubernetes.Interface `name:"inClusterClientSet"`
}

func NewInClusterClientSet() InClusterClientSet {
	// cfg, _ := rest.InClusterConfig()
	// if err != nil {
	// 	panic(err)
	// }
	// k8sClusterClient, err := kubernetes.NewForConfig(cfg)
	// if err != nil {
	// 	panic(err)
	// }
	return InClusterClientSet{
		ClientSet: nil,
	}
}

// Result has the result of the constructor
type Result struct {
	fx.Out

	SecretProvider SecretProvider
}

// New provides new Secrets generator
func New(p Params) Result {
	return Result{
		SecretProvider: Provider{
			k8sClusterClient: p.ClientSet,
			logger:           p.Logger,
		},
	}
}

// GetKubeSecretName gets the k8s secret name using the job name
func GetKubeSecretName(jobName string) string {
	return constants.SecretNamePrefix + jobName
}

// retrieveAndDecodeSecret retrieves and decodes a secret from the Kubernetes cluster
func (p Provider) retrieveSecretData(ctx context.Context, secretName, dataKey string) (string, error) {
	secret, err := p.k8sClusterClient.CoreV1().Secrets("default").Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get %s: %w", dataKey, err)
	}
	return string(secret.Data[dataKey]), nil
}

// GetClusterClientAuth retrieves the client authentication data for a given cluster
func (p Provider) GetClusterClientAuth(ctx context.Context, cluster *v2pb.Cluster) (ClientAuth, error) {
	var kubeClusterSpec *v2pb.KubernetesSpec

	switch cluster.Spec.GetCluster().(type) {
	case *v2pb.ClusterSpec_Kubernetes:
		kubeClusterSpec = cluster.Spec.GetKubernetes()
	default:
		return ClientAuth{}, fmt.Errorf("cluster type %s not supported", cluster.Spec.GetCluster())
	}

	// Get the certificate authority data secret
	caDataDecoded, err := p.retrieveSecretData(ctx, kubeClusterSpec.Rest.CaDataTag, "cadata")
	if err != nil {
		return ClientAuth{}, err
	}

	// Get the Token secret
	clientTokenDecoded, err := p.retrieveSecretData(ctx, kubeClusterSpec.Rest.TokenTag, "token")
	if err != nil {
		return ClientAuth{}, err
	}

	clientAuth := ClientAuth{
		CertificateAuthorityData: caDataDecoded,
		ClientTokenData:          clientTokenDecoded,
	}
	return clientAuth, nil
}

func (p Provider) GetSecretsForDataStore(ctx context.Context, jobObject runtime.Object, cluster *v2pb.Cluster) (map[string][]byte, error) {
	return nil, nil
}
