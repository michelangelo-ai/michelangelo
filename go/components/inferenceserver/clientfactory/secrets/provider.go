// Package secrets retrieves Kubernetes API credentials for ClusterTargets from
// a Kubernetes-backed secret store.
//
//go:generate mamockgen SecretProvider
package secrets

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// secretsNamespace is where cluster-specific credential secrets are expected to live.
// The secret names themselves are taken from ClusterTarget.Kubernetes.{CaDataTag,TokenTag}.
const secretsNamespace = "default"

// Keys within each Kubernetes Secret's .data map.
const (
	caDataKey = "cadata"
	tokenKey  = "token"
)

// ClientAuth contains the credentials needed to authenticate to a Kubernetes cluster.
type ClientAuth struct {
	// CertificateAuthorityData is the PEM-encoded CA bundle that signs the API server's
	// serving certificate.
	CertificateAuthorityData string
	// ClientTokenData is the bearer token presented to the API server.
	ClientTokenData string
}

// SecretProvider retrieves cluster authentication credentials for a ClusterTarget.
type SecretProvider interface {
	GetClientAuth(ctx context.Context, cluster *v2pb.ClusterTarget) (ClientAuth, error)
}

// Provider implements SecretProvider by reading two Kubernetes Secret objects from the
// control-plane cluster:
// 1. One for the CA bundle
// 2. One for the bearer token
// The secret names are pulled from the ClusterTarget's `CaDataTag` and `TokenTag` fields.
//
// NOTE: This implementation is intended for sandbox and testing use. Production deployments
// should use an external secret manager (e.g. HashiCorp Vault, AWS Secrets Manager, GCP
// Secret Manager) and provide their own SecretProvider implementation.
type Provider struct {
	kubeClient client.Client
}

// NewProvider returns a SecretProvider backed by the given Kubernetes client.
func NewProvider(kubeClient client.Client) *Provider {
	return &Provider{kubeClient: kubeClient}
}

// GetClientAuth fetches the CA certificate and bearer token secrets for the given
// ClusterTarget and returns them as a ClientAuth value.
func (p *Provider) GetClientAuth(ctx context.Context, cluster *v2pb.ClusterTarget) (ClientAuth, error) {
	if cluster.GetKubernetes() == nil {
		return ClientAuth{}, fmt.Errorf("cluster %q has no kubernetes connection spec", cluster.GetClusterId())
	}

	caSecretName := cluster.GetKubernetes().GetCaDataTag()
	caSecret, err := p.fetchSecret(ctx, caSecretName)
	if err != nil {
		return ClientAuth{}, fmt.Errorf("CA secret for cluster %q: %w", cluster.GetClusterId(), err)
	}

	tokenSecretName := cluster.GetKubernetes().GetTokenTag()
	tokenSecret, err := p.fetchSecret(ctx, tokenSecretName)
	if err != nil {
		return ClientAuth{}, fmt.Errorf("token secret for cluster %q: %w", cluster.GetClusterId(), err)
	}

	return ClientAuth{
		CertificateAuthorityData: string(caSecret.Data[caDataKey]),
		ClientTokenData:          string(tokenSecret.Data[tokenKey]),
	}, nil
}

func (p *Provider) fetchSecret(ctx context.Context, name string) (*corev1.Secret, error) {
	if name == "" {
		return nil, fmt.Errorf("empty secret name")
	}
	secret := &corev1.Secret{}
	key := types.NamespacedName{Name: name, Namespace: secretsNamespace}
	if err := p.kubeClient.Get(ctx, key, secret); err != nil {
		return nil, fmt.Errorf("get secret %s/%s: %w", secretsNamespace, name, err)
	}
	return secret, nil
}
