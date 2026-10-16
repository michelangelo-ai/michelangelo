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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/config"
	"go.uber.org/fx"
	"go.uber.org/yarpc/api/middleware"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
)

func deploymentOutbound() middleware.UnaryOutbound { return middleware.NopUnaryOutbound }

func providerOption(t *testing.T, yamlConfStr string) fx.Option {
	t.Helper()
	provider := newTestProvider(t, yamlConfStr)
	return fx.Provide(func() config.Provider { return provider })
}

// TestModuleAcceptsDeploymentProvidedOutboundMiddleware pins the guarantee
// downstream deployments rely on: Module alone never claims the optional
// middleware.UnaryOutbound slot, so a deployment-specific module that
// provides its own composes with it. fx reports a duplicate provider while
// the graph is being built, before any dependency is resolved, so the
// assertion does not depend on the rest of the graph being satisfiable.
func TestModuleAcceptsDeploymentProvidedOutboundMiddleware(t *testing.T) {
	err := fx.ValidateApp(
		Module,
		providerOption(t, "worker:\n  maApiServiceName: ma-api\n"),
		fx.Provide(deploymentOutbound),
	)
	if err != nil {
		assert.NotContains(t, err.Error(), "already provided")
	}
}

// TestRunnerTokenModuleConflictsWithDeploymentMiddleware documents why
// RunnerTokenModule is opt-in: it occupies the same slot, so a deployment
// that provides its own outbound middleware must leave it out.
func TestRunnerTokenModuleConflictsWithDeploymentMiddleware(t *testing.T) {
	err := fx.ValidateApp(
		Module,
		RunnerTokenModule,
		providerOption(t, "worker:\n  maApiServiceName: ma-api\n"),
		fx.Provide(deploymentOutbound),
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already provided")
}

// TestCheckRunnerTokenWiring is the truth table for the opt-in: the check
// keys on whether RunnerTokenModule is installed (its marker), never on
// whether some middleware occupies the slot.
func TestCheckRunnerTokenWiring(t *testing.T) {
	tests := []struct {
		name      string
		enabled   bool
		installed bool
		wantErr   bool
	}{
		{name: "disabled, module absent: the unchanged default"},
		{name: "disabled, module installed", installed: true},
		{name: "enabled, module installed", enabled: true, installed: true},
		{name: "enabled, module absent fails closed", enabled: true, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var conf Config
			conf.RunnerToken.Enabled = tt.enabled
			err := checkRunnerTokenWiring(conf, tt.installed)
			if !tt.wantErr {
				assert.NoError(t, err)
				return
			}
			assert.ErrorContains(t, err, "worker.RunnerTokenModule is not installed")
		})
	}
}

// TestNewYARPCDispatcherRejectsEnabledRunnerTokenWithoutTheModule covers the
// row a slot-is-filled check would miss: runnerToken enabled, the module left
// out, and the deployment's own outbound middleware in the slot. It must fail
// before a dispatcher is built or started.
func TestNewYARPCDispatcherRejectsEnabledRunnerTokenWithoutTheModule(t *testing.T) {
	var conf Config
	conf.RunnerToken.Enabled = true
	dispatcher, err := NewYARPCDispatcher(Params{
		Config:             conf,
		OutboundMiddleware: middleware.NopUnaryOutbound,
	})
	assert.Nil(t, dispatcher)
	assert.ErrorContains(t, err, "worker.RunnerTokenModule is not installed")
}

// withFakeKubernetesClient swaps the client builder so the enabled path runs
// without a cluster.
func withFakeKubernetesClient(t *testing.T) {
	t.Helper()
	orig := newKubernetesClient
	newKubernetesClient = func(config.Provider) (kubernetes.Interface, error) {
		return fake.NewSimpleClientset(), nil
	}
	t.Cleanup(func() { newKubernetesClient = orig })
}

// TestEnabledRunnerTokenInstallsMiddlewareAndMarker covers the success path
// of the enabled branch: both the middleware and the marker must be set. A
// missing marker here would make the stock worker reject its own module.
func TestEnabledRunnerTokenInstallsMiddlewareAndMarker(t *testing.T) {
	withFakeKubernetesClient(t)
	result, err := NewRunnerTokenOutboundMiddleware(RunnerTokenMiddlewareParams{
		Provider: newTestProvider(t, "worker:\n  runnerToken:\n    enabled: true\n"),
	})
	require.NoError(t, err)
	assert.NotNil(t, result.OutboundMiddleware)
	assert.NotNil(t, result.Installed)
}

// wiringCheckDeps is what NewYARPCDispatcher consumes for the check.
type wiringCheckDeps struct {
	fx.In

	Config    Config
	Installed *RunnerTokenInstalled `optional:"true"`
}

// TestRunnerTokenModuleSatisfiesTheWiringCheckThroughFx runs the real fx
// path, since fx provides a nil pointer for an unset fx.Out field and that is
// what the check would then see. With the module installed, the check must
// pass whether the feature is enabled or not.
func TestRunnerTokenModuleSatisfiesTheWiringCheckThroughFx(t *testing.T) {
	withFakeKubernetesClient(t)
	for name, yamlConfStr := range map[string]string{
		"enabled":  "worker:\n  runnerToken:\n    enabled: true\n",
		"disabled": "worker:\n  maApiServiceName: ma-api\n",
	} {
		t.Run(name, func(t *testing.T) {
			app := fx.New(
				fx.NopLogger,
				RunnerTokenModule,
				fx.Provide(NewConfig),
				providerOption(t, yamlConfStr),
				fx.Invoke(func(d wiringCheckDeps) error {
					return checkRunnerTokenWiring(d.Config, d.Installed != nil)
				}),
			)
			require.NoError(t, app.Err())
		})
	}
}

// TestRunnerTokenModuleProvidesTheMarkerEvenWhenDisabled keeps the marker
// independent of the feature flag: installing the module is what the check
// looks for, and a disabled worker still builds no middleware.
func TestRunnerTokenModuleProvidesTheMarkerEvenWhenDisabled(t *testing.T) {
	result, err := NewRunnerTokenOutboundMiddleware(RunnerTokenMiddlewareParams{
		Provider: newTestProvider(t, "worker:\n  maApiServiceName: ma-api\n"),
	})
	require.NoError(t, err)
	assert.NotNil(t, result.Installed)
	assert.Nil(t, result.OutboundMiddleware)
}
