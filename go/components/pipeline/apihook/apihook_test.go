package apihook

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protowire"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/michelangelo-ai/michelangelo/go/api/apimocks"
	"github.com/michelangelo-ai/michelangelo/go/components/pipeline/declarative"
	v2 "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

const (
	ns   = "proj"
	name = "train"
)

func loadDeclarative(t *testing.T, enabled bool, def string) *Declarative {
	t.Helper()
	if !enabled {
		return &Declarative{}
	}
	d, err := NewDeclarative(Config{Enabled: true, DefaultInterpreter: def, ReleasesDir: releasesDir})
	require.NoError(t, err)
	return d
}

func newHook(t *testing.T, d *Declarative) (apiHook, *apimocks.MockHandler) {
	t.Helper()
	h := apimocks.NewMockHandler(gomock.NewController(t))
	return apiHook{apiHandler: h, d: d}, h
}

func strVal(s string) *types.Value {
	return &types.Value{Kind: &types.Value_StringValue{StringValue: s}}
}

func numVal(f float64) *types.Value {
	return &types.Value{Kind: &types.Value_NumberValue{NumberValue: f}}
}

// workflow returns a two-task workflow that every test release accepts.
// extra adds config fields to task "b".
func workflow(extra map[string]*types.Value) *v2.DeclarativeWorkflow {
	b := &v2.DeclarativeTask{
		TaskFunction: "core/python",
		Config:       &types.Struct{Fields: map[string]*types.Value{"entry_point": strVal("b.py")}},
		DependsOn:    []*v2.TaskDependency{{Task: "a"}},
	}
	for k, v := range extra {
		b.Config.Fields[k] = v
	}
	return &v2.DeclarativeWorkflow{
		SchemaVersion: declarative.SchemaVersion,
		Tasks: map[string]*v2.DeclarativeTask{
			"a": {
				TaskFunction: "core/python",
				Config:       &types.Struct{Fields: map[string]*types.Value{"entry_point": strVal("a.py")}},
			},
			"b": b,
		},
	}
}

// declarativePipeline packs w in the byte form that the hook stores.
func declarativePipeline(t *testing.T, w *v2.DeclarativeWorkflow) *v2.Pipeline {
	t.Helper()
	raw, err := declarative.Encode(w)
	require.NoError(t, err)
	return &v2.Pipeline{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name},
		Spec: v2.PipelineSpec{Manifest: &v2.PipelineManifest{
			Type:    v2.PIPELINE_MANIFEST_TYPE_DECLARATIVE,
			Content: &types.Any{TypeUrl: declarative.TypeURL, Value: raw},
		}},
	}
}

// encodeInOrder encodes w by hand. It writes the task map entries, and the
// config fields of each task, in sorted key order or in reverse order.
// Repeated message fields merge, so each config field is a separate Struct.
func encodeInOrder(t *testing.T, w *v2.DeclarativeWorkflow, reverse bool) []byte {
	t.Helper()
	keys := func(n int, at func(int) string) []string {
		out := make([]string, n)
		for i := range out {
			out[i] = at(i)
		}
		sort.Strings(out)
		if reverse {
			sort.Sort(sort.Reverse(sort.StringSlice(out)))
		}
		return out
	}
	marshal := func(m proto.Message) []byte {
		b, err := proto.Marshal(m)
		require.NoError(t, err)
		return b
	}
	head := proto.Clone(w).(*v2.DeclarativeWorkflow)
	head.Tasks = nil
	out := marshal(head)
	taskNames := make([]string, 0, len(w.Tasks))
	for k := range w.Tasks {
		taskNames = append(taskNames, k)
	}
	for _, k := range keys(len(taskNames), func(i int) string { return taskNames[i] }) {
		bare := proto.Clone(w.Tasks[k]).(*v2.DeclarativeTask)
		bare.Config = nil
		task := marshal(bare)
		fields := w.Tasks[k].GetConfig().GetFields()
		fieldNames := make([]string, 0, len(fields))
		for f := range fields {
			fieldNames = append(fieldNames, f)
		}
		for _, f := range keys(len(fieldNames), func(i int) string { return fieldNames[i] }) {
			one := &types.Struct{Fields: map[string]*types.Value{f: fields[f]}}
			task = append(task, marshal(&v2.DeclarativeTask{Config: one})...)
		}
		var entry []byte
		entry = protowire.AppendTag(entry, 1, protowire.BytesType)
		entry = protowire.AppendString(entry, k)
		entry = protowire.AppendTag(entry, 2, protowire.BytesType)
		entry = protowire.AppendBytes(entry, task)
		out = protowire.AppendTag(out, 4, protowire.BytesType) // DeclarativeWorkflow.tasks
		out = protowire.AppendBytes(out, entry)
	}
	return out
}

func typedPipeline(typ v2.PipelineManifest_Type) *v2.Pipeline {
	return &v2.Pipeline{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   ns,
			Name:        name,
			Annotations: map[string]string{explainPlanAnnotation: "client", "team": "ml"},
		},
		Spec: v2.PipelineSpec{Manifest: &v2.PipelineManifest{
			Type:       typ,
			UniflowTar: "s3://bucket/user.tar.gz",
		}},
	}
}

func requireStatus(t *testing.T, err error, code codes.Code, msg string) {
	t.Helper()
	require.Error(t, err)
	st := status.Convert(err)
	assert.Equal(t, code, st.Code(), st.Message())
	assert.Equal(t, msg, st.Message())
}

func TestPassThrough(t *testing.T) {
	pipelines := map[string]func() *v2.Pipeline{
		"yaml":         func() *v2.Pipeline { return typedPipeline(v2.PIPELINE_MANIFEST_TYPE_YAML) },
		"uniflow":      func() *v2.Pipeline { return typedPipeline(v2.PIPELINE_MANIFEST_TYPE_UNIFLOW) },
		"no manifest":  func() *v2.Pipeline { return &v2.Pipeline{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name}} },
		"invalid type": func() *v2.Pipeline { return typedPipeline(v2.PIPELINE_MANIFEST_TYPE_INVALID) },
	}
	for pname, mk := range pipelines {
		for _, enabled := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/enabled=%t", pname, enabled), func(t *testing.T) {
				// The mock has no expectations, so any Get fails the test.
				hook, _ := newHook(t, loadDeclarative(t, enabled, "core-v1"))

				// The hook removes only the explain-plan annotation, so a
				// client cannot store a plan on a non-declarative pipeline.
				want := mk()
				delete(want.Annotations, explainPlanAnnotation)

				create := &v2.CreatePipelineRequest{Pipeline: mk(), CreateOptions: &metav1.CreateOptions{DryRun: []string{metav1.DryRunAll}}}
				require.NoError(t, hook.BeforeCreate(context.Background(), create))
				assert.Equal(t, want, create.Pipeline)

				update := &v2.UpdatePipelineRequest{Pipeline: mk(), UpdateOptions: &metav1.UpdateOptions{DryRun: []string{metav1.DryRunAll}}}
				require.NoError(t, hook.BeforeUpdate(context.Background(), update))
				assert.Equal(t, want, update.Pipeline)
			})
		}
	}
}

// No code reads interpreter_pin on a non-declarative pipeline, so the hook
// removes it without an error.
func TestNonDeclarativePinIsRemoved(t *testing.T) {
	manifestTypes := []v2.PipelineManifest_Type{
		v2.PIPELINE_MANIFEST_TYPE_YAML,
		v2.PIPELINE_MANIFEST_TYPE_UNIFLOW,
		v2.PIPELINE_MANIFEST_TYPE_INVALID,
	}
	for _, typ := range manifestTypes {
		for _, enabled := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/enabled=%t", typ, enabled), func(t *testing.T) {
				// The mock has no expectations, so any Get fails the test.
				hook, _ := newHook(t, loadDeclarative(t, enabled, "core-v1"))
				pinned := func() *v2.Pipeline {
					p := typedPipeline(typ)
					p.Spec.Manifest.InterpreterPin = &v2.InterpreterPin{Version: "core-v1"}
					return p
				}
				want := typedPipeline(typ)
				delete(want.Annotations, explainPlanAnnotation)

				create := &v2.CreatePipelineRequest{Pipeline: pinned()}
				require.NoError(t, hook.BeforeCreate(context.Background(), create))
				assert.Equal(t, want, create.Pipeline)

				update := &v2.UpdatePipelineRequest{Pipeline: pinned()}
				require.NoError(t, hook.BeforeUpdate(context.Background(), update))
				assert.Equal(t, want, update.Pipeline)
			})
		}
	}
}

func TestFlagOff(t *testing.T) {
	hook, _ := newHook(t, loadDeclarative(t, false, ""))
	const msg = "declarative pipelines are not enabled on this apiserver; an operator must set apiserver.declarative.enabled=true"

	err := hook.BeforeCreate(context.Background(), &v2.CreatePipelineRequest{Pipeline: declarativePipeline(t, workflow(nil))})
	requireStatus(t, err, codes.FailedPrecondition, msg)

	err = hook.BeforeUpdate(context.Background(), &v2.UpdatePipelineRequest{Pipeline: declarativePipeline(t, workflow(nil))})
	requireStatus(t, err, codes.FailedPrecondition, msg)
}

func dryRunCreate(p *v2.Pipeline) *v2.CreatePipelineRequest {
	return &v2.CreatePipelineRequest{Pipeline: p, CreateOptions: &metav1.CreateOptions{DryRun: []string{metav1.DryRunAll}}}
}

func release(t *testing.T, d *Declarative, version string) *declarative.Release {
	t.Helper()
	r, ok := d.releases[version]
	require.True(t, ok, version)
	return r
}

// explainPlan parses the annotation on p and returns nil when it is absent.
func explainPlan(t *testing.T, p *v2.Pipeline) *v2.ExplainPlan {
	t.Helper()
	raw, ok := p.Annotations[explainPlanAnnotation]
	if !ok {
		return nil
	}
	got := &v2.ExplainPlan{}
	require.NoError(t, jsonpb.UnmarshalString(raw, got))
	return got
}

func wantPlan(t *testing.T, w *v2.DeclarativeWorkflow, r *declarative.Release) *v2.ExplainPlan {
	t.Helper()
	p, err := declarative.Plan(w, declarative.DefaultLimits(), r, nil)
	require.NoError(t, err)
	return p
}

func TestBeforeCreate(t *testing.T) {
	d := loadDeclarative(t, true, "core-v1")
	v1 := release(t, d, "core-v1")
	ctx := context.Background()

	t.Run("client pin", func(t *testing.T) {
		v2Rel := release(t, d, "core-v2")
		onlyV2 := map[string]*types.Value{"timeout_seconds": numVal(30)}
		const versions = "[core-v1, core-v2]"
		tests := []struct {
			name     string
			extra    map[string]*types.Value
			reqPin   *v2.InterpreterPin
			reqTar   string
			wantPin  *v2.InterpreterPin
			wantCode codes.Code
			wantMsg  string
		}{
			{
				name:    "a loaded version is validated and stamped",
				extra:   onlyV2,
				reqPin:  &v2.InterpreterPin{Version: "core-v2"},
				wantPin: pinOf(v2Rel),
			},
			{
				name:    "the full pin of a loaded release is accepted",
				reqPin:  pinOf(v2Rel),
				reqTar:  v2Rel.Artifact,
				wantPin: pinOf(v2Rel),
			},
			{
				name:    "no version with the default artifact gives the default pin",
				reqPin:  &v2.InterpreterPin{Artifact: v1.Artifact},
				wantPin: pinOf(v1),
			},
			{
				name:     "a field the named release does not declare is rejected",
				extra:    onlyV2,
				reqPin:   &v2.InterpreterPin{Version: "core-v1"},
				wantCode: codes.InvalidArgument,
			},
			{
				name:     "an unknown version is rejected",
				reqPin:   &v2.InterpreterPin{Version: "core-v9"},
				wantCode: codes.InvalidArgument,
				wantMsg:  `interpreter_pin.version "core-v9" is not a known release; use one of ` + versions,
			},
			{
				name:     "an artifact that differs from the named release is rejected",
				reqPin:   &v2.InterpreterPin{Version: "core-v2", Artifact: "s3://evil/tar.gz"},
				wantCode: codes.InvalidArgument,
				wantMsg:  `interpreter_pin.artifact "s3://evil/tar.gz" does not match the pin for interpreter core-v2; leave it unset`,
			},
			{
				name:     "the default artifact beside another version is rejected",
				reqPin:   &v2.InterpreterPin{Version: "core-v2", Artifact: v1.Artifact},
				wantCode: codes.InvalidArgument,
				wantMsg:  `interpreter_pin.artifact "` + v1.Artifact + `" does not match the pin for interpreter core-v2; leave it unset`,
			},
			{
				name:     "a digest that differs from the named release is rejected",
				reqPin:   &v2.InterpreterPin{Version: "core-v2", Digest: "sha256:bad"},
				wantCode: codes.InvalidArgument,
				wantMsg:  `interpreter_pin.digest "sha256:bad" does not match the pin for interpreter core-v2; leave it unset`,
			},
			{
				name:     "a uniflow_tar of the default release beside another version is rejected",
				reqPin:   &v2.InterpreterPin{Version: "core-v2"},
				reqTar:   v1.Artifact,
				wantCode: codes.InvalidArgument,
				wantMsg:  "uniflow_tar is server-owned for declarative pipelines; remove uniflow_tar from the request",
			},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				hook, _ := newHook(t, d)
				p := declarativePipeline(t, workflow(tc.extra))
				p.Spec.Manifest.InterpreterPin = tc.reqPin
				p.Spec.Manifest.UniflowTar = tc.reqTar
				err := hook.BeforeCreate(ctx, &v2.CreatePipelineRequest{Pipeline: p})
				if tc.wantCode != codes.OK {
					require.Error(t, err)
					st := status.Convert(err)
					assert.Equal(t, tc.wantCode, st.Code(), st.Message())
					if tc.wantMsg != "" {
						assert.Equal(t, tc.wantMsg, st.Message())
					}
					return
				}
				require.NoError(t, err)
				assert.Equal(t, tc.wantPin, p.Spec.Manifest.InterpreterPin)
				assert.Equal(t, tc.wantPin.Artifact, p.Spec.Manifest.UniflowTar)
			})
		}
	})

	t.Run("client uniflow_tar is rejected", func(t *testing.T) {
		hook, _ := newHook(t, d)
		p := declarativePipeline(t, workflow(nil))
		p.Spec.Manifest.UniflowTar = "s3://bucket/user.tar.gz"
		requireStatus(t, hook.BeforeCreate(ctx, &v2.CreatePipelineRequest{Pipeline: p}),
			codes.InvalidArgument, "uniflow_tar is server-owned for declarative pipelines; remove uniflow_tar from the request")
	})

	t.Run("client uniflow_tar equal to the default artifact is accepted", func(t *testing.T) {
		hook, _ := newHook(t, d)
		p := declarativePipeline(t, workflow(nil))
		p.Spec.Manifest.UniflowTar = v1.Artifact
		require.NoError(t, hook.BeforeCreate(ctx, &v2.CreatePipelineRequest{Pipeline: p}))
		assert.Equal(t, v1.Artifact, p.Spec.Manifest.UniflowTar)
	})

	t.Run("manifest that does not decode is rejected", func(t *testing.T) {
		hook, _ := newHook(t, d)
		p := declarativePipeline(t, workflow(nil))
		p.Spec.Manifest.Content.TypeUrl = "type.googleapis.com/other"
		_, wantErr := declarative.Decode(p.Spec.Manifest)
		require.Error(t, wantErr)
		requireStatus(t, hook.BeforeCreate(ctx, &v2.CreatePipelineRequest{Pipeline: p}),
			codes.InvalidArgument, wantErr.Error())
	})

	t.Run("unknown task_function gives the planner error", func(t *testing.T) {
		hook, _ := newHook(t, d)
		w := workflow(nil)
		w.Tasks["b"].TaskFunction = "core/nope"
		_, wantErr := declarative.Plan(w, declarative.DefaultLimits(), v1, nil)
		require.Error(t, wantErr)
		requireStatus(t, hook.BeforeCreate(ctx, &v2.CreatePipelineRequest{Pipeline: declarativePipeline(t, w)}),
			codes.InvalidArgument, wantErr.Error())
	})

	t.Run("field the default release does not declare is rejected", func(t *testing.T) {
		hook, _ := newHook(t, d)
		w := workflow(map[string]*types.Value{"timeout_seconds": numVal(30)})
		err := hook.BeforeCreate(ctx, &v2.CreatePipelineRequest{Pipeline: declarativePipeline(t, w)})
		assert.Equal(t, codes.InvalidArgument, status.Code(err), err)
	})

	t.Run("real create stamps the default pin and removes a client annotation", func(t *testing.T) {
		hook, _ := newHook(t, d)
		p := declarativePipeline(t, workflow(nil))
		p.Annotations = map[string]string{explainPlanAnnotation: `{"taskCount":99}`, "keep": "me"}
		require.NoError(t, hook.BeforeCreate(ctx, &v2.CreatePipelineRequest{
			Pipeline: p, CreateOptions: &metav1.CreateOptions{},
		}))
		assert.Equal(t, pinOf(v1), p.Spec.Manifest.InterpreterPin)
		assert.Equal(t, v1.Artifact, p.Spec.Manifest.UniflowTar)
		assert.Equal(t, map[string]string{"keep": "me"}, p.Annotations)
	})

	t.Run("nil CreateOptions is not a dry run", func(t *testing.T) {
		hook, _ := newHook(t, d)
		p := declarativePipeline(t, workflow(nil))
		require.NoError(t, hook.BeforeCreate(ctx, &v2.CreatePipelineRequest{Pipeline: p}))
		assert.Equal(t, pinOf(v1), p.Spec.Manifest.InterpreterPin)
		assert.Nil(t, explainPlan(t, p))
	})

	t.Run("dry run sets the explain plan annotation", func(t *testing.T) {
		hook, _ := newHook(t, d)
		w := workflow(nil)
		p := declarativePipeline(t, w)
		p.Annotations = map[string]string{explainPlanAnnotation: `{"taskCount":99}`}
		require.NoError(t, hook.BeforeCreate(ctx, dryRunCreate(p)))
		assert.Equal(t, pinOf(v1), p.Spec.Manifest.InterpreterPin)
		got := explainPlan(t, p)
		require.NotNil(t, got)
		assert.Equal(t, wantPlan(t, w, v1), got)
		assert.Equal(t, "core-v1", got.InterpreterVersion)
		assert.Equal(t, uint32(2), got.TaskCount)
		// jsonpb.Unmarshal accepts both name styles, so check the raw text
		// for the lowerCamel names that protojson uses.
		raw := p.Annotations[explainPlanAnnotation]
		assert.Contains(t, raw, `"interpreterVersion":"core-v1"`)
		assert.Contains(t, raw, `"taskCount":2`)
	})
}

// paddedPipeline returns a valid pipeline whose content is exactly n bytes.
// It pads one args string of task "b".
func paddedPipeline(t *testing.T, n int) *v2.Pipeline {
	t.Helper()
	pad := n
	for i := 0; i < 8; i++ {
		args := &types.ListValue{Values: []*types.Value{strVal(strings.Repeat("x", pad))}}
		p := declarativePipeline(t, workflow(map[string]*types.Value{"args": {Kind: &types.Value_ListValue{ListValue: args}}}))
		got := len(p.Spec.Manifest.Content.Value)
		if got == n {
			return p
		}
		pad += n - got
	}
	t.Fatalf("no padding gives content of %d bytes", n)
	return nil
}

func TestContentLimit(t *testing.T) {
	d := loadDeclarative(t, true, "core-v1")
	ctx := context.Background()

	t.Run("create at the limit is accepted", func(t *testing.T) {
		hook, _ := newHook(t, d)
		p := paddedPipeline(t, maxAPIContentBytes)
		require.NoError(t, hook.BeforeCreate(ctx, &v2.CreatePipelineRequest{Pipeline: p}))
		assert.Len(t, p.Spec.Manifest.Content.Value, maxAPIContentBytes)
	})
	t.Run("create one byte over the limit is rejected", func(t *testing.T) {
		hook, _ := newHook(t, d)
		requireStatus(t, hook.BeforeCreate(ctx, dryRunCreate(paddedPipeline(t, maxAPIContentBytes+1))),
			codes.InvalidArgument, "manifest.content is 524289 bytes; the limit for the API is 524288 bytes")
	})
	t.Run("update one byte over the limit is rejected", func(t *testing.T) {
		hook, h := newHook(t, d)
		expectGet(h, storedPipeline(t, pinOf(release(t, d, "core-v1"))), nil)
		requireStatus(t, hook.BeforeUpdate(ctx, &v2.UpdatePipelineRequest{Pipeline: paddedPipeline(t, maxAPIContentBytes+1)}),
			codes.InvalidArgument, "manifest.content is 524289 bytes; the limit for the API is 524288 bytes")
	})
}

// The hook clips caller text in an error, so a long value does not give a
// long error or log line.
func TestErrorsClipCallerText(t *testing.T) {
	d := loadDeclarative(t, true, "core-v1")
	long := strings.Repeat("x", 200)
	clipped := strings.Repeat("x", 128) + "..."
	tests := []struct {
		name    string
		reqPin  *v2.InterpreterPin
		wantMsg string
	}{
		{"version", &v2.InterpreterPin{Version: long},
			`interpreter_pin.version "` + clipped + `" is not a known release; use one of [core-v1, core-v2]`},
		{"artifact", &v2.InterpreterPin{Artifact: long},
			`interpreter_pin.artifact "` + clipped + `" does not match the pin for interpreter core-v1; leave it unset`},
		{"digest", &v2.InterpreterPin{Digest: long},
			`interpreter_pin.digest "` + clipped + `" does not match the pin for interpreter core-v1; leave it unset`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			hook, h := newHook(t, d)
			p := declarativePipeline(t, workflow(nil))
			p.Spec.Manifest.InterpreterPin = tc.reqPin
			requireStatus(t, hook.BeforeCreate(context.Background(), &v2.CreatePipelineRequest{Pipeline: p}),
				codes.InvalidArgument, tc.wantMsg)

			expectGet(h, storedPipeline(t, pinOf(release(t, d, "core-v1"))), nil)
			p = declarativePipeline(t, workflow(nil))
			p.Spec.Manifest.InterpreterPin = tc.reqPin
			requireStatus(t, hook.BeforeUpdate(context.Background(), &v2.UpdatePipelineRequest{Pipeline: p}),
				codes.InvalidArgument, tc.wantMsg)
		})
	}
}

// expectGet makes the mock return stored, or err when it is set.
func expectGet(h *apimocks.MockHandler, stored *v2.Pipeline, err error) {
	h.EXPECT().Get(gomock.Any(), ns, name, gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _, _ string, _ *metav1.GetOptions, obj client.Object) error {
			if err != nil {
				return err
			}
			stored.DeepCopyInto(obj.(*v2.Pipeline))
			return nil
		})
}

func storedPipeline(t *testing.T, pin *v2.InterpreterPin) *v2.Pipeline {
	t.Helper()
	p := declarativePipeline(t, workflow(nil))
	p.Spec.Manifest.InterpreterPin = pin
	return p
}

func TestBeforeUpdate(t *testing.T) {
	ctx := context.Background()
	d := loadDeclarative(t, true, "core-v1")
	v1, v2Rel := release(t, d, "core-v1"), release(t, d, "core-v2")
	onlyV2 := map[string]*types.Value{"timeout_seconds": numVal(30)}
	// oldV1 is a stored pin whose artifact differs from the release file, so
	// a test can tell a pin kept verbatim from a pin stamped again.
	oldV1 := &v2.InterpreterPin{Version: "core-v1", Artifact: "s3://old/core-v1.tar.gz", Digest: v1.Digest}
	// driftV1 is a stored pin whose digest differs from the release file. An
	// operator changed core-v1.yaml after the pin was stamped.
	driftV1 := &v2.InterpreterPin{Version: "core-v1", Artifact: v1.Artifact, Digest: "sha256:stale"}
	const versions = "[core-v1, core-v2]"
	driftMsg := "the digest in the release file for interpreter core-v1 is " + v1.Digest +
		", but this pipeline was pinned with sha256:stale; set interpreter_pin.version to another release in " + versions

	tests := []struct {
		name     string
		d        *Declarative
		stored   *v2.Pipeline
		getErr   error
		extra    map[string]*types.Value
		reqPin   *v2.InterpreterPin
		reqTar   string
		dryRun   bool
		wantPin  *v2.InterpreterPin
		wantCode codes.Code
		wantMsg  string
		wantPlan *declarative.Release
	}{
		{
			name:    "pin absent keeps the stored pin verbatim",
			stored:  storedPipeline(t, oldV1),
			wantPin: oldV1,
		},
		{
			name:    "the artifact of a verbatim pin is accepted as uniflow_tar",
			stored:  storedPipeline(t, oldV1),
			reqTar:  oldV1.Artifact,
			wantPin: oldV1,
		},
		{
			name:     "same version rejects the artifact of another release",
			stored:   storedPipeline(t, pinOf(v1)),
			reqTar:   v2Rel.Artifact,
			wantCode: codes.InvalidArgument,
			wantMsg:  "uniflow_tar is server-owned for declarative pipelines; remove uniflow_tar from the request",
		},
		{
			name:     "a uniflow_tar from another source is rejected",
			stored:   storedPipeline(t, pinOf(v1)),
			reqTar:   "s3://bucket/user.tar.gz",
			wantCode: codes.InvalidArgument,
			wantMsg:  "uniflow_tar is server-owned for declarative pipelines; remove uniflow_tar from the request",
		},
		{
			name:    "upgrade replaces an echoed old artifact",
			stored:  storedPipeline(t, pinOf(v1)),
			reqPin:  &v2.InterpreterPin{Version: "core-v2"},
			reqTar:  v1.Artifact,
			wantPin: pinOf(v2Rel),
		},
		{
			name:     "upgrade rejects a uniflow_tar from another source",
			stored:   storedPipeline(t, pinOf(v1)),
			reqPin:   &v2.InterpreterPin{Version: "core-v2"},
			reqTar:   "s3://bucket/user.tar.gz",
			wantCode: codes.InvalidArgument,
			wantMsg:  "uniflow_tar is server-owned for declarative pipelines; remove uniflow_tar from the request",
		},
		{
			name:     "stored UNIFLOW pipeline rejects its echoed user tar",
			stored:   typedPipeline(v2.PIPELINE_MANIFEST_TYPE_UNIFLOW),
			reqTar:   "s3://bucket/user.tar.gz",
			wantCode: codes.InvalidArgument,
			wantMsg:  "uniflow_tar is server-owned for declarative pipelines; remove uniflow_tar from the request",
		},
		{
			name:    "same version keeps the stored pin verbatim",
			stored:  storedPipeline(t, oldV1),
			reqPin:  &v2.InterpreterPin{Version: "core-v1"},
			wantPin: oldV1,
		},
		{
			name:    "same version accepts an echo of a verbatim pin and its uniflow_tar",
			stored:  storedPipeline(t, oldV1),
			reqPin:  proto.Clone(oldV1).(*v2.InterpreterPin),
			reqTar:  oldV1.Artifact,
			wantPin: oldV1,
		},
		{
			name:     "same version rejects the release file artifact beside a verbatim pin",
			stored:   storedPipeline(t, oldV1),
			reqPin:   &v2.InterpreterPin{Version: "core-v1", Artifact: v1.Artifact},
			wantCode: codes.InvalidArgument,
			wantMsg:  `interpreter_pin.artifact "` + v1.Artifact + `" does not match the pin for interpreter core-v1; leave it unset`,
		},
		{
			name:     "same version fails when the release file changed after the pin",
			stored:   storedPipeline(t, driftV1),
			wantCode: codes.FailedPrecondition,
			wantMsg:  driftMsg,
		},
		{
			name:     "an echo of a drifted pin also fails",
			stored:   storedPipeline(t, driftV1),
			reqPin:   proto.Clone(driftV1).(*v2.InterpreterPin),
			wantCode: codes.FailedPrecondition,
			wantMsg:  driftMsg,
		},
		{
			name:    "an upgrade away from a drifted pin succeeds",
			stored:  storedPipeline(t, driftV1),
			reqPin:  &v2.InterpreterPin{Version: "core-v2", Artifact: driftV1.Artifact, Digest: driftV1.Digest},
			wantPin: pinOf(v2Rel),
		},
		{
			name:    "a stored pin with no digest is stamped again from its own release",
			stored:  storedPipeline(t, &v2.InterpreterPin{Version: "core-v1", Artifact: "s3://old/core-v1.tar.gz"}),
			reqPin:  &v2.InterpreterPin{Version: "core-v1", Artifact: "s3://old/core-v1.tar.gz"},
			reqTar:  "s3://old/core-v1.tar.gz",
			wantPin: pinOf(v1),
		},
		{
			name:    "a round trip of the stored pin is accepted",
			stored:  storedPipeline(t, pinOf(v1)),
			reqPin:  pinOf(v1),
			wantPin: pinOf(v1),
		},
		{
			name:     "stickiness: a new default does not move the pin",
			d:        loadDeclarative(t, true, "core-v2"),
			stored:   storedPipeline(t, pinOf(v1)),
			dryRun:   true,
			wantPin:  pinOf(v1),
			wantPlan: v1,
		},
		{
			name:     "upgrade stamps the new release and accepts its fields",
			stored:   storedPipeline(t, pinOf(v1)),
			extra:    onlyV2,
			reqPin:   &v2.InterpreterPin{Version: "core-v2"},
			dryRun:   true,
			wantPin:  pinOf(v2Rel),
			wantPlan: v2Rel,
		},
		{
			name:     "pinned-version validation rejects a field the pinned release does not declare",
			d:        loadDeclarative(t, true, "core-v2"),
			stored:   storedPipeline(t, pinOf(v1)),
			extra:    onlyV2,
			wantCode: codes.InvalidArgument,
		},
		{
			name:     "unknown version",
			stored:   storedPipeline(t, pinOf(v1)),
			reqPin:   &v2.InterpreterPin{Version: "core-v9"},
			wantCode: codes.InvalidArgument,
			wantMsg:  `interpreter_pin.version "core-v9" is not a known release; use one of ` + versions,
		},
		{
			name:     "artifact differs from the target release",
			stored:   storedPipeline(t, pinOf(v1)),
			reqPin:   &v2.InterpreterPin{Version: "core-v1", Artifact: "s3://evil/tar.gz"},
			wantCode: codes.InvalidArgument,
			wantMsg:  `interpreter_pin.artifact "s3://evil/tar.gz" does not match the pin for interpreter core-v1; leave it unset`,
		},
		{
			name:     "digest differs from the target release",
			stored:   storedPipeline(t, pinOf(v1)),
			reqPin:   &v2.InterpreterPin{Version: "core-v2", Digest: "sha256:bad"},
			wantCode: codes.InvalidArgument,
			wantMsg:  `interpreter_pin.digest "sha256:bad" does not match the pin for interpreter core-v2; leave it unset`,
		},
		{
			name:    "upgrade replaces an echoed stored pin artifact and digest",
			stored:  storedPipeline(t, pinOf(v1)),
			extra:   onlyV2,
			reqPin:  &v2.InterpreterPin{Version: "core-v2", Artifact: v1.Artifact, Digest: v1.Digest},
			reqTar:  v1.Artifact,
			wantPin: pinOf(v2Rel),
		},
		{
			name:    "upgrade compares the echo with the stored pin, not the release file",
			stored:  storedPipeline(t, oldV1),
			reqPin:  &v2.InterpreterPin{Version: "core-v2", Artifact: oldV1.Artifact, Digest: oldV1.Digest},
			reqTar:  oldV1.Artifact,
			wantPin: pinOf(v2Rel),
		},
		{
			name:     "upgrade rejects an artifact that is neither the stored nor the target",
			stored:   storedPipeline(t, pinOf(v1)),
			reqPin:   &v2.InterpreterPin{Version: "core-v2", Artifact: "s3://evil/tar.gz", Digest: v1.Digest},
			wantCode: codes.InvalidArgument,
			wantMsg:  `interpreter_pin.artifact "s3://evil/tar.gz" does not match the pin for interpreter core-v2; leave it unset`,
		},
		{
			name:     "upgrade rejects a foreign digest beside an echoed artifact",
			stored:   storedPipeline(t, pinOf(v1)),
			reqPin:   &v2.InterpreterPin{Version: "core-v2", Artifact: v1.Artifact, Digest: "sha256:bad"},
			wantCode: codes.InvalidArgument,
			wantMsg:  `interpreter_pin.digest "sha256:bad" does not match the pin for interpreter core-v2; leave it unset`,
		},
		{
			name:     "same version rejects the artifact of the previous stored release",
			stored:   storedPipeline(t, pinOf(v1)),
			reqPin:   &v2.InterpreterPin{Version: "core-v1", Artifact: v2Rel.Artifact},
			wantCode: codes.InvalidArgument,
			wantMsg:  `interpreter_pin.artifact "` + v2Rel.Artifact + `" does not match the pin for interpreter core-v1; leave it unset`,
		},
		{
			name:     "stored pin names a removed release",
			stored:   storedPipeline(t, &v2.InterpreterPin{Version: "core-v0"}),
			wantCode: codes.FailedPrecondition,
			wantMsg:  "interpreter core-v0 is not loaded on this apiserver; set interpreter_pin.version to one of " + versions,
		},
		{
			name:    "stored pin names a removed release and the request upgrades",
			stored:  storedPipeline(t, &v2.InterpreterPin{Version: "core-v0"}),
			reqPin:  &v2.InterpreterPin{Version: "core-v2"},
			wantPin: pinOf(v2Rel),
		},
		{
			name:    "stored pipeline not found follows the create rules",
			getErr:  apierrors.NewNotFound(schema.GroupResource{Resource: "pipelines"}, name),
			wantPin: pinOf(v1),
		},
		{
			name:    "stored pipeline not found accepts a loaded client version",
			getErr:  status.Error(codes.NotFound, "pipeline not found"),
			extra:   onlyV2,
			reqPin:  &v2.InterpreterPin{Version: "core-v2"},
			wantPin: pinOf(v2Rel),
		},
		{
			name:     "stored pipeline not found rejects a forged pin",
			getErr:   status.Error(codes.NotFound, "pipeline not found"),
			reqPin:   &v2.InterpreterPin{Version: "evil", Artifact: "s3://attacker/x.tar.gz"},
			wantCode: codes.InvalidArgument,
			wantMsg:  `interpreter_pin.version "evil" is not a known release; use one of ` + versions,
		},
		{
			name:     "stored pipeline not found rejects a forged uniflow_tar",
			getErr:   status.Error(codes.NotFound, "pipeline not found"),
			reqTar:   "s3://attacker/x.tar.gz",
			wantCode: codes.InvalidArgument,
			wantMsg:  "uniflow_tar is server-owned for declarative pipelines; remove uniflow_tar from the request",
		},
		{
			name:     "stored pipeline not found validates the content",
			getErr:   status.Error(codes.NotFound, "pipeline not found"),
			extra:    onlyV2,
			wantCode: codes.InvalidArgument,
		},
		{
			name:     "other Get error is returned",
			getErr:   status.Error(codes.Unavailable, "etcd down"),
			wantCode: codes.Unavailable,
			wantMsg:  "etcd down",
		},
		{
			name:    "stored UNIFLOW pipeline follows the create rules",
			d:       loadDeclarative(t, true, "core-v2"),
			stored:  typedPipeline(v2.PIPELINE_MANIFEST_TYPE_UNIFLOW),
			wantPin: pinOf(v2Rel),
		},
		{
			name:    "stored UNIFLOW pipeline accepts a loaded client version",
			stored:  typedPipeline(v2.PIPELINE_MANIFEST_TYPE_UNIFLOW),
			reqPin:  &v2.InterpreterPin{Version: "core-v2"},
			wantPin: pinOf(v2Rel),
		},
		{
			name:     "stored UNIFLOW pipeline rejects an unknown client version",
			stored:   typedPipeline(v2.PIPELINE_MANIFEST_TYPE_UNIFLOW),
			reqPin:   &v2.InterpreterPin{Version: "core-v9"},
			wantCode: codes.InvalidArgument,
			wantMsg:  `interpreter_pin.version "core-v9" is not a known release; use one of ` + versions,
		},
		{
			name:     "stored DECLARATIVE pipeline with no pin rejects a foreign artifact",
			stored:   storedPipeline(t, nil),
			reqPin:   &v2.InterpreterPin{Artifact: "s3://evil/tar.gz"},
			wantCode: codes.InvalidArgument,
			wantMsg:  `interpreter_pin.artifact "s3://evil/tar.gz" does not match the pin for interpreter core-v1; leave it unset`,
		},
		{
			name:    "stored DECLARATIVE pipeline with no pin follows the create rules",
			stored:  storedPipeline(t, nil),
			wantPin: pinOf(v1),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			hd := tc.d
			if hd == nil {
				hd = d
			}
			hook, h := newHook(t, hd)
			expectGet(h, tc.stored, tc.getErr)
			p := declarativePipeline(t, workflow(tc.extra))
			p.Spec.Manifest.InterpreterPin = tc.reqPin
			p.Spec.Manifest.UniflowTar = tc.reqTar
			p.Annotations = map[string]string{explainPlanAnnotation: `{"taskCount":99}`}
			req := &v2.UpdatePipelineRequest{Pipeline: p, UpdateOptions: &metav1.UpdateOptions{}}
			if tc.dryRun {
				req.UpdateOptions.DryRun = []string{metav1.DryRunAll}
			}

			err := hook.BeforeUpdate(ctx, req)

			if tc.wantCode != codes.OK {
				require.Error(t, err)
				st := status.Convert(err)
				assert.Equal(t, tc.wantCode, st.Code(), st.Message())
				if tc.wantMsg != "" {
					assert.Equal(t, tc.wantMsg, st.Message())
				}
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantPin, p.Spec.Manifest.InterpreterPin)
			assert.Equal(t, tc.wantPin.Artifact, p.Spec.Manifest.UniflowTar, "uniflow_tar follows the pin")
			if tc.wantPlan == nil {
				assert.Nil(t, explainPlan(t, p), "a real update removes the client annotation")
				return
			}
			assert.Equal(t, wantPlan(t, workflow(tc.extra), tc.wantPlan), explainPlan(t, p))
		})
	}
}

// A client that gets a pipeline and sends it back unchanged must succeed.
func TestBeforeUpdate_GetModifyUpdateRoundTrip(t *testing.T) {
	d := loadDeclarative(t, true, "core-v1")
	hook, h := newHook(t, d)
	stored := storedPipeline(t, pinOf(release(t, d, "core-v1")))
	stored.Spec.Manifest.UniflowTar = release(t, d, "core-v1").Artifact
	expectGet(h, stored, nil)
	req := &v2.UpdatePipelineRequest{Pipeline: stored.DeepCopy()}

	require.NoError(t, hook.BeforeUpdate(context.Background(), req))
	assert.Equal(t, stored, req.Pipeline)
}

// A client that gets a pipeline and changes only interpreter_pin.version
// must succeed. The hook stores the artifact, the digest, and uniflow_tar of
// the new release.
func TestBeforeUpdate_GetModifyUpdateUpgrade(t *testing.T) {
	d := loadDeclarative(t, true, "core-v1")
	hook, h := newHook(t, d)
	v1, v2Rel := release(t, d, "core-v1"), release(t, d, "core-v2")
	stored := storedPipeline(t, pinOf(v1))
	stored.Spec.Manifest.UniflowTar = v1.Artifact
	expectGet(h, stored, nil)
	p := stored.DeepCopy()
	p.Spec.Manifest.InterpreterPin.Version = "core-v2"

	require.NoError(t, hook.BeforeUpdate(context.Background(), &v2.UpdatePipelineRequest{Pipeline: p}))
	assert.Equal(t, pinOf(v2Rel), p.Spec.Manifest.InterpreterPin)
	assert.Equal(t, v2Rel.Artifact, p.Spec.Manifest.UniflowTar)
}

// One workflow sent with its map entries in two orders must be stored as the
// same bytes, so a re-apply does not change the spec.
func TestStoredContentIsCanonical(t *testing.T) {
	d := loadDeclarative(t, true, "core-v1")
	w := workflow(map[string]*types.Value{
		"args": {Kind: &types.Value_ListValue{ListValue: &types.ListValue{Values: []*types.Value{strVal("--x")}}}},
	})
	sorted, reversed := encodeInOrder(t, w, false), encodeInOrder(t, w, true)
	require.NotEqual(t, sorted, reversed, "the two inputs must differ")

	withContent := func(raw []byte) *v2.Pipeline {
		p := declarativePipeline(t, w)
		p.Spec.Manifest.Content.Value = raw
		return p
	}
	assertStored := func(t *testing.T, a, b *v2.Pipeline) {
		t.Helper()
		assert.Equal(t, declarative.TypeURL, a.Spec.Manifest.Content.TypeUrl)
		assert.Equal(t, a.Spec.Manifest.Content.Value, b.Spec.Manifest.Content.Value)
		got := &v2.DeclarativeWorkflow{}
		require.NoError(t, proto.Unmarshal(a.Spec.Manifest.Content.Value, got))
		assert.True(t, proto.Equal(w, got), "the stored bytes decode to the same workflow")
	}

	for _, dryRun := range []bool{false, true} {
		t.Run(fmt.Sprintf("create/dryRun=%t", dryRun), func(t *testing.T) {
			hook, _ := newHook(t, d)
			var got []*v2.Pipeline
			for _, raw := range [][]byte{sorted, reversed} {
				req := &v2.CreatePipelineRequest{Pipeline: withContent(raw), CreateOptions: &metav1.CreateOptions{}}
				if dryRun {
					req.CreateOptions.DryRun = []string{metav1.DryRunAll}
				}
				require.NoError(t, hook.BeforeCreate(context.Background(), req))
				got = append(got, req.Pipeline)
			}
			assertStored(t, got[0], got[1])
		})
	}
	t.Run("update", func(t *testing.T) {
		hook, h := newHook(t, d)
		stored := storedPipeline(t, pinOf(release(t, d, "core-v1")))
		var got []*v2.Pipeline
		for _, raw := range [][]byte{sorted, reversed} {
			expectGet(h, stored, nil)
			req := &v2.UpdatePipelineRequest{Pipeline: withContent(raw)}
			require.NoError(t, hook.BeforeUpdate(context.Background(), req))
			got = append(got, req.Pipeline)
		}
		assertStored(t, got[0], got[1])
	})
}

// A type change from DECLARATIVE to YAML or UNIFLOW is a plain update of a
// non-declarative pipeline. The hook does not read the stored pipeline.
func TestBeforeUpdate_TypeChangeFromDeclarative(t *testing.T) {
	for _, typ := range []v2.PipelineManifest_Type{v2.PIPELINE_MANIFEST_TYPE_YAML, v2.PIPELINE_MANIFEST_TYPE_UNIFLOW} {
		t.Run(typ.String(), func(t *testing.T) {
			// The mock has no expectations, so any Get fails the test.
			hook, _ := newHook(t, loadDeclarative(t, true, "core-v1"))

			p := typedPipeline(typ)
			want := p.DeepCopy()
			delete(want.Annotations, explainPlanAnnotation)
			require.NoError(t, hook.BeforeUpdate(context.Background(), &v2.UpdatePipelineRequest{Pipeline: p}))
			assert.Equal(t, want, p, "a request without a pin passes through")

			p = typedPipeline(typ)
			p.Spec.Manifest.InterpreterPin = &v2.InterpreterPin{Version: "core-v1", Artifact: "s3://ma-artifacts/interpreters/core-v1.tar.gz"}
			require.NoError(t, hook.BeforeUpdate(context.Background(), &v2.UpdatePipelineRequest{Pipeline: p}))
			assert.Equal(t, want, p, "the hook removes an echoed pin")
		})
	}
}
