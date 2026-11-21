package apihook

import (
	"context"
	"fmt"
	"testing"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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
			Type:           typ,
			UniflowTar:     "s3://bucket/user.tar.gz",
			InterpreterPin: &v2.InterpreterPin{Version: "core-v1"},
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

func TestFlagOff(t *testing.T) {
	hook, _ := newHook(t, loadDeclarative(t, false, ""))
	const msg = "declarative pipelines are not enabled on this apiserver"

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
			codes.InvalidArgument, "interpreter_pin is server-owned")
	})

	t.Run("client uniflow_tar is rejected", func(t *testing.T) {
		hook, _ := newHook(t, d)
		p := declarativePipeline(t, workflow(nil))
		p.Spec.Manifest.UniflowTar = "s3://bucket/user.tar.gz"
		requireStatus(t, hook.BeforeCreate(ctx, &v2.CreatePipelineRequest{Pipeline: p}),
			codes.InvalidArgument, "uniflow_tar is server-owned for declarative pipelines")
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
