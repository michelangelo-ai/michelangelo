package secrets

import (
	"context"
	"errors"
	"testing"

	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	fakediscovery "k8s.io/client-go/discovery/fake"
	"k8s.io/client-go/kubernetes"
	fakek8sclient "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

// newFakeClientSetWithESO returns a fake clientset whose discovery reports
// the external-secrets.io API group as present (operator installed).
func newFakeClientSetWithESO(t *testing.T, objects ...runtime.Object) kubernetes.Interface {
	t.Helper()
	fakeClientSet := fakek8sclient.NewSimpleClientset(objects...)
	discovery, ok := fakeClientSet.Discovery().(*fakediscovery.FakeDiscovery)
	require.True(t, ok)
	discovery.Resources = []*v1.APIResourceList{
		{GroupVersion: esoAPIGroup + "/v1beta1"},
	}
	return fakeClientSet
}

func kubeCluster(caTag, tokenTag string) *v2pb.Cluster {
	return &v2pb.Cluster{
		ObjectMeta: v1.ObjectMeta{Name: "test-cluster", Namespace: "default"},
		Spec: v2pb.ClusterSpec{
			Cluster: &v2pb.ClusterSpec_Kubernetes{
				Kubernetes: &v2pb.KubernetesSpec{
					Rest: &v2pb.ConnectionSpec{
						Host:      "https://k8s-cluster.example.com",
						Port:      "443",
						CaDataTag: caTag,
						TokenTag:  tokenTag,
					},
				},
			},
		},
	}
}

func credentialSecret(name, namespace, key, value string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: v1.ObjectMeta{Name: name, Namespace: namespace},
		Data:       map[string][]byte{key: []byte(value)},
	}
}

func TestESOProviderGetClusterClientAuth(t *testing.T) {
	fakeClientSet := newFakeClientSetWithESO(t,
		credentialSecret("ca-secret", "ma-secrets", "cadata", "fake-ca-data"),
		credentialSecret("token-secret", "ma-secrets", "token", "fake-token-data"),
	)
	provider := NewESOProvider(fakeClientSet, ESOConfig{Namespace: "ma-secrets"}, nil)

	auth, err := provider.GetClusterClientAuth(context.Background(), kubeCluster("ca-secret", "token-secret"))
	require.NoError(t, err)
	require.Equal(t, ClientAuth{
		CertificateAuthorityData: "fake-ca-data",
		ClientTokenData:          "fake-token-data",
	}, auth)
}

func TestESOProviderDefaultNamespace(t *testing.T) {
	fakeClientSet := newFakeClientSetWithESO(t,
		credentialSecret("ca-secret", "default", "cadata", "fake-ca-data"),
		credentialSecret("token-secret", "default", "token", "fake-token-data"),
	)
	provider := NewESOProvider(fakeClientSet, ESOConfig{}, nil)

	auth, err := provider.GetClusterClientAuth(context.Background(), kubeCluster("ca-secret", "token-secret"))
	require.NoError(t, err)
	require.Equal(t, "fake-ca-data", auth.CertificateAuthorityData)
}

func TestESOProviderOperatorAbsent(t *testing.T) {
	fakeClientSet := fakek8sclient.NewSimpleClientset()
	provider := NewESOProvider(fakeClientSet, ESOConfig{}, nil)

	_, err := provider.GetClusterClientAuth(context.Background(), kubeCluster("ca-secret", "token-secret"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "external secrets operator does not appear to be installed")
	require.Contains(t, err.Error(), esoAPIGroup)
}

func TestESOProviderMissingSecretGuidance(t *testing.T) {
	fakeClientSet := newFakeClientSetWithESO(t)
	provider := NewESOProvider(fakeClientSet, ESOConfig{Namespace: "ma-secrets"}, nil)

	_, err := provider.GetClusterClientAuth(context.Background(), kubeCluster("missing-ca", "missing-token"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "check that the ExternalSecret exists and has synced")
	require.Contains(t, err.Error(), "ma-secrets/missing-ca")
}

func TestESOProviderMissingDataKey(t *testing.T) {
	// The secret exists but lacks the "cadata" key entirely (e.g. the
	// ExternalSecret's template was misconfigured). This must fail loudly
	// rather than silently returning an empty credential.
	fakeClientSet := newFakeClientSetWithESO(t,
		&corev1.Secret{
			ObjectMeta: v1.ObjectMeta{Name: "ca-secret", Namespace: "default"},
			Data:       map[string][]byte{"not-cadata": []byte("fake-ca-data")},
		},
		credentialSecret("token-secret", "default", "token", "fake-token-data"),
	)
	provider := NewESOProvider(fakeClientSet, ESOConfig{}, nil)

	_, err := provider.GetClusterClientAuth(context.Background(), kubeCluster("ca-secret", "token-secret"))
	require.Error(t, err)
	require.Contains(t, err.Error(), `has no "cadata" key`)
}

func TestESOProviderEmptyDataKey(t *testing.T) {
	// The secret has the "cadata" key, but its value is empty. This must
	// also fail loudly rather than silently returning an empty credential.
	fakeClientSet := newFakeClientSetWithESO(t,
		credentialSecret("ca-secret", "default", "cadata", ""),
		credentialSecret("token-secret", "default", "token", "fake-token-data"),
	)
	provider := NewESOProvider(fakeClientSet, ESOConfig{}, nil)

	_, err := provider.GetClusterClientAuth(context.Background(), kubeCluster("ca-secret", "token-secret"))
	require.Error(t, err)
	require.Contains(t, err.Error(), `has no "cadata" key`)
}

func TestESOProviderNilRestSpec(t *testing.T) {
	// A Kubernetes cluster spec with no rest connection info must fail with
	// a clear error, not panic on a nil pointer dereference.
	fakeClientSet := newFakeClientSetWithESO(t)
	provider := NewESOProvider(fakeClientSet, ESOConfig{}, nil)

	cluster := &v2pb.Cluster{
		ObjectMeta: v1.ObjectMeta{Name: "test-cluster", Namespace: "default"},
		Spec: v2pb.ClusterSpec{
			Cluster: &v2pb.ClusterSpec_Kubernetes{
				Kubernetes: &v2pb.KubernetesSpec{},
			},
		},
	}
	_, err := provider.GetClusterClientAuth(context.Background(), cluster)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no rest connection spec")
}

func TestESOProviderUnsupportedClusterType(t *testing.T) {
	fakeClientSet := newFakeClientSetWithESO(t)
	provider := NewESOProvider(fakeClientSet, ESOConfig{}, nil)

	cluster := &v2pb.Cluster{
		ObjectMeta: v1.ObjectMeta{Name: "test-cluster", Namespace: "default"},
		Spec:       v2pb.ClusterSpec{},
	}
	_, err := provider.GetClusterClientAuth(context.Background(), cluster)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not supported")
}

func TestESOProviderGetSecretsForDataStore(t *testing.T) {
	provider := NewESOProvider(newFakeClientSetWithESO(t), ESOConfig{}, nil)
	data, err := provider.GetSecretsForDataStore(context.Background(), nil, nil)
	require.NoError(t, err)
	require.Nil(t, data)
}

func TestNewSelectsProviderFromConfig(t *testing.T) {
	fakeClientSet := fakek8sclient.NewSimpleClientset()

	tests := []struct {
		name         string
		cfg          Config
		expectSample bool
		expectESO    bool
		expectErr    string
	}{
		{name: "default is sample", cfg: Config{}, expectSample: true},
		{name: "explicit sample", cfg: Config{Provider: ProviderSample}, expectSample: true},
		{name: "eso", cfg: Config{Provider: ProviderESO}, expectESO: true},
		{name: "unknown provider", cfg: Config{Provider: "vault"}, expectErr: "unknown secrets.provider"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := New(Params{ClientSet: fakeClientSet, Cfg: tt.cfg})
			if tt.expectErr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.expectErr)
				return
			}
			require.NoError(t, err)
			if tt.expectSample {
				require.IsType(t, Provider{}, result.SecretProvider)
			}
			if tt.expectESO {
				require.IsType(t, &ESOProvider{}, result.SecretProvider)
			}
		})
	}
}

func TestESOProviderCachesOperatorCheck(t *testing.T) {
	fakeClientSet := newFakeClientSetWithESO(t,
		credentialSecret("ca-secret", "default", "cadata", "fake-ca-data"),
		credentialSecret("token-secret", "default", "token", "fake-token-data"),
	)
	provider := NewESOProvider(fakeClientSet, ESOConfig{}, nil)

	_, err := provider.GetClusterClientAuth(context.Background(), kubeCluster("ca-secret", "token-secret"))
	require.NoError(t, err)

	// Simulate the operator being uninstalled after the first check. If
	// the verification were not cached, this call would now fail; it
	// succeeding proves the cached result is being reused instead of
	// re-querying discovery.
	discovery, ok := fakeClientSet.Discovery().(*fakediscovery.FakeDiscovery)
	require.True(t, ok)
	discovery.Resources = nil

	_, err = provider.GetClusterClientAuth(context.Background(), kubeCluster("ca-secret", "token-secret"))
	require.NoError(t, err)
}

func TestESOProviderFailedOperatorCheckNotCached(t *testing.T) {
	fakeClientSet := fakek8sclient.NewSimpleClientset(
		credentialSecret("ca-secret", "default", "cadata", "fake-ca-data"),
		credentialSecret("token-secret", "default", "token", "fake-token-data"),
	)
	provider := NewESOProvider(fakeClientSet, ESOConfig{}, nil)

	// The operator is not installed yet, so this must fail.
	_, err := provider.GetClusterClientAuth(context.Background(), kubeCluster("ca-secret", "token-secret"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "external secrets operator does not appear to be installed")

	// Install the operator and retry. If the failed check had been
	// cached, this would fail again even though the operator is now
	// present.
	discovery, ok := fakeClientSet.Discovery().(*fakediscovery.FakeDiscovery)
	require.True(t, ok)
	discovery.Resources = []*v1.APIResourceList{{GroupVersion: esoAPIGroup + "/v1beta1"}}

	_, err = provider.GetClusterClientAuth(context.Background(), kubeCluster("ca-secret", "token-secret"))
	require.NoError(t, err)
}

func TestESOProviderGenericGetError(t *testing.T) {
	fakeClientSet := newFakeClientSetWithESO(t)
	fakeClientSet.(*fakek8sclient.Clientset).PrependReactor("get", "secrets",
		func(action k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, errors.New("api server unavailable")
		})
	provider := NewESOProvider(fakeClientSet, ESOConfig{}, nil)

	_, err := provider.GetClusterClientAuth(context.Background(), kubeCluster("ca-secret", "token-secret"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "api server unavailable")
	require.NotContains(t, err.Error(), "ExternalSecret")
}
