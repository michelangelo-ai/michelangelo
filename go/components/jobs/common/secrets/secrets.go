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
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

const (
	// ClientCertTagAnnotation names the Secret holding the PEM client certificate, under
	// tls.crt, that the Provider presents to a cluster requiring mTLS. Set it together with
	// ClientKeyTagAnnotation on the Cluster; the token tag then becomes optional.
	ClientCertTagAnnotation = "michelangelo/client-cert-tag"
	// ClientKeyTagAnnotation names the Secret holding the client certificate's PEM private
	// key, under tls.key.
	ClientKeyTagAnnotation = "michelangelo/client-key-tag"
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
	// Try in-cluster config first, then fall back to kubeconfig for local development
	cfg, err := rest.InClusterConfig()
	if err != nil {
		// Fall back to kubeconfig (controller-runtime's GetConfig handles this)
		cfg, err = ctrl.GetConfig()
		if err != nil {
			panic(err)
		}
	}
	k8sClusterClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		panic(err)
	}
	return InClusterClientSet{
		ClientSet: k8sClusterClient,
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

// retrieveRequiredSecretData is retrieveSecretData for data that must not be empty.
func (p Provider) retrieveRequiredSecretData(ctx context.Context, secretName, dataKey string) (string, error) {
	data, err := p.retrieveSecretData(ctx, secretName, dataKey)
	if err == nil && data == "" {
		err = fmt.Errorf("secret %s has no %s", secretName, dataKey)
	}
	return data, err
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

	certTag := cluster.GetAnnotations()[ClientCertTagAnnotation]
	keyTag := cluster.GetAnnotations()[ClientKeyTagAnnotation]
	if (certTag == "") != (keyTag == "") {
		return ClientAuth{}, fmt.Errorf("cluster %s/%s must set both %s and %s, or neither",
			cluster.Namespace, cluster.Name, ClientCertTagAnnotation, ClientKeyTagAnnotation)
	}

	// Get the certificate authority data secret
	caDataDecoded, err := p.retrieveSecretData(ctx, kubeClusterSpec.Rest.CaDataTag, "cadata")
	if err != nil {
		return ClientAuth{}, err
	}
	clientAuth := ClientAuth{
		CertificateAuthorityData: caDataDecoded,
	}

	// Get the client certificate and key secrets
	if certTag != "" {
		if clientAuth.ClientCertificateData, err = p.retrieveRequiredSecretData(ctx, certTag, "tls.crt"); err != nil {
			return ClientAuth{}, err
		}
		if clientAuth.ClientKeyData, err = p.retrieveRequiredSecretData(ctx, keyTag, "tls.key"); err != nil {
			return ClientAuth{}, err
		}
	}

	// Get the Token secret. A client certificate authenticates on its own, so with one
	// the token is read only when the cluster names it.
	if certTag == "" || kubeClusterSpec.Rest.TokenTag != "" {
		if clientAuth.ClientTokenData, err = p.retrieveSecretData(ctx, kubeClusterSpec.Rest.TokenTag, "token"); err != nil {
			return ClientAuth{}, err
		}
	}
	return clientAuth, nil
}

func (p Provider) GetSecretsForDataStore(ctx context.Context, jobObject runtime.Object, cluster *v2pb.Cluster) (map[string][]byte, error) {
	return nil, nil
}
