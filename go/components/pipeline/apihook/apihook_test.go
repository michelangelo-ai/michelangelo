package apihook

import (
	"context"
	"fmt"
	"testing"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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

func declarativePipeline(t *testing.T, w *v2.DeclarativeWorkflow) *v2.Pipeline {
	t.Helper()
	content, err := types.MarshalAny(w)
	require.NoError(t, err)
	return &v2.Pipeline{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name},
		Spec: v2.PipelineSpec{Manifest: &v2.PipelineManifest{
			Type:    v2.PIPELINE_MANIFEST_TYPE_DECLARATIVE,
			Content: content,
		}},
	}
}

func typedPipeline(typ v2.PipelineManifest_Type) *v2.Pipeline {
	return &v2.Pipeline{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   ns,
			Name:        name,
			Annotations: map[string]string{explainPlanAnnotation: "client"},
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

				create := &v2.CreatePipelineRequest{Pipeline: mk(), CreateOptions: &metav1.CreateOptions{DryRun: []string{metav1.DryRunAll}}}
				want := create.Pipeline.DeepCopy()
				require.NoError(t, hook.BeforeCreate(context.Background(), create))
				assert.Equal(t, want, create.Pipeline)

				update := &v2.UpdatePipelineRequest{Pipeline: mk(), UpdateOptions: &metav1.UpdateOptions{DryRun: []string{metav1.DryRunAll}}}
				want = update.Pipeline.DeepCopy()
				require.NoError(t, hook.BeforeUpdate(context.Background(), update))
				assert.Equal(t, want, update.Pipeline)
			})
		}
	}
}

func TestNonDeclarativePinIsRejected(t *testing.T) {
	const msg = "interpreter_pin is only valid for declarative pipelines"
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
				requireStatus(t, hook.BeforeCreate(context.Background(), &v2.CreatePipelineRequest{Pipeline: pinned()}),
					codes.InvalidArgument, msg)
				requireStatus(t, hook.BeforeUpdate(context.Background(), &v2.UpdatePipelineRequest{Pipeline: pinned()}),
					codes.InvalidArgument, msg)
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

	t.Run("client pin is rejected", func(t *testing.T) {
		hook, _ := newHook(t, d)
		p := declarativePipeline(t, workflow(nil))
		p.Spec.Manifest.InterpreterPin = &v2.InterpreterPin{Version: "core-v1"}
		requireStatus(t, hook.BeforeCreate(ctx, &v2.CreatePipelineRequest{Pipeline: p}),
			codes.InvalidArgument, "interpreter_pin is server-owned; remove interpreter_pin from the request")
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

	tests := []struct {
		name      string
		d         *Declarative
		stored    *v2.Pipeline
		getErr    error
		extra     map[string]*types.Value
		reqPin    *v2.InterpreterPin
		reqTar    string
		dryRun    bool
		wantPin   *v2.InterpreterPin
		wantCode  codes.Code
		wantMsg   string
		wantPlan  *declarative.Release
		unchanged bool
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
			wantMsg:  `interpreter_pin.artifact "` + v1.Artifact + `" does not match release core-v1; leave it unset`,
		},
		{
			name:     "same version fails when the release file changed after the pin",
			stored:   storedPipeline(t, driftV1),
			wantCode: codes.FailedPrecondition,
			wantMsg:  "the release file for interpreter core-v1 changed after this pipeline was pinned; set interpreter_pin.version to another release in " + versions,
		},
		{
			name:     "an echo of a drifted pin also fails",
			stored:   storedPipeline(t, driftV1),
			reqPin:   proto.Clone(driftV1).(*v2.InterpreterPin),
			wantCode: codes.FailedPrecondition,
			wantMsg:  "the release file for interpreter core-v1 changed after this pipeline was pinned; set interpreter_pin.version to another release in " + versions,
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
			wantMsg:  `interpreter_pin.artifact "s3://evil/tar.gz" does not match release core-v1; leave it unset`,
		},
		{
			name:     "digest differs from the target release",
			stored:   storedPipeline(t, pinOf(v1)),
			reqPin:   &v2.InterpreterPin{Version: "core-v2", Digest: "sha256:bad"},
			wantCode: codes.InvalidArgument,
			wantMsg:  `interpreter_pin.digest "sha256:bad" does not match release core-v2; leave it unset`,
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
			wantMsg:  `interpreter_pin.artifact "s3://evil/tar.gz" does not match release core-v2; leave it unset`,
		},
		{
			name:     "upgrade rejects a foreign digest beside an echoed artifact",
			stored:   storedPipeline(t, pinOf(v1)),
			reqPin:   &v2.InterpreterPin{Version: "core-v2", Artifact: v1.Artifact, Digest: "sha256:bad"},
			wantCode: codes.InvalidArgument,
			wantMsg:  `interpreter_pin.digest "sha256:bad" does not match release core-v2; leave it unset`,
		},
		{
			name:     "same version rejects the artifact of the previous stored release",
			stored:   storedPipeline(t, pinOf(v1)),
			reqPin:   &v2.InterpreterPin{Version: "core-v1", Artifact: v2Rel.Artifact},
			wantCode: codes.InvalidArgument,
			wantMsg:  `interpreter_pin.artifact "` + v2Rel.Artifact + `" does not match release core-v1; leave it unset`,
		},
		{
			name:     "stored pin names a removed release",
			stored:   storedPipeline(t, &v2.InterpreterPin{Version: "core-v0"}),
			wantCode: codes.InvalidArgument,
			wantMsg:  "interpreter core-v0 is no longer available; set interpreter_pin.version to one of " + versions,
		},
		{
			name:    "stored pin names a removed release and the request upgrades",
			stored:  storedPipeline(t, &v2.InterpreterPin{Version: "core-v0"}),
			reqPin:  &v2.InterpreterPin{Version: "core-v2"},
			wantPin: pinOf(v2Rel),
		},
		{
			name:      "stored pipeline not found",
			getErr:    apierrors.NewNotFound(schema.GroupResource{Resource: "pipelines"}, name),
			unchanged: true,
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
			name:     "stored UNIFLOW pipeline rejects a client pin",
			stored:   typedPipeline(v2.PIPELINE_MANIFEST_TYPE_UNIFLOW),
			reqPin:   &v2.InterpreterPin{Version: "core-v1"},
			wantCode: codes.InvalidArgument,
			wantMsg:  "interpreter_pin is server-owned; remove interpreter_pin from the request",
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
			want := p.DeepCopy()
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
			if tc.unchanged {
				assert.Equal(t, want, p)
				return
			}
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

// A type change from DECLARATIVE to YAML or UNIFLOW is a plain update of a
// non-declarative pipeline. The hook does not read the stored pipeline.
func TestBeforeUpdate_TypeChangeFromDeclarative(t *testing.T) {
	for _, typ := range []v2.PipelineManifest_Type{v2.PIPELINE_MANIFEST_TYPE_YAML, v2.PIPELINE_MANIFEST_TYPE_UNIFLOW} {
		t.Run(typ.String(), func(t *testing.T) {
			// The mock has no expectations, so any Get fails the test.
			hook, _ := newHook(t, loadDeclarative(t, true, "core-v1"))

			p := typedPipeline(typ)
			want := p.DeepCopy()
			require.NoError(t, hook.BeforeUpdate(context.Background(), &v2.UpdatePipelineRequest{Pipeline: p}))
			assert.Equal(t, want, p, "a request without a pin passes through unchanged")

			p = typedPipeline(typ)
			p.Spec.Manifest.InterpreterPin = &v2.InterpreterPin{Version: "core-v1"}
			requireStatus(t, hook.BeforeUpdate(context.Background(), &v2.UpdatePipelineRequest{Pipeline: p}),
				codes.InvalidArgument, "interpreter_pin is only valid for declarative pipelines")
		})
	}
}
