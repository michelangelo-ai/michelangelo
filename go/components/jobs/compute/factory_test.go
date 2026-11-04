package compute

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/secrets"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/secrets/secretsmocks"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
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
		require.Contains(t, err.Error(), "get client auth for cluster")
		require.Contains(t, err.Error(), "mock error")
	})
}

const (
	_caSecretName    = "ca-secret"
	_tokenSecretName = "token-secret"
)

// remoteCluster is a TLS API server stand-in that records the bearer token of every request
// and answers each one with an empty JSON object.
type remoteCluster struct {
	server *httptest.Server
	caPEM  string

	mu     sync.Mutex
	tokens []string
}

func newRemoteCluster(t *testing.T) *remoteCluster {
	t.Helper()
	rc := &remoteCluster{}
	rc.server = httptest.NewTLSServer(http.HandlerFunc(rc.serve))
	t.Cleanup(rc.server.Close)
	certBlock := &pem.Block{Type: "CERTIFICATE", Bytes: rc.server.Certificate().Raw}
	rc.caPEM = string(pem.EncodeToMemory(certBlock))
	return rc
}

func (rc *remoteCluster) serve(w http.ResponseWriter, r *http.Request) {
	rc.mu.Lock()
	rc.tokens = append(rc.tokens, strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	rc.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, `{}`)
}

func (rc *remoteCluster) receivedTokens() []string {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	return append([]string(nil), rc.tokens...)
}

// cluster returns a Cluster named name that points at the server and reads its credentials
// from the Secrets named caSecret and tokenSecret.
func (rc *remoteCluster) cluster(t *testing.T, name, caSecret, tokenSecret string) *v2pb.Cluster {
	t.Helper()
	u, err := url.Parse(rc.server.URL)
	require.NoError(t, err)
	return &v2pb.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: constants.ClustersNamespace},
		Spec: v2pb.ClusterSpec{
			Cluster: &v2pb.ClusterSpec_Kubernetes{
				Kubernetes: &v2pb.KubernetesSpec{
					Rest: &v2pb.ConnectionSpec{
						Host:      "https://" + u.Hostname(),
						Port:      u.Port(),
						CaDataTag: caSecret,
						TokenTag:  tokenSecret,
					},
				},
			},
		},
	}
}

func newCASecret(name, caPEM string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Data:       map[string][]byte{"cadata": []byte(caPEM)},
	}
}

func newTokenSecret(name, token string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Data:       map[string][]byte{"token": []byte(token)},
	}
}

// newSecretBackedFactory returns a factory whose credentials come from the sample
// secrets.Provider reading the given Secrets, and the client that serves those Secrets.
func newSecretBackedFactory(t *testing.T, objs ...client.Object) (Factory, client.Client) {
	t.Helper()
	kubeClient := fake.NewClientBuilder().WithObjects(objs...).Build()
	params := secrets.Params{KubeClient: kubeClient, Logger: zap.NewNop()}
	return NewClientSetFactory(secrets.New(params).SecretProvider), kubeClient
}

func updateSecret(t *testing.T, kubeClient client.Client, secret *corev1.Secret) {
	t.Helper()
	current := &corev1.Secret{}
	key := client.ObjectKeyFromObject(secret)
	require.NoError(t, kubeClient.Get(context.Background(), key, current))
	current.Data = secret.Data
	require.NoError(t, kubeClient.Update(context.Background(), current))
}

func getConfigMap(cs *ClientSet) error {
	return cs.CoreV1.Get().Namespace("default").Resource("configmaps").Name("cm").
		Do(context.Background()).Error()
}

func getRayJob(cs *ClientSet) error {
	return cs.Ray.Get().Namespace("default").Resource("rayjobs").Name("job").
		Do(context.Background()).Error()
}

// unrelatedCAPEM returns a self-signed CA certificate that did not sign the test server's
// certificate.
func unrelatedCAPEM(t *testing.T) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "unrelated-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

func TestGetClientSetForCluster_RotatedTokenSecret_SendsNewToken(t *testing.T) {
	rc := newRemoteCluster(t)
	factory, kubeClient := newSecretBackedFactory(t,
		newCASecret(_caSecretName, rc.caPEM), newTokenSecret(_tokenSecretName, "token-a"))
	cluster := rc.cluster(t, "remote", _caSecretName, _tokenSecretName)

	before, err := factory.GetClientSetForCluster(cluster)
	require.NoError(t, err)
	require.NoError(t, getConfigMap(before))
	require.NoError(t, getRayJob(before))

	updateSecret(t, kubeClient, newTokenSecret(_tokenSecretName, "token-b"))
	after, err := factory.GetClientSetForCluster(cluster)
	require.NoError(t, err)
	require.NoError(t, getConfigMap(after))
	require.NoError(t, getRayJob(after))

	assert.NotSame(t, before, after)
	assert.Equal(t, []string{"token-a", "token-a", "token-b", "token-b"}, rc.receivedTokens())
}

func TestGetClientSetForCluster_RotatedCASecret_TrustsNewCA(t *testing.T) {
	rc := newRemoteCluster(t)
	factory, kubeClient := newSecretBackedFactory(t,
		newCASecret(_caSecretName, unrelatedCAPEM(t)), newTokenSecret(_tokenSecretName, "token-a"))
	cluster := rc.cluster(t, "remote", _caSecretName, _tokenSecretName)

	cs, err := factory.GetClientSetForCluster(cluster)
	require.NoError(t, err)
	var unknownAuthority x509.UnknownAuthorityError
	require.ErrorAs(t, getConfigMap(cs), &unknownAuthority)

	updateSecret(t, kubeClient, newCASecret(_caSecretName, rc.caPEM))
	cs, err = factory.GetClientSetForCluster(cluster)
	require.NoError(t, err)

	assert.NoError(t, getConfigMap(cs))
	assert.NoError(t, getRayJob(cs))
}

func TestGetClientSetForCluster_UnchangedCredentials_ReusesClientSet(t *testing.T) {
	rc := newRemoteCluster(t)
	factory, _ := newSecretBackedFactory(t,
		newCASecret(_caSecretName, rc.caPEM), newTokenSecret(_tokenSecretName, "token-a"))
	cluster := rc.cluster(t, "remote", _caSecretName, _tokenSecretName)

	first, err := factory.GetClientSetForCluster(cluster)
	require.NoError(t, err)
	second, err := factory.GetClientSetForCluster(cluster)
	require.NoError(t, err)

	assert.Same(t, first, second)
}

func TestGetClientSetForCluster_DeletedTokenSecret_ReturnsErrorAndKeepsCache(t *testing.T) {
	rc := newRemoteCluster(t)
	tokenSecret := newTokenSecret(_tokenSecretName, "token-a")
	factory, kubeClient := newSecretBackedFactory(t, newCASecret(_caSecretName, rc.caPEM), tokenSecret)
	cluster := rc.cluster(t, "remote", _caSecretName, _tokenSecretName)
	cached, err := factory.GetClientSetForCluster(cluster)
	require.NoError(t, err)

	require.NoError(t, kubeClient.Delete(context.Background(), tokenSecret))
	_, err = factory.GetClientSetForCluster(cluster)
	assert.ErrorContains(t, err, "not found")

	restored := newTokenSecret(_tokenSecretName, "token-a")
	require.NoError(t, kubeClient.Create(context.Background(), restored))
	recovered, err := factory.GetClientSetForCluster(cluster)
	require.NoError(t, err)
	assert.Same(t, cached, recovered)
}

func TestGetClientSetForCluster_SameServerDifferentCredentials_KeepsSeparateClientSets(
	t *testing.T,
) {
	rc := newRemoteCluster(t)
	factory, _ := newSecretBackedFactory(t,
		newCASecret(_caSecretName, rc.caPEM),
		newTokenSecret("token-secret-a", "token-a"),
		newTokenSecret("token-secret-b", "token-b"))
	clusterA := rc.cluster(t, "remote-a", _caSecretName, "token-secret-a")
	clusterB := rc.cluster(t, "remote-b", _caSecretName, "token-secret-b")

	firstA, err := factory.GetClientSetForCluster(clusterA)
	require.NoError(t, err)
	firstB, err := factory.GetClientSetForCluster(clusterB)
	require.NoError(t, err)
	secondA, err := factory.GetClientSetForCluster(clusterA)
	require.NoError(t, err)
	require.NoError(t, getConfigMap(secondA))
	require.NoError(t, getConfigMap(firstB))

	assert.Same(t, firstA, secondA)
	assert.Equal(t, []string{"token-a", "token-b"}, rc.receivedTokens())
}

// sequenceSecretProvider returns the credentials in auths in order, then repeats the last.
type sequenceSecretProvider struct {
	secrets.SecretProvider

	mu    sync.Mutex
	auths []secrets.ClientAuth
}

func (p *sequenceSecretProvider) GetClusterClientAuth(
	context.Context,
	*v2pb.Cluster,
) (secrets.ClientAuth, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.auths) == 0 {
		return secrets.ClientAuth{}, errors.New("no credentials")
	}
	auth := p.auths[0]
	if len(p.auths) > 1 {
		p.auths = p.auths[1:]
	}
	return auth, nil
}

func (p *sequenceSecretProvider) push(auths ...secrets.ClientAuth) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.auths = append(auths, p.auths...)
}

func TestGetClientSetForCluster_StaleCredentialsAfterRotation_KeepsNewerClientSet(t *testing.T) {
	rc := newRemoteCluster(t)
	older := secrets.ClientAuth{CertificateAuthorityData: rc.caPEM, ClientTokenData: "token-a"}
	newer := secrets.ClientAuth{CertificateAuthorityData: rc.caPEM, ClientTokenData: "token-b"}
	provider := &sequenceSecretProvider{auths: []secrets.ClientAuth{newer}}
	factory := NewClientSetFactory(provider)
	cluster := rc.cluster(t, "remote", _caSecretName, _tokenSecretName)
	cached, err := factory.GetClientSetForCluster(cluster)
	require.NoError(t, err)

	// A lookup whose first read raced a rotation sees the older credentials, then reads the
	// newer ones again under the lock.
	provider.push(older)
	got, err := factory.GetClientSetForCluster(cluster)
	require.NoError(t, err)

	assert.Same(t, cached, got)
}

func TestGetClientSetForCluster_ConcurrentLookups_ShareOneClientSet(t *testing.T) {
	rc := newRemoteCluster(t)
	factory, _ := newSecretBackedFactory(t,
		newCASecret(_caSecretName, rc.caPEM), newTokenSecret(_tokenSecretName, "token-a"))
	cluster := rc.cluster(t, "remote", _caSecretName, _tokenSecretName)

	results := make([]*ClientSet, 16)
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cs, err := factory.GetClientSetForCluster(cluster)
			assert.NoError(t, err)
			results[i] = cs
		}()
	}
	wg.Wait()

	for _, cs := range results[1:] {
		assert.Same(t, results[0], cs)
	}
}

// blockingSecretProvider blocks every read until its context is done, like a cache read
// waiting on a Secret informer that never syncs.
type blockingSecretProvider struct {
	secrets.SecretProvider
}

func (blockingSecretProvider) GetClusterClientAuth(
	ctx context.Context,
	_ *v2pb.Cluster,
) (secrets.ClientAuth, error) {
	<-ctx.Done()
	return secrets.ClientAuth{}, ctx.Err()
}

func TestGetClientSetForCluster_BlockedCredentialRead_ReturnsDeadlineError(t *testing.T) {
	rc := newRemoteCluster(t)
	f := &factory{
		secretProvider:        blockingSecretProvider{},
		credentialReadTimeout: 10 * time.Millisecond,
	}

	_, err := f.GetClientSetForCluster(rc.cluster(t, "remote", _caSecretName, _tokenSecretName))

	assert.ErrorIs(t, err, context.DeadlineExceeded)
}
