package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"

	pipelineapihook "github.com/michelangelo-ai/michelangelo/go/components/pipeline/apihook"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

func TestDependenciesAreSatisfied(t *testing.T) {
	assert.NoError(t, fx.ValidateApp(opts()))
}

func TestGetTallyScope(t *testing.T) {
	scope, err := getTallyScope()
	assert.NoError(t, err)
	assert.NotNil(t, scope)
}

func TestGetScheme(t *testing.T) {
	scheme, err := getScheme()
	assert.NoError(t, err)
	assert.NotNil(t, scheme)
}

func TestRegisterDeclarativeHook(t *testing.T) {
	t.Cleanup(v2pb.ClearPipelineAPIHooks)

	v2pb.ClearPipelineAPIHooks()
	assert.NoError(t, registerDeclarativeHook(nil, pipelineapihook.Config{}))
	assert.Len(t, v2pb.GetPipelineAPIHooks(), 1)

	v2pb.ClearPipelineAPIHooks()
	err := registerDeclarativeHook(nil, pipelineapihook.Config{Enabled: true, DefaultInterpreter: "x", ReleasesDir: t.TempDir()})
	assert.ErrorContains(t, err, `default interpreter "x" has no release file`)
	assert.Empty(t, v2pb.GetPipelineAPIHooks())
}
