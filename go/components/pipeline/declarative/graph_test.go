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

func TestGraphTransitiveDeps(t *testing.T) {
	g, err := buildGraph(diamond())
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{"extract": true, "train": true, "evaluate": true}, g.transitiveDeps("publish"))
	assert.Equal(t, map[string]bool{}, g.transitiveDeps("extract"))
}
