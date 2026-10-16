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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/config"
	"go.uber.org/fx"
	"go.uber.org/yarpc/api/middleware"
	"go.uber.org/zap"
	"k8s.io/client-go/rest"
)

// defaultModuleDeps is what a consumer of AuthModule asks the graph for: the
// Auth implementation and, optionally, the inbound middleware.
type defaultModuleDeps struct {
	fx.In

	Auth    Auth
	Inbound middleware.UnaryInbound `optional:"true"`
}

// TestAuthModuleDefaultsToDummyWithoutAKubernetesConfig composes AuthModule
// in a real fx graph the way a deployment would, with no apiserver.auth key
// and a nil *rest.Config, and proves the "unchanged unless enabled"
// guarantee: allow-all Auth, no inbound middleware (no token required), and
// no Kubernetes client built.
func TestAuthModuleDefaultsToDummyWithoutAKubernetesConfig(t *testing.T) {
	provider := newTestProvider(t, "unrelated:\n  key: value\n")
	var deps defaultModuleDeps
	app := fx.New(
		fx.NopLogger,
		AuthModule,
		fx.Provide(
			func() config.Provider { return provider },
			func() *rest.Config { return nil },
			zap.NewNop,
		),
		fx.Invoke(func(d defaultModuleDeps) { deps = d }),
	)
	require.NoError(t, app.Err())
	requireAllowAll(t, AuthResult{Auth: deps.Auth, InboundMiddleware: deps.Inbound})
}

// TestNewAuthResultRejectsUnknownKeysUnderAuth pins the strict parse of
// apiserver.auth: a mistyped key is a startup error, never a silent fall
// back to dummy, so a typo in "mode" cannot read as allow-all. An unknown
// key next to an explicit dummy mode is rejected too; relaxing that is a
// deliberate decision, not something to change by accident.
func TestNewAuthResultRejectsUnknownKeysUnderAuth(t *testing.T) {
	tests := map[string]string{
		"mistyped mode key":      "apiserver:\n  auth:\n    mod: k8s-rbac\n",
		"unknown key with dummy": "apiserver:\n  auth:\n    mode: dummy\n    rbacEnabled: true\n",
	}
	for name, yamlConfStr := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := NewAuthResult(newAuthResultParams(t, yamlConfStr))
			assert.ErrorContains(t, err, "cannot parse apiserver.auth configuration")
		})
	}
}

func newAuthResultParams(t *testing.T, yamlConfStr string) AuthResultParams {
	t.Helper()
	return AuthResultParams{
		Provider:  newTestProvider(t, yamlConfStr),
		K8sConfig: &rest.Config{Host: "https://127.0.0.1:1"},
		Logger:    zap.NewNop(),
	}
}

// requireAllowAll asserts the result behaves exactly like the permissive
// default: no inbound middleware (no token required), every action on
// every kind allowed.
func requireAllowAll(t *testing.T, result AuthResult) {
	t.Helper()
	assert.Nil(t, result.InboundMiddleware)

	authenticated, err := result.Auth.UserAuthenticated(context.Background())
	require.NoError(t, err)
	assert.True(t, authenticated)

	for action := range verbForAction {
		for kind := range resourceForKind {
			authorized, err := result.Auth.UserAuthorized(context.Background(), "namespace", action, kind)
			require.NoError(t, err)
			assert.True(t, authorized)
		}
	}
}

func TestNewAuthResultDefaultsToDummy(t *testing.T) {
	result, err := NewAuthResult(newAuthResultParams(t, `
apiserver:
  yarpc:
    host: "127.0.0.1"
`))
	require.NoError(t, err)
	requireAllowAll(t, result)
}

func TestNewAuthResultExplicitDummy(t *testing.T) {
	result, err := NewAuthResult(newAuthResultParams(t, `
apiserver:
  auth:
    mode: "dummy"
`))
	require.NoError(t, err)
	requireAllowAll(t, result)
}

func TestNewAuthResultUnknownModeIsAStartupError(t *testing.T) {
	_, err := NewAuthResult(newAuthResultParams(t, `
apiserver:
  auth:
    mode: "basic"
`))
	assert.ErrorContains(t, err, `unknown auth mode "basic"`)
}

func TestNewAuthResultK8sRBACWithServiceAccounts(t *testing.T) {
	result, err := NewAuthResult(newAuthResultParams(t, `
apiserver:
  auth:
    mode: "k8s-rbac"
    serviceAccounts:
      enabled: true
`))
	require.NoError(t, err)
	assert.NotNil(t, result.Auth)
	assert.IsType(t, K8sRBACAuth{}, result.Auth)
	assert.NotNil(t, result.InboundMiddleware)
}

func TestNewAuthResultK8sRBACRequiresAnAuthenticator(t *testing.T) {
	_, err := NewAuthResult(newAuthResultParams(t, `
apiserver:
  auth:
    mode: "k8s-rbac"
`))
	assert.ErrorContains(t, err, "requires oidc.issuerUrl and/or serviceAccounts.enabled")
}

func TestNewAuthResultK8sRBACFailsFastOnUnreachableIssuer(t *testing.T) {
	_, err := NewAuthResult(newAuthResultParams(t, `
apiserver:
  auth:
    mode: "k8s-rbac"
    oidc:
      issuerUrl: "http://127.0.0.1:1"
      audiences: ["ma-api"]
`))
	assert.ErrorContains(t, err, "issuer discovery failed")
}
