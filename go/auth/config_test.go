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
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/config"
)

func newTestProvider(t *testing.T, yamlConfStr string) config.Provider {
	t.Helper()
	provider, err := config.NewYAML(config.Source(strings.NewReader(yamlConfStr)))
	require.NoError(t, err)
	return provider
}

func TestGetConfig(t *testing.T) {
	provider := newTestProvider(t, `
apiserver:
  auth:
    mode: "k8s-rbac"
    oidc:
      issuerUrl: "https://issuer.example.com"
      audiences: ["ma-api", "ma-web"]
      usernameClaim: "sub"
      groupsClaim: "roles"
      clockSkewLeeway: 1m
    serviceAccounts:
      enabled: true
      audiences: ["ma-api"]
    sarCache:
      allowTTL: 30s
      denyTTL: 5s
`)

	authConfig, err := GetConfig(provider)
	require.NoError(t, err)
	assert.Equal(t, ModeK8sRBAC, authConfig.Mode)
	assert.Equal(t, "https://issuer.example.com", authConfig.OIDC.IssuerURL)
	assert.Equal(t, []string{"ma-api", "ma-web"}, authConfig.OIDC.Audiences)
	assert.Equal(t, "sub", authConfig.OIDC.UsernameClaim)
	assert.Equal(t, "roles", authConfig.OIDC.GroupsClaim)
	assert.Equal(t, time.Minute, authConfig.OIDC.ClockSkewLeeway)
	assert.True(t, authConfig.ServiceAccounts.Enabled)
	assert.Equal(t, []string{"ma-api"}, authConfig.ServiceAccounts.Audiences)
	assert.Equal(t, 30*time.Second, authConfig.SARCache.allowTTL())
	assert.Equal(t, 5*time.Second, authConfig.SARCache.denyTTL())
}

func TestGetConfigAbsentKeyIsDummyMode(t *testing.T) {
	provider := newTestProvider(t, `
apiserver:
  yarpc:
    host: "127.0.0.1"
`)

	authConfig, err := GetConfig(provider)
	require.NoError(t, err)
	assert.Equal(t, Config{}, authConfig)
}

func TestSARCacheTTLDefaultsAndExplicitZero(t *testing.T) {
	// Unset TTLs take the 10s default.
	authConfig, err := GetConfig(newTestProvider(t, `
apiserver:
  auth:
    mode: "k8s-rbac"
`))
	require.NoError(t, err)
	assert.Equal(t, 10*time.Second, authConfig.SARCache.allowTTL())
	assert.Equal(t, 10*time.Second, authConfig.SARCache.denyTTL())

	// An explicit 0 disables that cache; the TTLs are independent.
	authConfig, err = GetConfig(newTestProvider(t, `
apiserver:
  auth:
    sarCache:
      allowTTL: 0s
      denyTTL: 1m
`))
	require.NoError(t, err)
	assert.Equal(t, time.Duration(0), authConfig.SARCache.allowTTL())
	assert.Equal(t, time.Minute, authConfig.SARCache.denyTTL())

	// Negative values are treated as disabled, not as a parse error.
	negative := -time.Second
	assert.Equal(t, time.Duration(0), SARCacheConfig{AllowTTL: &negative}.allowTTL())
}
