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
	"errors"
	"time"

	"k8s.io/apiserver/pkg/authentication/authenticator"
	tokencache "k8s.io/apiserver/pkg/authentication/token/cache"
	tokenunion "k8s.io/apiserver/pkg/authentication/token/union"
	"k8s.io/client-go/kubernetes"
)

// TokenAuthenticatorOptions selects which token authenticators to compose.
type TokenAuthenticatorOptions struct {
	// OIDC enables ID-token verification when non-nil. Discovery runs at
	// construction, so a misconfigured or unreachable issuer fails startup
	// instead of denying every request at runtime.
	OIDC *Config
	// TokenReviewClient enables Kubernetes ServiceAccount token
	// authentication via TokenReview when non-nil.
	TokenReviewClient kubernetes.Interface
	// TokenReviewAudiences, when non-empty, restricts which ServiceAccount
	// tokens the TokenReview path accepts.
	TokenReviewAudiences []string
	// CacheTTL bounds how long an authenticated token's identity is
	// cached, sparing repeated verification (and TokenReview round trips).
	// Zero or negative disables the cache.
	CacheTTL time.Duration
}

// NewTokenAuthenticator composes the configured token authenticators --
// OIDC and/or ServiceAccount TokenReview, each tried in turn -- and wraps
// the union in the upstream token cache.
func NewTokenAuthenticator(ctx context.Context, options TokenAuthenticatorOptions) (authenticator.Token, error) {
	var authenticators []authenticator.Token
	if options.OIDC != nil {
		oidcAuthenticator, err := New(ctx, *options.OIDC)
		if err != nil {
			return nil, err
		}
		authenticators = append(authenticators, oidcAuthenticator)
	}
	if options.TokenReviewClient != nil {
		authenticators = append(authenticators, NewServiceAccountAuthenticator(options.TokenReviewClient, options.TokenReviewAudiences))
	}
	if len(authenticators) == 0 {
		return nil, errors.New("oidc: no token authenticator configured")
	}

	combined := tokenunion.New(authenticators...)
	if options.CacheTTL > 0 {
		combined = tokencache.New(combined, false, options.CacheTTL, options.CacheTTL)
	}
	return combined, nil
}
