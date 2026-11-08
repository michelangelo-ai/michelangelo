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
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/secrets"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/secrets/secretsmocks"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

func TestNewClientSetFactory_Default_UsesCredentialReadTimeout(t *testing.T) {
	f, ok := NewClientSetFactory(nil, zap.NewNop()).(*factory)
	require.True(t, ok)
	assert.Equal(t, _credentialReadTimeout, f.credentialReadTimeout)
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
			Return(secrets.ClientAuth{}, errors.New("mock error"))

		f := NewClientSetFactory(mockProvider, zap.NewNop())

		_, err := f.GetClientSetForCluster(&testCluster)
		require.ErrorContains(t, err, "get client auth for cluster")
		require.ErrorContains(t, err, "mock error")
	})

	t.Run("SecretProvider error on the re-read under the lock", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockProvider := secretsmocks.NewMockSecretProvider(ctrl)
		gomock.InOrder(
			mockProvider.EXPECT().
				GetClusterClientAuth(gomock.Any(), &testCluster).
				Return(secrets.ClientAuth{ClientTokenData: "token-a"}, nil),
			mockProvider.EXPECT().
				GetClusterClientAuth(gomock.Any(), &testCluster).
				Return(secrets.ClientAuth{}, errors.New("mock error")),
		)

		f := NewClientSetFactory(mockProvider, zap.NewNop()).(*factory)

		_, err := f.GetClientSetForCluster(&testCluster)
		require.ErrorContains(t, err, "mock error")
		_, cached := f.clients.Load(getCacheKey(&testCluster))
		assert.False(t, cached)
	})

	t.Run("malformed host", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		badCluster := testCluster.DeepCopy()
		badCluster.Spec.GetKubernetes().GetRest().Host = "https://bad host"

		mockProvider := secretsmocks.NewMockSecretProvider(ctrl)
		mockProvider.EXPECT().
			GetClusterClientAuth(gomock.Any(), badCluster).
			Return(secrets.ClientAuth{ClientTokenData: "token-a"}, nil).
			Times(2)

		f := NewClientSetFactory(mockProvider, zap.NewNop()).(*factory)

		_, err := f.GetClientSetForCluster(badCluster)
		require.ErrorContains(t, err, "host must be a URL")
		_, cached := f.clients.Load(getCacheKey(badCluster))
		assert.False(t, cached)
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

// newSecretBackedFactory returns a factory backed by a secrets.Provider that reads the given
// Secrets, along with the fake client that holds those Secrets.
func newSecretBackedFactory(t *testing.T, objs ...client.Object) (Factory, client.Client) {
	t.Helper()
	kubeClient := fake.NewClientBuilder().WithObjects(objs...).Build()
	params := secrets.Params{KubeClient: kubeClient, Logger: zap.NewNop()}
	return NewClientSetFactory(secrets.New(params).SecretProvider, zap.NewNop()), kubeClient
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
	f, kubeClient := newSecretBackedFactory(t,
		newCASecret(_caSecretName, rc.caPEM), newTokenSecret(_tokenSecretName, "token-a"))
	cluster := rc.cluster(t, "remote", _caSecretName, _tokenSecretName)

	before, err := f.GetClientSetForCluster(cluster)
	require.NoError(t, err)
	require.NoError(t, getConfigMap(before))
	require.NoError(t, getRayJob(before))

	updateSecret(t, kubeClient, newTokenSecret(_tokenSecretName, "token-b"))
	after, err := f.GetClientSetForCluster(cluster)
	require.NoError(t, err)
	require.NoError(t, getConfigMap(after))
	require.NoError(t, getRayJob(after))

	assert.NotSame(t, before, after)
	assert.Equal(t, []string{"token-a", "token-a", "token-b", "token-b"}, rc.receivedTokens())
}

func TestGetClientSetForCluster_RotatedCASecret_TrustsNewCA(t *testing.T) {
	rc := newRemoteCluster(t)
	f, kubeClient := newSecretBackedFactory(t,
		newCASecret(_caSecretName, unrelatedCAPEM(t)), newTokenSecret(_tokenSecretName, "token-a"))
	cluster := rc.cluster(t, "remote", _caSecretName, _tokenSecretName)

	cs, err := f.GetClientSetForCluster(cluster)
	require.NoError(t, err)
	var unknownAuthority x509.UnknownAuthorityError
	require.ErrorAs(t, getConfigMap(cs), &unknownAuthority)

	updateSecret(t, kubeClient, newCASecret(_caSecretName, rc.caPEM))
	cs, err = f.GetClientSetForCluster(cluster)
	require.NoError(t, err)

	assert.NoError(t, getConfigMap(cs))
	assert.NoError(t, getRayJob(cs))
}

func TestGetClientSetForCluster_UnchangedCredentials_ReusesClientSet(t *testing.T) {
	rc := newRemoteCluster(t)
	f, _ := newSecretBackedFactory(t,
		newCASecret(_caSecretName, rc.caPEM), newTokenSecret(_tokenSecretName, "token-a"))
	cluster := rc.cluster(t, "remote", _caSecretName, _tokenSecretName)

	first, err := f.GetClientSetForCluster(cluster)
	require.NoError(t, err)
	second, err := f.GetClientSetForCluster(cluster)
	require.NoError(t, err)

	assert.Same(t, first, second)
}

func TestGetClientSetForCluster_DeletedTokenSecret_ReturnsErrorAndKeepsCache(t *testing.T) {
	rc := newRemoteCluster(t)
	tokenSecret := newTokenSecret(_tokenSecretName, "token-a")
	f, kubeClient := newSecretBackedFactory(t, newCASecret(_caSecretName, rc.caPEM), tokenSecret)
	cluster := rc.cluster(t, "remote", _caSecretName, _tokenSecretName)
	cached, err := f.GetClientSetForCluster(cluster)
	require.NoError(t, err)

	require.NoError(t, kubeClient.Delete(context.Background(), tokenSecret))
	_, err = f.GetClientSetForCluster(cluster)
	require.True(t, apierrors.IsNotFound(err), "got %v", err)

	restored := newTokenSecret(_tokenSecretName, "token-a")
	require.NoError(t, kubeClient.Create(context.Background(), restored))
	recovered, err := f.GetClientSetForCluster(cluster)
	require.NoError(t, err)
	assert.Same(t, cached, recovered)
}

func TestGetClientSetForCluster_SameServerDifferentCredentials_KeepsSeparateClientSets(
	t *testing.T,
) {
	rc := newRemoteCluster(t)
	f, _ := newSecretBackedFactory(t,
		newCASecret(_caSecretName, rc.caPEM),
		newTokenSecret("token-secret-a", "token-a"),
		newTokenSecret("token-secret-b", "token-b"))
	clusterA := rc.cluster(t, "remote-a", _caSecretName, "token-secret-a")
	clusterB := rc.cluster(t, "remote-b", _caSecretName, "token-secret-b")

	firstA, err := f.GetClientSetForCluster(clusterA)
	require.NoError(t, err)
	firstB, err := f.GetClientSetForCluster(clusterB)
	require.NoError(t, err)
	secondA, err := f.GetClientSetForCluster(clusterA)
	require.NoError(t, err)
	require.NoError(t, getConfigMap(secondA))
	require.NoError(t, getConfigMap(firstB))

	assert.Same(t, firstA, secondA)
	assert.Equal(t, []string{"token-a", "token-b"}, rc.receivedTokens())
}

// sequenceSecretProvider returns the credentials in auths in order, then repeats the last. auths
// must not be empty.
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
	auth := p.auths[0]
	if len(p.auths) > 1 {
		p.auths = p.auths[1:]
	}
	return auth, nil
}

func (p *sequenceSecretProvider) prepend(auths ...secrets.ClientAuth) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.auths = append(auths, p.auths...)
}

func TestGetClientSetForCluster_StaleCredentialsAfterRotation_KeepsNewerClientSet(t *testing.T) {
	rc := newRemoteCluster(t)
	older := secrets.ClientAuth{CertificateAuthorityData: rc.caPEM, ClientTokenData: "token-a"}
	newer := secrets.ClientAuth{CertificateAuthorityData: rc.caPEM, ClientTokenData: "token-b"}
	provider := &sequenceSecretProvider{auths: []secrets.ClientAuth{newer}}
	f := NewClientSetFactory(provider, zap.NewNop())
	cluster := rc.cluster(t, "remote", _caSecretName, _tokenSecretName)
	cached, err := f.GetClientSetForCluster(cluster)
	require.NoError(t, err)

	// A lookup whose first read raced a rotation sees the older credentials, then reads the
	// newer ones again under the lock.
	provider.prepend(older)
	got, err := f.GetClientSetForCluster(cluster)
	require.NoError(t, err)

	assert.Same(t, cached, got)
}

func TestGetClientSetForCluster_ConcurrentLookups_ShareOneClientSet(t *testing.T) {
	rc := newRemoteCluster(t)
	f, _ := newSecretBackedFactory(t,
		newCASecret(_caSecretName, rc.caPEM), newTokenSecret(_tokenSecretName, "token-a"))
	cluster := rc.cluster(t, "remote", _caSecretName, _tokenSecretName)

	results := make([]*ClientSet, 16)
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cs, err := f.GetClientSetForCluster(cluster)
			assert.NoError(t, err)
			results[i] = cs
		}()
	}
	wg.Wait()

	for _, cs := range results[1:] {
		assert.Same(t, results[0], cs)
	}
}

// blockingSecretProvider answers the first unblockedReads reads with auth and blocks every later
// one until its context is done, then fails the way a cache read does when the Secret informer
// never syncs.
type blockingSecretProvider struct {
	secrets.SecretProvider

	auth           secrets.ClientAuth
	unblockedReads int32
	reads          atomic.Int32
}

func (p *blockingSecretProvider) GetClusterClientAuth(
	ctx context.Context,
	_ *v2pb.Cluster,
) (secrets.ClientAuth, error) {
	if p.reads.Add(1) <= p.unblockedReads {
		return p.auth, nil
	}
	<-ctx.Done()
	return secrets.ClientAuth{}, apierrors.NewTimeoutError(
		"failed waiting for Secret informer to sync", 0)
}

// lookupWithin fails the test if GetClientSetForCluster does not return within limit.
func lookupWithin(t *testing.T, f Factory, c *v2pb.Cluster, limit time.Duration) error {
	t.Helper()
	done := make(chan error, 1)
	go func() {
		_, err := f.GetClientSetForCluster(c)
		done <- err
	}()
	select {
	case err := <-done:
		return err
	case <-time.After(limit):
		t.Fatalf("GetClientSetForCluster did not return within %s", limit)
		return nil
	}
}

func TestGetClientSetForCluster_BlockedCredentialRead_ReturnsTimeoutError(t *testing.T) {
	rc := newRemoteCluster(t)
	f := &factory{
		secretProvider:        &blockingSecretProvider{},
		logger:                zap.NewNop(),
		credentialReadTimeout: 10 * time.Millisecond,
	}
	cluster := rc.cluster(t, "remote", _caSecretName, _tokenSecretName)

	err := lookupWithin(t, f, cluster, 5*time.Second)

	assert.True(t, apierrors.IsTimeout(err), "got %v", err)
}

func TestGetClientSetForCluster_BlockedReadUnderLock_ReturnsTimeoutErrorAndReleasesLock(
	t *testing.T,
) {
	rc := newRemoteCluster(t)
	auth := secrets.ClientAuth{CertificateAuthorityData: rc.caPEM, ClientTokenData: "token-a"}
	provider := &blockingSecretProvider{auth: auth, unblockedReads: 1}
	f := &factory{
		secretProvider:        provider,
		logger:                zap.NewNop(),
		credentialReadTimeout: 10 * time.Millisecond,
	}
	cluster := rc.cluster(t, "remote", _caSecretName, _tokenSecretName)

	err := lookupWithin(t, f, cluster, 5*time.Second)

	assert.True(t, apierrors.IsTimeout(err), "got %v", err)
	assert.EqualValues(t, 2, provider.reads.Load())
	mu := f.lockFor(getCacheKey(cluster))
	require.True(t, mu.TryLock())
	mu.Unlock()
}

// stallingSecretProvider answers every read with auth, except the second read for the cluster
// named stalledCluster: that one signals readStarted and waits for release.
type stallingSecretProvider struct {
	secrets.SecretProvider

	auth           secrets.ClientAuth
	stalledCluster string
	readStarted    chan struct{}
	release        chan struct{}
	stalledReads   atomic.Int32
}

func (p *stallingSecretProvider) GetClusterClientAuth(
	ctx context.Context,
	c *v2pb.Cluster,
) (secrets.ClientAuth, error) {
	if c.GetName() != p.stalledCluster || p.stalledReads.Add(1) != 2 {
		return p.auth, nil
	}
	close(p.readStarted)
	select {
	case <-p.release:
		return p.auth, nil
	case <-ctx.Done():
		return secrets.ClientAuth{}, ctx.Err()
	}
}

func TestGetClientSetForCluster_StalledReadForOneCluster_ServesOtherClusters(t *testing.T) {
	rc := newRemoteCluster(t)
	provider := &stallingSecretProvider{
		auth:           secrets.ClientAuth{CertificateAuthorityData: rc.caPEM, ClientTokenData: "token"},
		stalledCluster: "remote-a",
		readStarted:    make(chan struct{}),
		release:        make(chan struct{}),
	}
	release := sync.OnceFunc(func() { close(provider.release) })
	t.Cleanup(release)
	f := &factory{secretProvider: provider, logger: zap.NewNop(), credentialReadTimeout: time.Minute}
	clusterA := rc.cluster(t, "remote-a", _caSecretName, _tokenSecretName)
	clusterB := rc.cluster(t, "remote-b", _caSecretName, _tokenSecretName)

	stalled := make(chan error, 1)
	go func() {
		_, err := f.GetClientSetForCluster(clusterA)
		stalled <- err
	}()
	select {
	case <-provider.readStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("the stalled cluster's build never re-read its credentials under the lock")
	}

	require.NoError(t, lookupWithin(t, f, clusterB, 5*time.Second))
	require.NoError(t, lookupWithin(t, f, clusterB, 5*time.Second))

	release()
	assert.NoError(t, <-stalled)
}

func TestGetClientSetForCluster_CAAndTokenRotatedTogether_UsesBoth(t *testing.T) {
	rc := newRemoteCluster(t)
	f, kubeClient := newSecretBackedFactory(t,
		newCASecret(_caSecretName, unrelatedCAPEM(t)), newTokenSecret(_tokenSecretName, "token-a"))
	cluster := rc.cluster(t, "remote", _caSecretName, _tokenSecretName)
	before, err := f.GetClientSetForCluster(cluster)
	require.NoError(t, err)
	var unknownAuthority x509.UnknownAuthorityError
	require.ErrorAs(t, getConfigMap(before), &unknownAuthority)

	updateSecret(t, kubeClient, newCASecret(_caSecretName, rc.caPEM))
	updateSecret(t, kubeClient, newTokenSecret(_tokenSecretName, "token-b"))
	after, err := f.GetClientSetForCluster(cluster)
	require.NoError(t, err)

	assert.NotSame(t, before, after)
	assert.NoError(t, getConfigMap(after))
	assert.NoError(t, getRayJob(after))
	assert.Equal(t, []string{"token-b", "token-b"}, rc.receivedTokens())
}

func TestGetClientSetForCluster_RotationDuringConcurrentLookups_EndsOnLatestToken(t *testing.T) {
	rc := newRemoteCluster(t)
	f, kubeClient := newSecretBackedFactory(t,
		newCASecret(_caSecretName, rc.caPEM), newTokenSecret(_tokenSecretName, "token-0"))
	cluster := rc.cluster(t, "remote", _caSecretName, _tokenSecretName)

	const rotations = 5
	stop := make(chan struct{})
	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		seen = make(map[*ClientSet]struct{})
	)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				cs, err := f.GetClientSetForCluster(cluster)
				assert.NoError(t, err)
				mu.Lock()
				seen[cs] = struct{}{}
				mu.Unlock()
			}
		}()
	}
	for i := 1; i <= rotations; i++ {
		updateSecret(t, kubeClient, newTokenSecret(_tokenSecretName, fmt.Sprintf("token-%d", i)))
	}
	close(stop)
	wg.Wait()

	cs, err := f.GetClientSetForCluster(cluster)
	require.NoError(t, err)
	require.NoError(t, getConfigMap(cs))
	tokens := rc.receivedTokens()
	assert.Equal(t, "token-5", tokens[len(tokens)-1])
	// One ClientSet per token at most: a duplicate build, or a stale read rebuilding an older
	// token over a newer one, would hand out more.
	assert.LessOrEqual(t, len(seen), rotations+1)
}

func TestGetClientSetForCluster_TokenSecretMissingKey_ReturnsErrorAndKeepsCache(t *testing.T) {
	rc := newRemoteCluster(t)
	f, kubeClient := newSecretBackedFactory(t,
		newCASecret(_caSecretName, rc.caPEM), newTokenSecret(_tokenSecretName, "token-a"))
	cluster := rc.cluster(t, "remote", _caSecretName, _tokenSecretName)
	cached, err := f.GetClientSetForCluster(cluster)
	require.NoError(t, err)

	updateSecret(t, kubeClient, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: _tokenSecretName, Namespace: "default"},
		Data:       map[string][]byte{"wrong-key": []byte("token-b")},
	})
	cs, err := f.GetClientSetForCluster(cluster)
	assert.ErrorContains(t, err, `key "token" is missing or empty`)
	assert.Nil(t, cs)

	updateSecret(t, kubeClient, newTokenSecret(_tokenSecretName, "token-a"))
	recovered, err := f.GetClientSetForCluster(cluster)
	require.NoError(t, err)
	assert.Same(t, cached, recovered)
}

func TestGetClientSetForCluster_ClusterMovedToNewServer_SendsToNewServer(t *testing.T) {
	// httptest TLS servers share one certificate, so the credentials stay the same and only the
	// server address changes.
	oldServer := newRemoteCluster(t)
	newServer := newRemoteCluster(t)
	f, _ := newSecretBackedFactory(t,
		newCASecret(_caSecretName, oldServer.caPEM), newTokenSecret(_tokenSecretName, "token-a"))

	before, err := f.GetClientSetForCluster(
		oldServer.cluster(t, "remote", _caSecretName, _tokenSecretName))
	require.NoError(t, err)
	after, err := f.GetClientSetForCluster(
		newServer.cluster(t, "remote", _caSecretName, _tokenSecretName))
	require.NoError(t, err)
	require.NoError(t, getConfigMap(after))

	assert.NotSame(t, before, after)
	assert.Empty(t, oldServer.receivedTokens())
	assert.Equal(t, []string{"token-a"}, newServer.receivedTokens())
}

func TestGetClientSetForCluster_RotatedToInvalidCA_ReturnsErrorInsteadOfCachedClientSet(t *testing.T) {
	rc := newRemoteCluster(t)
	f, kubeClient := newSecretBackedFactory(t,
		newCASecret(_caSecretName, rc.caPEM), newTokenSecret(_tokenSecretName, "token-a"))
	cluster := rc.cluster(t, "remote", _caSecretName, _tokenSecretName)
	_, err := f.GetClientSetForCluster(cluster)
	require.NoError(t, err)

	updateSecret(t, kubeClient, newCASecret(_caSecretName, "not a PEM block"))
	cs, err := f.GetClientSetForCluster(cluster)

	assert.ErrorContains(t, err, "build client set for cluster")
	assert.Nil(t, cs)
}

func TestGetClientSetForCluster_LogsBuildAndRebuildWithoutSecretValues(t *testing.T) {
	rc := newRemoteCluster(t)
	kubeClient := fake.NewClientBuilder().WithObjects(
		newCASecret(_caSecretName, rc.caPEM), newTokenSecret(_tokenSecretName, "token-a")).Build()
	provider := secrets.New(secrets.Params{KubeClient: kubeClient, Logger: zap.NewNop()}).SecretProvider
	core, logs := observer.New(zap.InfoLevel)
	f := NewClientSetFactory(provider, zap.New(core))
	cluster := rc.cluster(t, "remote", _caSecretName, _tokenSecretName)

	_, err := f.GetClientSetForCluster(cluster)
	require.NoError(t, err)
	_, err = f.GetClientSetForCluster(cluster)
	require.NoError(t, err)
	updateSecret(t, kubeClient, newTokenSecret(_tokenSecretName, "token-b"))
	_, err = f.GetClientSetForCluster(cluster)
	require.NoError(t, err)

	entries := logs.AllUntimed()
	require.Len(t, entries, 2)
	assert.Equal(t, "Built client set for cluster", entries[0].Message)
	assert.Equal(t, "Rebuilt client set for cluster after its credentials changed", entries[1].Message)
	rebuilt := entries[1].ContextMap()
	assert.Equal(t, "remote", rebuilt["cluster"])
	assert.Equal(t, _tokenSecretName, rebuilt["token_secret"])
	assert.Equal(t, _caSecretName, rebuilt["ca_secret"])
	assert.Equal(t, true, rebuilt["token_changed"])
	assert.Equal(t, false, rebuilt["ca_changed"])
	for _, entry := range entries {
		for _, value := range entry.ContextMap() {
			text := fmt.Sprint(value)
			for _, secret := range []string{"token-a", "token-b", rc.caPEM} {
				assert.NotContains(t, text, secret)
			}
		}
	}
}
