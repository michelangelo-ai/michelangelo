package rollout

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/michelangelo-ai/michelangelo/proto-go/api"
)

func TestNewRolloutPlugin(t *testing.T) {
	// PlacementPrep has not written the cluster snapshot yet, so the strategy contributes
	// no actors and what remains is the chain bracketing every rollout, whatever it targets.
	plugin, err := NewRolloutPlugin(context.Background(), Params{Logger: zap.NewNop()},
		assetDeployment(&api.ResourceIdentifier{Name: assetModelName}))

	require.NoError(t, err)
	types := make([]string, 0, len(plugin.GetActors()))
	for _, actor := range plugin.GetActors() {
		types = append(types, actor.GetType())
	}
	assert.Equal(t, []string{"Validated", "AssetsPrepared", "PlacementPrepared", "RolloutCompleted"}, types)
}
