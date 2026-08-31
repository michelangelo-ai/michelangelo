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
	"time"

	"go.uber.org/config"

	"github.com/michelangelo-ai/michelangelo/go/auth/oidc"
)

const (
	configKey = "apiserver.auth"

	// ModeDummy allows every request -- the shipped default, unchanged.
	ModeDummy = "dummy"
	// ModeK8sRBAC authenticates bearer tokens (OIDC and/or Kubernetes
	// ServiceAccount TokenReview) and authorizes through Kubernetes RBAC
	// via SubjectAccessReview.
	ModeK8sRBAC = "k8s-rbac"
)

const (
	defaultSARCacheTTL = 10 * time.Second
	tokenCacheTTL      = 10 * time.Second
	discoveryTimeout   = 30 * time.Second
)

// Config is the API server's auth configuration, read from the
// apiserver.auth key.
type Config struct {
	// Mode selects the implementation: ModeDummy (also the default when
	// unset) or ModeK8sRBAC.
	Mode string `yaml:"mode"`
	// OIDC configures ID-token verification for human callers; enabled by
	// setting oidc.issuerUrl.
	OIDC oidc.Config `yaml:"oidc"`
	// ServiceAccounts configures TokenReview-based authentication for
	// in-cluster callers.
	ServiceAccounts ServiceAccountsConfig `yaml:"serviceAccounts"`
	// SARCache bounds how long SubjectAccessReview decisions are cached.
	SARCache SARCacheConfig `yaml:"sarCache"`
}

// ServiceAccountsConfig configures ServiceAccount token authentication.
type ServiceAccountsConfig struct {
	// Enabled turns on TokenReview-based authentication.
	Enabled bool `yaml:"enabled"`
	// Audiences, when non-empty, is passed to TokenReview so only tokens
	// bound to one of them are accepted.
	Audiences []string `yaml:"audiences"`
}

// SARCacheConfig holds the SubjectAccessReview decision-cache TTLs. Each
// TTL defaults to 10s when unset; an explicit 0 disables that cache.
type SARCacheConfig struct {
	AllowTTL *time.Duration `yaml:"allowTTL"`
	DenyTTL  *time.Duration `yaml:"denyTTL"`
}

func (c SARCacheConfig) allowTTL() time.Duration { return ttlOrDefault(c.AllowTTL) }

func (c SARCacheConfig) denyTTL() time.Duration { return ttlOrDefault(c.DenyTTL) }

func ttlOrDefault(ttl *time.Duration) time.Duration {
	if ttl == nil {
		return defaultSARCacheTTL
	}
	if *ttl < 0 {
		return 0
	}
	return *ttl
}

// GetConfig parses the apiserver.auth configuration. An absent key yields
// the zero Config, i.e. dummy mode.
func GetConfig(provider config.Provider) (Config, error) {
	authConfig := Config{}
	err := provider.Get(configKey).Populate(&authConfig)
	return authConfig, err
}
