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
	"fmt"
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
	"go.uber.org/zap/zaptest/observer"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/clientfactory/secrets"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

const (
	builtMessage   = "Built client for cluster"
	rebuiltMessage = "Rebuilt client for cluster after its credentials changed"

	configMapJSON = `{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"cm","namespace":"default"}}`
)

type authResult struct {
	auth secrets.ClientAuth
	err  error
}

type fakeSecretProvider struct {
	mu     sync.Mutex
	auth   secrets.ClientAuth
	err    error
	queued []authResult // returned, in order, before auth and err
	calls  int
}

func (p *fakeSecretProvider) GetClientAuth(context.Context, *v2pb.ClusterTarget) (secrets.ClientAuth, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	if len(p.queued) > 0 {
		next := p.queued[0]
		p.queued = p.queued[1:]
		return next.auth, next.err
	}
	return p.auth, p.err
}

func (p *fakeSecretProvider) set(auth secrets.ClientAuth, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.auth = auth
	p.err = err
}

func (p *fakeSecretProvider) queue(results ...authResult) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.queued = append(p.queued, results...)
}

func (p *fakeSecretProvider) callCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
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
	_, _ = io.WriteString(w, configMapJSON)
}

func (rc *remoteCluster) target(t *testing.T) *v2pb.ClusterTarget {
	t.Helper()
	u, err := url.Parse(rc.server.URL)
	require.NoError(t, err)
	return &v2pb.ClusterTarget{
		ClusterId: "remote",
		Connection: &v2pb.ClusterTarget_Kubernetes{Kubernetes: &v2pb.ConnectionSpec{
			Host:      "https://" + u.Hostname(),
			Port:      u.Port(),
			TokenTag:  "remote-token",
			CaDataTag: "remote-ca",
		}},
	}
}

// auth returns credentials that the server trusts, with the given token.
func (rc *remoteCluster) auth(token string) secrets.ClientAuth {
	return secrets.ClientAuth{CertificateAuthorityData: rc.caPEM, ClientTokenData: token}
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

func getHTTP(httpClient *http.Client, rawURL string) error {
	resp, err := httpClient.Get(rawURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, err = io.Copy(io.Discard, resp.Body)
	return err
}

func getConfigMap(ctx context.Context, factory ClientFactory, target *v2pb.ClusterTarget) error {
	dynamicClient, err := factory.GetDynamicClient(ctx, target)
	if err != nil {
		return err
	}
	configMaps := schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}
	_, err = dynamicClient.Resource(configMaps).Namespace("default").Get(ctx, "cm", metav1.GetOptions{})
	return err
}

type getter func(ClientFactory, context.Context, *v2pb.ClusterTarget) (any, error)

// getters returns the client as `any` holding nil, rather than a nil `any`, on error, so
// assertions check what callers actually receive.
var getters = map[string]getter{
	"GetClient": func(f ClientFactory, ctx context.Context, c *v2pb.ClusterTarget) (any, error) {
		kubeClient, err := f.GetClient(ctx, c)
		if kubeClient == nil {
			return nil, err
		}
		return kubeClient, err
	},
	"GetHTTPClient": func(f ClientFactory, ctx context.Context, c *v2pb.ClusterTarget) (any, error) {
		httpClient, err := f.GetHTTPClient(ctx, c)
		if httpClient == nil {
			return nil, err
		}
		return httpClient, err
	},
	"GetDynamicClient": func(f ClientFactory, ctx context.Context, c *v2pb.ClusterTarget) (any, error) {
		dynamicClient, err := f.GetDynamicClient(ctx, c)
		if dynamicClient == nil {
			return nil, err
		}
		return dynamicClient, err
	},
}

func newTestFactory(provider secrets.SecretProvider) ClientFactory {
	return NewRemoteClientFactory(provider, runtime.NewScheme(), zap.NewNop())
}

func newObservedFactory(provider secrets.SecretProvider) (ClientFactory, *observer.ObservedLogs) {
	core, logs := observer.New(zap.InfoLevel)
	return NewRemoteClientFactory(provider, runtime.NewScheme(), zap.New(core)), logs
}

func TestRemoteClientFactory_UnchangedCredentials_ReusesClient(t *testing.T) {
	rc := newRemoteCluster(t)

	for name, get := range getters {
		t.Run(name, func(t *testing.T) {
			factory := newTestFactory(&fakeSecretProvider{auth: rc.auth("token-a")})

			first, err := get(factory, context.Background(), rc.target(t))
			require.NoError(t, err)
			second, err := get(factory, context.Background(), rc.target(t))
			require.NoError(t, err)

			assert.Same(t, first, second)
		})
	}
}

func TestRemoteClientFactory_RotatedCredentials_RebuildsClientAndLogsWhatChanged(t *testing.T) {
	rc := newRemoteCluster(t)
	otherCA := unrelatedCAPEM(t)

	rotations := map[string]struct {
		auth         secrets.ClientAuth
		tokenChanged bool
		caChanged    bool
	}{
		"token": {auth: rc.auth("token-b"), tokenChanged: true},
		"CA": {
			auth:      secrets.ClientAuth{CertificateAuthorityData: otherCA, ClientTokenData: "token-a"},
			caChanged: true,
		},
	}
	for rotated, rotation := range rotations {
		for name, get := range getters {
			t.Run(rotated+"/"+name, func(t *testing.T) {
				provider := &fakeSecretProvider{auth: rc.auth("token-a")}
				factory, logs := newObservedFactory(provider)

				before, err := get(factory, context.Background(), rc.target(t))
				require.NoError(t, err)
				provider.set(rotation.auth, nil)
				after, err := get(factory, context.Background(), rc.target(t))
				require.NoError(t, err)

				assert.NotSame(t, before, after)
				assert.Equal(t, 1, logs.FilterMessage(builtMessage).Len())
				rebuilt := logs.FilterMessage(rebuiltMessage).AllUntimed()
				require.Len(t, rebuilt, 1)
				fields := rebuilt[0].ContextMap()
				assert.Equal(t, rotation.tokenChanged, fields["token_changed"])
				assert.Equal(t, rotation.caChanged, fields["ca_changed"])
				assert.Equal(t, "remote-token", fields["token_secret"])
				assert.Equal(t, "remote-ca", fields["ca_secret"])
				for _, entry := range logs.AllUntimed() {
					logged := entry.Message + fmt.Sprint(entry.ContextMap())
					for _, secret := range []string{"token-a", "token-b", rc.caPEM, otherCA} {
						assert.NotContains(t, logged, secret)
					}
				}
			})
		}
	}
}

func TestRemoteClientFactory_RotatedToken_SendsNewToken(t *testing.T) {
	requests := map[string]func(ClientFactory, *remoteCluster, *v2pb.ClusterTarget) error{
		"GetHTTPClient": func(factory ClientFactory, rc *remoteCluster, target *v2pb.ClusterTarget) error {
			httpClient, err := factory.GetHTTPClient(context.Background(), target)
			if err != nil {
				return err
			}
			return getHTTP(httpClient, rc.server.URL)
		},
		"GetDynamicClient": func(factory ClientFactory, _ *remoteCluster, target *v2pb.ClusterTarget) error {
			return getConfigMap(context.Background(), factory, target)
		},
	}

	for name, request := range requests {
		t.Run(name, func(t *testing.T) {
			rc := newRemoteCluster(t)
			provider := &fakeSecretProvider{auth: rc.auth("token-a")}
			factory := newTestFactory(provider)

			require.NoError(t, request(factory, rc, rc.target(t)))
			provider.set(rc.auth("token-b"), nil)
			require.NoError(t, request(factory, rc, rc.target(t)))

			assert.Equal(t, []string{"token-a", "token-b"}, rc.receivedTokens())
		})
	}
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
	require.ErrorAs(t, getHTTP(httpClient, rc.server.URL), &unknownAuthority)

	provider.set(rc.auth("token-a"), nil)
	httpClient, err = factory.GetHTTPClient(context.Background(), rc.target(t))
	require.NoError(t, err)

	assert.NoError(t, getHTTP(httpClient, rc.server.URL))
}

func TestRemoteClientFactory_GetHTTPClient_RotatedToken_ClosesReplacedIdleConnections(t *testing.T) {
	rc := newRemoteCluster(t)
	provider := &fakeSecretProvider{auth: rc.auth("token-a")}
	factory := newTestFactory(provider)

	httpClient, err := factory.GetHTTPClient(context.Background(), rc.target(t))
	require.NoError(t, err)
	require.NoError(t, getHTTP(httpClient, rc.server.URL))
	require.Zero(t, rc.closedConnections())

	provider.set(rc.auth("token-b"), nil)
	_, err = factory.GetHTTPClient(context.Background(), rc.target(t))
	require.NoError(t, err)

	assert.Eventually(t, func() bool { return rc.closedConnections() == 1 }, 5*time.Second, 10*time.Millisecond)
}

func TestRemoteClientFactory_GetHTTPClient_SetsIdleConnTimeout(t *testing.T) {
	rc := newRemoteCluster(t)
	factory := newTestFactory(&fakeSecretProvider{auth: rc.auth("token-a")})

	httpClient, err := factory.GetHTTPClient(context.Background(), rc.target(t))
	require.NoError(t, err)

	roundTripper, ok := httpClient.Transport.(*bearerTokenRoundTripper)
	require.True(t, ok)
	transport, ok := roundTripper.rt.(*http.Transport)
	require.True(t, ok)
	assert.Equal(t, httpIdleConnTimeout, transport.IdleConnTimeout)
}

func TestRemoteClientFactory_SecretProviderError_ReturnsErrorAndKeepsCache(t *testing.T) {
	rc := newRemoteCluster(t)

	for name, get := range getters {
		t.Run(name, func(t *testing.T) {
			provider := &fakeSecretProvider{auth: rc.auth("token-a")}
			factory := newTestFactory(provider)
			cached, err := get(factory, context.Background(), rc.target(t))
			require.NoError(t, err)

			provider.set(secrets.ClientAuth{}, errors.New("token secret not found"))
			failed, err := get(factory, context.Background(), rc.target(t))
			assert.ErrorContains(t, err, "token secret not found")
			assert.Nil(t, failed)

			provider.set(rc.auth("token-a"), nil)
			recovered, err := get(factory, context.Background(), rc.target(t))
			require.NoError(t, err)
			assert.Same(t, cached, recovered)
		})
	}
}

func TestRemoteClientFactory_EmptyCredentials_ReturnsErrorAndKeepsCache(t *testing.T) {
	rc := newRemoteCluster(t)

	empties := map[string]struct {
		auth secrets.ClientAuth
		want string
	}{
		"token": {auth: rc.auth(""), want: `token from secret "remote-token" is empty`},
		"CA": {
			auth: secrets.ClientAuth{ClientTokenData: "token-a"},
			want: `CA certificate from secret "remote-ca" is empty`,
		},
	}
	for emptied, empty := range empties {
		for name, get := range getters {
			t.Run(emptied+"/"+name, func(t *testing.T) {
				provider := &fakeSecretProvider{auth: rc.auth("token-a")}
				factory := newTestFactory(provider)
				cached, err := get(factory, context.Background(), rc.target(t))
				require.NoError(t, err)

				provider.set(empty.auth, nil)
				failed, err := get(factory, context.Background(), rc.target(t))
				assert.ErrorContains(t, err, empty.want)
				assert.Nil(t, failed)

				provider.set(rc.auth("token-a"), nil)
				recovered, err := get(factory, context.Background(), rc.target(t))
				require.NoError(t, err)
				assert.Same(t, cached, recovered)
			})
		}
	}
}

func TestRemoteClientFactory_FailedRebuild_ReturnsErrorWithoutCachingIt(t *testing.T) {
	rc := newRemoteCluster(t)
	invalidCA := secrets.ClientAuth{CertificateAuthorityData: "not a certificate", ClientTokenData: "token-b"}

	for name, get := range getters {
		t.Run(name, func(t *testing.T) {
			provider := &fakeSecretProvider{auth: rc.auth("token-a")}
			factory := newTestFactory(provider)
			_, err := get(factory, context.Background(), rc.target(t))
			require.NoError(t, err)

			provider.set(invalidCA, nil)
			for range 2 {
				failed, err := get(factory, context.Background(), rc.target(t))
				assert.Error(t, err)
				assert.Nil(t, failed)
			}
		})
	}
}

func TestRemoteClientFactory_GetHTTPClient_InvalidCAPEM_NamesSecret(t *testing.T) {
	rc := newRemoteCluster(t)
	factory := newTestFactory(&fakeSecretProvider{
		auth: secrets.ClientAuth{CertificateAuthorityData: "not a certificate", ClientTokenData: "token-a"},
	})

	httpClient, err := factory.GetHTTPClient(context.Background(), rc.target(t))

	assert.ErrorContains(t, err, `parse CA certificate from secret "remote-ca": invalid PEM`)
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

	for name, get := range getters {
		t.Run(name, func(t *testing.T) {
			provider := &fakeSecretProvider{auth: rc.auth("token-b")}
			factory := newTestFactory(provider)
			cached, err := get(factory, context.Background(), rc.target(t))
			require.NoError(t, err)

			provider.queue(authResult{auth: rc.auth("token-a")})
			got, err := get(factory, context.Background(), rc.target(t))
			require.NoError(t, err)

			assert.Same(t, cached, got)
		})
	}
}

func TestRemoteClientFactory_ReReadUnderLockFails_ReturnsError(t *testing.T) {
	rc := newRemoteCluster(t)

	for name, get := range getters {
		t.Run(name, func(t *testing.T) {
			provider := &fakeSecretProvider{auth: rc.auth("token-a")}
			factory := newTestFactory(provider)
			_, err := get(factory, context.Background(), rc.target(t))
			require.NoError(t, err)

			provider.queue(
				authResult{auth: rc.auth("token-b")},
				authResult{err: errors.New("token secret not found")},
			)
			failed, err := get(factory, context.Background(), rc.target(t))

			assert.ErrorContains(t, err, "re-read client auth")
			assert.ErrorContains(t, err, "token secret not found")
			assert.Nil(t, failed)
		})
	}
}

func TestRemoteClientFactory_SameClusterIDOnDifferentEndpoints_UsesSeparateClients(t *testing.T) {
	first := newRemoteCluster(t)
	second := newRemoteCluster(t)
	require.Equal(t, first.caPEM, second.caPEM)
	factory := newTestFactory(&fakeSecretProvider{auth: first.auth("token-a")})

	require.NoError(t, getConfigMap(context.Background(), factory, first.target(t)))
	require.NoError(t, getConfigMap(context.Background(), factory, second.target(t)))

	assert.Equal(t, []string{"token-a"}, first.receivedTokens())
	assert.Equal(t, []string{"token-a"}, second.receivedTokens())
}

func TestRemoteClientFactory_ConcurrentLookups_BuildOneClient(t *testing.T) {
	rc := newRemoteCluster(t)

	for name, get := range getters {
		t.Run(name, func(t *testing.T) {
			factory, logs := newObservedFactory(&fakeSecretProvider{auth: rc.auth("token-a")})
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

			assert.Equal(t, 1, logs.FilterMessage(builtMessage).Len())
			assert.Zero(t, logs.FilterMessage(rebuiltMessage).Len())
			for _, c := range results[1:] {
				assert.Same(t, results[0], c)
			}
		})
	}
}
