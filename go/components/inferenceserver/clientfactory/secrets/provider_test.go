package secrets

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

func TestGetClientAuth(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	caSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ca-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"cadata": []byte("test-ca-data"),
		},
	}

	tokenSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-token-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"token": []byte("test-token-data"),
		},
	}

	tests := []struct {
		name        string
		cluster     *v2pb.ClusterTarget
		secrets     []runtime.Object
		wantAuth    ClientAuth
		wantErr     bool
		errContains string
	}{
		{
			name: "successfully retrieves credentials",
			cluster: &v2pb.ClusterTarget{
				ClusterId: "test-cluster",
				Config: &v2pb.ClusterTarget_Kubernetes{
					Kubernetes: &v2pb.ConnectionSpec{
						CaDataTag: "test-ca-secret",
						TokenTag:  "test-token-secret",
					},
				},
			},
			secrets: []runtime.Object{caSecret, tokenSecret},
			wantAuth: ClientAuth{
				CertificateAuthorityData: "test-ca-data",
				ClientTokenData:          "test-token-data",
			},
			wantErr: false,
		},
		{
			name: "returns error when CA secret not found",
			cluster: &v2pb.ClusterTarget{
				ClusterId: "test-cluster",
				Config: &v2pb.ClusterTarget_Kubernetes{
					Kubernetes: &v2pb.ConnectionSpec{
						CaDataTag: "missing-ca-secret",
						TokenTag:  "test-token-secret",
					},
				},
			},
			secrets:     []runtime.Object{tokenSecret},
			wantErr:     true,
			errContains: "failed to get CA secret",
		},
		{
			name: "returns error when token secret not found",
			cluster: &v2pb.ClusterTarget{
				ClusterId: "test-cluster",
				Config: &v2pb.ClusterTarget_Kubernetes{
					Kubernetes: &v2pb.ConnectionSpec{
						CaDataTag: "test-ca-secret",
						TokenTag:  "missing-token-secret",
					},
				},
			},
			secrets:     []runtime.Object{caSecret},
			wantErr:     true,
			errContains: "failed to get token secret",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(tt.secrets...).
				Build()

			provider := NewProvider(fakeClient)

			auth, err := provider.GetClientAuth(context.Background(), tt.cluster)

			if tt.wantErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.errContains)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.wantAuth, auth)
		})
	}
}
