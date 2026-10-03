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

package auth

import (
	"context"
	"errors"
	"fmt"

	"go.uber.org/config"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/michelangelo-ai/michelangelo/go/auth/k8srbac"
	"github.com/michelangelo-ai/michelangelo/go/auth/oidc"
)

// AuthModule provides the authenticator/authorizer pair selected by the
// apiserver.auth configuration: the allow-all bundle when the mode is
// "dummy" or unset (the backward-compatible default), or the k8s-rbac
// assembly, which authenticates bearer tokens and authorizes through
// Kubernetes RBAC via SubjectAccessReview.
var AuthModule = fx.Options(
	fx.Provide(NewAuthBundle),
)

// BundleParams collects NewAuthBundle's dependencies.
type BundleParams struct {
	fx.In

	Provider  config.Provider
	K8sConfig *rest.Config
	Logger    *zap.Logger
}

// NewAuthBundle builds the bundle the configuration asks for. An unknown
// mode is a startup error, never a silent fallback to allow-all.
func NewAuthBundle(params BundleParams) (AuthBundle, error) {
	authConfig, err := GetConfig(params.Provider)
	if err != nil {
		return AuthBundle{}, fmt.Errorf("auth: cannot parse %s configuration: %w", configKey, err)
	}
	switch authConfig.Mode {
	case "", ModeDummy:
		return NewAllowAllBundle(), nil
	case ModeK8sRBAC:
		return newK8sRBACBundle(params, authConfig)
	default:
		return AuthBundle{}, fmt.Errorf("auth: unknown auth mode %q (want %q or %q)", authConfig.Mode, ModeDummy, ModeK8sRBAC)
	}
}

func newK8sRBACBundle(params BundleParams, authConfig Config) (AuthBundle, error) {
	if authConfig.OIDC.IssuerURL == "" && !authConfig.ServiceAccounts.Enabled {
		return AuthBundle{}, errors.New("auth: mode k8s-rbac requires oidc.issuerUrl and/or serviceAccounts.enabled")
	}
	client, err := kubernetes.NewForConfig(params.K8sConfig)
	if err != nil {
		return AuthBundle{}, fmt.Errorf("auth: cannot build kubernetes client: %w", err)
	}

	options := oidc.TokenAuthenticatorOptions{CacheTTL: tokenCacheTTL}
	if authConfig.OIDC.IssuerURL != "" {
		options.OIDC = &authConfig.OIDC
		params.Logger.Info("auth: OIDC token authentication enabled",
			zap.String("issuer", authConfig.OIDC.IssuerURL))
	}
	if authConfig.ServiceAccounts.Enabled {
		options.TokenReviewClient = client
		options.TokenReviewAudiences = authConfig.ServiceAccounts.Audiences
		params.Logger.Info("auth: ServiceAccount TokenReview authentication enabled")
	}
	// Discovery runs at construction so a misconfigured or unreachable
	// issuer fails startup instead of denying every request at runtime.
	ctx, cancel := context.WithTimeout(context.Background(), discoveryTimeout)
	defer cancel()
	tokenAuthenticator, err := oidc.NewTokenAuthenticator(ctx, options)
	if err != nil {
		return AuthBundle{}, err
	}

	sarAuthorizer, err := k8srbac.NewSARAuthorizer(client.AuthorizationV1(),
		authConfig.SARCache.allowTTL(), authConfig.SARCache.denyTTL())
	if err != nil {
		return AuthBundle{}, err
	}
	params.Logger.Info("auth: Kubernetes RBAC authorization enabled",
		zap.String("group", APIGroup))

	return AuthBundle{Authenticator: tokenAuthenticator, Authorizer: sarAuthorizer}, nil
}
