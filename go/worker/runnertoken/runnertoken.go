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

// Package runnertoken attaches per-project machine identity to the
// worker's outbound RPCs: for an activity targeting project namespace ns,
// it mints a short-lived token for the michelangelo-runner ServiceAccount
// in ns via the TokenRequest API and sends it as the bearer header, so the
// API server authorizes the call against that namespace's RoleBinding
// rather than any platform-wide worker identity.
package runnertoken

import (
	"context"
	"fmt"
	"sync"
	"time"

	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// ServiceAccountName is the per-project runner ServiceAccount whose tokens
// the worker mints.
const ServiceAccountName = "michelangelo-runner"

// refreshMargin re-mints a cached token this long before it expires.
const refreshMargin = time.Minute

// Config configures runner-token minting, read from the worker.runnerToken
// key.
type Config struct {
	// Enabled turns the outbound middleware on. Off (the default), the
	// worker attaches nothing and its traffic is unchanged.
	Enabled bool `yaml:"enabled"`
	// Audiences requested for the minted tokens; when non-empty, the API
	// server's TokenReview must be configured to accept one of them.
	Audiences []string `yaml:"audiences"`
	// TTL is the requested token lifetime. Zero leaves the cluster
	// default; Kubernetes enforces its own minimum (10 minutes).
	TTL time.Duration `yaml:"ttl"`
}

type namespaceKey struct{}

// WithNamespace annotates ctx with the project namespace the next outbound
// RPC targets; the middleware reads it to pick which runner token to mint.
func WithNamespace(ctx context.Context, namespace string) context.Context {
	return context.WithValue(ctx, namespaceKey{}, namespace)
}

// NamespaceFromContext returns the project namespace set by WithNamespace.
func NamespaceFromContext(ctx context.Context) (string, bool) {
	namespace, ok := ctx.Value(namespaceKey{}).(string)
	return namespace, ok
}

type cachedToken struct {
	token        string
	refreshAfter time.Time
}

// Minter requests michelangelo-runner ServiceAccount tokens per project
// namespace and caches each until shortly before it expires.
type Minter struct {
	client    kubernetes.Interface
	audiences []string
	ttl       time.Duration
	now       func() time.Time

	mu     sync.Mutex
	tokens map[string]cachedToken
}

// NewMinter returns a Minter backed by the given Kubernetes client.
func NewMinter(client kubernetes.Interface, config Config) *Minter {
	return &Minter{
		client:    client,
		audiences: config.Audiences,
		ttl:       config.TTL,
		now:       time.Now,
		tokens:    map[string]cachedToken{},
	}
}

// Token returns a valid runner token for the namespace, minting a fresh one
// through the TokenRequest API when none is cached or the cached one is
// near expiry. The lock is held across the mint so concurrent activities
// for one namespace produce a single request.
func (m *Minter) Token(ctx context.Context, namespace string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if cached, ok := m.tokens[namespace]; ok && m.now().Before(cached.refreshAfter) {
		return cached.token, nil
	}

	request := &authenticationv1.TokenRequest{
		Spec: authenticationv1.TokenRequestSpec{Audiences: m.audiences},
	}
	if m.ttl > 0 {
		seconds := int64(m.ttl / time.Second)
		request.Spec.ExpirationSeconds = &seconds
	}
	response, err := m.client.CoreV1().ServiceAccounts(namespace).CreateToken(ctx, ServiceAccountName, request, metav1.CreateOptions{})
	if err != nil {
		return "", fmt.Errorf("runnertoken: cannot mint a %s token in namespace %q: %w", ServiceAccountName, namespace, err)
	}

	m.tokens[namespace] = cachedToken{
		token:        response.Status.Token,
		refreshAfter: response.Status.ExpirationTimestamp.Time.Add(-refreshMargin),
	}
	return response.Status.Token, nil
}
