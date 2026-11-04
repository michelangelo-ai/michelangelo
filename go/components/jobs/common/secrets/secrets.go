//go:generate mamockgen SecretProvider
package secrets

import (
	"context"
	"fmt"

	"go.uber.org/fx"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// SecretProvider defines the interface for secret management.
//
// GetClusterClientAuth is called on every compute client lookup so that rotated credentials
// are picked up, so it should be served from memory (for example an informer cache) rather
// than a remote call per request.
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
	kubeClient client.Client
	logger     *zap.Logger
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

	// KubeClient is expected to be the controller manager's client, whose reads are served
	// from the shared informer cache.
	KubeClient client.Client
	Logger     *zap.Logger
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
			kubeClient: p.KubeClient,
			logger:     p.Logger,
		},
	}
}

// GetKubeSecretName gets the k8s secret name using the job name
func GetKubeSecretName(jobName string) string {
	return constants.SecretNamePrefix + jobName
}

// retrieveAndDecodeSecret retrieves and decodes a secret from the Kubernetes cluster
func (p Provider) retrieveSecretData(ctx context.Context, secretName, dataKey string) (string, error) {
	secret := &corev1.Secret{}
	key := types.NamespacedName{Namespace: "default", Name: secretName}
	if err := p.kubeClient.Get(ctx, key, secret); err != nil {
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

	conn := kubeClusterSpec.GetRest()

	// Get the certificate authority data secret
	caDataDecoded, err := p.retrieveSecretData(ctx, conn.GetCaDataTag(), "cadata")
	if err != nil {
		return ClientAuth{}, err
	}

	// Get the Token secret
	clientTokenDecoded, err := p.retrieveSecretData(ctx, conn.GetTokenTag(), "token")
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
