package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

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
	core, logs := observer.New(zap.InfoLevel)
	assert.NoError(t, registerDeclarativeHook(zap.New(core), nil, pipelineapihook.Config{}))
	assert.Len(t, v2pb.GetPipelineAPIHooks(), 1)
	assertDeclarativeLog(t, logs, false, "", []any{})

	v2pb.ClearPipelineAPIHooks()
	dir := t.TempDir()
	release := "version: core-v1\n" +
		"artifact: s3://ma-artifacts/interpreters/core-v1.tar.gz\n" +
		"digest: sha256:1111111111111111111111111111111111111111111111111111111111111111\n" +
		"task_types:\n" +
		"  core/python:\n" +
		"    config:\n" +
		"      entry_point: {type: string, required: true}\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "core-v1.yaml"), []byte(release), 0o600))
	core, logs = observer.New(zap.InfoLevel)
	conf := pipelineapihook.Config{Enabled: true, DefaultInterpreter: "core-v1", ReleasesDir: dir}
	assert.NoError(t, registerDeclarativeHook(zap.New(core), nil, conf))
	assert.Len(t, v2pb.GetPipelineAPIHooks(), 1)
	assertDeclarativeLog(t, logs, true, "core-v1", []any{"core-v1"})

	v2pb.ClearPipelineAPIHooks()
	err := registerDeclarativeHook(zap.NewNop(), nil, pipelineapihook.Config{Enabled: true, DefaultInterpreter: "x", ReleasesDir: t.TempDir()})
	assert.ErrorContains(t, err, `default interpreter "x" has no release file`)
	assert.Empty(t, v2pb.GetPipelineAPIHooks())
}

func assertDeclarativeLog(t *testing.T, logs *observer.ObservedLogs, enabled bool, def string, versions []any) {
	t.Helper()
	// ContextMap returns an array field as []any.
	entries := logs.FilterMessage("declarative pipelines").All()
	require.Len(t, entries, 1)
	assert.Equal(t, map[string]any{
		"enabled":            enabled,
		"defaultInterpreter": def,
		"releases":           versions,
	}, entries[0].ContextMap())
}
