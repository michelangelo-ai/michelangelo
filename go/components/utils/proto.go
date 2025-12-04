// Package utils provides utility functions for protobuf type conversions.
//
// This package offers bidirectional conversion between Protocol Buffer types
// (proto.Value, proto.Struct, proto.ListValue) and Go's native interface{},
// map[string]interface{}, and []interface{} types.
//
// The conversions support the full range of JSON-compatible types including:
//   - Primitive types (string, bool, numbers)
//   - Null values
//   - Nested structures (maps and lists)
//   - Binary data (encoded as base64 strings)
//
// These utilities are particularly useful when working with dynamic data that
// needs to be passed between Go code and workflow engines (Cadence/Temporal) or
// other systems that use Protocol Buffers for serialization.
//
// Usage:
//
//	// Convert proto.Value to Go interface{}
//	goValue, err := MapProtoValueToInterface(protoValue)
//
//	// Convert Go interface{} to proto.Value
//	protoValue, err := NewValue(goValue)
package utils

import (
	"encoding/base64"
	"fmt"
	"unicode/utf8"

	"github.com/gogo/protobuf/types"
)

// MapProtoValueToInterface converts a proto.Value to a Go interface{}.
//
// This function recursively converts Protocol Buffer Value types to their
// corresponding Go representations:
//   - StringValue → string
//   - BoolValue → bool
//   - NumberValue → float64
//   - NullValue → nil
//   - StructValue → map[string]interface{} (recursive)
//   - ListValue → []interface{} (recursive)
//
// Returns an error if the proto.Value contains an unexpected or unsupported kind.
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

// MapProtoStructToInterface converts a proto.Struct to a Go map[string]interface{}.
//
// This function recursively converts a Protocol Buffer Struct (similar to a JSON object)
// to a Go map. Each field value is converted using MapProtoValueToInterface.
//
// Returns an error if any field value conversion fails.
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

// MapProtoListValueToInterface converts a proto.ListValue to a Go []interface{}.
//
// This function recursively converts a Protocol Buffer ListValue (similar to a JSON array)
// to a Go slice. Each element is converted using MapProtoValueToInterface.
//
// Returns an error if any element conversion fails.
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
//		╔════════════════════════╤════════════════════════════════════════════╗
//		║ Go type                │ Conversion                                 ║
//		╠════════════════════════╪════════════════════════════════════════════╣
//		║ nil                    │ stored as NullValue                        ║
//		║ bool                   │ stored as BoolValue                        ║
//		║ int, int32, int64      │ stored as NumberValue                      ║
//		║ uint, uint32, uint64   │ stored as NumberValue                      ║
//		║ float32, float64       │ stored as NumberValue                      ║
//		║ string                 │ stored as StringValue; must be valid UTF-8 ║
//		║ []byte                 │ stored as StringValue; base64-encoded      ║
//		║ map[string]interface{} │ stored as StructValue                      ║
//		║ []interface{}          │ stored as ListValue                        ║
//	 ║ []map[string]interface{} | stored as ListValue					  ║
//		╚════════════════════════╧════════════════════════════════════════════╝
//
// When converting an int64 or uint64 to a NumberValue, numeric precision loss
// is possible since they are stored as a float64.
//
// NewValue converts a Go interface{} to a proto.Value.
//
// This function performs type-based conversion from Go types to Protocol Buffer
// Value types. It supports all primitive types, maps, slices, and byte arrays.
//
// Supported conversions are documented in the type table above. The function
// recursively converts nested structures and validates UTF-8 encoding for strings.
//
// Returns an error if:
//   - The input type is not supported
//   - String values contain invalid UTF-8
//   - Nested structure conversion fails
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

// NewNullValue constructs a proto.Value representing a null value.
//
// This is equivalent to JSON's null value.
func NewNullValue() *types.Value {
	return &types.Value{Kind: &types.Value_NullValue{NullValue: types.NullValue_NULL_VALUE}}
}

// NewBoolValue constructs a proto.Value representing a boolean.
func NewBoolValue(v bool) *types.Value {
	return &types.Value{Kind: &types.Value_BoolValue{BoolValue: v}}
}

// NewNumberValue constructs a proto.Value representing a number.
//
// All numeric types are stored as float64 in Protocol Buffers.
func NewNumberValue(v float64) *types.Value {
	return &types.Value{Kind: &types.Value_NumberValue{NumberValue: v}}
}

// NewStringValue constructs a proto.Value representing a string.
func NewStringValue(v string) *types.Value {
	return &types.Value{Kind: &types.Value_StringValue{StringValue: v}}
}

// NewStructValue constructs a proto.Value wrapping a proto.Struct.
func NewStructValue(v *types.Struct) *types.Value {
	return &types.Value{Kind: &types.Value_StructValue{StructValue: v}}
}

// NewListValue constructs a proto.Value wrapping a proto.ListValue.
func NewListValue(v *types.ListValue) *types.Value {
	return &types.Value{Kind: &types.Value_ListValue{ListValue: v}}
}

// NewStruct constructs a proto.Struct from a Go map[string]interface{}.
//
// The map keys must be valid UTF-8 strings. The map values are recursively
// converted using NewValue.
//
// Returns an error if any key is invalid UTF-8 or any value conversion fails.
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

// NewList constructs a proto.ListValue from a Go []interface{}.
//
// The slice elements are recursively converted using NewValue.
//
// Returns an error if any element conversion fails.
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

// NewStringList constructs a proto.ListValue from a Go []string.
//
// This is a specialized version of NewList optimized for string slices.
// Each string is converted to a proto.Value using NewStringValue.
func NewStringList(v []string) (*types.ListValue, error) {
	x := &types.ListValue{Values: make([]*types.Value, len(v))}
	for i, v := range v {
		x.Values[i] = NewStringValue(v)
	}
	return x, nil
}

// NewListMap constructs a proto.ListValue from a Go []map[string]interface{}.
//
// This is a specialized version of NewList for slices of maps. Each map is
// converted to a proto.Struct using NewStruct.
//
// Returns an error if any map conversion fails.
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
