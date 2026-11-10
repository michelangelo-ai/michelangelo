package compute

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/secrets"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/secrets/secretsmocks"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// selfSignedCAPEM returns a freshly generated, PEM-encoded self-signed certificate.
// Its contents don't matter for these tests beyond being valid PEM that client-go's
// transport construction accepts, so a successful client build can be tested without
// a real Kubernetes cluster.
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

	t.Run("rebuilds the client set when credentials rotate", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		caPEM := selfSignedCAPEM(t)
		originalAuth := secrets.ClientAuth{CertificateAuthorityData: caPEM, ClientTokenData: "token-v1"}
		rotatedAuth := secrets.ClientAuth{CertificateAuthorityData: caPEM, ClientTokenData: "token-v2"}

		mockProvider := secretsmocks.NewMockSecretProvider(ctrl)
		f := NewClientSetFactory(mockProvider)

		mockProvider.EXPECT().GetClusterClientAuth(gomock.Any(), &testCluster).Return(originalAuth, nil).Times(1)
		first, err := f.GetClientSetForCluster(&testCluster)
		require.NoError(t, err)
		require.NotNil(t, first)

		// Same credentials on the next lookup: the SecretProvider is still consulted
		// (so rotation is never missed), but the cached ClientSet is reused.
		mockProvider.EXPECT().GetClusterClientAuth(gomock.Any(), &testCluster).Return(originalAuth, nil).Times(1)
		second, err := f.GetClientSetForCluster(&testCluster)
		require.NoError(t, err)
		assert.Same(t, first, second, "unchanged credentials should return the cached ClientSet")

		// Credentials rotate: the factory must rebuild rather than keep serving the old ClientSet.
		mockProvider.EXPECT().GetClusterClientAuth(gomock.Any(), &testCluster).Return(rotatedAuth, nil).Times(1)
		third, err := f.GetClientSetForCluster(&testCluster)
		require.NoError(t, err)
		assert.NotSame(t, first, third, "rotated credentials must produce a new ClientSet")
	})
}

func TestAuthFingerprint_DistinguishesAllCredentialFields(t *testing.T) {
	base := secrets.ClientAuth{
		CertificateAuthorityData: "ca-1",
		ClientCertificateData:    "cert-1",
		ClientKeyData:            "key-1",
		ClientTokenData:          "token-1",
	}
	sameAsBase := base
	variants := []secrets.ClientAuth{
		{CertificateAuthorityData: "ca-2", ClientCertificateData: "cert-1", ClientKeyData: "key-1", ClientTokenData: "token-1"},
		{CertificateAuthorityData: "ca-1", ClientCertificateData: "cert-2", ClientKeyData: "key-1", ClientTokenData: "token-1"},
		{CertificateAuthorityData: "ca-1", ClientCertificateData: "cert-1", ClientKeyData: "key-2", ClientTokenData: "token-1"},
		{CertificateAuthorityData: "ca-1", ClientCertificateData: "cert-1", ClientKeyData: "key-1", ClientTokenData: "token-2"},
	}

	assert.Equal(t, authFingerprint(base), authFingerprint(sameAsBase))
	for _, v := range variants {
		assert.NotEqual(t, authFingerprint(base), authFingerprint(v))
	}
}
