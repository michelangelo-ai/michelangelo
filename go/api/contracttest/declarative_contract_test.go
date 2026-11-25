package contracttest

import (
	"strings"
	"testing"

	"github.com/gogo/protobuf/jsonpb"
	pbtypes "github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/require"

	v2 "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

const declarativeTypeURL = "type.googleapis.com/michelangelo.api.v2.DeclarativeWorkflow"

func TestDeclarativeWorkflowAnyRoundTrip(t *testing.T) {
	in := &v2.DeclarativeWorkflow{
		SchemaVersion: "v1alpha1",
		Parameters: map[string]*v2.ParameterSpec{
			"n": {
				Type:    v2.PARAMETER_TYPE_INTEGER,
				Default: &pbtypes.Value{Kind: &pbtypes.Value_NumberValue{NumberValue: 4}},
			},
		},
		Tasks: map[string]*v2.DeclarativeTask{
			"root": {TaskFunction: "core/ray"},
		},
	}
	anyMsg, err := pbtypes.MarshalAny(in)
	require.NoError(t, err)
	require.Equal(t, declarativeTypeURL, anyMsg.GetTypeUrl())

	manifest := &v2.PipelineManifest{
		Type:    v2.PIPELINE_MANIFEST_TYPE_DECLARATIVE,
		Content: anyMsg,
		InterpreterPin: &v2.InterpreterPin{
			Version: "core-v1.4.2", Artifact: "s3://b/core-v1.4.2.tar.gz", Digest: "sha256:00",
		},
	}
	raw, err := manifest.Marshal()
	require.NoError(t, err)

	back := &v2.PipelineManifest{}
	require.NoError(t, back.Unmarshal(raw))
	require.Equal(t, v2.PIPELINE_MANIFEST_TYPE_DECLARATIVE, back.GetType())
	require.Equal(t, "core-v1.4.2", back.GetInterpreterPin().GetVersion())

	out := &v2.DeclarativeWorkflow{}
	require.NoError(t, pbtypes.UnmarshalAny(back.GetContent(), out))
	require.Equal(t, "v1alpha1", out.GetSchemaVersion())
	require.Equal(t, v2.PARAMETER_TYPE_INTEGER, out.GetParameters()["n"].GetType())
	require.Equal(t, float64(4), out.GetParameters()["n"].GetDefault().GetNumberValue())
	require.Equal(t, "core/ray", out.GetTasks()["root"].GetTaskFunction())
}

func TestManifestTypeEnumValues(t *testing.T) {
	require.EqualValues(t, 4, v2.PIPELINE_MANIFEST_TYPE_DECLARATIVE)
	_, hasTwo := v2.PipelineManifest_Type_name[2]
	require.False(t, hasTwo, "enum value 2 must stay reserved")
	_, hasASL := v2.PipelineManifest_Type_value["PIPELINE_MANIFEST_TYPE_ASL"]
	require.False(t, hasASL, "enum name PIPELINE_MANIFEST_TYPE_ASL must stay reserved")
}

// TestParameterSpecDefaultRoundTrip packs one ParameterSpec per declared type
// through Any and through protojson and checks that the default keeps its
// Value kind. Validation of default against type is the apiserver's job.
func TestParameterSpecDefaultRoundTrip(t *testing.T) {
	listDefault := &pbtypes.Value{Kind: &pbtypes.Value_ListValue{ListValue: &pbtypes.ListValue{Values: []*pbtypes.Value{
		{Kind: &pbtypes.Value_StringValue{StringValue: "a"}},
		{Kind: &pbtypes.Value_NumberValue{NumberValue: 7}},
	}}}}
	dictDefault := &pbtypes.Value{Kind: &pbtypes.Value_StructValue{StructValue: &pbtypes.Struct{Fields: map[string]*pbtypes.Value{
		"depth": {Kind: &pbtypes.Value_NumberValue{NumberValue: 6}},
	}}}}
	cases := []struct {
		name string
		typ  v2.ParameterType
		def  *pbtypes.Value
		json string
	}{
		{"string", v2.PARAMETER_TYPE_STRING, &pbtypes.Value{Kind: &pbtypes.Value_StringValue{StringValue: "demo"}}, `"demo"`},
		{"integer", v2.PARAMETER_TYPE_INTEGER, &pbtypes.Value{Kind: &pbtypes.Value_NumberValue{NumberValue: 2000}}, `2000`},
		{"number", v2.PARAMETER_TYPE_NUMBER, &pbtypes.Value{Kind: &pbtypes.Value_NumberValue{NumberValue: 0.001}}, `0.001`},
		{"boolean", v2.PARAMETER_TYPE_BOOLEAN, &pbtypes.Value{Kind: &pbtypes.Value_BoolValue{BoolValue: true}}, `true`},
		{"list", v2.PARAMETER_TYPE_LIST, listDefault, `["a",7]`},
		{"dict", v2.PARAMETER_TYPE_DICT, dictDefault, `{"depth":6}`},
		{"null", v2.PARAMETER_TYPE_STRING, &pbtypes.Value{Kind: &pbtypes.Value_NullValue{}}, `null`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := &v2.DeclarativeWorkflow{
				SchemaVersion: "v1alpha1",
				Parameters:    map[string]*v2.ParameterSpec{"p": {Type: tc.typ, Default: tc.def}},
			}
			anyMsg, err := pbtypes.MarshalAny(in)
			require.NoError(t, err)
			out := &v2.DeclarativeWorkflow{}
			require.NoError(t, pbtypes.UnmarshalAny(anyMsg, out))
			require.Equal(t, tc.typ, out.GetParameters()["p"].GetType())
			require.Equal(t, tc.def, out.GetParameters()["p"].GetDefault())

			js, err := (&jsonpb.Marshaler{}).MarshalToString(in)
			require.NoError(t, err)
			require.Contains(t, js, `"default":`+tc.json)
			require.Contains(t, js, `"type":"`+tc.typ.String()+`"`)
			fromJSON := &v2.DeclarativeWorkflow{}
			require.NoError(t, jsonpb.UnmarshalString(js, fromJSON))
			require.Equal(t, tc.def, fromJSON.GetParameters()["p"].GetDefault())
		})
	}
}

// TestParameterSpecUnsetVersusNull keeps the two states apart on the wire:
// no default at all (required parameter) and an explicit JSON null.
func TestParameterSpecUnsetVersusNull(t *testing.T) {
	unset := &v2.ParameterSpec{Type: v2.PARAMETER_TYPE_STRING, Required: true}
	null := &v2.ParameterSpec{Type: v2.PARAMETER_TYPE_STRING, Default: &pbtypes.Value{Kind: &pbtypes.Value_NullValue{}}}
	for _, spec := range []*v2.ParameterSpec{unset, null} {
		raw, err := spec.Marshal()
		require.NoError(t, err)
		back := &v2.ParameterSpec{}
		require.NoError(t, back.Unmarshal(raw))
		require.Equal(t, spec.GetDefault() == nil, back.GetDefault() == nil)
	}
	require.Nil(t, unset.GetDefault())
	require.NotNil(t, null.GetDefault())
	_, isNull := null.GetDefault().GetKind().(*pbtypes.Value_NullValue)
	require.True(t, isNull)
}

// TestEnumWireFormIsFullName pins the protojson rule: full enum names decode,
// short names are rejected. Authoring tools map short names before packing.
func TestEnumWireFormIsFullName(t *testing.T) {
	cases := []struct {
		name string
		json string
		ok   bool
	}{
		{"outcome full", `{"tasks":{"t":{"dependsOn":[{"task":"a","when":"OUTCOME_FAILED"}]}}}`, true},
		{"outcome short", `{"tasks":{"t":{"dependsOn":[{"task":"a","when":"FAILED"}]}}}`, false},
		{"type full", `{"parameters":{"p":{"type":"PARAMETER_TYPE_INTEGER"}}}`, true},
		{"type short", `{"parameters":{"p":{"type":"integer"}}}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := &v2.DeclarativeWorkflow{}
			err := jsonpb.Unmarshal(strings.NewReader(tc.json), out)
			if tc.ok {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
		})
	}
	out := &v2.DeclarativeWorkflow{}
	require.NoError(t, jsonpb.UnmarshalString(cases[0].json, out))
	require.Equal(t, v2.OUTCOME_FAILED, out.GetTasks()["t"].GetDependsOn()[0].GetWhen())
}
