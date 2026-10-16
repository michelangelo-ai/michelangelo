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

package worker

import (
	"path/filepath"
	"strings"
	"testing"

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

// TestNewRunnerTokenOutboundMiddlewareDisabledByDefault pins the
// documented guarantee: a disabled worker builds no Kubernetes client and
// needs no cluster credentials -- if it did, this test would fail trying
// to reach a cluster, since no kubeconfig is available in a unit test.
func TestNewRunnerTokenOutboundMiddlewareDisabledByDefault(t *testing.T) {
	result, err := NewRunnerTokenOutboundMiddleware(RunnerTokenMiddlewareParams{
		Provider: newTestProvider(t, `
worker:
  maApiServiceName: "ma-api"
`),
	})
	require.NoError(t, err)
	assert.Nil(t, result.OutboundMiddleware)
}

func TestNewRunnerTokenOutboundMiddlewareExplicitlyDisabled(t *testing.T) {
	result, err := NewRunnerTokenOutboundMiddleware(RunnerTokenMiddlewareParams{
		Provider: newTestProvider(t, `
worker:
  runnerToken:
    enabled: false
`),
	})
	require.NoError(t, err)
	assert.Nil(t, result.OutboundMiddleware)
}

// TestNewRunnerTokenOutboundMiddlewareEnabledNeedsAKubernetesClient proves
// the feature fails closed (an error, not a silent no-op or a panic) when
// enabled outside a cluster with no kubeconfig to build a client from.
func TestNewRunnerTokenOutboundMiddlewareEnabledNeedsAKubernetesClient(t *testing.T) {
	// Hermetic: without these, the test only passes on a machine or CI job
	// where no kubeconfig is discoverable (KUBECONFIG, in-cluster env, or
	// $HOME/.kube/config).
	t.Setenv("KUBECONFIG", filepath.Join(t.TempDir(), "absent"))
	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	t.Setenv("HOME", t.TempDir())

	_, err := NewRunnerTokenOutboundMiddleware(RunnerTokenMiddlewareParams{
		Provider: newTestProvider(t, `
worker:
  runnerToken:
    enabled: true
`),
	})
	assert.ErrorContains(t, err, "runner-token middleware needs a kubernetes client")
}
