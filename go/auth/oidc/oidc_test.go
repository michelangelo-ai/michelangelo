package oidc

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	jose "github.com/go-jose/go-jose/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apiserver/pkg/authentication/authenticator"
)

const testKeyID = "test-key"

// testIssuer is a minimal OIDC issuer: a discovery document, a JWKS
// endpoint, and an RSA signer for minting ID tokens.
type testIssuer struct {
	server *httptest.Server
	key    *rsa.PrivateKey
	signer jose.Signer
}

func newTestIssuer(t *testing.T) *testIssuer {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":   server.URL,
			"jwks_uri": server.URL + "/keys",
		})
	})
	mux.HandleFunc("/keys", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{
			Keys: []jose.JSONWebKey{{
				Key:       &key.PublicKey,
				KeyID:     testKeyID,
				Algorithm: "RS256",
				Use:       "sig",
			}},
		})
	})

	signer, err := jose.NewSigner(
		jose.SigningKey{
			Algorithm: jose.RS256,
			Key:       jose.JSONWebKey{Key: key, KeyID: testKeyID},
		},
		nil,
	)
	require.NoError(t, err)

	return &testIssuer{server: server, key: key, signer: signer}
}

// newTestIssuerTLS is newTestIssuer's TLS counterpart: a self-signed HTTPS
// issuer, plus the PEM file of its certificate (as a private-CA bundle
// would be), so tests can exercise Config.CABundleFile end to end.
func newTestIssuerTLS(t *testing.T) (issuer *testIssuer, caBundleFile string) {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	mux := http.NewServeMux()
	server := httptest.NewUnstartedServer(mux)
	server.StartTLS()
	t.Cleanup(server.Close)

	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":   server.URL,
			"jwks_uri": server.URL + "/keys",
		})
	})
	mux.HandleFunc("/keys", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{
			Keys: []jose.JSONWebKey{{
				Key:       &key.PublicKey,
				KeyID:     testKeyID,
				Algorithm: "RS256",
				Use:       "sig",
			}},
		})
	})

	signer, err := jose.NewSigner(
		jose.SigningKey{
			Algorithm: jose.RS256,
			Key:       jose.JSONWebKey{Key: key, KeyID: testKeyID},
		},
		nil,
	)
	require.NoError(t, err)

	caFile := filepath.Join(t.TempDir(), "ca.pem")
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	require.NoError(t, os.WriteFile(caFile, certPEM, 0o600))

	return &testIssuer{server: server, key: key, signer: signer}, caFile
}

// baseClaims returns a valid claim set for this issuer; tests override or
// delete entries to produce the failure cases.
func (i *testIssuer) baseClaims() map[string]any {
	return map[string]any{
		"iss":            i.server.URL,
		"aud":            "ma-api",
		"sub":            "alice",
		"email":          "alice@example.com",
		"email_verified": true,
		"groups":         []string{"dev", "ops"},
		"exp":            time.Now().Add(time.Hour).Unix(),
		"iat":            time.Now().Add(-time.Minute).Unix(),
	}
}

func (i *testIssuer) signToken(t *testing.T, claims map[string]any) string {
	t.Helper()
	payload, err := json.Marshal(claims)
	require.NoError(t, err)
	jws, err := i.signer.Sign(payload)
	require.NoError(t, err)
	raw, err := jws.CompactSerialize()
	require.NoError(t, err)
	return raw
}

func (i *testIssuer) newAuthenticator(t *testing.T, config Config) *Authenticator {
	t.Helper()
	if config.IssuerURL == "" {
		config.IssuerURL = i.server.URL
	}
	if len(config.Audiences) == 0 {
		config.Audiences = []string{"ma-api"}
	}
	authnImpl, err := New(context.Background(), config)
	require.NoError(t, err)
	return authnImpl
}

func TestNewRequiresIssuerAndAudience(t *testing.T) {
	_, err := New(context.Background(), Config{Audiences: []string{"ma-api"}})
	assert.ErrorContains(t, err, "issuerUrl is required")

	_, err = New(context.Background(), Config{IssuerURL: "https://issuer.example.com"})
	assert.ErrorContains(t, err, "at least one audience is required")
}

func TestNewFailsFastOnDiscoveryError(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()

	_, err := New(context.Background(), Config{
		IssuerURL: server.URL,
		Audiences: []string{"ma-api"},
	})
	assert.ErrorContains(t, err, "issuer discovery failed")
}

func TestAuthenticateTokenValid(t *testing.T) {
	issuer := newTestIssuer(t)
	authnImpl := issuer.newAuthenticator(t, Config{})

	response, ok, err := authnImpl.AuthenticateToken(context.Background(), issuer.signToken(t, issuer.baseClaims()))
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "alice@example.com", response.User.GetName())
	assert.Equal(t, []string{"dev", "ops"}, response.User.GetGroups())
	assert.Equal(t, "alice", response.User.GetUID(), "UID should come from the sub claim")
	assert.Equal(t, authenticator.Audiences{"ma-api"}, response.Audiences, "Audiences should be the matched accepted audiences")
}

func TestAuthenticateTokenCustomClaims(t *testing.T) {
	issuer := newTestIssuer(t)
	authnImpl := issuer.newAuthenticator(t, Config{
		UsernameClaim: "sub",
		GroupsClaim:   "roles",
	})

	claims := issuer.baseClaims()
	claims["roles"] = "admin" // a single-string claim must normalize to one group
	delete(claims, "groups")

	response, ok, err := authnImpl.AuthenticateToken(context.Background(), issuer.signToken(t, claims))
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "alice", response.User.GetName())
	assert.Equal(t, []string{"admin"}, response.User.GetGroups())
}

func TestAuthenticateTokenMissingGroupsClaimIsAllowed(t *testing.T) {
	issuer := newTestIssuer(t)
	authnImpl := issuer.newAuthenticator(t, Config{})

	claims := issuer.baseClaims()
	delete(claims, "groups")

	response, ok, err := authnImpl.AuthenticateToken(context.Background(), issuer.signToken(t, claims))
	require.NoError(t, err)
	require.True(t, ok)
	assert.Empty(t, response.User.GetGroups())
}

func TestAuthenticateTokenMalformedGroupsClaim(t *testing.T) {
	issuer := newTestIssuer(t)
	authnImpl := issuer.newAuthenticator(t, Config{})

	claims := issuer.baseClaims()
	claims["groups"] = 42

	_, _, err := authnImpl.AuthenticateToken(context.Background(), issuer.signToken(t, claims))
	assert.ErrorContains(t, err, `malformed "groups" claim`)
}

func TestAuthenticateTokenMissingUsernameClaim(t *testing.T) {
	issuer := newTestIssuer(t)
	authnImpl := issuer.newAuthenticator(t, Config{})

	claims := issuer.baseClaims()
	delete(claims, "email")

	_, _, err := authnImpl.AuthenticateToken(context.Background(), issuer.signToken(t, claims))
	assert.ErrorContains(t, err, `no usable "email" claim`)
}

func TestAuthenticateTokenWrongAudience(t *testing.T) {
	issuer := newTestIssuer(t)
	authnImpl := issuer.newAuthenticator(t, Config{})

	claims := issuer.baseClaims()
	claims["aud"] = "someone-else"

	_, _, err := authnImpl.AuthenticateToken(context.Background(), issuer.signToken(t, claims))
	assert.ErrorContains(t, err, "matches none of the accepted audiences")
}

func TestAuthenticateTokenMultiAudienceAnyOf(t *testing.T) {
	issuer := newTestIssuer(t)

	// A token carrying several audiences is accepted when any one of them
	// is configured.
	authnImpl := issuer.newAuthenticator(t, Config{})
	claims := issuer.baseClaims()
	claims["aud"] = []string{"someone-else", "ma-api"}
	_, _, err := authnImpl.AuthenticateToken(context.Background(), issuer.signToken(t, claims))
	assert.NoError(t, err)

	// And a token with a single audience is accepted when the configuration
	// lists several accepted values.
	authnImpl = issuer.newAuthenticator(t, Config{Audiences: []string{"ma-web", "ma-api"}})
	claims = issuer.baseClaims()
	claims["aud"] = "ma-web"
	_, _, err = authnImpl.AuthenticateToken(context.Background(), issuer.signToken(t, claims))
	assert.NoError(t, err)
}

func TestAuthenticateTokenExpired(t *testing.T) {
	issuer := newTestIssuer(t)
	authnImpl := issuer.newAuthenticator(t, Config{})

	claims := issuer.baseClaims()
	claims["exp"] = time.Now().Add(-10 * time.Minute).Unix()

	_, _, err := authnImpl.AuthenticateToken(context.Background(), issuer.signToken(t, claims))
	assert.ErrorContains(t, err, "token is expired")
}

func TestAuthenticateTokenExpiredWithinSkewIsAllowed(t *testing.T) {
	issuer := newTestIssuer(t)
	authnImpl := issuer.newAuthenticator(t, Config{ClockSkewLeeway: 30 * time.Second})

	claims := issuer.baseClaims()
	claims["exp"] = time.Now().Add(-10 * time.Second).Unix()

	_, _, err := authnImpl.AuthenticateToken(context.Background(), issuer.signToken(t, claims))
	assert.NoError(t, err)
}

func TestAuthenticateTokenWithoutExpiry(t *testing.T) {
	issuer := newTestIssuer(t)
	authnImpl := issuer.newAuthenticator(t, Config{})

	claims := issuer.baseClaims()
	delete(claims, "exp")

	_, _, err := authnImpl.AuthenticateToken(context.Background(), issuer.signToken(t, claims))
	assert.ErrorContains(t, err, "no expiry")
}

func TestAuthenticateTokenNotYetValid(t *testing.T) {
	issuer := newTestIssuer(t)
	authnImpl := issuer.newAuthenticator(t, Config{})

	claims := issuer.baseClaims()
	claims["nbf"] = time.Now().Add(10 * time.Minute).Unix()

	_, _, err := authnImpl.AuthenticateToken(context.Background(), issuer.signToken(t, claims))
	assert.ErrorContains(t, err, "not valid yet")
}

func TestAuthenticateTokenNbfWithinSkewIsAllowed(t *testing.T) {
	issuer := newTestIssuer(t)
	authnImpl := issuer.newAuthenticator(t, Config{ClockSkewLeeway: 30 * time.Second})

	claims := issuer.baseClaims()
	claims["nbf"] = time.Now().Add(10 * time.Second).Unix()

	_, _, err := authnImpl.AuthenticateToken(context.Background(), issuer.signToken(t, claims))
	assert.NoError(t, err)
}

func TestAuthenticateTokenWrongIssuer(t *testing.T) {
	issuer := newTestIssuer(t)
	authnImpl := issuer.newAuthenticator(t, Config{})

	claims := issuer.baseClaims()
	claims["iss"] = "https://not-the-issuer.example.com"

	_, _, err := authnImpl.AuthenticateToken(context.Background(), issuer.signToken(t, claims))
	assert.ErrorContains(t, err, "token verification failed")
}

func TestAuthenticateTokenBadSignature(t *testing.T) {
	issuer := newTestIssuer(t)
	authnImpl := issuer.newAuthenticator(t, Config{})

	// Sign with a different key under the same key ID, so only the
	// signature check can catch it.
	rogueKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	rogueSigner, err := jose.NewSigner(
		jose.SigningKey{
			Algorithm: jose.RS256,
			Key:       jose.JSONWebKey{Key: rogueKey, KeyID: testKeyID},
		},
		nil,
	)
	require.NoError(t, err)

	payload, err := json.Marshal(issuer.baseClaims())
	require.NoError(t, err)
	jws, err := rogueSigner.Sign(payload)
	require.NoError(t, err)
	raw, err := jws.CompactSerialize()
	require.NoError(t, err)

	_, _, err = authnImpl.AuthenticateToken(context.Background(), raw)
	assert.ErrorContains(t, err, "token verification failed")
}

func TestAuthenticateTokenGarbage(t *testing.T) {
	issuer := newTestIssuer(t)
	authnImpl := issuer.newAuthenticator(t, Config{})

	_, _, err := authnImpl.AuthenticateToken(context.Background(), "not-a-jwt")
	assert.ErrorContains(t, err, "token verification failed")
}

// --- issuer URL scheme ---

func TestNewRejectsNonLoopbackHTTPIssuer(t *testing.T) {
	_, err := New(context.Background(), Config{
		IssuerURL: "http://issuer.example.com",
		Audiences: []string{"ma-api"},
	})
	assert.ErrorContains(t, err, "must use https")
}

func TestNewAllowsLoopbackHTTPIssuer(t *testing.T) {
	// newTestIssuer's httptest.Server is plain HTTP on a loopback address;
	// every other test in this file depends on that working, but this one
	// asserts it directly against validateIssuerURL's intent.
	issuer := newTestIssuer(t)
	_, err := New(context.Background(), Config{
		IssuerURL: issuer.server.URL,
		Audiences: []string{"ma-api"},
	})
	assert.NoError(t, err)
}

// --- HTTP client: timeout and CA bundle ---

func TestNewHTTPClientHasATimeout(t *testing.T) {
	client, err := newHTTPClient("")
	require.NoError(t, err)
	assert.Equal(t, httpClientTimeout, client.Timeout)
}

func TestNewFailsOnUnreadableCABundleFile(t *testing.T) {
	issuer := newTestIssuer(t)
	_, err := New(context.Background(), Config{
		IssuerURL:    issuer.server.URL,
		Audiences:    []string{"ma-api"},
		CABundleFile: filepath.Join(t.TempDir(), "does-not-exist.pem"),
	})
	assert.ErrorContains(t, err, "cannot build HTTP client")
}

func TestAuthenticateTokenThroughPrivateCA(t *testing.T) {
	issuer, caBundleFile := newTestIssuerTLS(t)
	authnImpl := issuer.newAuthenticator(t, Config{CABundleFile: caBundleFile})

	_, ok, err := authnImpl.AuthenticateToken(context.Background(), issuer.signToken(t, issuer.baseClaims()))
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestNewFailsWithoutTheIssuersPrivateCA(t *testing.T) {
	issuer, _ := newTestIssuerTLS(t)
	// No CABundleFile: the system root pool alone must not trust this
	// self-signed server.
	_, err := New(context.Background(), Config{
		IssuerURL: issuer.server.URL,
		Audiences: []string{"ma-api"},
	})
	assert.ErrorContains(t, err, "issuer discovery failed")
}

// --- signing algorithm allowlist ---

func TestAuthenticateTokenRejectsAnAlgorithmOutsideTheAllowlist(t *testing.T) {
	issuer := newTestIssuer(t)
	// The test issuer only ever signs with RS256; restricting the
	// allowlist to an algorithm it doesn't use proves the allowlist is
	// actually enforced, without needing a second signer.
	authnImpl := issuer.newAuthenticator(t, Config{SupportedSigningAlgs: []string{"ES256"}})

	_, _, err := authnImpl.AuthenticateToken(context.Background(), issuer.signToken(t, issuer.baseClaims()))
	assert.Error(t, err)
}

// --- nbf ---

func TestAuthenticateTokenMalformedNbf(t *testing.T) {
	issuer := newTestIssuer(t)
	authnImpl := issuer.newAuthenticator(t, Config{})

	claims := issuer.baseClaims()
	claims["nbf"] = "not-a-number"

	// go-oidc's own Verify unmarshals the payload into a struct with a
	// numeric nbf field before this package's AuthenticateToken ever sees
	// it, so a non-numeric nbf fails there first with the generic
	// verification error, never reaching numericDate's own rejection. That
	// rejection stays in place as defense-in-depth regardless.
	_, _, err := authnImpl.AuthenticateToken(context.Background(), issuer.signToken(t, claims))
	assert.ErrorContains(t, err, "token verification failed")
}

func TestNumericDateRejectsNonNumeric(t *testing.T) {
	_, ok := numericDate("not-a-number")
	assert.False(t, ok)
}

func TestNumericDateAcceptsAFloat64(t *testing.T) {
	ts, ok := numericDate(float64(1700000000))
	require.True(t, ok)
	assert.Equal(t, int64(1700000000), ts.Unix())
}

// --- email_verified ---

func TestAuthenticateTokenRequiresEmailVerifiedByDefault(t *testing.T) {
	issuer := newTestIssuer(t)
	authnImpl := issuer.newAuthenticator(t, Config{})

	for _, value := range []any{false, nil} {
		claims := issuer.baseClaims()
		if value == nil {
			delete(claims, "email_verified")
		} else {
			claims["email_verified"] = value
		}
		_, _, err := authnImpl.AuthenticateToken(context.Background(), issuer.signToken(t, claims))
		assert.ErrorContains(t, err, "email_verified")
	}
}

func TestAuthenticateTokenAllowUnverifiedEmailOptsOut(t *testing.T) {
	issuer := newTestIssuer(t)
	authnImpl := issuer.newAuthenticator(t, Config{AllowUnverifiedEmail: true})

	claims := issuer.baseClaims()
	delete(claims, "email_verified")

	_, ok, err := authnImpl.AuthenticateToken(context.Background(), issuer.signToken(t, claims))
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestAuthenticateTokenEmailVerifiedIrrelevantForNonEmailUsernameClaim(t *testing.T) {
	issuer := newTestIssuer(t)
	authnImpl := issuer.newAuthenticator(t, Config{UsernameClaim: "sub"})

	claims := issuer.baseClaims()
	delete(claims, "email_verified")

	_, ok, err := authnImpl.AuthenticateToken(context.Background(), issuer.signToken(t, claims))
	require.NoError(t, err)
	assert.True(t, ok)
}

// --- username/groups prefix and the "system:" collision guard ---

func TestAuthenticateTokenRejectsSystemPrefixedUsernameWithoutAPrefixConfigured(t *testing.T) {
	issuer := newTestIssuer(t)
	authnImpl := issuer.newAuthenticator(t, Config{UsernameClaim: "sub"})

	claims := issuer.baseClaims()
	claims["sub"] = "system:anonymous"

	_, _, err := authnImpl.AuthenticateToken(context.Background(), issuer.signToken(t, claims))
	assert.ErrorContains(t, err, "collides with a Kubernetes built-in identity")
}

func TestAuthenticateTokenAppliesUsernamePrefix(t *testing.T) {
	issuer := newTestIssuer(t)
	authnImpl := issuer.newAuthenticator(t, Config{UsernameClaim: "sub", UsernamePrefix: "oidc:"})

	claims := issuer.baseClaims()
	claims["sub"] = "system:anonymous" // would be rejected unprefixed; see above

	response, ok, err := authnImpl.AuthenticateToken(context.Background(), issuer.signToken(t, claims))
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "oidc:system:anonymous", response.User.GetName())
}

func TestAuthenticateTokenRejectsSystemPrefixedGroupWithoutAPrefixConfigured(t *testing.T) {
	issuer := newTestIssuer(t)
	authnImpl := issuer.newAuthenticator(t, Config{})

	claims := issuer.baseClaims()
	claims["groups"] = []string{"dev", "system:masters"}

	_, _, err := authnImpl.AuthenticateToken(context.Background(), issuer.signToken(t, claims))
	assert.ErrorContains(t, err, "collides with a Kubernetes built-in identity")
}

func TestAuthenticateTokenAppliesGroupsPrefix(t *testing.T) {
	issuer := newTestIssuer(t)
	authnImpl := issuer.newAuthenticator(t, Config{GroupsPrefix: "oidc:"})

	response, ok, err := authnImpl.AuthenticateToken(context.Background(), issuer.signToken(t, issuer.baseClaims()))
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, []string{"oidc:dev", "oidc:ops"}, response.User.GetGroups())
}

// --- Config validation ---

func TestNewRejectsEmptyStringAudience(t *testing.T) {
	issuer := newTestIssuer(t)
	_, err := New(context.Background(), Config{
		IssuerURL: issuer.server.URL,
		Audiences: []string{"ma-api", ""},
	})
	assert.ErrorContains(t, err, "must not contain an empty string")
}

func TestNewRejectsNegativeClockSkewLeeway(t *testing.T) {
	issuer := newTestIssuer(t)
	_, err := New(context.Background(), Config{
		IssuerURL:       issuer.server.URL,
		Audiences:       []string{"ma-api"},
		ClockSkewLeeway: -time.Second,
	})
	assert.ErrorContains(t, err, "must not be negative")
}

func TestNewRejectsClockSkewLeewayAboveTheMax(t *testing.T) {
	issuer := newTestIssuer(t)
	_, err := New(context.Background(), Config{
		IssuerURL:       issuer.server.URL,
		Audiences:       []string{"ma-api"},
		ClockSkewLeeway: maxClockSkewLeeway + time.Second,
	})
	assert.ErrorContains(t, err, "must not exceed")
}
