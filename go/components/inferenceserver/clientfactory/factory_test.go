package clientfactory

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/clientfactory/secrets"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

type fakeSecretProvider struct {
	mu   sync.Mutex
	auth secrets.ClientAuth
	err  error
	// stale credentials are returned, in order, before auth.
	stale []secrets.ClientAuth
	calls int
}

func (p *fakeSecretProvider) GetClientAuth(context.Context, *v2pb.ClusterTarget) (secrets.ClientAuth, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	if len(p.stale) > 0 {
		auth := p.stale[0]
		p.stale = p.stale[1:]
		return auth, nil
	}
	return p.auth, p.err
}

func (p *fakeSecretProvider) callCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

func (p *fakeSecretProvider) set(auth secrets.ClientAuth, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.auth = auth
	p.err = err
}

// remoteCluster is a TLS API server stand-in that records the bearer token of every request
// and answers each one with a ConfigMap.
type remoteCluster struct {
	server *httptest.Server
	caPEM  string

	mu     sync.Mutex
	tokens []string
	closed int
}

func newRemoteCluster(t *testing.T) *remoteCluster {
	t.Helper()
	rc := &remoteCluster{}
	rc.server = httptest.NewUnstartedServer(http.HandlerFunc(rc.serveConfigMap))
	rc.server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateClosed {
			rc.mu.Lock()
			rc.closed++
			rc.mu.Unlock()
		}
	}
	rc.server.StartTLS()
	t.Cleanup(rc.server.Close)
	rc.caPEM = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: rc.server.Certificate().Raw}))
	return rc
}

func (rc *remoteCluster) serveConfigMap(w http.ResponseWriter, r *http.Request) {
	rc.mu.Lock()
	rc.tokens = append(rc.tokens, strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	rc.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, `{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"cm","namespace":"default"}}`)
}

func (rc *remoteCluster) target(t *testing.T) *v2pb.ClusterTarget {
	t.Helper()
	u, err := url.Parse(rc.server.URL)
	require.NoError(t, err)
	return &v2pb.ClusterTarget{
		ClusterId: "remote",
		Connection: &v2pb.ClusterTarget_Kubernetes{Kubernetes: &v2pb.ConnectionSpec{
			Host: "https://" + u.Hostname(),
			Port: u.Port(),
		}},
	}
}

func (rc *remoteCluster) receivedTokens() []string {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	return append([]string(nil), rc.tokens...)
}

func (rc *remoteCluster) closedConnections() int {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	return rc.closed
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

func getHTTP(t *testing.T, httpClient *http.Client, rawURL string) error {
	t.Helper()
	resp, err := httpClient.Get(rawURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, err = io.Copy(io.Discard, resp.Body)
	return err
}

type getter func(ClientFactory, context.Context, *v2pb.ClusterTarget) (any, error)

var getters = map[string]getter{
	"GetClient": func(f ClientFactory, ctx context.Context, c *v2pb.ClusterTarget) (any, error) {
		return f.GetClient(ctx, c)
	},
	"GetHTTPClient": func(f ClientFactory, ctx context.Context, c *v2pb.ClusterTarget) (any, error) {
		return f.GetHTTPClient(ctx, c)
	},
	"GetDynamicClient": func(f ClientFactory, ctx context.Context, c *v2pb.ClusterTarget) (any, error) {
		return f.GetDynamicClient(ctx, c)
	},
}

func newTestFactory(provider secrets.SecretProvider) ClientFactory {
	return NewRemoteClientFactory(provider, runtime.NewScheme(), zap.NewNop())
}

func TestRemoteClientFactory_UnchangedCredentials_ReusesClient(t *testing.T) {
	rc := newRemoteCluster(t)
	auth := secrets.ClientAuth{CertificateAuthorityData: rc.caPEM, ClientTokenData: "token-a"}

	for name, get := range getters {
		t.Run(name, func(t *testing.T) {
			factory := newTestFactory(&fakeSecretProvider{auth: auth})

			first, err := get(factory, context.Background(), rc.target(t))
			require.NoError(t, err)
			second, err := get(factory, context.Background(), rc.target(t))
			require.NoError(t, err)

			assert.Same(t, first, second)
		})
	}
}

func TestRemoteClientFactory_RotatedCredentials_RebuildsClient(t *testing.T) {
	rc := newRemoteCluster(t)
	original := secrets.ClientAuth{CertificateAuthorityData: rc.caPEM, ClientTokenData: "token-a"}

	rotations := map[string]secrets.ClientAuth{
		"token": {CertificateAuthorityData: rc.caPEM, ClientTokenData: "token-b"},
		"CA":    {CertificateAuthorityData: unrelatedCAPEM(t), ClientTokenData: "token-a"},
	}
	for rotated, auth := range rotations {
		for name, get := range getters {
			t.Run(rotated+"/"+name, func(t *testing.T) {
				provider := &fakeSecretProvider{auth: original}
				factory := newTestFactory(provider)

				before, err := get(factory, context.Background(), rc.target(t))
				require.NoError(t, err)
				provider.set(auth, nil)
				after, err := get(factory, context.Background(), rc.target(t))
				require.NoError(t, err)

				assert.NotSame(t, before, after)
			})
		}
	}
}

func TestRemoteClientFactory_GetHTTPClient_RotatedToken_SendsNewToken(t *testing.T) {
	rc := newRemoteCluster(t)
	provider := &fakeSecretProvider{
		auth: secrets.ClientAuth{CertificateAuthorityData: rc.caPEM, ClientTokenData: "token-a"},
	}
	factory := newTestFactory(provider)

	httpClient, err := factory.GetHTTPClient(context.Background(), rc.target(t))
	require.NoError(t, err)
	require.NoError(t, getHTTP(t, httpClient, rc.server.URL))

	provider.set(secrets.ClientAuth{CertificateAuthorityData: rc.caPEM, ClientTokenData: "token-b"}, nil)
	httpClient, err = factory.GetHTTPClient(context.Background(), rc.target(t))
	require.NoError(t, err)
	require.NoError(t, getHTTP(t, httpClient, rc.server.URL))

	assert.Equal(t, []string{"token-a", "token-b"}, rc.receivedTokens())
}

func TestRemoteClientFactory_GetDynamicClient_RotatedToken_SendsNewToken(t *testing.T) {
	rc := newRemoteCluster(t)
	provider := &fakeSecretProvider{
		auth: secrets.ClientAuth{CertificateAuthorityData: rc.caPEM, ClientTokenData: "token-a"},
	}
	factory := newTestFactory(provider)
	configMaps := schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}

	for _, token := range []string{"token-a", "token-b"} {
		provider.set(secrets.ClientAuth{CertificateAuthorityData: rc.caPEM, ClientTokenData: token}, nil)
		dynamicClient, err := factory.GetDynamicClient(context.Background(), rc.target(t))
		require.NoError(t, err)
		_, err = dynamicClient.Resource(configMaps).Namespace("default").
			Get(context.Background(), "cm", metav1.GetOptions{})
		require.NoError(t, err)
	}

	assert.Equal(t, []string{"token-a", "token-b"}, rc.receivedTokens())
}

func TestRemoteClientFactory_GetHTTPClient_RotatedCA_TrustsNewCA(t *testing.T) {
	rc := newRemoteCluster(t)
	provider := &fakeSecretProvider{
		auth: secrets.ClientAuth{CertificateAuthorityData: unrelatedCAPEM(t), ClientTokenData: "token-a"},
	}
	factory := newTestFactory(provider)

	httpClient, err := factory.GetHTTPClient(context.Background(), rc.target(t))
	require.NoError(t, err)
	var unknownAuthority x509.UnknownAuthorityError
	require.ErrorAs(t, getHTTP(t, httpClient, rc.server.URL), &unknownAuthority)

	provider.set(secrets.ClientAuth{CertificateAuthorityData: rc.caPEM, ClientTokenData: "token-a"}, nil)
	httpClient, err = factory.GetHTTPClient(context.Background(), rc.target(t))
	require.NoError(t, err)

	assert.NoError(t, getHTTP(t, httpClient, rc.server.URL))
}

func TestRemoteClientFactory_GetHTTPClient_RotatedToken_ClosesReplacedIdleConnections(t *testing.T) {
	rc := newRemoteCluster(t)
	provider := &fakeSecretProvider{
		auth: secrets.ClientAuth{CertificateAuthorityData: rc.caPEM, ClientTokenData: "token-a"},
	}
	factory := newTestFactory(provider)

	httpClient, err := factory.GetHTTPClient(context.Background(), rc.target(t))
	require.NoError(t, err)
	require.NoError(t, getHTTP(t, httpClient, rc.server.URL))
	require.Zero(t, rc.closedConnections())

	provider.set(secrets.ClientAuth{CertificateAuthorityData: rc.caPEM, ClientTokenData: "token-b"}, nil)
	_, err = factory.GetHTTPClient(context.Background(), rc.target(t))
	require.NoError(t, err)

	assert.Eventually(t, func() bool { return rc.closedConnections() == 1 }, 5*time.Second, 10*time.Millisecond)
}

func TestRemoteClientFactory_SecretProviderError_ReturnsErrorAndKeepsCache(t *testing.T) {
	rc := newRemoteCluster(t)
	auth := secrets.ClientAuth{CertificateAuthorityData: rc.caPEM, ClientTokenData: "token-a"}

	for name, get := range getters {
		t.Run(name, func(t *testing.T) {
			provider := &fakeSecretProvider{auth: auth}
			factory := newTestFactory(provider)
			cached, err := get(factory, context.Background(), rc.target(t))
			require.NoError(t, err)

			provider.set(secrets.ClientAuth{}, errors.New("token secret not found"))
			_, err = get(factory, context.Background(), rc.target(t))
			assert.ErrorContains(t, err, "token secret not found")

			provider.set(auth, nil)
			recovered, err := get(factory, context.Background(), rc.target(t))
			require.NoError(t, err)
			assert.Same(t, cached, recovered)
		})
	}
}

func TestRemoteClientFactory_InvalidCAPEM_ReturnsError(t *testing.T) {
	rc := newRemoteCluster(t)
	factory := newTestFactory(&fakeSecretProvider{
		auth: secrets.ClientAuth{CertificateAuthorityData: "not a certificate", ClientTokenData: "token-a"},
	})

	httpClient, err := factory.GetHTTPClient(context.Background(), rc.target(t))

	assert.ErrorContains(t, err, "invalid PEM")
	assert.Nil(t, httpClient)
}

func TestRemoteClientFactory_NoKubernetesSpec_ReturnsErrorWithoutReadingSecrets(t *testing.T) {
	for name, get := range getters {
		t.Run(name, func(t *testing.T) {
			provider := &fakeSecretProvider{}
			factory := newTestFactory(provider)

			_, err := get(factory, context.Background(), &v2pb.ClusterTarget{ClusterId: "remote"})

			assert.ErrorContains(t, err, "no kubernetes connection spec")
			assert.Zero(t, provider.callCount())
		})
	}
}

func TestRemoteClientFactory_StaleCredentialsAfterRotation_KeepsNewerClient(t *testing.T) {
	rc := newRemoteCluster(t)
	older := secrets.ClientAuth{CertificateAuthorityData: rc.caPEM, ClientTokenData: "token-a"}
	newer := secrets.ClientAuth{CertificateAuthorityData: rc.caPEM, ClientTokenData: "token-b"}

	for name, get := range getters {
		t.Run(name, func(t *testing.T) {
			provider := &fakeSecretProvider{auth: newer}
			factory := newTestFactory(provider)
			cached, err := get(factory, context.Background(), rc.target(t))
			require.NoError(t, err)

			provider.mu.Lock()
			provider.stale = []secrets.ClientAuth{older}
			provider.mu.Unlock()
			got, err := get(factory, context.Background(), rc.target(t))
			require.NoError(t, err)

			assert.Same(t, cached, got)
		})
	}
}

func TestRemoteClientFactory_ConcurrentLookups_ShareOneClient(t *testing.T) {
	rc := newRemoteCluster(t)
	auth := secrets.ClientAuth{CertificateAuthorityData: rc.caPEM, ClientTokenData: "token-a"}

	for name, get := range getters {
		t.Run(name, func(t *testing.T) {
			factory := newTestFactory(&fakeSecretProvider{auth: auth})
			target := rc.target(t)
			results := make([]any, 16)
			var wg sync.WaitGroup
			for i := range results {
				wg.Add(1)
				go func() {
					defer wg.Done()
					c, err := get(factory, context.Background(), target)
					assert.NoError(t, err)
					results[i] = c
				}()
			}
			wg.Wait()

			for _, c := range results[1:] {
				assert.Same(t, results[0], c)
			}
		})
	}
}
