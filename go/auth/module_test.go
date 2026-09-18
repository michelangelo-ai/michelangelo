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
	"go.uber.org/zap"
	"k8s.io/client-go/rest"
)

func newBundleParams(t *testing.T, yamlConfStr string) BundleParams {
	t.Helper()
	return BundleParams{
		Provider:  newTestProvider(t, yamlConfStr),
		K8sConfig: &rest.Config{Host: "https://127.0.0.1:1"},
		Logger:    zap.NewNop(),
	}
}

// requireAllowAll asserts the bundle behaves exactly like the permissive
// default: no token, every action on every kind allowed.
func requireAllowAll(t *testing.T, bundle AuthBundle) {
	t.Helper()
	userInfo, err := Authenticate(context.Background(), bundle.Authenticator)
	require.NoError(t, err)
	for action := range verbForAction {
		for kind := range resourceForKind {
			require.NoError(t, Authorize(context.Background(), bundle.Authorizer, userInfo, "namespace", action, kind))
		}
	}
}

func TestNewAuthBundleDefaultsToDummy(t *testing.T) {
	bundle, err := NewAuthBundle(newBundleParams(t, `
apiserver:
  yarpc:
    host: "127.0.0.1"
`))
	require.NoError(t, err)
	requireAllowAll(t, bundle)
}

func TestNewAuthBundleExplicitDummy(t *testing.T) {
	bundle, err := NewAuthBundle(newBundleParams(t, `
apiserver:
  auth:
    mode: "dummy"
`))
	require.NoError(t, err)
	requireAllowAll(t, bundle)
}

func TestNewAuthBundleUnknownModeIsAStartupError(t *testing.T) {
	_, err := NewAuthBundle(newBundleParams(t, `
apiserver:
  auth:
    mode: "basic"
`))
	assert.ErrorContains(t, err, `unknown auth mode "basic"`)
}

func TestNewAuthBundleK8sRBACWithServiceAccounts(t *testing.T) {
	bundle, err := NewAuthBundle(newBundleParams(t, `
apiserver:
  auth:
    mode: "k8s-rbac"
    serviceAccounts:
      enabled: true
`))
	require.NoError(t, err)
	assert.NotNil(t, bundle.Authenticator)
	assert.NotNil(t, bundle.Authorizer)
}

func TestNewAuthBundleK8sRBACRequiresAnAuthenticator(t *testing.T) {
	_, err := NewAuthBundle(newBundleParams(t, `
apiserver:
  auth:
    mode: "k8s-rbac"
`))
	assert.ErrorContains(t, err, "requires oidc.issuerUrl and/or serviceAccounts.enabled")
}

func TestNewAuthBundleK8sRBACFailsFastOnUnreachableIssuer(t *testing.T) {
	_, err := NewAuthBundle(newBundleParams(t, `
apiserver:
  auth:
    mode: "k8s-rbac"
    oidc:
      issuerUrl: "http://127.0.0.1:1"
      audiences: ["ma-api"]
`))
	assert.ErrorContains(t, err, "issuer discovery failed")
}
