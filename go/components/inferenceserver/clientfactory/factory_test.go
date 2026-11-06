package clientfactory

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/clientfactory/secrets"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/clientfactory/secrets/secretsmocks"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// selfSignedCAPEM returns a freshly generated, PEM-encoded self-signed certificate.
// Its contents don't matter for these tests beyond being valid PEM that
// x509.CertPool.AppendCertsFromPEM accepts.
func selfSignedCAPEM(t *testing.T) string {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test-ca"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour),
		IsCA:         true,
		KeyUsage:     x509.KeyUsageCertSign,
	}

	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)

	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

func testCluster(clusterID string) *v2pb.ClusterTarget {
	return &v2pb.ClusterTarget{
		ClusterId: clusterID,
		Connection: &v2pb.ClusterTarget_Kubernetes{
			Kubernetes: &v2pb.ConnectionSpec{
				Host:      "https://remote.example.com",
				Port:      "6443",
				TokenTag:  "token-secret",
				CaDataTag: "ca-secret",
			},
		},
	}
}

func newTestFactory(t *testing.T, secretProvider secrets.SecretProvider) *remoteClientFactory {
	t.Helper()
	return &remoteClientFactory{
		secretProvider: secretProvider,
		scheme:         runtime.NewScheme(),
		logger:         zap.NewNop(),
	}
}

func TestGetHTTPClient_RebuildsWhenCredentialsRotate(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockProvider := secretsmocks.NewMockSecretProvider(ctrl)
	factory := newTestFactory(t, mockProvider)
	cluster := testCluster("cluster-1")
	ctx := context.Background()

	caPEM := selfSignedCAPEM(t)
	originalAuth := secrets.ClientAuth{CertificateAuthorityData: caPEM, ClientTokenData: "token-v1"}
	rotatedAuth := secrets.ClientAuth{CertificateAuthorityData: caPEM, ClientTokenData: "token-v2"}

	mockProvider.EXPECT().GetClientAuth(ctx, cluster).Return(originalAuth, nil).Times(1)
	first, err := factory.GetHTTPClient(ctx, cluster)
	require.NoError(t, err)
	require.NotNil(t, first)

	// Same credentials on the next lookup: SecretProvider is still consulted (so
	// rotation is never missed), but the cached client is reused rather than rebuilt.
	mockProvider.EXPECT().GetClientAuth(ctx, cluster).Return(originalAuth, nil).Times(1)
	second, err := factory.GetHTTPClient(ctx, cluster)
	require.NoError(t, err)
	assert.Same(t, first, second, "unchanged credentials should return the cached client")

	// Credentials rotate: the factory must rebuild rather than keep serving the old client.
	mockProvider.EXPECT().GetClientAuth(ctx, cluster).Return(rotatedAuth, nil).Times(1)
	third, err := factory.GetHTTPClient(ctx, cluster)
	require.NoError(t, err)
	assert.NotSame(t, first, third, "rotated credentials must produce a new client")

	bearerRT, ok := third.Transport.(*bearerTokenRoundTripper)
	require.True(t, ok)
	assert.Equal(t, "token-v2", bearerRT.token, "the rebuilt client must carry the rotated token")
}

func TestGetHTTPClient_PropagatesSecretProviderError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockProvider := secretsmocks.NewMockSecretProvider(ctrl)
	factory := newTestFactory(t, mockProvider)
	cluster := testCluster("cluster-1")
	ctx := context.Background()

	mockProvider.EXPECT().GetClientAuth(ctx, cluster).Return(secrets.ClientAuth{}, assert.AnError)

	client, err := factory.GetHTTPClient(ctx, cluster)
	require.Error(t, err)
	assert.Nil(t, client)
}

func TestGetHTTPClient_RejectsClusterWithoutKubernetesSpec(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockProvider := secretsmocks.NewMockSecretProvider(ctrl)
	factory := newTestFactory(t, mockProvider)
	ctx := context.Background()

	client, err := factory.GetHTTPClient(ctx, &v2pb.ClusterTarget{ClusterId: "no-connection"})
	require.Error(t, err)
	assert.Nil(t, client)
}

func TestGetDynamicClient_RebuildsWhenCredentialsRotate(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockProvider := secretsmocks.NewMockSecretProvider(ctrl)
	factory := newTestFactory(t, mockProvider)
	cluster := testCluster("cluster-1")
	ctx := context.Background()

	caPEM := selfSignedCAPEM(t)
	originalAuth := secrets.ClientAuth{CertificateAuthorityData: caPEM, ClientTokenData: "token-v1"}
	rotatedAuth := secrets.ClientAuth{CertificateAuthorityData: caPEM, ClientTokenData: "token-v2"}

	mockProvider.EXPECT().GetClientAuth(ctx, cluster).Return(originalAuth, nil).Times(1)
	first, err := factory.GetDynamicClient(ctx, cluster)
	require.NoError(t, err)
	require.NotNil(t, first)

	mockProvider.EXPECT().GetClientAuth(ctx, cluster).Return(originalAuth, nil).Times(1)
	second, err := factory.GetDynamicClient(ctx, cluster)
	require.NoError(t, err)
	assert.Same(t, first, second, "unchanged credentials should return the cached client")

	mockProvider.EXPECT().GetClientAuth(ctx, cluster).Return(rotatedAuth, nil).Times(1)
	third, err := factory.GetDynamicClient(ctx, cluster)
	require.NoError(t, err)
	assert.NotSame(t, first, third, "rotated credentials must produce a new client")
}

func TestGetClient_RebuildsWhenCredentialsRotate(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockProvider := secretsmocks.NewMockSecretProvider(ctrl)
	factory := newTestFactory(t, mockProvider)
	cluster := testCluster("cluster-1")
	ctx := context.Background()

	caPEM := selfSignedCAPEM(t)
	originalAuth := secrets.ClientAuth{CertificateAuthorityData: caPEM, ClientTokenData: "token-v1"}
	rotatedAuth := secrets.ClientAuth{CertificateAuthorityData: caPEM, ClientTokenData: "token-v2"}

	mockProvider.EXPECT().GetClientAuth(ctx, cluster).Return(originalAuth, nil).Times(1)
	first, err := factory.GetClient(ctx, cluster)
	require.NoError(t, err)
	require.NotNil(t, first)

	mockProvider.EXPECT().GetClientAuth(ctx, cluster).Return(originalAuth, nil).Times(1)
	second, err := factory.GetClient(ctx, cluster)
	require.NoError(t, err)
	assert.Same(t, first, second, "unchanged credentials should return the cached client")

	mockProvider.EXPECT().GetClientAuth(ctx, cluster).Return(rotatedAuth, nil).Times(1)
	third, err := factory.GetClient(ctx, cluster)
	require.NoError(t, err)
	assert.NotSame(t, first, third, "rotated credentials must produce a new client")
}

func TestAuthFingerprint_DistinguishesTokenAndCA(t *testing.T) {
	base := secrets.ClientAuth{CertificateAuthorityData: "ca-1", ClientTokenData: "token-1"}
	sameAsBase := secrets.ClientAuth{CertificateAuthorityData: "ca-1", ClientTokenData: "token-1"}
	differentToken := secrets.ClientAuth{CertificateAuthorityData: "ca-1", ClientTokenData: "token-2"}
	differentCA := secrets.ClientAuth{CertificateAuthorityData: "ca-2", ClientTokenData: "token-1"}

	assert.Equal(t, authFingerprint(base), authFingerprint(sameAsBase))
	assert.NotEqual(t, authFingerprint(base), authFingerprint(differentToken))
	assert.NotEqual(t, authFingerprint(base), authFingerprint(differentCA))
}
