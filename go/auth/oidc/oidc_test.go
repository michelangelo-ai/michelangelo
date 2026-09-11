package oidc

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	jose "github.com/go-jose/go-jose/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// baseClaims returns a valid claim set for this issuer; tests override or
// delete entries to produce the failure cases.
func (i *testIssuer) baseClaims() map[string]any {
	return map[string]any{
		"iss":    i.server.URL,
		"aud":    "ma-api",
		"sub":    "alice",
		"email":  "alice@example.com",
		"groups": []string{"dev", "ops"},
		"exp":    time.Now().Add(time.Hour).Unix(),
		"iat":    time.Now().Add(-time.Minute).Unix(),
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
