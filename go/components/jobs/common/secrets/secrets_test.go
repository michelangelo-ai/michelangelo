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

	tlsSecret := &corev1.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name:      "tls-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"tls.crt": []byte("fake-cert-data"),
			"tls.key": []byte("fake-key-data"),
		},
	}

	// Create fake Kubernetes clientset with the secrets
	fakeClientSet := fakek8sclient.NewSimpleClientset(caSecret, tokenSecret, tlsSecret)

	// kubernetesCluster returns a Kubernetes cluster that names its secrets with the given
	// tags and annotations.
	kubernetesCluster := func(caTag, tokenTag string, annotations map[string]string) *v2pb.Cluster {
		return &v2pb.Cluster{
			ObjectMeta: v1.ObjectMeta{
				Name:        "test-cluster",
				Namespace:   "default",
				Annotations: annotations,
			},
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
	clientCertAnnotations := map[string]string{
		ClientCertTagAnnotation: "tls-secret",
		ClientKeyTagAnnotation:  "tls-secret",
	}

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
		{
			name:    "client certificate and key without a token",
			cluster: kubernetesCluster("ca-secret", "", clientCertAnnotations),
			expectAuth: ClientAuth{
				CertificateAuthorityData: "fake-ca-data",
				ClientCertificateData:    "fake-cert-data",
				ClientKeyData:            "fake-key-data",
			},
		},
		{
			name:    "client certificate and key with a token",
			cluster: kubernetesCluster("ca-secret", "token-secret", clientCertAnnotations),
			expectAuth: ClientAuth{
				CertificateAuthorityData: "fake-ca-data",
				ClientCertificateData:    "fake-cert-data",
				ClientKeyData:            "fake-key-data",
				ClientTokenData:          "fake-token-data",
			},
		},
		{
			name: "client certificate without a key",
			cluster: kubernetesCluster("ca-secret", "", map[string]string{
				ClientCertTagAnnotation: "tls-secret",
			}),
			expectedError: true,
			errorContains: "must set both",
		},
		{
			name: "client key secret without tls.key",
			cluster: kubernetesCluster("ca-secret", "", map[string]string{
				ClientCertTagAnnotation: "tls-secret",
				ClientKeyTagAnnotation:  "ca-secret",
			}),
			expectedError: true,
			errorContains: "secret ca-secret has no tls.key",
		},
		{
			name: "missing client certificate secret",
			cluster: kubernetesCluster("ca-secret", "", map[string]string{
				ClientCertTagAnnotation: "missing-tls-secret",
				ClientKeyTagAnnotation:  "tls-secret",
			}),
			expectedError: true,
			errorContains: "failed to get tls.crt",
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
				require.ErrorContains(t, err, tt.errorContains)
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
