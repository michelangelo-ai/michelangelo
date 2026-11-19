package declarative

import (
	"testing"

	v2 "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func diamond() *v2.DeclarativeWorkflow {
	return newWorkflow(map[string]*v2.DeclarativeTask{
		"extract":  task("core/python"),
		"train":    task("core/ray", "extract"),
		"evaluate": task("core/python", "extract"),
		"publish":  task("core/python", "train", "evaluate"),
	})
}

func TestGraphLevels(t *testing.T) {
	g, err := buildGraph(diamond())
	require.NoError(t, err)
	assert.Equal(t, [][]string{{"extract"}, {"evaluate", "train"}, {"publish"}}, g.levels())
}

func TestGraphReachability(t *testing.T) {
	g, err := buildGraph(diamondFailure())
	require.NoError(t, err)
	r := g.reachability()
	for _, d := range []string{"extract", "train", "evaluate"} {
		assert.True(t, r.dependsOn("publish", d), d)
	}
	assert.False(t, r.dependsOn("extract", "publish"))
	assert.False(t, r.dependsOn("extract", "extract"))
	assert.True(t, r.succeeded("publish", "extract"))
	assert.True(t, r.dependsOn("notify", "extract"))
	assert.True(t, r.succeeded("notify", "extract"), "publish ran, so its success gates held")
	assert.False(t, r.succeeded("notify", "publish"), "a FAILED edge does not prove success")
	assert.False(t, r.succeeded("cleanup", "train"), "a COMPLETED edge does not prove success")
	assert.True(t, r.succeeded("cleanup", "extract"))
	for _, n := range g.names {
		for _, d := range g.names {
			if r.succeeded(n, d) {
				assert.True(t, r.dependsOn(n, d), "%s succeeded-dep %s must be a dep", n, d)
			}
		}
	}
}
