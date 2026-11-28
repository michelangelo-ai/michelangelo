package declarative

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/gogo/protobuf/jsonpb"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func readWorkflow(t *testing.T, path string) *v2pb.DeclarativeWorkflow {
	t.Helper()
	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()
	w := &v2pb.DeclarativeWorkflow{}
	require.NoError(t, jsonpb.Unmarshal(f, w))
	return w
}

func TestWorkflowArgsGolden(t *testing.T) {
	args, err := WorkflowArgs(readWorkflow(t, filepath.Join("testdata", "inputs", "diamond.spec.json")))
	require.NoError(t, err)
	got, err := json.Marshal(args)
	require.NoError(t, err)
	want, err := os.ReadFile(filepath.Join("testdata", "inputs", "diamond.json"))
	require.NoError(t, err)
	assert.JSONEq(t, string(want), string(got))
}

func TestWorkflowArgsRequiredParam(t *testing.T) {
	w := typedDiamond()
	w.Parameters["x"] = &v2pb.ParameterSpec{Type: v2pb.PARAMETER_TYPE_STRING, Required: true}
	_, err := WorkflowArgs(w)
	assert.ErrorContains(t, err, `parameter "x" has no value`)
}
