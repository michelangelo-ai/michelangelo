package starlark

import (
	"testing"

	"github.com/cadence-workflow/starlark-worker/service"
	"github.com/michelangelo-ai/michelangelo/go/worker/plugins/buildkite"
	"github.com/stretchr/testify/require"
)

func TestRegisterBuildkitePlugin(t *testing.T) {
	registry := make(map[string]service.IPlugin)
	RegisterBuildkitePlugin(registry)
	require.Same(t, buildkite.Plugin, registry["buildkite"])
}
