package main

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
)

// TestOptions_GraphResolves checks that every dependency in the controllermgr fx
// graph has a provider. fx resolves the graph only when the process starts, so a
// missing or mis-named provider — say, a controller's params struct asking for a
// Mapper without the name the k8sengine Mapper is provided under — still builds
// and only fails at startup. ValidateApp runs in dry-run mode: no constructor or
// invoke is called, so the test needs no cluster, database or configuration.
func TestOptions_GraphResolves(t *testing.T) {
	require.NoError(t, fx.ValidateApp(options(), fx.NopLogger))
}
