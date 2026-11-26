package declarative

import (
	"errors"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckRunInput(t *testing.T) {
	str := func(s string) *types.Value { return &types.Value{Kind: &types.Value_StringValue{StringValue: s}} }
	structOf := func(f map[string]*types.Value) *types.Value {
		return &types.Value{Kind: &types.Value_StructValue{StructValue: &types.Struct{Fields: f}}}
	}
	listOf := func(v ...*types.Value) *types.Value {
		return &types.Value{Kind: &types.Value_ListValue{ListValue: &types.ListValue{Values: v}}}
	}
	for _, tc := range []struct {
		name  string
		input *types.Struct
		want  string
	}{
		{name: "nil input"},
		{name: "cron input", input: &types.Struct{Fields: map[string]*types.Value{
			"args":    listOf(),
			"kw_args": structOf(map[string]*types.Value{}),
			"environ": structOf(map[string]*types.Value{LastScheduleTimestampEnv: str("1764115200")}),
		}}},
		{name: "empty values", input: &types.Struct{Fields: map[string]*types.Value{
			"args":          {Kind: &types.Value_NullValue{}},
			"parameter_map": structOf(map[string]*types.Value{}),
			"environ":       structOf(map[string]*types.Value{}),
		}}},
		{
			name:  "task_configs",
			input: &types.Struct{Fields: map[string]*types.Value{"task_configs": structOf(map[string]*types.Value{})}},
			want:  "run input task_configs is not supported for declarative pipelines",
		},
		{
			name:  "workflow_config",
			input: &types.Struct{Fields: map[string]*types.Value{"workflow_config": structOf(map[string]*types.Value{})}},
			want:  "run input workflow_config is not supported for declarative pipelines",
		},
		{
			name:  "args",
			input: &types.Struct{Fields: map[string]*types.Value{"args": listOf(str("x"))}},
			want:  "run input args must be empty for declarative pipelines",
		},
		{
			name:  "kw_args",
			input: &types.Struct{Fields: map[string]*types.Value{"kw_args": structOf(map[string]*types.Value{"k": str("v")})}},
			want:  "run input kw_args must be empty for declarative pipelines",
		},
		{
			name:  "parameter_map",
			input: &types.Struct{Fields: map[string]*types.Value{"parameter_map": structOf(map[string]*types.Value{"epochs": str("5")})}},
			want:  "parameter overrides are not supported yet",
		},
		{
			name: "environ key",
			input: &types.Struct{Fields: map[string]*types.Value{"environ": structOf(map[string]*types.Value{
				LastScheduleTimestampEnv: str("1"), "UF_TASK_IMAGE": str("img"),
			})}},
			want: `run input environ key "UF_TASK_IMAGE" is not supported for declarative pipelines; only LAST_SCHEDULE_TIMESTAMP is allowed`,
		},
		{
			name:  "environ is not a struct",
			input: &types.Struct{Fields: map[string]*types.Value{"environ": str("x")}},
			want:  "run input environ must be a struct",
		},
		{
			name: "LAST_SCHEDULE_TIMESTAMP is not a string",
			input: &types.Struct{Fields: map[string]*types.Value{"environ": structOf(map[string]*types.Value{
				LastScheduleTimestampEnv: {Kind: &types.Value_NumberValue{NumberValue: 1764115200}},
			})}},
			want: "run input environ LAST_SCHEDULE_TIMESTAMP must be a string",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := CheckRunInput(tc.input)
			if tc.want == "" {
				require.NoError(t, err)
				return
			}
			var declErr *Error
			require.True(t, errors.As(err, &declErr), "not an *Error: %v", err)
			assert.Equal(t, tc.want, err.Error())
		})
	}
}
