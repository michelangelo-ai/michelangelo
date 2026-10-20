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
	"go.uber.org/yarpc/api/middleware"
	"go.uber.org/zap"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/michelangelo-ai/michelangelo/go/auth/k8srbac"
	"github.com/michelangelo-ai/michelangelo/go/auth/oidc"
)

// AuthModule provides the Auth implementation the generated handlers
// consume and, in modes that authenticate bearer tokens, the inbound
// middleware that verifies them and populates the request context with
// the caller's Principal (see NewInboundMiddleware) -- picked by the
// apiserver.auth configuration: DummyAuth with no middleware when the mode
// is "dummy" or unset (the backward-compatible default, replacing
// DummyAuthModule), or the OIDC/ServiceAccount authentication and
// Kubernetes RBAC authorization assembly, adapted onto this package's
// contract via FromTokenAuthenticator and NewK8sRBACAuth.
var AuthModule = fx.Options(
	fx.Provide(NewAuthResult),
)

// AuthResultParams collects NewAuthResult's dependencies.
type AuthResultParams struct {
	fx.In

	Provider  config.Provider
	K8sConfig *rest.Config
	Logger    *zap.Logger
}

// AuthResult carries the Auth implementation and, in modes that
// authenticate bearer tokens, the inbound middleware the apiserver's
// dispatcher applies to every call (see cmd/apiserver/yarpc.go's
// DispatcherParams.InboundMiddleware). InboundMiddleware is left nil in
// dummy mode: no token is required, exactly matching the old
// DummyAuthModule's behavior.
type AuthResult struct {
	fx.Out

	Auth              Auth
	InboundMiddleware middleware.UnaryInbound `optional:"true"`
}

// NewAuthResult builds the result the configuration asks for. An unknown
// mode is a startup error, never a silent fallback to allow-all.
func NewAuthResult(params AuthResultParams) (AuthResult, error) {
	authConfig, err := GetConfig(params.Provider)
	if err != nil {
		return AuthResult{}, fmt.Errorf("auth: cannot parse %s configuration: %w", configKey, err)
	}
	switch authConfig.Mode {
	case "", ModeDummy:
		return AuthResult{Auth: DummyAuth{}}, nil
	case ModeK8sRBAC:
		return newK8sRBACResult(params, authConfig)
	default:
		return AuthResult{}, fmt.Errorf("auth: unknown auth mode %q (want %q or %q)", authConfig.Mode, ModeDummy, ModeK8sRBAC)
	}
}

func newK8sRBACResult(params AuthResultParams, authConfig Config) (AuthResult, error) {
	if authConfig.OIDC.IssuerURL == "" && !authConfig.ServiceAccounts.Enabled {
		return AuthResult{}, errors.New("auth: mode k8s-rbac requires oidc.issuerUrl and/or serviceAccounts.enabled")
	}
	client, err := kubernetes.NewForConfig(params.K8sConfig)
	if err != nil {
		return AuthResult{}, fmt.Errorf("auth: cannot build kubernetes client: %w", err)
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
		return AuthResult{}, err
	}

	sarAuthorizer, err := k8srbac.NewSARAuthorizer(client.AuthorizationV1(),
		authConfig.SARCache.allowTTL(), authConfig.SARCache.denyTTL())
	if err != nil {
		return AuthResult{}, err
	}
	params.Logger.Info("auth: Kubernetes RBAC authorization enabled",
		zap.String("group", APIGroup))

	// InboundConfig{} accepts any authenticated principal with no service
	// allowlist; restricting callers by service name is a future config
	// knob, not something this mode change needs to decide today.
	return AuthResult{
		Auth:              NewK8sRBACAuth(sarAuthorizer, params.Logger),
		InboundMiddleware: NewInboundMiddleware(FromTokenAuthenticator(tokenAuthenticator), InboundConfig{}, params.Logger),
	}, nil
}
