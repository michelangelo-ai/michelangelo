package compute

import (
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/secrets"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/secrets/secretsmocks"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
