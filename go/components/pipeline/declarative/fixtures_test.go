package declarative

import (
	"testing"

	"github.com/gogo/protobuf/types"
	v2 "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"github.com/stretchr/testify/require"
)

func task(fn string, deps ...string) *v2.DeclarativeTask {
	t := &v2.DeclarativeTask{TaskFunction: fn, Config: &types.Struct{Fields: map[string]*types.Value{}}}
	for _, d := range deps {
		t.DependsOn = append(t.DependsOn, &v2.TaskDependency{Task: d})
	}
	return t
}

func newWorkflow(tasks map[string]*v2.DeclarativeTask) *v2.DeclarativeWorkflow {
	return &v2.DeclarativeWorkflow{
		SchemaVersion: SchemaVersion,
		Parameters:    map[string]*v2.ParameterSpec{},
		Tasks:         tasks,
	}
}

func manifestOf(t *testing.T, w *v2.DeclarativeWorkflow) *v2.PipelineManifest {
	t.Helper()
	anyMsg, err := types.MarshalAny(w)
	require.NoError(t, err)
	return &v2.PipelineManifest{Type: v2.PIPELINE_MANIFEST_TYPE_DECLARATIVE, Content: anyMsg}
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

func paramSpec(typ v2.ParameterType, def *types.Value) *v2.ParameterSpec {
	return &v2.ParameterSpec{Type: typ, Default: def}
}
