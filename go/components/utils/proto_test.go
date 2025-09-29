package utils

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/gogo/protobuf/types"

	"github.com/stretchr/testify/require"
)

func TestMapProtoValueToInterface(t *testing.T) {
	res, err := MapProtoValueToInterface(&types.Value{Kind: &types.Value_StructValue{
		StructValue: &types.Struct{Fields: map[string]*types.Value{
			"string": {Kind: &types.Value_StringValue{StringValue: "hi"}},
			"bool":   {Kind: &types.Value_BoolValue{BoolValue: true}},
			"number": {Kind: &types.Value_NumberValue{NumberValue: 3.14}},
			"list": {Kind: &types.Value_ListValue{ListValue: &types.ListValue{Values: []*types.Value{
				{Kind: &types.Value_StringValue{StringValue: "lorem"}},
				{Kind: &types.Value_NumberValue{NumberValue: 1}},
				nil,
			}}}},
			"nil":  nil,
			"null": {Kind: &types.Value_NullValue{}},
		}},
	}})

	expected := map[string]interface{}{
		"string": "hi",
		"bool":   true,
		"number": 3.14,
		"list":   []interface{}{"lorem", 1},
		"nil":    nil,
		"null":   nil,
	}

	require.NotNil(t, expected, res)
	require.NoError(t, err)
}

func TestMapProtoNil(t *testing.T) {
	var res interface{}
	var err error

	res, err = MapProtoValueToInterface(nil)
	require.Nil(t, res)
	require.NoError(t, err)

	res, err = MapProtoStructToInterface(nil)
	require.Nil(t, res)
	require.NoError(t, err)

	res, err = MapProtoListValueToInterface(nil)
	require.Nil(t, res)
	require.NoError(t, err)
}

type testType string

func TestToStruct(t *testing.T) {
	tests := []struct {
		in             map[string]interface{}
		wantPB         *types.Struct
		wantErr        bool
		failureMessage string
	}{{
		in:             nil,
		wantPB:         new(types.Struct),
		failureMessage: "nil map map[string]interface{}",
	}, {
		in:             make(map[string]interface{}),
		wantPB:         new(types.Struct),
		failureMessage: "empty map map[string]interface{}",
	}, {
		in: map[string]interface{}{
			"nil":     nil,
			"bool":    bool(false),
			"int":     int(-123),
			"int32":   int32(math.MinInt32),
			"int64":   int64(math.MinInt64),
			"uint":    uint(123),
			"uint32":  uint32(math.MaxInt32),
			"uint64":  uint64(math.MaxInt64),
			"float32": float32(123.456),
			"float64": float64(123.456),
			"string":  string("hello, world!"),
			"bytes":   []byte("\xde\xad\xbe\xef"),
			"map":     map[string]interface{}{"k1": "v1", "k2": "v2"},
			"slice":   []interface{}{"one", "two", "three"},
		},
		wantPB: &types.Struct{Fields: map[string]*types.Value{
			"nil":     NewNullValue(),
			"bool":    NewBoolValue(false),
			"int":     NewNumberValue(float64(-123)),
			"int32":   NewNumberValue(float64(math.MinInt32)),
			"int64":   NewNumberValue(float64(math.MinInt64)),
			"uint":    NewNumberValue(float64(123)),
			"uint32":  NewNumberValue(float64(math.MaxInt32)),
			"uint64":  NewNumberValue(float64(math.MaxInt64)),
			"float32": NewNumberValue(float64(float32(123.456))),
			"float64": NewNumberValue(float64(float64(123.456))),
			"string":  NewStringValue("hello, world!"),
			"bytes":   NewStringValue("3q2+7w=="),
			"map": NewStructValue(&types.Struct{Fields: map[string]*types.Value{
				"k1": NewStringValue("v1"),
				"k2": NewStringValue("v2"),
			}}),
			"slice": NewListValue(&types.ListValue{Values: []*types.Value{
				NewStringValue("one"),
				NewStringValue("two"),
				NewStringValue("three"),
			}}),
		}},
		failureMessage: "success with data in map[string]interface{}",
	}, {
		in:             map[string]interface{}{"\xde\xad\xbe\xef": "<invalid UTF-8>"},
		wantErr:        true,
		failureMessage: "invalid utf-8 key",
	}, {
		in:             map[string]interface{}{"string": "\xde\xad\xbe\xef"},
		wantErr:        true,
		failureMessage: "invalid utf-8 string value",
	}, {
		in: map[string]interface{}{
			"map[string]interface{}": map[string]interface{}{"\xde\xad\xbe\xef": "<invalid UTF-8>"},
		},
		wantErr:        true,
		failureMessage: "invalid map[string]interface{}",
	}, {
		in:             map[string]interface{}{"[]interface": []interface{}{testType("unsupported type")}},
		wantErr:        true,
		failureMessage: "invalid []interface",
	}, {
		in:             map[string]interface{}{"testType": testType("unsupported type")},
		wantErr:        true,
		failureMessage: "invalid type",
	}}

	for _, tt := range tests {
		gotPB, gotErr := NewStruct(tt.in)
		if tt.wantErr {
			assert.Error(t, gotErr, tt.failureMessage)
			continue
		}
		assert.True(t, tt.wantPB.Equal(gotPB), tt.failureMessage)
	}
}
