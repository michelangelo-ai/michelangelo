package compute

import (
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/secrets"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/secrets/secretsmocks"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"

	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
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

	// The errors of a secret read on the host API server, wrapped with %w as
	// secrets.Provider wraps them. A timeout must reach the caller as one, so
	// that the job is retried rather than failed. A missing secret must not
	// reach it as NotFound, which the callers of the federated client read as
	// "the object on the member cluster is gone".
	secretsResource := schema.GroupResource{Resource: "secrets"}
	secretReadErrors := []struct {
		name    string
		readErr error
		check   func(t *testing.T, err error)
	}{
		{
			name:    "a secret read that timed out stays a ServerTimeout",
			readErr: fmt.Errorf("failed to get token: %w", apierrors.NewServerTimeout(secretsResource, "get", 1)),
			check: func(t *testing.T, err error) {
				require.True(t, apierrors.IsServerTimeout(err), "got: %v", err)
			},
		},
		{
			name:    "a missing credentials secret is not a NotFound",
			readErr: fmt.Errorf("failed to get token: %w", apierrors.NewNotFound(secretsResource, "token-secret")),
			check: func(t *testing.T, err error) {
				require.False(t, apierrors.IsNotFound(err), "got: %v", err)
				require.Contains(t, err.Error(), `secrets "token-secret" not found`, "the cause stays in the message")
			},
		},
	}
	for _, tt := range secretReadErrors {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockProvider := secretsmocks.NewMockSecretProvider(ctrl)
			mockProvider.EXPECT().
				GetClusterClientAuth(gomock.Any(), &testCluster).
				Return(secrets.ClientAuth{}, tt.readErr)

			_, err := NewClientSetFactory(mockProvider).GetClientSetForCluster(&testCluster)
			require.Error(t, err)
			require.Contains(t, err.Error(), "client cfg err")
			tt.check(t, err)
		})
	}

	t.Run("all clients are populated", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockProvider := secretsmocks.NewMockSecretProvider(ctrl)
		mockProvider.EXPECT().
			GetClusterClientAuth(gomock.Any(), &testCluster).
			Return(secrets.ClientAuth{ClientTokenData: "test-token"}, nil)

		// A fresh factory: it holds the secret provider it was built with, and each sub-test
		// owns its own gomock controller and expectations, so the one from the sub-test above
		// cannot be reused. (Nothing is memoised across them either: that sub-test fails before
		// the factory stores a ClientSet.)
		factory := NewClientSetFactory(mockProvider)

		cs, err := factory.GetClientSetForCluster(&testCluster)
		require.NoError(t, err)
		require.NotNil(t, cs)

		// Every client in the set is built from the one *rest.Config returned by getClientCfg.
		// These assertions prove each constructor returned a client rather than erroring; they do
		// NOT detect a constructor that mutated the shared config in place, because client-go and
		// kuberay each overwrite GroupVersion/NegotiatedSerializer on their own by-value copy.
		// That guard is sparkoperator/rest_client_test.go, which inspects the caller's config
		// after the call. Do not delete it as redundant with this one.
		require.NotNil(t, cs.Spark, "Spark client was not populated")
		require.NotNil(t, cs.Ray, "Ray client was not populated")
		require.NotNil(t, cs.CoreV1, "CoreV1 client was not populated")
	})
}
