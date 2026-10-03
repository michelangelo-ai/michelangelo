// Copyright (c) 2023 Uber Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package oidc

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	authenticationv1 "k8s.io/api/authentication/v1"
)

func TestNewTokenAuthenticatorRequiresAtLeastOnePath(t *testing.T) {
	_, err := NewTokenAuthenticator(context.Background(), TokenAuthenticatorOptions{})
	assert.ErrorContains(t, err, "no token authenticator configured")
}

func TestNewTokenAuthenticatorOIDCOnly(t *testing.T) {
	issuer := newTestIssuer(t)
	tokenAuthenticator, err := NewTokenAuthenticator(context.Background(), TokenAuthenticatorOptions{
		OIDC: &Config{IssuerURL: issuer.server.URL, Audiences: []string{"ma-api"}},
	})
	require.NoError(t, err)

	response, ok, err := tokenAuthenticator.AuthenticateToken(context.Background(), issuer.signToken(t, issuer.baseClaims()))
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "alice@example.com", response.User.GetName())

	_, ok, _ = tokenAuthenticator.AuthenticateToken(context.Background(), "not-a-jwt")
	assert.False(t, ok)
}

// TestNewTokenAuthenticatorUnionFallsThrough exercises the union: a token
// the OIDC verifier rejects is still accepted when the TokenReview path
// authenticates it.
func TestNewTokenAuthenticatorUnionFallsThrough(t *testing.T) {
	issuer := newTestIssuer(t)
	client, reviews := newTokenReviewClient(authenticationv1.TokenReviewStatus{
		Authenticated: true,
		User:          authenticationv1.UserInfo{Username: "system:serviceaccount:team-a:michelangelo-runner"},
	}, nil)
	tokenAuthenticator, err := NewTokenAuthenticator(context.Background(), TokenAuthenticatorOptions{
		OIDC:              &Config{IssuerURL: issuer.server.URL, Audiences: []string{"ma-api"}},
		TokenReviewClient: client,
	})
	require.NoError(t, err)

	response, ok, err := tokenAuthenticator.AuthenticateToken(context.Background(), "projected-sa-token")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "system:serviceaccount:team-a:michelangelo-runner", response.User.GetName())
	assert.Len(t, *reviews, 1)

	// An OIDC token never reaches TokenReview: the OIDC path wins first.
	response, ok, err = tokenAuthenticator.AuthenticateToken(context.Background(), issuer.signToken(t, issuer.baseClaims()))
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "alice@example.com", response.User.GetName())
	assert.Len(t, *reviews, 1)
}

func TestNewTokenAuthenticatorCachesAuthenticatedTokens(t *testing.T) {
	client, reviews := newTokenReviewClient(authenticationv1.TokenReviewStatus{
		Authenticated: true,
		User:          authenticationv1.UserInfo{Username: "system:serviceaccount:team-a:michelangelo-runner"},
	}, nil)
	tokenAuthenticator, err := NewTokenAuthenticator(context.Background(), TokenAuthenticatorOptions{
		TokenReviewClient: client,
		CacheTTL:          time.Hour,
	})
	require.NoError(t, err)

	for i := 0; i < 3; i++ {
		_, ok, err := tokenAuthenticator.AuthenticateToken(context.Background(), "projected-sa-token")
		require.NoError(t, err)
		require.True(t, ok)
	}
	assert.Len(t, *reviews, 1)

	// A different token is a different cache key.
	_, ok, err := tokenAuthenticator.AuthenticateToken(context.Background(), "another-token")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Len(t, *reviews, 2)
}

func TestNewTokenAuthenticatorZeroTTLDisablesCache(t *testing.T) {
	client, reviews := newTokenReviewClient(authenticationv1.TokenReviewStatus{
		Authenticated: true,
		User:          authenticationv1.UserInfo{Username: "system:serviceaccount:team-a:michelangelo-runner"},
	}, nil)
	tokenAuthenticator, err := NewTokenAuthenticator(context.Background(), TokenAuthenticatorOptions{TokenReviewClient: client})
	require.NoError(t, err)

	for i := 0; i < 2; i++ {
		_, ok, err := tokenAuthenticator.AuthenticateToken(context.Background(), "projected-sa-token")
		require.NoError(t, err)
		require.True(t, ok)
	}
	assert.Len(t, *reviews, 2)
}
