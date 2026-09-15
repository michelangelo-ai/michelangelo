package secrets

import (
	"context"
	"fmt"
	"sync/atomic"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// Provider selection values for Config.Provider.
const (
	// ProviderSample selects the sample implementation that reads secrets
	// from the control-plane cluster. This is the default.
	ProviderSample = "sample"
	// ProviderESO selects the External Secrets Operator backed
	// implementation.
	ProviderESO = "eso"
)

// ConfigKey is the YAML key the secrets configuration is read from. The jobs
// engine reads the same key, so one config block selects the secret provider
// for both consumers.
const ConfigKey = "secrets"

// esoAPIGroup is the API group registered by the External Secrets Operator's
// CRDs. Its presence is how the provider verifies the operator is installed.
const esoAPIGroup = "external-secrets.io"

// Config selects and configures the SecretProvider implementation.
type Config struct {
	// Provider is "sample" (default) or "eso".
	Provider string `yaml:"provider"`
	// ESO configures the External Secrets Operator backed provider.
	ESO ESOConfig `yaml:"eso"`
}

// ESOConfig configures the External Secrets Operator backed provider.
type ESOConfig struct {
	// Namespace is where the operator-synced credential Secrets live.
	// Defaults to "default", matching the sample provider.
	Namespace string `yaml:"namespace"`
}

var _ SecretProvider = (*ESOProvider)(nil)

// ESOProvider implements SecretProvider on top of the External Secrets
// Operator: credential Secrets are expected to be materialized in-cluster by
// ExternalSecret resources synced from an external store (Vault, AWS Secrets
// Manager, GCP Secret Manager, ...). The provider reads those synced Secrets
// and fails with a clear error when the operator's CRDs are absent, so a
// misconfigured deployment degrades loudly rather than mysteriously.
type ESOProvider struct {
	kubeClient client.Client
	namespace  string

	// operatorVerified caches a successful CRD-presence check. Failed checks
	// are not cached, so a transient discovery error does not wedge the
	// provider.
	operatorVerified atomic.Bool
}

// NewESOProvider returns a SecretProvider backed by External Secrets
// Operator managed Secrets.
func NewESOProvider(kubeClient client.Client, cfg ESOConfig) *ESOProvider {
	namespace := cfg.Namespace
	if namespace == "" {
		namespace = secretsNamespace
	}
	return &ESOProvider{kubeClient: kubeClient, namespace: namespace}
}

// ensureOperator verifies the External Secrets Operator CRDs are installed,
// caching a successful check for the lifetime of the provider.
func (p *ESOProvider) ensureOperator() error {
	if p.operatorVerified.Load() {
		return nil
	}
	_, err := p.kubeClient.RESTMapper().RESTMapping(schema.GroupKind{Group: esoAPIGroup, Kind: "ExternalSecret"})
	if err != nil {
		if meta.IsNoMatchError(err) {
			return fmt.Errorf(
				"secrets provider is configured as %q but the ExternalSecret kind (%s) is not registered: "+
					"the external secrets operator does not appear to be installed in this cluster; "+
					"install it or set secrets.provider to %q", ProviderESO, esoAPIGroup, ProviderSample)
		}
		return fmt.Errorf("failed to check for the external secrets operator: %w", err)
	}
	p.operatorVerified.Store(true)
	return nil
}

// GetClientAuth fetches the CA certificate and bearer token from
// operator-synced Secrets for the given ClusterTarget.
func (p *ESOProvider) GetClientAuth(ctx context.Context, cluster *v2pb.ClusterTarget) (ClientAuth, error) {
	if err := p.ensureOperator(); err != nil {
		return ClientAuth{}, err
	}
	if cluster.GetKubernetes() == nil {
		return ClientAuth{}, fmt.Errorf("cluster %q has no kubernetes connection spec", cluster.GetClusterId())
	}

	caSecret, err := p.fetchSecret(ctx, cluster.GetKubernetes().GetCaDataTag())
	if err != nil {
		return ClientAuth{}, fmt.Errorf("CA secret for cluster %q: %w", cluster.GetClusterId(), err)
	}
	tokenSecret, err := p.fetchSecret(ctx, cluster.GetKubernetes().GetTokenTag())
	if err != nil {
		return ClientAuth{}, fmt.Errorf("token secret for cluster %q: %w", cluster.GetClusterId(), err)
	}

	return ClientAuth{
		CertificateAuthorityData: string(caSecret.Data[caDataKey]),
		ClientTokenData:          string(tokenSecret.Data[tokenKey]),
	}, nil
}

func (p *ESOProvider) fetchSecret(ctx context.Context, name string) (*corev1.Secret, error) {
	if name == "" {
		return nil, fmt.Errorf("empty secret name")
	}
	secret := &corev1.Secret{}
	key := types.NamespacedName{Name: name, Namespace: p.namespace}
	if err := p.kubeClient.Get(ctx, key, secret); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, fmt.Errorf(
				"get secret %s/%s: not found; if it is managed by an ExternalSecret, check that the "+
					"ExternalSecret exists and has synced (kubectl get externalsecret -n %s): %w",
				p.namespace, name, p.namespace, err)
		}
		return nil, fmt.Errorf("get secret %s/%s: %w", p.namespace, name, err)
	}
	return secret, nil
}
