package declarative

import (
	"testing"

	"github.com/gogo/protobuf/types"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"github.com/stretchr/testify/require"
)

func task(fn string, deps ...string) *v2pb.DeclarativeTask {
	t := &v2pb.DeclarativeTask{TaskFunction: fn, Config: &types.Struct{Fields: map[string]*types.Value{}}}
	for _, d := range deps {
		t.DependsOn = append(t.DependsOn, &v2pb.TaskDependency{Task: d})
	}
	return t
}

// typed sets the entry_point that every testdata release task type requires.
func typed(w *v2pb.DeclarativeWorkflow) *v2pb.DeclarativeWorkflow {
	for _, tk := range w.Tasks {
		tk.Config.Fields["entry_point"] = strVal("main.py")
	}
	return w
}

func newWorkflow(tasks map[string]*v2pb.DeclarativeTask) *v2pb.DeclarativeWorkflow {
	return &v2pb.DeclarativeWorkflow{
		SchemaVersion: SchemaVersion,
		Parameters:    map[string]*v2pb.ParameterSpec{},
		Tasks:         tasks,
	}
}

func manifestOf(t *testing.T, w *v2pb.DeclarativeWorkflow) *v2pb.PipelineManifest {
	t.Helper()
	anyMsg, err := types.MarshalAny(w)
	require.NoError(t, err)
	return &v2pb.PipelineManifest{Type: v2pb.PIPELINE_MANIFEST_TYPE_DECLARATIVE, Content: anyMsg}
}

func strVal(s string) *types.Value {
	return &types.Value{Kind: &types.Value_StringValue{StringValue: s}}
}
func numVal(f float64) *types.Value {
	return &types.Value{Kind: &types.Value_NumberValue{NumberValue: f}}
}
func boolVal(b bool) *types.Value { return &types.Value{Kind: &types.Value_BoolValue{BoolValue: b}} }
func nullVal() *types.Value       { return &types.Value{Kind: &types.Value_NullValue{}} }
func listVal(vs ...*types.Value) *types.Value {
	return &types.Value{Kind: &types.Value_ListValue{ListValue: &types.ListValue{Values: vs}}}
}
func structVal(fields map[string]*types.Value) *types.Value {
	return &types.Value{Kind: &types.Value_StructValue{StructValue: &types.Struct{Fields: fields}}}
}

func paramSpec(typ v2pb.ParameterType, def *types.Value) *v2pb.ParameterSpec {
	return &v2pb.ParameterSpec{Type: typ, Default: def}
}
