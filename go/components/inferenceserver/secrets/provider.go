package secrets

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// ClientAuth contains the credentials needed to authenticate to a Kubernetes cluster.
type ClientAuth struct {
	// CertificateAuthorityData contains PEM-encoded certificate authority certificates.
	CertificateAuthorityData string
	// ClientTokenData contains the bearer token for the client.
	ClientTokenData string
}

// SecretProvider retrieves cluster authentication credentials from a secret store.
type SecretProvider interface {
	// GetClientAuth retrieves the authentication credentials for a given connection spec.
	GetClientAuth(ctx context.Context, connectionSpec *v2pb.ConnectionSpec) (ClientAuth, error)
}

// Provider implements SecretProvider using Kubernetes secrets.
type Provider struct {
	kubeClient client.Client
}

// NewProvider creates a new secret provider.
func NewProvider(kubeClient client.Client) *Provider {
	return &Provider{kubeClient: kubeClient}
}

// GetClientAuth retrieves authentication credentials from Kubernetes secrets.
func (s *Provider) GetClientAuth(ctx context.Context, connectionSpec *v2pb.ConnectionSpec) (ClientAuth, error) {
	// Retrieve CA data from secret
	caSecret := &corev1.Secret{}
	if err := s.kubeClient.Get(ctx, types.NamespacedName{Name: connectionSpec.CaDataTag, Namespace: "default"}, caSecret); err != nil {
		return ClientAuth{}, fmt.Errorf("failed to get CA secret %s: %w", connectionSpec.CaDataTag, err)
	}

	// Retrieve token from secret
	tokenSecret := &corev1.Secret{}
	if err := s.kubeClient.Get(ctx, types.NamespacedName{Name: connectionSpec.TokenTag, Namespace: "default"}, tokenSecret); err != nil {
		return ClientAuth{}, fmt.Errorf("failed to get token secret %s: %w", connectionSpec.TokenTag, err)
	}

	return ClientAuth{
		CertificateAuthorityData: string(caSecret.Data["cadata"]),
		ClientTokenData:          string(tokenSecret.Data["token"]),
	}, nil
}
