package compute

import (
	"context"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/secrets"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/secrets/secretsmocks"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestNewClientSetFactory(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockProvider := secretsmocks.NewMockSecretProvider(ctrl)
	factory := NewClientSetFactory(mockProvider)
	require.NotNil(t, factory)
}

func TestGetClientSetForCluster(t *testing.T) {
	testCluster := v2pb.Cluster{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Cluster",
			APIVersion: "michelangelo.uber.com/v2beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testCluster",
			Namespace: constants.ClustersNamespace,
		},
		Spec: v2pb.ClusterSpec{
			Region: "phx",
			Zone:   "phx5",
			Dc:     v2pb.DC_TYPE_ON_PREM,
			Cluster: &v2pb.ClusterSpec_Kubernetes{
				Kubernetes: &v2pb.KubernetesSpec{
					Rest: &v2pb.ConnectionSpec{
						Host:      "https://k8s-apiserver-kubernetes-batch01.phx5.uber.internal",
						Port:      "80",
						CaDataTag: "ca-secret",
						TokenTag:  "token-secret",
					},
				},
			},
		},
	}

	t.Run("secret provider error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockProvider := secretsmocks.NewMockSecretProvider(ctrl)
		mockProvider.EXPECT().
			GetClusterClientAuth(gomock.Any(), &testCluster).
			Return(secrets.ClientAuth{}, fmt.Errorf("mock error"))

		factory := NewClientSetFactory(mockProvider)

		_, err := factory.GetClientSetForCluster(&testCluster)
		require.Error(t, err)
		require.Contains(t, err.Error(), "client cfg err")
	})

	// Note: We can't easily test successful client creation without a real Kubernetes cluster
	// or more sophisticated mocking of the REST client creation
}

func TestInvalidate(t *testing.T) {
	newCluster := func(host, port string) *v2pb.Cluster {
		return &v2pb.Cluster{
			ObjectMeta: metav1.ObjectMeta{Name: "testCluster", Namespace: constants.ClustersNamespace},
			Spec: v2pb.ClusterSpec{
				Cluster: &v2pb.ClusterSpec_Kubernetes{
					Kubernetes: &v2pb.KubernetesSpec{
						Rest: &v2pb.ConnectionSpec{Host: host, Port: port, TokenTag: "token-secret"},
					},
				},
			},
		}
	}

	t.Run("rebuilds the client with the current credentials", func(t *testing.T) {
		var mu sync.Mutex
		var gotAuth []string
		// clients send their token only over TLS
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			gotAuth = append(gotAuth, r.Header.Get("Authorization"))
			mu.Unlock()
			_, _ = w.Write([]byte("ok"))
		}))
		defer server.Close()
		host, port, ok := strings.Cut(strings.TrimPrefix(server.URL, "https://"), ":")
		require.True(t, ok)
		cluster := newCluster("https://"+host, port)
		caData := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}))

		ctrl := gomock.NewController(t)
		mockProvider := secretsmocks.NewMockSecretProvider(ctrl)
		token := "token-1"
		mockProvider.EXPECT().GetClusterClientAuth(gomock.Any(), cluster).DoAndReturn(
			func(context.Context, *v2pb.Cluster) (secrets.ClientAuth, error) {
				return secrets.ClientAuth{CertificateAuthorityData: caData, ClientTokenData: token}, nil
			}).Times(2)
		factory := NewClientSetFactory(mockProvider)

		first, err := factory.GetClientSetForCluster(cluster)
		require.NoError(t, err)
		cached, err := factory.GetClientSetForCluster(cluster)
		require.NoError(t, err)
		require.Same(t, first, cached, "the client is cached until invalidated")
		_, err = first.CoreV1.Get().AbsPath("/healthz").Do(context.Background()).Raw()
		require.NoError(t, err)

		// rotate the token in place, then invalidate
		token = "token-2"
		factory.Invalidate(&cluster.Spec)

		rebuilt, err := factory.GetClientSetForCluster(cluster)
		require.NoError(t, err)
		require.NotSame(t, first, rebuilt)
		_, err = rebuilt.CoreV1.Get().AbsPath("/healthz").Do(context.Background()).Raw()
		require.NoError(t, err)

		mu.Lock()
		defer mu.Unlock()
		require.Equal(t, []string{"Bearer token-1", "Bearer token-2"}, gotAuth)
	})

	t.Run("drops the entry keyed by the old spec", func(t *testing.T) {
		oldCluster := newCluster("https://old.example.com", "6443")
		updatedCluster := newCluster("https://new.example.com", "443")

		ctrl := gomock.NewController(t)
		mockProvider := secretsmocks.NewMockSecretProvider(ctrl)
		mockProvider.EXPECT().GetClusterClientAuth(gomock.Any(), gomock.Any()).
			Return(secrets.ClientAuth{ClientTokenData: "token"}, nil).Times(1)
		f := NewClientSetFactory(mockProvider).(*factory)

		_, err := f.GetClientSetForCluster(oldCluster)
		require.NoError(t, err)

		f.Invalidate(&updatedCluster.Spec)
		_, ok := f.clients.Load("https://old.example.com:6443")
		require.True(t, ok, "invalidating the new spec leaves the old host:port entry")

		f.Invalidate(&oldCluster.Spec)
		_, ok = f.clients.Load("https://old.example.com:6443")
		require.False(t, ok, "invalidating the old spec drops its host:port entry")
	})

	t.Run("a spec without a cached client is a no-op", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		factory := NewClientSetFactory(secretsmocks.NewMockSecretProvider(ctrl))
		factory.Invalidate(&newCluster("https://host.example.com", "6443").Spec)
		factory.Invalidate(&v2pb.ClusterSpec{})
	})
}

func TestGetRestConfigForCluster(t *testing.T) {
	cluster := &v2pb.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: "testCluster", Namespace: constants.ClustersNamespace},
		Spec: v2pb.ClusterSpec{
			Cluster: &v2pb.ClusterSpec_Kubernetes{
				Kubernetes: &v2pb.KubernetesSpec{
					Rest: &v2pb.ConnectionSpec{Host: "https://host.example.com", Port: "6443", TokenTag: "token-secret"},
				},
			},
		},
	}

	t.Run("builds the config from the current credentials on every call", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockProvider := secretsmocks.NewMockSecretProvider(ctrl)
		token := "token-1"
		mockProvider.EXPECT().GetClusterClientAuth(gomock.Any(), cluster).DoAndReturn(
			func(context.Context, *v2pb.Cluster) (secrets.ClientAuth, error) {
				return secrets.ClientAuth{CertificateAuthorityData: "ca-data", ClientTokenData: token}, nil
			}).Times(2)
		factory := NewClientSetFactory(mockProvider)

		cfg, err := factory.GetRestConfigForCluster(cluster)
		require.NoError(t, err)
		require.Equal(t, "https://host.example.com:6443", cfg.Host)
		require.Equal(t, "token-1", cfg.BearerToken)
		require.Equal(t, []byte("ca-data"), cfg.CAData)
		require.Equal(t, runtime.ContentTypeJSON, cfg.ContentType)

		// a rotated token shows on the next call, with no Invalidate
		token = "token-2"
		cfg, err = factory.GetRestConfigForCluster(cluster)
		require.NoError(t, err)
		require.Equal(t, "token-2", cfg.BearerToken)
	})

	t.Run("secret provider error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockProvider := secretsmocks.NewMockSecretProvider(ctrl)
		mockProvider.EXPECT().GetClusterClientAuth(gomock.Any(), cluster).
			Return(secrets.ClientAuth{}, fmt.Errorf("mock error"))
		factory := NewClientSetFactory(mockProvider)

		_, err := factory.GetRestConfigForCluster(cluster)
		require.ErrorContains(t, err, "mock error")
	})
}
