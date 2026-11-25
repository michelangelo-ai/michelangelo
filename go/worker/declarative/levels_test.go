package declarative

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gogo/protobuf/jsonpb"
	gogoproto "github.com/gogo/protobuf/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pipelinedecl "github.com/michelangelo-ai/michelangelo/go/components/pipeline/declarative"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

func readProto(t *testing.T, path string, m gogoproto.Message) {
	t.Helper()
	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()
	require.NoError(t, jsonpb.Unmarshal(f, m))
}

// The engine and the planner each compute levels. A drift between them would
// make the explain plan show an order that the run does not follow.
func TestEngineLevelsMatchPlanner(t *testing.T) {
	tar := casesTar(t)
	paths, err := filepath.Glob("testdata/specs/*.json")
	require.NoError(t, err)
	var names []string
	for _, p := range paths {
		names = append(names, strings.TrimSuffix(filepath.Base(p), ".json"))
	}
	require.Equal(t, []string{"alt_shape", "diamond", "diamond_failure"}, names)

	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			w := &v2pb.DeclarativeWorkflow{}
			readProto(t, filepath.Join("testdata", "specs", name+".json"), w)
			plan := &v2pb.ExplainPlan{}
			readProto(t, filepath.Join("testdata", "plans", name+".json"), plan)
			want := []any{}
			for _, lvl := range plan.Levels {
				tasks := []any{}
				for _, task := range lvl.Tasks {
					tasks = append(tasks, task)
				}
				want = append(want, tasks)
			}

			args, err := pipelinedecl.WorkflowArgs(w)
			require.NoError(t, err)
			res, _, err := runStar(t, tar, "/cases.star", "levels_of", args[0])
			require.NoError(t, err)
			assert.Equal(t, map[string]any{"levels": want}, toGo(t, res))
		})
	}
}
