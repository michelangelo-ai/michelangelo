package modelconfig

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// A rollout moves one entry through canary -> staged -> serving. Each step must update the
// existing entry in place rather than append a duplicate, and must clear the canary pod once
// the model is no longer pinned to one replica.
func TestAddModelToConfigRefreshesPhase(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(emptyModelConfigMap()).Build()
	provider := NewDefaultModelConfigProvider()
	ctx := context.Background()

	canary := ModelConfigEntry{
		Name:           "model-v1",
		StoragePath:    "s3://bucket/model-v1",
		DeploymentName: "deployment-a",
		Phase:          ModelPhaseCanary,
		CanaryPod:      "pod-a",
	}
	require.NoError(t, provider.AddModelToConfig(ctx, zap.NewNop(), fakeClient, "test-server", "default", canary))
	assert.Equal(t, []ModelConfigEntry{canary}, readEntries(t, fakeClient))

	staged := canary
	staged.Phase = ModelPhaseStaged
	staged.CanaryPod = ""
	require.NoError(t, provider.AddModelToConfig(ctx, zap.NewNop(), fakeClient, "test-server", "default", staged))
	assert.Equal(t, []ModelConfigEntry{staged}, readEntries(t, fakeClient), "the canary pod is cleared when the phase moves on")

	serving := staged
	serving.Phase = ModelPhaseServing
	require.NoError(t, provider.AddModelToConfig(ctx, zap.NewNop(), fakeClient, "test-server", "default", serving))
	entries := readEntries(t, fakeClient)
	require.Len(t, entries, 1)
	assert.Equal(t, serving, entries[0])

	// Another deployment sharing the model keeps its own phase.
	other := ModelConfigEntry{Name: "model-v1", StoragePath: "s3://bucket/model-v1", DeploymentName: "deployment-b", Phase: ModelPhaseStaged}
	require.NoError(t, provider.AddModelToConfig(ctx, zap.NewNop(), fakeClient, "test-server", "default", other))
	assert.ElementsMatch(t, []ModelConfigEntry{serving, other}, readEntries(t, fakeClient))
}

func TestCurrentPhase(t *testing.T) {
	assert.Equal(t, ModelPhaseServing, ModelConfigEntry{}.CurrentPhase(), "legacy entries without a phase are serving")
	assert.Equal(t, ModelPhaseCanary, ModelConfigEntry{Phase: ModelPhaseCanary}.CurrentPhase())
	assert.Equal(t, ModelPhaseStaged, ModelConfigEntry{Phase: ModelPhaseStaged}.CurrentPhase())
}

func TestFindEntry(t *testing.T) {
	entries := []ModelConfigEntry{
		{Name: "model-v1", DeploymentName: "deployment-a"},
		{Name: "model-v1", DeploymentName: "deployment-b", Phase: ModelPhaseStaged},
	}

	got, ok := FindEntry(entries, "deployment-b", "model-v1")
	require.True(t, ok)
	assert.Equal(t, entries[1], got)

	_, ok = FindEntry(entries, "deployment-c", "model-v1")
	assert.False(t, ok)

	_, ok = FindEntry(entries, "deployment-a", "model-v2")
	assert.False(t, ok)
}
