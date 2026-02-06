package clientfactory

import (
	"context"
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/secrets"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/secrets/secretsmocks"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

func TestGetClient(t *testing.T) {
	scheme := runtime.NewScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	tests := []struct {
		name              string
		cluster           *v2pb.ClusterTarget
		setupMock         func(*secretsmocks.MockSecretProvider)
		wantDefaultClient bool
		wantErr           bool
		errContains       string
	}{
		{
			name:              "returns default client when cluster is nil",
			cluster:           nil,
			setupMock:         func(m *secretsmocks.MockSecretProvider) {},
			wantDefaultClient: true,
			wantErr:           false,
		},
		{
			name: "returns error when remote cluster missing kubernetes config",
			cluster: &v2pb.ClusterTarget{
				ClusterId: "remote-cluster",
			},
			setupMock:   func(m *secretsmocks.MockSecretProvider) {},
			wantErr:     true,
			errContains: "missing kubernetes connection details",
		},
		{
			name: "returns error when remote cluster missing host",
			cluster: &v2pb.ClusterTarget{
				ClusterId: "remote-cluster",
				Config: &v2pb.ClusterTarget_Kubernetes{
					Kubernetes: &v2pb.ConnectionSpec{
						Port: "6443",
					},
				},
			},
			setupMock:   func(m *secretsmocks.MockSecretProvider) {},
			wantErr:     true,
			errContains: "missing kubernetes connection details",
		},
		{
			name: "returns error when secret provider fails",
			cluster: &v2pb.ClusterTarget{
				ClusterId: "remote-cluster",
				Config: &v2pb.ClusterTarget_Kubernetes{
					Kubernetes: &v2pb.ConnectionSpec{
						Host:      "https://api.remote.cluster",
						Port:      "6443",
						TokenTag:  "token-secret",
						CaDataTag: "ca-secret",
					},
				},
			},
			setupMock: func(m *secretsmocks.MockSecretProvider) {
				m.EXPECT().
					GetClientAuth(gomock.Any(), gomock.Any()).
					Return(secrets.ClientAuth{}, fmt.Errorf("secret not found"))
			},
			wantErr:     true,
			errContains: "failed to get client auth",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockProvider := secretsmocks.NewMockSecretProvider(ctrl)
			tt.setupMock(mockProvider)

			factory := NewClientFactory(fakeClient, mockProvider, scheme, zap.NewNop())

			result, err := factory.GetClient(context.Background(), tt.cluster)

			if tt.wantErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.errContains)
				return
			}

			require.NoError(t, err)
			if tt.wantDefaultClient {
				require.Equal(t, fakeClient, result)
			}
		})
	}
}

func TestGetHTTPClient(t *testing.T) {
	scheme := runtime.NewScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	tests := []struct {
		name             string
		cluster          *v2pb.ClusterTarget
		setupMock        func(*secretsmocks.MockSecretProvider)
		wantSimpleClient bool
		wantErr          bool
		errContains      string
	}{
		{
			name:             "returns simple HTTP client when cluster is nil",
			cluster:          nil,
			setupMock:        func(m *secretsmocks.MockSecretProvider) {},
			wantSimpleClient: true,
			wantErr:          false,
		},
		{
			name: "returns error when remote cluster missing kubernetes config",
			cluster: &v2pb.ClusterTarget{
				ClusterId: "remote-cluster",
			},
			setupMock:   func(m *secretsmocks.MockSecretProvider) {},
			wantErr:     true,
			errContains: "missing kubernetes connection details",
		},
		{
			name: "returns error when secret provider fails",
			cluster: &v2pb.ClusterTarget{
				ClusterId: "remote-cluster",
				Config: &v2pb.ClusterTarget_Kubernetes{
					Kubernetes: &v2pb.ConnectionSpec{
						Host:      "https://api.remote.cluster",
						Port:      "6443",
						TokenTag:  "token-secret",
						CaDataTag: "ca-secret",
					},
				},
			},
			setupMock: func(m *secretsmocks.MockSecretProvider) {
				m.EXPECT().
					GetClientAuth(gomock.Any(), gomock.Any()).
					Return(secrets.ClientAuth{}, fmt.Errorf("secret not found"))
			},
			wantErr:     true,
			errContains: "failed to get client auth",
		},
		{
			name: "returns error when CA certificate is invalid",
			cluster: &v2pb.ClusterTarget{
				ClusterId: "remote-cluster",
				Config: &v2pb.ClusterTarget_Kubernetes{
					Kubernetes: &v2pb.ConnectionSpec{
						Host:      "https://api.remote.cluster",
						Port:      "6443",
						TokenTag:  "token-secret",
						CaDataTag: "ca-secret",
					},
				},
			},
			setupMock: func(m *secretsmocks.MockSecretProvider) {
				m.EXPECT().
					GetClientAuth(gomock.Any(), gomock.Any()).
					Return(secrets.ClientAuth{
						CertificateAuthorityData: "invalid-ca-data",
						ClientTokenData:          "test-token",
					}, nil)
			},
			wantErr:     true,
			errContains: "failed to parse CA certificate",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockProvider := secretsmocks.NewMockSecretProvider(ctrl)
			tt.setupMock(mockProvider)

			factory := NewClientFactory(fakeClient, mockProvider, scheme, zap.NewNop())

			result, err := factory.GetHTTPClient(context.Background(), tt.cluster)

			if tt.wantErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.errContains)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)
			if tt.wantSimpleClient {
				require.Nil(t, result.Transport)
			}
		})
	}
}
