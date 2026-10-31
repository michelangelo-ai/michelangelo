package secrets

import (
	"context"
	"testing"

	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// newRESTMapper returns a RESTMapper that optionally knows about the
// ExternalSecret kind, simulating the operator's CRDs being installed.
func newRESTMapper(withESO bool) meta.RESTMapper {
	corev1GV := schema.GroupVersion{Version: "v1"}
	esoGV := schema.GroupVersion{Group: esoAPIGroup, Version: "v1beta1"}
	groupVersions := []schema.GroupVersion{corev1GV}
	if withESO {
		groupVersions = append(groupVersions, esoGV)
	}
	mapper := meta.NewDefaultRESTMapper(groupVersions)
	mapper.Add(corev1GV.WithKind("Secret"), meta.RESTScopeNamespace)
	if withESO {
		mapper.Add(esoGV.WithKind("ExternalSecret"), meta.RESTScopeNamespace)
	}
	return mapper
}

func newFakeClient(withESO bool, objects ...client.Object) client.Client {
	return fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithRESTMapper(newRESTMapper(withESO)).
		WithObjects(objects...).
		Build()
}

func clusterTarget(caTag, tokenTag string) *v2pb.ClusterTarget {
	return &v2pb.ClusterTarget{
		ClusterId: "test-cluster",
		Connection: &v2pb.ClusterTarget_Kubernetes{
			Kubernetes: &v2pb.ConnectionSpec{
				CaDataTag: caTag,
				TokenTag:  tokenTag,
			},
		},
	}
}

func credentialSecret(name, namespace string, data map[string][]byte) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Data:       data,
	}
}

func TestESOProviderGetClientAuth(t *testing.T) {
	kubeClient := newFakeClient(true,
		credentialSecret("ca-secret", "ma-secrets", map[string][]byte{caDataKey: []byte("fake-ca")}),
		credentialSecret("token-secret", "ma-secrets", map[string][]byte{tokenKey: []byte("fake-token")}),
	)
	provider := NewESOProvider(kubeClient, ESOConfig{Namespace: "ma-secrets"})

	auth, err := provider.GetClientAuth(context.Background(), clusterTarget("ca-secret", "token-secret"))
	require.NoError(t, err)
	require.Equal(t, ClientAuth{
		CertificateAuthorityData: "fake-ca",
		ClientTokenData:          "fake-token",
	}, auth)
}

func TestESOProviderDefaultNamespace(t *testing.T) {
	kubeClient := newFakeClient(true,
		credentialSecret("ca-secret", secretsNamespace, map[string][]byte{caDataKey: []byte("fake-ca")}),
		credentialSecret("token-secret", secretsNamespace, map[string][]byte{tokenKey: []byte("fake-token")}),
	)
	provider := NewESOProvider(kubeClient, ESOConfig{})

	auth, err := provider.GetClientAuth(context.Background(), clusterTarget("ca-secret", "token-secret"))
	require.NoError(t, err)
	require.Equal(t, "fake-ca", auth.CertificateAuthorityData)
}

func TestESOProviderOperatorAbsent(t *testing.T) {
	provider := NewESOProvider(newFakeClient(false), ESOConfig{})

	_, err := provider.GetClientAuth(context.Background(), clusterTarget("ca-secret", "token-secret"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "external secrets operator does not appear to be installed")
}

func TestESOProviderMissingSecretGuidance(t *testing.T) {
	provider := NewESOProvider(newFakeClient(true), ESOConfig{Namespace: "ma-secrets"})

	_, err := provider.GetClientAuth(context.Background(), clusterTarget("missing-ca", "missing-token"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "check that the ExternalSecret exists and has synced")
	require.Contains(t, err.Error(), "ma-secrets/missing-ca")
}

func TestESOProviderMissingDataKey(t *testing.T) {
	// The secret exists but lacks the "cadata" key entirely (e.g. the
	// ExternalSecret's template was misconfigured). This must fail loudly
	// rather than silently returning an empty credential.
	kubeClient := newFakeClient(true,
		credentialSecret("ca-secret", "ma-secrets", map[string][]byte{"not-cadata": []byte("fake-ca")}),
		credentialSecret("token-secret", "ma-secrets", map[string][]byte{tokenKey: []byte("fake-token")}),
	)
	provider := NewESOProvider(kubeClient, ESOConfig{Namespace: "ma-secrets"})

	_, err := provider.GetClientAuth(context.Background(), clusterTarget("ca-secret", "token-secret"))
	require.Error(t, err)
	require.Contains(t, err.Error(), `has no "cadata" key`)
}

func TestESOProviderEmptyDataKey(t *testing.T) {
	// The secret has the "cadata" key, but its value is empty. This must
	// also fail loudly rather than silently returning an empty credential.
	kubeClient := newFakeClient(true,
		credentialSecret("ca-secret", "ma-secrets", map[string][]byte{caDataKey: []byte("")}),
		credentialSecret("token-secret", "ma-secrets", map[string][]byte{tokenKey: []byte("fake-token")}),
	)
	provider := NewESOProvider(kubeClient, ESOConfig{Namespace: "ma-secrets"})

	_, err := provider.GetClientAuth(context.Background(), clusterTarget("ca-secret", "token-secret"))
	require.Error(t, err)
	require.Contains(t, err.Error(), `has no "cadata" key`)
}

func TestESOProviderNoKubernetesSpec(t *testing.T) {
	provider := NewESOProvider(newFakeClient(true), ESOConfig{})

	_, err := provider.GetClientAuth(context.Background(), &v2pb.ClusterTarget{ClusterId: "test-cluster"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "no kubernetes connection spec")
}

func TestESOProviderEmptySecretName(t *testing.T) {
	provider := NewESOProvider(newFakeClient(true), ESOConfig{})

	_, err := provider.GetClientAuth(context.Background(), clusterTarget("", ""))
	require.Error(t, err)
	require.Contains(t, err.Error(), "empty secret name")
}

// countingRESTMapper wraps a meta.RESTMapper and counts calls to
// RESTMapping, the only method the ESO provider's operator check uses.
type countingRESTMapper struct {
	meta.RESTMapper
	calls int
}

func (c *countingRESTMapper) RESTMapping(gk schema.GroupKind, versions ...string) (*meta.RESTMapping, error) {
	c.calls++
	return c.RESTMapper.RESTMapping(gk, versions...)
}

func TestESOProviderCachesOperatorCheck(t *testing.T) {
	mapper := &countingRESTMapper{RESTMapper: newRESTMapper(true)}
	kubeClient := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithRESTMapper(mapper).
		WithObjects(
			credentialSecret("ca-secret", secretsNamespace, map[string][]byte{caDataKey: []byte("fake-ca")}),
			credentialSecret("token-secret", secretsNamespace, map[string][]byte{tokenKey: []byte("fake-token")}),
		).
		Build()
	provider := NewESOProvider(kubeClient, ESOConfig{})

	_, err := provider.GetClientAuth(context.Background(), clusterTarget("ca-secret", "token-secret"))
	require.NoError(t, err)
	require.Equal(t, 1, mapper.calls)

	// Second call must take the cached-verification path rather than
	// querying the RESTMapper again.
	_, err = provider.GetClientAuth(context.Background(), clusterTarget("ca-secret", "token-secret"))
	require.NoError(t, err)
	require.Equal(t, 1, mapper.calls)
}

func TestESOProviderFailedOperatorCheckNotCached(t *testing.T) {
	corev1GV := schema.GroupVersion{Version: "v1"}
	esoGV := schema.GroupVersion{Group: esoAPIGroup, Version: "v1beta1"}
	// esoGV must be a known default version from construction, or
	// RESTMapping will never consider it regardless of what's Add()-ed
	// later; only the ExternalSecret *kind* mapping is installed late, to
	// simulate the operator's CRDs appearing after the first check.
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{corev1GV, esoGV})
	mapper.Add(corev1GV.WithKind("Secret"), meta.RESTScopeNamespace)
	kubeClient := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithRESTMapper(mapper).
		WithObjects(
			credentialSecret("ca-secret", secretsNamespace, map[string][]byte{caDataKey: []byte("fake-ca")}),
			credentialSecret("token-secret", secretsNamespace, map[string][]byte{tokenKey: []byte("fake-token")}),
		).
		Build()
	provider := NewESOProvider(kubeClient, ESOConfig{})

	// The operator is not installed yet, so this must fail.
	_, err := provider.GetClientAuth(context.Background(), clusterTarget("ca-secret", "token-secret"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "external secrets operator does not appear to be installed")

	// Install the operator and retry. If the failed check had been
	// cached, this would fail again even though the operator is now
	// present.
	mapper.Add(esoGV.WithKind("ExternalSecret"), meta.RESTScopeNamespace)

	_, err = provider.GetClientAuth(context.Background(), clusterTarget("ca-secret", "token-secret"))
	require.NoError(t, err)
}
