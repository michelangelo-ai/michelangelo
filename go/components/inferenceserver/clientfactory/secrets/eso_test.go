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

func TestESOProviderCachesOperatorCheck(t *testing.T) {
	kubeClient := newFakeClient(true,
		credentialSecret("ca-secret", secretsNamespace, map[string][]byte{caDataKey: []byte("fake-ca")}),
		credentialSecret("token-secret", secretsNamespace, map[string][]byte{tokenKey: []byte("fake-token")}),
	)
	provider := NewESOProvider(kubeClient, ESOConfig{})

	_, err := provider.GetClientAuth(context.Background(), clusterTarget("ca-secret", "token-secret"))
	require.NoError(t, err)
	// Second call takes the cached-verification path.
	_, err = provider.GetClientAuth(context.Background(), clusterTarget("ca-secret", "token-secret"))
	require.NoError(t, err)
}
