package declarative

import (
	"sort"
	"testing"

	gogoproto "github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protowire"
)

// mapEntries returns the key and the value bytes of each entry of map field
// num in b, in wire order.
func mapEntries(t *testing.T, b []byte, num protowire.Number) (keys []string, vals [][]byte) {
	t.Helper()
	for len(b) > 0 {
		n, typ, l := protowire.ConsumeTag(b)
		require.GreaterOrEqual(t, l, 0)
		b = b[l:]
		if n != num {
			l = protowire.ConsumeFieldValue(n, typ, b)
			require.GreaterOrEqual(t, l, 0)
			b = b[l:]
			continue
		}
		entry, l := protowire.ConsumeBytes(b)
		require.GreaterOrEqual(t, l, 0)
		b = b[l:]
		var key string
		var val []byte
		for len(entry) > 0 {
			en, _, el := protowire.ConsumeTag(entry)
			require.GreaterOrEqual(t, el, 0)
			entry = entry[el:]
			v, vl := protowire.ConsumeBytes(entry)
			require.GreaterOrEqual(t, vl, 0)
			entry = entry[vl:]
			if en == 1 {
				key = string(v)
			} else {
				val = v
			}
		}
		keys = append(keys, key)
		vals = append(vals, val)
	}
	return keys, vals
}

// field returns the bytes of the last occurrence of field num in b.
func field(t *testing.T, b []byte, num protowire.Number) []byte {
	t.Helper()
	var out []byte
	for len(b) > 0 {
		n, typ, l := protowire.ConsumeTag(b)
		require.GreaterOrEqual(t, l, 0)
		b = b[l:]
		if n == num {
			v, vl := protowire.ConsumeBytes(b)
			require.GreaterOrEqual(t, vl, 0)
			out = v
		}
		l = protowire.ConsumeFieldValue(n, typ, b)
		require.GreaterOrEqual(t, l, 0)
		b = b[l:]
	}
	return out
}

func sortedCopy(s []string) []string {
	out := append([]string(nil), s...)
	sort.Strings(out)
	return out
}

func TestEncodeSortsMapKeysAtEveryLevel(t *testing.T) {
	names := []string{"f", "a", "d", "b", "e", "c"}
	config := func() *types.Struct {
		s := &types.Struct{Fields: map[string]*types.Value{}}
		for _, k := range names {
			s.Fields["k"+k] = &types.Value{Kind: &types.Value_StructValue{StructValue: &types.Struct{Fields: map[string]*types.Value{
				"y": {Kind: &types.Value_NumberValue{NumberValue: 1}},
				"x": {Kind: &types.Value_BoolValue{BoolValue: true}},
				"z": {Kind: &types.Value_StringValue{StringValue: k}},
			}}}}
		}
		return s
	}
	w := &v2pb.DeclarativeWorkflow{
		SchemaVersion: SchemaVersion,
		Parameters:    map[string]*v2pb.ParameterSpec{},
		Tasks:         map[string]*v2pb.DeclarativeTask{},
	}
	for _, k := range names {
		w.Parameters["p"+k] = &v2pb.ParameterSpec{Default: &types.Value{Kind: &types.Value_StructValue{StructValue: config()}}}
		w.Tasks[k] = &v2pb.DeclarativeTask{TaskFunction: "core/python", Config: config(), JobSpecs: config()}
	}

	got, err := Encode(w)
	require.NoError(t, err)
	for i := 0; i < 50; i++ {
		again, err := Encode(w)
		require.NoError(t, err)
		require.Equal(t, got, again, "call %d", i)
	}

	back := &v2pb.DeclarativeWorkflow{}
	require.NoError(t, gogoproto.Unmarshal(got, back))
	assert.True(t, gogoproto.Equal(w, back), "the bytes decode to the same workflow")

	// assertSortedStruct checks Struct.fields (field 1) and each nested Struct.
	var assertSortedStruct func(path string, s []byte)
	assertSortedStruct = func(path string, s []byte) {
		keys, vals := mapEntries(t, s, 1)
		assert.Equal(t, sortedCopy(keys), keys, path)
		for i, v := range vals {
			if nested := field(t, v, 5); nested != nil { // Value.struct_value
				assertSortedStruct(path+"."+keys[i], nested)
			}
		}
	}
	taskKeys, tasks := mapEntries(t, got, 4)
	assert.Equal(t, sortedCopy(names), taskKeys)
	for i, task := range tasks {
		assertSortedStruct("tasks."+taskKeys[i]+".config", field(t, task, 2))
		assertSortedStruct("tasks."+taskKeys[i]+".job_specs", field(t, task, 3))
	}
	paramKeys, params := mapEntries(t, got, 3)
	assert.Len(t, paramKeys, len(names))
	assert.Equal(t, sortedCopy(paramKeys), paramKeys)
	for i, p := range params {
		assertSortedStruct("parameters."+paramKeys[i]+".default", field(t, field(t, p, 1), 5))
	}
}
