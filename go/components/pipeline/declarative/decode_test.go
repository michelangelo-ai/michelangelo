package declarative

import (
	"testing"

	gogoproto "github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	v2 "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protowire"
)

func TestDecodeRoundTrip(t *testing.T) {
	w := newWorkflow(map[string]*v2.DeclarativeTask{"a": task("core/python")})
	got, err := Decode(manifestOf(t, w))
	require.NoError(t, err)
	assert.True(t, gogoproto.Equal(w, got))
}

func TestDecodeRejectsWrongTypeURL(t *testing.T) {
	m := manifestOf(t, newWorkflow(map[string]*v2.DeclarativeTask{"a": task("core/python")}))
	m.Content.TypeUrl = "type.googleapis.com/michelangelo.api.v2.Pipeline"
	_, err := Decode(m)
	assert.EqualError(t, err, `content type URL "type.googleapis.com/michelangelo.api.v2.Pipeline" is not "`+TypeURL+`"`)
}

func TestDecodeRejectsNilContent(t *testing.T) {
	_, err := Decode(&v2.PipelineManifest{Type: v2.PIPELINE_MANIFEST_TYPE_DECLARATIVE})
	assert.EqualError(t, err, "content is not set")
}

func TestDecodeRejectsSchemaVersion(t *testing.T) {
	w := newWorkflow(map[string]*v2.DeclarativeTask{"a": task("core/python")})
	w.SchemaVersion = "v2"
	_, err := Decode(manifestOf(t, w))
	assert.EqualError(t, err, `schema_version "v2" is not supported; use "v1alpha1"`)
}

const (
	unknownFieldNum        = 2047
	workflowConfigFieldNum = 2
	tasksFieldNum          = 4
	mapKeyFieldNum         = 1
	mapValueFieldNum       = 2
)

func appendUnknown(b []byte) []byte {
	b = protowire.AppendTag(b, unknownFieldNum, protowire.VarintType)
	return protowire.AppendVarint(b, 1)
}

func appendBytesField(b []byte, num protowire.Number, v []byte) []byte {
	b = protowire.AppendTag(b, num, protowire.BytesType)
	return protowire.AppendBytes(b, v)
}

func TestDecodeRejectsUnknownTopLevelField(t *testing.T) {
	w := newWorkflow(map[string]*v2.DeclarativeTask{"a": task("core/python")})
	raw, err := gogoproto.Marshal(w)
	require.NoError(t, err)
	m := &v2.PipelineManifest{Type: v2.PIPELINE_MANIFEST_TYPE_DECLARATIVE,
		Content: &types.Any{TypeUrl: TypeURL, Value: appendUnknown(raw)}}
	_, err = Decode(m)
	assert.EqualError(t, err, "DeclarativeWorkflow has unknown field number 2047")
}

func TestDecodeRejectsUnknownNestedField(t *testing.T) {
	tk := task("core/python")
	tkRaw, err := gogoproto.Marshal(tk)
	require.NoError(t, err)
	tkRaw = appendUnknown(tkRaw)
	w := &v2.DeclarativeWorkflow{SchemaVersion: SchemaVersion}
	wRaw, err := gogoproto.Marshal(w)
	require.NoError(t, err)
	entry := appendBytesField(nil, mapKeyFieldNum, []byte("a"))
	entry = appendBytesField(entry, mapValueFieldNum, tkRaw)
	wRaw = appendBytesField(wRaw, tasksFieldNum, entry)
	m := &v2.PipelineManifest{Type: v2.PIPELINE_MANIFEST_TYPE_DECLARATIVE,
		Content: &types.Any{TypeUrl: TypeURL, Value: wRaw}}
	_, err = Decode(m)
	assert.EqualError(t, err, "DeclarativeWorkflow.tasks[a] has unknown field number 2047")
}

func TestDecodeRejectsUnknownFieldInSettings(t *testing.T) {
	s := &v2.WorkflowSettings{MaxConcurrency: 2}
	sRaw, err := gogoproto.Marshal(s)
	require.NoError(t, err)
	sRaw = appendUnknown(sRaw)
	w := &v2.DeclarativeWorkflow{SchemaVersion: SchemaVersion}
	wRaw, err := gogoproto.Marshal(w)
	require.NoError(t, err)
	wRaw = appendBytesField(wRaw, workflowConfigFieldNum, sRaw)
	m := &v2.PipelineManifest{Type: v2.PIPELINE_MANIFEST_TYPE_DECLARATIVE,
		Content: &types.Any{TypeUrl: TypeURL, Value: wRaw}}
	_, err = Decode(m)
	assert.EqualError(t, err, "DeclarativeWorkflow.workflow_config has unknown field number 2047")
}
