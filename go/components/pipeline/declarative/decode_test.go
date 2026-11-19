package declarative

import (
	"strings"
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
	taskConfigFieldNum     = 2
	structFieldsFieldNum   = 1
	structValueFieldNum    = 5
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
	assert.EqualError(t, err, `DeclarativeWorkflow.tasks["a"] has unknown field number 2047`)
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

func workflowWithTaskConfig(t *testing.T, cfgRaw []byte) *v2.PipelineManifest {
	t.Helper()
	tkRaw, err := gogoproto.Marshal(&v2.DeclarativeTask{TaskFunction: "core/python"})
	require.NoError(t, err)
	tkRaw = appendBytesField(tkRaw, taskConfigFieldNum, cfgRaw)
	wRaw, err := gogoproto.Marshal(&v2.DeclarativeWorkflow{SchemaVersion: SchemaVersion})
	require.NoError(t, err)
	entry := appendBytesField(nil, mapKeyFieldNum, []byte("a"))
	entry = appendBytesField(entry, mapValueFieldNum, tkRaw)
	wRaw = appendBytesField(wRaw, tasksFieldNum, entry)
	return &v2.PipelineManifest{Type: v2.PIPELINE_MANIFEST_TYPE_DECLARATIVE,
		Content: &types.Any{TypeUrl: TypeURL, Value: wRaw}}
}

func TestDecodeRejectsUnknownFieldInsideConfig(t *testing.T) {
	cfg := &types.Struct{Fields: map[string]*types.Value{"k": strVal("v")}}
	cfgRaw, err := gogoproto.Marshal(cfg)
	require.NoError(t, err)
	_, err = Decode(workflowWithTaskConfig(t, appendUnknown(cfgRaw)))
	assert.EqualError(t, err, `DeclarativeWorkflow.tasks["a"].config has unknown field number 2047`)
}

func TestDecodeAcceptsArbitraryConfigKeys(t *testing.T) {
	cfg := &types.Struct{Fields: map[string]*types.Value{
		"any_key": strVal("v"),
		"nested":  structVal(map[string]*types.Value{"deep": listVal(numVal(1), boolVal(true), nullVal())}),
	}}
	cfgRaw, err := gogoproto.Marshal(cfg)
	require.NoError(t, err)
	w, err := Decode(workflowWithTaskConfig(t, cfgRaw))
	require.NoError(t, err)
	assert.True(t, gogoproto.Equal(cfg, w.Tasks["a"].Config))
}

func TestDecodeQuotesUntrustedMapKeysInErrors(t *testing.T) {
	inner, err := gogoproto.Marshal(&types.Struct{})
	require.NoError(t, err)
	val := appendBytesField(nil, structValueFieldNum, appendUnknown(inner))
	entry := appendBytesField(nil, mapKeyFieldNum, []byte("x\nFAKE LOG LINE"))
	entry = appendBytesField(entry, mapValueFieldNum, val)
	cfgRaw := appendBytesField(nil, structFieldsFieldNum, entry)
	_, err = Decode(workflowWithTaskConfig(t, cfgRaw))
	assert.EqualError(t, err, `DeclarativeWorkflow.tasks["a"].config.fields["x\nFAKE LOG LINE"].struct_value has unknown field number 2047`)
}

func TestDecodeRejectsMapEntryWithRepeatedKeyOfWrongWireType(t *testing.T) {
	raw := []byte{0x22, 0x05, 0x0a, 0x01, 'a', 0x08, 0x00}
	var err error
	require.NotPanics(t, func() {
		_, err = Decode(&v2.PipelineManifest{Content: &types.Any{TypeUrl: TypeURL, Value: raw}})
	})
	var e *Error
	require.ErrorAs(t, err, &e)
	assert.Contains(t, e.Msg, "content does not decode")
}

func TestDecodeRejectsOversizedContent(t *testing.T) {
	raw := make([]byte, maxContentBytes+1)
	_, err := Decode(&v2.PipelineManifest{Content: &types.Any{TypeUrl: TypeURL, Value: raw}})
	assert.EqualError(t, err, "content is 4194305 bytes; the limit is 4194304")
}

func TestDecodeRejectsDeepNesting(t *testing.T) {
	v := strVal("leaf")
	for i := 0; i < recursionLimit; i++ {
		v = structVal(map[string]*types.Value{"k": v})
	}
	cfgRaw, err := gogoproto.Marshal(&types.Struct{Fields: map[string]*types.Value{"k": v}})
	require.NoError(t, err)
	_, err = Decode(workflowWithTaskConfig(t, cfgRaw))
	var e *Error
	require.ErrorAs(t, err, &e)
	assert.True(t, strings.HasPrefix(e.Msg, "content does not decode: "), e.Msg)
}

func TestDecodeRejectsUnknownFieldInDependsOn(t *testing.T) {
	const dependsOnFieldNum = 4
	depRaw, err := gogoproto.Marshal(&v2.TaskDependency{Task: "b"})
	require.NoError(t, err)
	tkRaw, err := gogoproto.Marshal(&v2.DeclarativeTask{TaskFunction: "core/python"})
	require.NoError(t, err)
	tkRaw = appendBytesField(tkRaw, dependsOnFieldNum, appendUnknown(depRaw))
	wRaw, err := gogoproto.Marshal(&v2.DeclarativeWorkflow{SchemaVersion: SchemaVersion})
	require.NoError(t, err)
	entry := appendBytesField(nil, mapKeyFieldNum, []byte("a"))
	entry = appendBytesField(entry, mapValueFieldNum, tkRaw)
	wRaw = appendBytesField(wRaw, tasksFieldNum, entry)
	_, err = Decode(&v2.PipelineManifest{Content: &types.Any{TypeUrl: TypeURL, Value: wRaw}})
	assert.EqualError(t, err, `DeclarativeWorkflow.tasks["a"].depends_on[0] has unknown field number 2047`)
}

func TestDecodeRejectsMalformedWireBytes(t *testing.T) {
	raw := protowire.AppendTag(nil, 1, protowire.BytesType)
	raw = protowire.AppendVarint(raw, 10)
	_, err := Decode(&v2.PipelineManifest{Content: &types.Any{TypeUrl: TypeURL, Value: raw}})
	var e *Error
	require.ErrorAs(t, err, &e)
	assert.True(t, strings.HasPrefix(e.Msg, "content does not decode: "), e.Msg)
}

func FuzzDecode(f *testing.F) {
	for _, w := range []*v2.DeclarativeWorkflow{typed(diamond()), typed(altShape()), typed(diamondFailure())} {
		b, err := gogoproto.Marshal(w)
		require.NoError(f, err)
		f.Add(b)
	}
	f.Add([]byte{0x22, 0x05, 0x0a, 0x01, 'a', 0x08, 0x00})
	f.Fuzz(func(t *testing.T, raw []byte) {
		w, err := Decode(&v2.PipelineManifest{Content: &types.Any{TypeUrl: TypeURL, Value: raw}})
		if err != nil {
			var e *Error
			require.ErrorAs(t, err, &e)
			return
		}
		_ = Validate(w, Limits{})
	})
}
