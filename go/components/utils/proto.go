package utils

import (
	"encoding/base64"
	"fmt"
	"unicode/utf8"

	"github.com/gogo/protobuf/types"
)

// MapProtoValueToInterface transforms given proto Value to interface{} generic object representation
func MapProtoValueToInterface(v *types.Value) (interface{}, error) {
	if v == nil {
		return nil, nil
	}
	kind := v.Kind
	if x, ok := kind.(*types.Value_StringValue); ok {
		return x.StringValue, nil
	}
	if x, ok := kind.(*types.Value_BoolValue); ok {
		return x.BoolValue, nil
	}
	if x, ok := kind.(*types.Value_NumberValue); ok {
		return x.NumberValue, nil
	}
	if _, ok := kind.(*types.Value_NullValue); ok {
		return nil, nil
	}
	if x, ok := kind.(*types.Value_StructValue); ok {
		return MapProtoStructToInterface(x.StructValue)
	}
	if x, ok := kind.(*types.Value_ListValue); ok {
		return MapProtoListValueToInterface(x.ListValue)
	}
	return nil, fmt.Errorf("failed to map proto Value; unexpected kind: %#v", kind)
}

// MapProtoStructToInterface transforms given proto Struct to map[string]interface{} generic object representation
func MapProtoStructToInterface(v *types.Struct) (map[string]interface{}, error) {
	if v == nil {
		return nil, nil
	}
	r := map[string]interface{}{}
	for k, v := range v.Fields {
		v0, e := MapProtoValueToInterface(v)
		if e != nil {
			return nil, e
		}
		r[k] = v0
	}
	return r, nil
}

// MapProtoListValueToInterface transforms given proto ListValue to []interface{} generic object representation
func MapProtoListValueToInterface(x *types.ListValue) ([]interface{}, error) {
	if x == nil {
		return nil, nil
	}
	r := make([]interface{}, len(x.Values))
	for i, v := range x.Values {
		_v, e := MapProtoValueToInterface(v)
		if e != nil {
			return nil, e
		}
		r[i] = _v
	}
	return r, nil
}

// Construct proto.Struct from map[string]interface{} type
// Reference: https://github.com/protocolbuffers/protobuf-go/blob/master/types/known/structpb/struct.pb.go
// Not adding the repo directly as it seems there are some compatibility issues tracked in https://code.uberinternal.com/T5741973.

// NewValue constructs a Value from a general-purpose Go interface.
//
//	╔════════════════════════╤════════════════════════════════════════════╗
//	║ Go type                │ Conversion                                 ║
//	╠════════════════════════╪════════════════════════════════════════════╣
//	║ nil                    │ stored as NullValue                        ║
//	║ bool                   │ stored as BoolValue                        ║
//	║ int, int32, int64      │ stored as NumberValue                      ║
//	║ uint, uint32, uint64   │ stored as NumberValue                      ║
//	║ float32, float64       │ stored as NumberValue                      ║
//	║ string                 │ stored as StringValue; must be valid UTF-8 ║
//	║ []byte                 │ stored as StringValue; base64-encoded      ║
//	║ map[string]interface{} │ stored as StructValue                      ║
//	║ []interface{}          │ stored as ListValue                        ║
//  ║ []map[string]interface{} | stored as ListValue					  ║
//	╚════════════════════════╧════════════════════════════════════════════╝
//
// When converting an int64 or uint64 to a NumberValue, numeric precision loss
// is possible since they are stored as a float64.

// NewValue given interface{} type, produce *types.Value
func NewValue(v interface{}) (*types.Value, error) {
	switch v := v.(type) {
	case nil:
		return NewNullValue(), nil
	case bool:
		return NewBoolValue(v), nil
	case int:
		return NewNumberValue(float64(v)), nil
	case int32:
		return NewNumberValue(float64(v)), nil
	case int64:
		return NewNumberValue(float64(v)), nil
	case uint:
		return NewNumberValue(float64(v)), nil
	case uint32:
		return NewNumberValue(float64(v)), nil
	case uint64:
		return NewNumberValue(float64(v)), nil
	case float32:
		return NewNumberValue(float64(v)), nil
	case float64:
		return NewNumberValue(float64(v)), nil
	case string:
		if !utf8.ValidString(v) {
			return nil, fmt.Errorf("invalid UTF-8 in string: %q", v)
		}
		return NewStringValue(v), nil
	case []byte:
		s := base64.StdEncoding.EncodeToString(v)
		return NewStringValue(s), nil
	case map[string]interface{}:
		v2, err := NewStruct(v)
		if err != nil {
			return nil, err
		}
		return NewStructValue(v2), nil
	case []interface{}:
		v2, err := NewList(v)
		if err != nil {
			return nil, err
		}
		return NewListValue(v2), nil
	case []string:
		v2, err := NewStringList(v)
		if err != nil {
			return nil, err
		}
		return NewListValue(v2), nil
	case []map[string]interface{}:
		v2, err := NewListMap(v)
		if err != nil {
			return nil, err
		}
		return NewListValue(v2), nil
	default:
		return nil, fmt.Errorf("invalid type: %T, %s", v, v)
	}
}

// NewNullValue constructs a new null Value.
func NewNullValue() *types.Value {
	return &types.Value{Kind: &types.Value_NullValue{NullValue: types.NullValue_NULL_VALUE}}
}

// NewBoolValue constructs a new boolean Value.
func NewBoolValue(v bool) *types.Value {
	return &types.Value{Kind: &types.Value_BoolValue{BoolValue: v}}
}

// NewNumberValue constructs a new number Value.
func NewNumberValue(v float64) *types.Value {
	return &types.Value{Kind: &types.Value_NumberValue{NumberValue: v}}
}

// NewStringValue constructs a new string Value.
func NewStringValue(v string) *types.Value {
	return &types.Value{Kind: &types.Value_StringValue{StringValue: v}}
}

// NewStructValue constructs a new struct Value.
func NewStructValue(v *types.Struct) *types.Value {
	return &types.Value{Kind: &types.Value_StructValue{StructValue: v}}
}

// NewListValue creates a new protobuf list value.
func NewListValue(v *types.ListValue) *types.Value {
	return &types.Value{Kind: &types.Value_ListValue{ListValue: v}}
}

// NewStruct constructs a Struct from a general-purpose Go map.
// The map keys must be valid UTF-8.
// The map values are converted using NewValue.
func NewStruct(v map[string]interface{}) (*types.Struct, error) {
	x := &types.Struct{Fields: make(map[string]*types.Value, len(v))}
	for k, v := range v {
		if !utf8.ValidString(k) {
			return nil, fmt.Errorf("invalid UTF-8 in string: %q", k)
		}
		var err error
		x.Fields[k], err = NewValue(v)
		if err != nil {
			return nil, err
		}
	}
	return x, nil
}

// NewList constructs a ListValue from a general-purpose Go slice.
// The slice elements are converted using NewValue.
func NewList(v []interface{}) (*types.ListValue, error) {
	x := &types.ListValue{Values: make([]*types.Value, len(v))}
	for i, v := range v {
		var err error
		x.Values[i], err = NewValue(v)
		if err != nil {
			return nil, err
		}
	}
	return x, nil
}

// NewStringList constructs a ListValue from a []string
func NewStringList(v []string) (*types.ListValue, error) {
	x := &types.ListValue{Values: make([]*types.Value, len(v))}
	for i, v := range v {
		x.Values[i] = NewStringValue(v)
	}
	return x, nil
}

// NewListMap constructs a ListValue from a map[string]interface.
// The slice elements are converted using NewStruct.
func NewListMap(v []map[string]interface{}) (*types.ListValue, error) {
	x := &types.ListValue{Values: make([]*types.Value, len(v))}
	for i, v := range v {
		var err error
		v2, err := NewStruct(v)
		if err != nil {
			return nil, err
		}
		x.Values[i] = NewStructValue(v2)
	}
	return x, nil
}
