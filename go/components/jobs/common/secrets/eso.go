package secrets

import (
	"context"
	"fmt"
	"sync/atomic"

	"go.uber.org/zap"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"

	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// esoAPIGroup is the API group registered by the External Secrets Operator's
// CRDs. Its presence is how the provider verifies the operator is installed.
const esoAPIGroup = "external-secrets.io"

var _ SecretProvider = (*ESOProvider)(nil)

// ESOProvider implements SecretProvider on top of the External Secrets
// Operator: credential Secrets are expected to be materialized in-cluster by
// ExternalSecret resources synced from an external store (Vault, AWS Secrets
// Manager, GCP Secret Manager, ...). The provider reads those synced Secrets
// and fails with a clear error when the operator's CRDs are absent, so a
// misconfigured deployment degrades loudly rather than mysteriously.
//
// The Secret read for a given cluster is named by that cluster's
// `CaDataTag`/`TokenTag` fields, resolved within the single configured
// namespace. Those fields are part of the Cluster spec, so anything with
// write access to Clusters can select any Secret name in that namespace;
// Cluster create/update should be restricted to trusted/admin callers.
type ESOProvider struct {
	k8sClusterClient kubernetes.Interface
	namespace        string
	logger           *zap.Logger

	// operatorVerified caches a successful CRD-presence check. Failed checks
	// are not cached, so a transient discovery error does not wedge the
	// provider.
	operatorVerified atomic.Bool
}

// NewESOProvider returns a SecretProvider backed by External Secrets
// Operator managed Secrets.
func NewESOProvider(clientSet kubernetes.Interface, cfg ESOConfig, logger *zap.Logger) *ESOProvider {
	namespace := cfg.Namespace
	if namespace == "" {
		namespace = "default"
	}
	return &ESOProvider{
		k8sClusterClient: clientSet,
		namespace:        namespace,
		logger:           logger,
	}
}

// ensureOperator verifies the External Secrets Operator CRDs are installed,
// caching a successful check for the lifetime of the provider.
func (p *ESOProvider) ensureOperator() error {
	if p.operatorVerified.Load() {
		return nil
	}
	groups, err := p.k8sClusterClient.Discovery().ServerGroups()
	if err != nil {
		return fmt.Errorf("failed to discover API groups while checking for the external secrets operator: %w", err)
	}
	for _, group := range groups.Groups {
		if group.Name == esoAPIGroup {
			p.operatorVerified.Store(true)
			return nil
		}
	}
	return fmt.Errorf(
		"secrets provider is configured as %q but the %s API group is not registered: "+
			"the external secrets operator does not appear to be installed in this cluster; "+
			"install it or set secrets.provider to %q", ProviderESO, esoAPIGroup, ProviderSample)
}

// retrieveSecretData reads one key from an operator-synced Secret. It
// returns an error if the key is absent or empty, rather than silently
// returning an empty credential.
func (p *ESOProvider) retrieveSecretData(ctx context.Context, secretName, dataKey string) (string, error) {
	if secretName == "" {
		return "", fmt.Errorf("empty secret name")
	}
	secret, err := p.k8sClusterClient.CoreV1().Secrets(p.namespace).Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return "", fmt.Errorf(
				"failed to get %s: secret %s/%s not found; if it is managed by an ExternalSecret, "+
					"check that the ExternalSecret exists and has synced "+
					"(kubectl get externalsecret -n %s): %w",
				dataKey, p.namespace, secretName, p.namespace, err)
		}
		return "", fmt.Errorf("failed to get %s: %w", dataKey, err)
	}
	data, ok := secret.Data[dataKey]
	if !ok || len(data) == 0 {
		return "", fmt.Errorf(
			"secret %s/%s has no %q key (or its value is empty); check that the ExternalSecret's "+
				"template populates it (kubectl get externalsecret -n %s)",
			p.namespace, secretName, dataKey, p.namespace)
	}
	return string(data), nil
}

// GetClusterClientAuth retrieves the client authentication data for a given
// cluster from operator-synced Secrets.
func (p *ESOProvider) GetClusterClientAuth(ctx context.Context, cluster *v2pb.Cluster) (ClientAuth, error) {
	if err := p.ensureOperator(); err != nil {
		return ClientAuth{}, err
	}

	var kubeClusterSpec *v2pb.KubernetesSpec
	switch cluster.Spec.GetCluster().(type) {
	case *v2pb.ClusterSpec_Kubernetes:
		kubeClusterSpec = cluster.Spec.GetKubernetes()
	default:
		return ClientAuth{}, fmt.Errorf("cluster type %s not supported", cluster.Spec.GetCluster())
	}
	rest := kubeClusterSpec.GetRest()
	if rest == nil {
		return ClientAuth{}, fmt.Errorf("cluster %q has no rest connection spec", cluster.GetName())
	}

	caDataDecoded, err := p.retrieveSecretData(ctx, rest.CaDataTag, "cadata")
	if err != nil {
		return ClientAuth{}, err
	}
	clientTokenDecoded, err := p.retrieveSecretData(ctx, rest.TokenTag, "token")
	if err != nil {
		return ClientAuth{}, err
	}

	return ClientAuth{
		CertificateAuthorityData: caDataDecoded,
		ClientTokenData:          clientTokenDecoded,
	}, nil
}

// GetSecretsForDataStore mirrors the sample provider's current contract: no
// data-store secrets are injected today by any provider, so this returns
// nil until the consumer defines what data-store secrets look like.
func (p *ESOProvider) GetSecretsForDataStore(ctx context.Context, jobObject runtime.Object, cluster *v2pb.Cluster) (map[string][]byte, error) {
	return nil, nil
}
