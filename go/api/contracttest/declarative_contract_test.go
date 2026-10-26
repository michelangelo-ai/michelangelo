package contracttest

import (
	"testing"

	pbtypes "github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/require"

	v2 "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

const declarativeTypeURL = "type.googleapis.com/michelangelo.api.v2.DeclarativeWorkflow"

func TestDeclarativeWorkflowAnyRoundTrip(t *testing.T) {
	in := &v2.DeclarativeWorkflow{
		SchemaVersion: "v1alpha1",
		Parameters: map[string]*v2.ParameterSpec{
			"n": {Default: &pbtypes.Value{Kind: &pbtypes.Value_NumberValue{NumberValue: 4}}},
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
	require.Equal(t, float64(4), out.GetParameters()["n"].GetDefault().GetNumberValue())
	require.Equal(t, "core/ray", out.GetTasks()["root"].GetTaskFunction())
}

func TestManifestTypeEnumValues(t *testing.T) {
	require.EqualValues(t, 4, v2.PIPELINE_MANIFEST_TYPE_DECLARATIVE)
	_, hasTwo := v2.PipelineManifest_Type_name[2]
	require.False(t, hasTwo, "enum value 2 must stay reserved")
}
