package secrets

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

func newSecret(name string, data map[string][]byte) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: secretsNamespace},
		Data:       data,
	}
}

func clusterTarget() *v2pb.ClusterTarget {
	return &v2pb.ClusterTarget{
		ClusterId: "remote",
		Connection: &v2pb.ClusterTarget_Kubernetes{Kubernetes: &v2pb.ConnectionSpec{
			CaDataTag: "remote-ca",
			TokenTag:  "remote-token",
		}},
	}
}

func TestProvider_GetClientAuth(t *testing.T) {
	tests := map[string]struct {
		secrets []*corev1.Secret
		cluster *v2pb.ClusterTarget // clusterTarget() when nil
		want    ClientAuth
		wantErr string
	}{
		"both secrets present": {
			secrets: []*corev1.Secret{
				newSecret("remote-ca", map[string][]byte{caDataKey: []byte("ca-pem")}),
				newSecret("remote-token", map[string][]byte{tokenKey: []byte("token-a")}),
			},
			want: ClientAuth{CertificateAuthorityData: "ca-pem", ClientTokenData: "token-a"},
		},
		"CA secret missing its key": {
			secrets: []*corev1.Secret{
				newSecret("remote-ca", map[string][]byte{"ca.crt": []byte("ca-pem")}),
				newSecret("remote-token", map[string][]byte{tokenKey: []byte("token-a")}),
			},
			wantErr: `read CA: secret default/remote-ca has no "cadata" key`,
		},
		"token secret missing its key": {
			secrets: []*corev1.Secret{
				newSecret("remote-ca", map[string][]byte{caDataKey: []byte("ca-pem")}),
				newSecret("remote-token", map[string][]byte{"tok": []byte("token-a")}),
			},
			wantErr: `read token: secret default/remote-token has no "token" key`,
		},
		"token secret not found": {
			secrets: []*corev1.Secret{
				newSecret("remote-ca", map[string][]byte{caDataKey: []byte("ca-pem")}),
			},
			wantErr: "read token: get secret default/remote-token",
		},
		"CA secret not found": {
			secrets: []*corev1.Secret{
				newSecret("remote-token", map[string][]byte{tokenKey: []byte("token-a")}),
			},
			wantErr: "read CA: get secret default/remote-ca",
		},
		"empty token secret name": {
			secrets: []*corev1.Secret{
				newSecret("remote-ca", map[string][]byte{caDataKey: []byte("ca-pem")}),
			},
			cluster: &v2pb.ClusterTarget{
				ClusterId:  "remote",
				Connection: &v2pb.ClusterTarget_Kubernetes{Kubernetes: &v2pb.ConnectionSpec{CaDataTag: "remote-ca"}},
			},
			wantErr: "read token: empty secret name",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			builder := fake.NewClientBuilder()
			for _, secret := range tt.secrets {
				builder = builder.WithObjects(secret)
			}
			provider := NewProvider(builder.Build())

			cluster := tt.cluster
			if cluster == nil {
				cluster = clusterTarget()
			}
			got, err := provider.GetClientAuth(context.Background(), cluster)

			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
