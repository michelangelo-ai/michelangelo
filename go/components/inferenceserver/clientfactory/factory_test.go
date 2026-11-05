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

// auth returns credentials whose CA trusts rc's server, carrying the given token.
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

func getViaHTTPClient(factory ClientFactory, rc *remoteCluster, target *v2pb.ClusterTarget) error {
	httpClient, err := factory.GetHTTPClient(context.Background(), target)
	if err != nil {
		return err
	}
	return getHTTP(httpClient, rc.server.URL)
}

func getConfigMap(factory ClientFactory, _ *remoteCluster, target *v2pb.ClusterTarget) error {
	dynamicClient, err := factory.GetDynamicClient(context.Background(), target)
	if err != nil {
		return err
	}
	configMaps := schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}
	_, err = dynamicClient.Resource(configMaps).Namespace("default").
		Get(context.Background(), "cm", metav1.GetOptions{})
	return err
}

type getter func(ClientFactory, context.Context, *v2pb.ClusterTarget) (any, error)

var getters = map[string]getter{
	"GetClient": func(f ClientFactory, ctx context.Context, c *v2pb.ClusterTarget) (any, error) {
		kubeClient, err := f.GetClient(ctx, c)
		return kubeClient, err
	},
	"GetHTTPClient": func(f ClientFactory, ctx context.Context, c *v2pb.ClusterTarget) (any, error) {
		httpClient, err := f.GetHTTPClient(ctx, c)
		return httpClient, err
	},
	"GetDynamicClient": func(f ClientFactory, ctx context.Context, c *v2pb.ClusterTarget) (any, error) {
		dynamicClient, err := f.GetDynamicClient(ctx, c)
		return dynamicClient, err
	},
}

var clientKinds = map[string]string{
	"GetClient":        "kube",
	"GetHTTPClient":    "HTTP",
	"GetDynamicClient": "dynamic",
}

// rotations returns credentials that differ from rc.auth("token-a") in the token, the CA, or both.
func rotations(t *testing.T, rc *remoteCluster) map[string]struct {
	auth         secrets.ClientAuth
	tokenChanged bool
	caChanged    bool
} {
	otherCA := unrelatedCAPEM(t)
	return map[string]struct {
		auth         secrets.ClientAuth
		tokenChanged bool
		caChanged    bool
	}{
		"token": {auth: rc.auth("token-b"), tokenChanged: true},
		"CA": {
			auth:      secrets.ClientAuth{CertificateAuthorityData: otherCA, ClientTokenData: "token-a"},
			caChanged: true,
		},
		"token and CA": {
			auth:         secrets.ClientAuth{CertificateAuthorityData: otherCA, ClientTokenData: "token-b"},
			tokenChanged: true,
			caChanged:    true,
		},
	}
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

func TestRemoteClientFactory_RotatedCredentials_RebuildsClientOnce(t *testing.T) {
	rc := newRemoteCluster(t)

	for rotated, rotation := range rotations(t, rc) {
		for name, get := range getters {
			t.Run(rotated+"/"+name, func(t *testing.T) {
				provider := &fakeSecretProvider{auth: rc.auth("token-a")}
				factory, logs := newObservedFactory(provider)

				before, err := get(factory, context.Background(), rc.target(t))
				require.NoError(t, err)
				provider.set(rotation.auth, nil)
				after, err := get(factory, context.Background(), rc.target(t))
				require.NoError(t, err)
				again, err := get(factory, context.Background(), rc.target(t))
				require.NoError(t, err)

				assert.NotSame(t, before, after)
				assert.Same(t, after, again)
				assert.Equal(t, 1, logs.FilterMessage(builtMessage).Len())
				rebuilt := logs.FilterMessage(rebuiltMessage).AllUntimed()
				require.Len(t, rebuilt, 1)
				assert.Equal(t, clientKinds[name], rebuilt[0].ContextMap()["client"])
			})
		}
	}
}

func TestRemoteClientFactory_RotatedCredentials_LogsWhatChangedWithoutSecretValues(t *testing.T) {
	rc := newRemoteCluster(t)

	for rotated, rotation := range rotations(t, rc) {
		t.Run(rotated, func(t *testing.T) {
			provider := &fakeSecretProvider{auth: rc.auth("token-a")}
			factory, logs := newObservedFactory(provider)

			_, err := factory.GetHTTPClient(context.Background(), rc.target(t))
			require.NoError(t, err)
			provider.set(rotation.auth, nil)
			_, err = factory.GetHTTPClient(context.Background(), rc.target(t))
			require.NoError(t, err)

			rebuilt := logs.FilterMessage(rebuiltMessage).AllUntimed()
			require.Len(t, rebuilt, 1)
			fields := rebuilt[0].ContextMap()
			assert.Equal(t, rotation.tokenChanged, fields["token_changed"])
			assert.Equal(t, rotation.caChanged, fields["ca_changed"])
			assert.Equal(t, "remote-token", fields["token_secret"])
			assert.Equal(t, "remote-ca", fields["ca_secret"])
			secretValues := []string{
				"token-a", "token-b", rc.caPEM, rotation.auth.CertificateAuthorityData,
			}
			for _, entry := range logs.AllUntimed() {
				logged := entry.Message + fmt.Sprint(entry.ContextMap())
				for _, value := range secretValues {
					assert.NotContains(t, logged, value)
				}
			}
		})
	}
}

func TestRemoteClientFactory_RotatedToken_SendsNewToken(t *testing.T) {
	requests := map[string]func(ClientFactory, *remoteCluster, *v2pb.ClusterTarget) error{
		"GetHTTPClient":    getViaHTTPClient,
		"GetDynamicClient": getConfigMap,
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

	var unknownAuthority x509.UnknownAuthorityError
	require.ErrorAs(t, getViaHTTPClient(factory, rc, rc.target(t)), &unknownAuthority)

	provider.set(rc.auth("token-a"), nil)

	assert.NoError(t, getViaHTTPClient(factory, rc, rc.target(t)))
}

func TestRemoteClientFactory_GetHTTPClient_RotatedCredentials_ClosesReplacedIdleConnections(t *testing.T) {
	for rotated, rotationFor := range map[string]func(*testing.T, *remoteCluster) secrets.ClientAuth{
		"token": func(_ *testing.T, rc *remoteCluster) secrets.ClientAuth { return rc.auth("token-b") },
		"CA": func(t *testing.T, _ *remoteCluster) secrets.ClientAuth {
			return secrets.ClientAuth{CertificateAuthorityData: unrelatedCAPEM(t), ClientTokenData: "token-a"}
		},
	} {
		t.Run(rotated, func(t *testing.T) {
			rc := newRemoteCluster(t)
			provider := &fakeSecretProvider{auth: rc.auth("token-a")}
			factory := newTestFactory(provider)

			require.NoError(t, getViaHTTPClient(factory, rc, rc.target(t)))
			require.Zero(t, rc.closedConnections())

			provider.set(rotationFor(t, rc), nil)
			_, err := factory.GetHTTPClient(context.Background(), rc.target(t))
			require.NoError(t, err)

			assert.Eventually(t, func() bool { return rc.closedConnections() == 1 },
				5*time.Second, 10*time.Millisecond)
		})
	}
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

func TestRemoteClientFactory_UnusableCredentials_ReturnsErrorAndKeepsCache(t *testing.T) {
	rc := newRemoteCluster(t)
	providerErr := errors.New("token secret not found")

	tests := map[string]struct {
		breakCredentials func(*fakeSecretProvider)
		want             string
	}{
		"provider error": {
			breakCredentials: func(p *fakeSecretProvider) { p.set(secrets.ClientAuth{}, providerErr) },
			want:             "token secret not found",
		},
		"empty token": {
			breakCredentials: func(p *fakeSecretProvider) { p.set(rc.auth(""), nil) },
			want:             `token from secret "remote-token" is empty`,
		},
		"empty CA": {
			breakCredentials: func(p *fakeSecretProvider) {
				p.set(secrets.ClientAuth{ClientTokenData: "token-a"}, nil)
			},
			want: `CA certificate from secret "remote-ca" is empty`,
		},
		"invalid CA": {
			breakCredentials: func(p *fakeSecretProvider) {
				p.set(secrets.ClientAuth{CertificateAuthorityData: "not a certificate", ClientTokenData: "token-b"}, nil)
			},
			want: "create ",
		},
		"provider error on the re-read under the lock": {
			breakCredentials: func(p *fakeSecretProvider) {
				p.queue(authResult{auth: rc.auth("token-b")}, authResult{err: providerErr})
			},
			want: "re-read client auth",
		},
		"empty token on the re-read under the lock": {
			breakCredentials: func(p *fakeSecretProvider) {
				p.queue(authResult{auth: rc.auth("token-b")}, authResult{auth: rc.auth("")})
			},
			want: `re-read client auth for cluster "remote": token from secret "remote-token" is empty`,
		},
		"empty CA on the re-read under the lock": {
			breakCredentials: func(p *fakeSecretProvider) {
				p.queue(
					authResult{auth: rc.auth("token-b")},
					authResult{auth: secrets.ClientAuth{ClientTokenData: "token-b"}},
				)
			},
			want: `re-read client auth for cluster "remote": CA certificate from secret "remote-ca" is empty`,
		},
	}
	for caseName, tt := range tests {
		for name, get := range getters {
			t.Run(caseName+"/"+name, func(t *testing.T) {
				provider := &fakeSecretProvider{auth: rc.auth("token-a")}
				factory := newTestFactory(provider)
				cached, err := get(factory, context.Background(), rc.target(t))
				require.NoError(t, err)

				tt.breakCredentials(provider)
				failed, err := get(factory, context.Background(), rc.target(t))
				require.ErrorContains(t, err, tt.want)
				for _, secret := range []string{"token-a", "token-b", rc.caPEM, "not a certificate"} {
					assert.NotContains(t, err.Error(), secret)
				}
				assert.Nil(t, failed)

				provider.set(rc.auth("token-a"), nil)
				recovered, err := get(factory, context.Background(), rc.target(t))
				require.NoError(t, err)
				assert.Same(t, cached, recovered)
			})
		}
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

func TestRemoteClientFactory_SameClusterIDOnDifferentEndpoints_UsesSeparateClients(t *testing.T) {
	first := newRemoteCluster(t)
	second := newRemoteCluster(t)
	require.Equal(t, first.caPEM, second.caPEM)
	factory := newTestFactory(&fakeSecretProvider{auth: first.auth("token-a")})

	require.NoError(t, getConfigMap(factory, first, first.target(t)))
	require.NoError(t, getConfigMap(factory, second, second.target(t)))

	assert.Equal(t, []string{"token-a"}, first.receivedTokens())
	assert.Equal(t, []string{"token-a"}, second.receivedTokens())
}

// tokenPerSecretProvider returns, as each target's token, the name of its token Secret.
type tokenPerSecretProvider struct {
	caPEM string
}

func (p tokenPerSecretProvider) GetClientAuth(
	_ context.Context, cluster *v2pb.ClusterTarget,
) (secrets.ClientAuth, error) {
	return secrets.ClientAuth{
		CertificateAuthorityData: p.caPEM,
		ClientTokenData:          cluster.GetKubernetes().GetTokenTag(),
	}, nil
}

func TestRemoteClientFactory_SameEndpointWithDifferentSecrets_KeepsBothClients(t *testing.T) {
	rc := newRemoteCluster(t)
	current := rc.target(t)
	renamed := rc.target(t)
	renamed.GetKubernetes().TokenTag = "remote-token-v2"

	for name, get := range getters {
		t.Run(name, func(t *testing.T) {
			factory, logs := newObservedFactory(tokenPerSecretProvider{caPEM: rc.caPEM})

			first, err := get(factory, context.Background(), current)
			require.NoError(t, err)
			second, err := get(factory, context.Background(), renamed)
			require.NoError(t, err)
			again, err := get(factory, context.Background(), current)
			require.NoError(t, err)

			assert.NotSame(t, first, second)
			assert.Same(t, first, again)
			assert.Equal(t, 2, logs.FilterMessage(builtMessage).Len())
			assert.Zero(t, logs.FilterMessage(rebuiltMessage).Len())
		})
	}
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
