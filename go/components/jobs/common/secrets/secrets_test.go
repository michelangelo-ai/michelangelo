package secrets

import (
	"context"
	"testing"

	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	fakek8sclient "k8s.io/client-go/kubernetes/fake"
)

func TestGetKubeSecretName(t *testing.T) {
	jobName := "test-job"
	secretName := GetKubeSecretName(jobName)
	expectedName := "ma-job-secret-" + jobName
	require.Equal(t, expectedName, secretName)
}

func TestProviderGetClusterClientAuth(t *testing.T) {
	// Create fake secrets for testing
	caSecret := &corev1.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name:      "ca-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"cadata": []byte("fake-ca-data"),
		},
	}

	tokenSecret := &corev1.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name:      "token-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"token": []byte("fake-token-data"),
		},
	}

	// Create fake Kubernetes clientset with the secrets
	fakeClientSet := fakek8sclient.NewSimpleClientset(caSecret, tokenSecret)

	tests := []struct {
		name          string
		cluster       *v2pb.Cluster
		expectedError bool
		errorContains string
		expectAuth    ClientAuth
	}{
		{
			name: "unsupported cluster type (no cluster spec provided)",
			cluster: &v2pb.Cluster{
				ObjectMeta: v1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "default",
				},
				Spec: v2pb.ClusterSpec{
					// No cluster type specified - this should trigger the default case
				},
			},
			expectedError: true,
			expectAuth:    ClientAuth{},
		},
		{
			name: "successfully configured Kubernetes cluster spec with secrets",
			cluster: &v2pb.Cluster{
				ObjectMeta: v1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "default",
				},
				Spec: v2pb.ClusterSpec{
					Cluster: &v2pb.ClusterSpec_Kubernetes{
						Kubernetes: &v2pb.KubernetesSpec{
							Rest: &v2pb.ConnectionSpec{
								Host:      "https://k8s-cluster.example.com",
								Port:      "443",
								CaDataTag: "ca-secret",
								TokenTag:  "token-secret",
							},
						},
					},
				},
			},
			expectAuth: ClientAuth{
				CertificateAuthorityData: "fake-ca-data",
				ClientTokenData:          "fake-token-data",
			},
		},
		{
			name: "Kubernetes cluster spec with missing ca secret",
			cluster: &v2pb.Cluster{
				ObjectMeta: v1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "default",
				},
				Spec: v2pb.ClusterSpec{
					Cluster: &v2pb.ClusterSpec_Kubernetes{
						Kubernetes: &v2pb.KubernetesSpec{
							Rest: &v2pb.ConnectionSpec{
								Host:      "https://k8s-cluster.example.com",
								Port:      "443",
								CaDataTag: "missing-ca-secret",
								TokenTag:  "token-secret",
							},
						},
					},
				},
			},
			expectedError: true,
		},
		{
			name: "Kubernetes cluster spec with missing token secret",
			cluster: &v2pb.Cluster{
				ObjectMeta: v1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "default",
				},
				Spec: v2pb.ClusterSpec{
					Cluster: &v2pb.ClusterSpec_Kubernetes{
						Kubernetes: &v2pb.KubernetesSpec{
							Rest: &v2pb.ConnectionSpec{
								Host:      "https://k8s-cluster.example.com",
								Port:      "443",
								CaDataTag: "ca-secret",
								TokenTag:  "missing-token-secret",
							},
						},
					},
				},
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := Provider{
				k8sClusterClient: fakeClientSet,
			}
			auth, err := provider.GetClusterClientAuth(context.Background(), tt.cluster)
			if tt.expectedError {
				require.Error(t, err)
				require.Equal(t, ClientAuth{}, auth)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expectAuth, auth)
			}
		})
	}
}

func TestProviderGenerateHadoopSecret(t *testing.T) {
	provider := Provider{}

	cluster := &v2pb.Cluster{
		ObjectMeta: v1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
	}

	var job runtime.Object = &v2pb.RayJob{
		ObjectMeta: v1.ObjectMeta{
			Name:      "test-ray-job",
			Namespace: "default",
		},
	}

	// Test that GenerateHadoopSecret returns empty for now
	result, err := provider.GetSecretsForDataStore(context.Background(), job, cluster)
	require.NoError(t, err)
	require.Nil(t, result)
}
