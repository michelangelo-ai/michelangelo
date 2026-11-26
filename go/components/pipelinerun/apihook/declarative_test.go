package apihook

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	pipelineapihook "github.com/michelangelo-ai/michelangelo/go/components/pipeline/apihook"
	"github.com/michelangelo-ai/michelangelo/go/components/pipeline/declarative"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2 "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

const (
	testRelease  = "core-v1"
	testArtifact = "s3://ma-artifacts/interpreters/core-v1.tar.gz"
	testDigest   = "sha256:1111111111111111111111111111111111111111111111111111111111111111"
)

func loadDeclarative(t *testing.T, enabled bool) *pipelineapihook.Declarative {
	t.Helper()
	dir := t.TempDir()
	release := "version: " + testRelease + "\n" +
		"artifact: " + testArtifact + "\n" +
		"digest: " + testDigest + "\n" +
		"task_types:\n" +
		"  core/python:\n" +
		"    config:\n" +
		"      entry_point: {type: string, required: true}\n" +
		"      epochs: {type: integer}\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, testRelease+".yaml"), []byte(release), 0o600))
	d, err := pipelineapihook.NewDeclarative(pipelineapihook.Config{Enabled: enabled, DefaultInterpreter: testRelease, ReleasesDir: dir})
	require.NoError(t, err)
	return d
}

func str(s string) *types.Value { return &types.Value{Kind: &types.Value_StringValue{StringValue: s}} }

func num(f float64) *types.Value { return &types.Value{Kind: &types.Value_NumberValue{NumberValue: f}} }

func structOf(fields map[string]*types.Value) *types.Value {
	return &types.Value{Kind: &types.Value_StructValue{StructValue: &types.Struct{Fields: fields}}}
}

func listOf(values ...*types.Value) *types.Value {
	return &types.Value{Kind: &types.Value_ListValue{ListValue: &types.ListValue{Values: values}}}
}

// runWorkflow has one parameter with a default and one task that uses it.
func runWorkflow() *v2.DeclarativeWorkflow {
	return &v2.DeclarativeWorkflow{
		SchemaVersion: declarative.SchemaVersion,
		Parameters: map[string]*v2.ParameterSpec{
			"epochs": {Type: v2.PARAMETER_TYPE_INTEGER, Default: num(3)},
		},
		Tasks: map[string]*v2.DeclarativeTask{
			"train": {
				TaskFunction: "core/python",
				Config: &types.Struct{Fields: map[string]*types.Value{
					"entry_point": str("train.py"),
					"epochs":      str("{{ params.epochs }}"),
				}},
			},
		},
	}
}

func pinnedPipeline(t *testing.T, namespace string, w *v2.DeclarativeWorkflow) *v2.Pipeline {
	t.Helper()
	raw, err := declarative.Encode(w)
	require.NoError(t, err)
	return &v2.Pipeline{
		ObjectMeta: metav1.ObjectMeta{Name: "decl", Namespace: namespace},
		Spec: v2.PipelineSpec{Manifest: &v2.PipelineManifest{
			Type:           v2.PIPELINE_MANIFEST_TYPE_DECLARATIVE,
			Content:        &types.Any{TypeUrl: declarative.TypeURL, Value: raw},
			InterpreterPin: &v2.InterpreterPin{Version: testRelease, Artifact: testArtifact, Digest: testDigest},
			UniflowTar:     testArtifact,
		}},
	}
}

// specSource builds the hook and the request for one way that a run names
// its pipeline spec.
type specSource struct {
	name  string
	setUp func(t *testing.T, p *v2.Pipeline, d *pipelineapihook.Declarative) (apiHook, *v2.CreatePipelineRunRequest)
}

func runRequest(spec v2.PipelineRunSpec) *v2.CreatePipelineRunRequest {
	return &v2.CreatePipelineRunRequest{PipelineRun: &v2.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{Name: "run", Namespace: testNamespace},
		Spec:       spec,
	}}
}

func hookWith(t *testing.T, d *pipelineapihook.Declarative, objects ...client.Object) apiHook {
	h := setUpHook(t, objects...)
	h.declarative = d
	return h
}

// withStalePin returns a copy of p pinned to a release that is not loaded.
func withStalePin(p *v2.Pipeline) *v2.Pipeline {
	stale := *p
	manifest := *p.Spec.Manifest
	manifest.InterpreterPin = &v2.InterpreterPin{Version: "core-v0", Artifact: "s3://old/core-v0.tar.gz", Digest: testDigest}
	manifest.UniflowTar = "s3://old/core-v0.tar.gz"
	stale.Spec.Manifest = &manifest
	return &stale
}

var specSources = []specSource{
	{
		name: "inline dev-run",
		setUp: func(t *testing.T, p *v2.Pipeline, d *pipelineapihook.Declarative) (apiHook, *v2.CreatePipelineRunRequest) {
			return hookWith(t, d), runRequest(v2.PipelineRunSpec{PipelineSpec: &p.Spec})
		},
	},
	{
		name: "explicit revision",
		setUp: func(t *testing.T, p *v2.Pipeline, d *pipelineapihook.Declarative) (apiHook, *v2.CreatePipelineRunRequest) {
			h := hookWith(t, d, snapshotRevision(t, "rev-1", p))
			return h, runRequest(v2.PipelineRunSpec{Revision: &apipb.ResourceIdentifier{Name: "rev-1", Namespace: testNamespace}})
		},
	},
	{
		name: "latest revision",
		setUp: func(t *testing.T, p *v2.Pipeline, d *pipelineapihook.Declarative) (apiHook, *v2.CreatePipelineRunRequest) {
			// The run takes the workflow from the snapshot and the pin from the
			// live Pipeline. The snapshot pin is stale, so a case passes only
			// when the hook checks the live pin.
			live := pinnedPipeline(t, p.Namespace, runWorkflow())
			live.Spec.Manifest.InterpreterPin = p.Spec.Manifest.InterpreterPin
			live.Status.LatestRevision = &apipb.ResourceIdentifier{Name: "rev-1", Namespace: testNamespace}
			h := hookWith(t, d, live, snapshotRevision(t, "rev-1", withStalePin(p)))
			return h, runRequest(v2.PipelineRunSpec{Pipeline: &apipb.ResourceIdentifier{Name: p.Name, Namespace: p.Namespace}})
		},
	},
	{
		name: "live pipeline",
		setUp: func(t *testing.T, p *v2.Pipeline, d *pipelineapihook.Declarative) (apiHook, *v2.CreatePipelineRunRequest) {
			return hookWith(t, d, p), runRequest(v2.PipelineRunSpec{Pipeline: &apipb.ResourceIdentifier{Name: p.Name, Namespace: p.Namespace}})
		},
	},
}

func TestBeforeCreate_DeclarativeRun(t *testing.T) {
	cronInput := &types.Struct{Fields: map[string]*types.Value{
		"args":    listOf(),
		"kw_args": structOf(map[string]*types.Value{}),
		"environ": structOf(map[string]*types.Value{"LAST_SCHEDULE_TIMESTAMP": str("1764115200")}),
	}}
	for _, tc := range []struct {
		name     string
		enabled  bool
		workflow func(w *v2.DeclarativeWorkflow)
		pipeline func(p *v2.Pipeline)
		input    *types.Struct
		code     codes.Code
		msg      string
		// revisionMsg replaces msg for a run of an explicit revision.
		revisionMsg string
	}{
		{name: "valid run", enabled: true},
		{name: "cron input", enabled: true, input: cronInput},
		{
			name: "empty input values", enabled: true,
			input: &types.Struct{Fields: map[string]*types.Value{
				"args":          {Kind: &types.Value_NullValue{}},
				"kw_args":       structOf(map[string]*types.Value{}),
				"parameter_map": structOf(map[string]*types.Value{}),
				"environ":       structOf(map[string]*types.Value{}),
			}},
		},
		{
			name: "flag off", enabled: false,
			code: codes.FailedPrecondition, msg: pipelineapihook.NotEnabledMessage,
		},
		{
			name: "no pin", enabled: true,
			pipeline: func(p *v2.Pipeline) { p.Spec.Manifest.InterpreterPin = nil },
			code:     codes.FailedPrecondition,
			msg:      "declarative pipeline has no interpreter pin; re-apply the pipeline through the API",
		},
		{
			name: "pin names no loaded release", enabled: true,
			pipeline: func(p *v2.Pipeline) { p.Spec.Manifest.InterpreterPin.Version = "core-v9" },
			code:     codes.FailedPrecondition,
			msg:      `interpreter "core-v9" is not loaded on this apiserver; loaded releases: [core-v1]`,
			revisionMsg: `revision "rev-1" pins interpreter "core-v9", which is not loaded on this apiserver; ` +
				"apply the pipeline from a new commit to create a new revision",
		},
		{
			name: "pin digest differs from the release", enabled: true,
			pipeline: func(p *v2.Pipeline) { p.Spec.Manifest.InterpreterPin.Digest = "sha256:22" },
			code:     codes.FailedPrecondition,
			msg:      `interpreter pin "core-v1" does not equal the loaded release; re-apply the pipeline through the API`,
			revisionMsg: `revision "rev-1" pins interpreter "core-v1", which does not equal the loaded release; ` +
				"apply the pipeline from a new commit to create a new revision",
		},
		{
			name: "pin artifact differs from the release", enabled: true,
			pipeline: func(p *v2.Pipeline) { p.Spec.Manifest.InterpreterPin.Artifact = "s3://other/core-v1.tar.gz" },
			code:     codes.FailedPrecondition,
			msg:      `interpreter pin "core-v1" does not equal the loaded release; re-apply the pipeline through the API`,
			revisionMsg: `revision "rev-1" pins interpreter "core-v1", which does not equal the loaded release; ` +
				"apply the pipeline from a new commit to create a new revision",
		},
		{
			name: "content does not decode", enabled: true,
			pipeline: func(p *v2.Pipeline) { p.Spec.Manifest.Content.TypeUrl = "type.googleapis.com/other" },
			code:     codes.InvalidArgument,
			msg:      `content type URL "type.googleapis.com/other" is not "type.googleapis.com/michelangelo.api.v2.DeclarativeWorkflow"`,
		},
		{
			name: "workflow does not conform to the release", enabled: true,
			workflow: func(w *v2.DeclarativeWorkflow) { w.Tasks["train"].TaskFunction = "core/ghost" },
			code:     codes.InvalidArgument,
			msg:      `task "train": task_function "core/ghost" is not a task type in core-v1`,
		},
		{
			name: "task_configs", enabled: true,
			input: &types.Struct{Fields: map[string]*types.Value{"task_configs": structOf(map[string]*types.Value{})}},
			code:  codes.InvalidArgument,
			msg:   "run input task_configs is not supported for declarative pipelines",
		},
		{
			name: "workflow_config", enabled: true,
			input: &types.Struct{Fields: map[string]*types.Value{"workflow_config": structOf(map[string]*types.Value{})}},
			code:  codes.InvalidArgument,
			msg:   "run input workflow_config is not supported for declarative pipelines",
		},
		{
			name: "args", enabled: true,
			input: &types.Struct{Fields: map[string]*types.Value{"args": listOf(str("x"))}},
			code:  codes.InvalidArgument,
			msg:   "run input args must be empty for declarative pipelines",
		},
		{
			name: "kw_args", enabled: true,
			input: &types.Struct{Fields: map[string]*types.Value{"kw_args": structOf(map[string]*types.Value{"k": str("v")})}},
			code:  codes.InvalidArgument,
			msg:   "run input kw_args must be empty for declarative pipelines",
		},
		{
			name: "parameter_map", enabled: true,
			input: &types.Struct{Fields: map[string]*types.Value{"parameter_map": structOf(map[string]*types.Value{"epochs": str("5")})}},
			code:  codes.InvalidArgument,
			msg:   "parameter overrides are not supported yet",
		},
		{
			name: "environ key", enabled: true,
			input: &types.Struct{Fields: map[string]*types.Value{"environ": structOf(map[string]*types.Value{
				"LAST_SCHEDULE_TIMESTAMP": str("1"), "UF_TASK_IMAGE": str("img"),
			})}},
			code: codes.InvalidArgument,
			msg:  `run input environ key "UF_TASK_IMAGE" is not supported for declarative pipelines; only LAST_SCHEDULE_TIMESTAMP is allowed`,
		},
		{
			name: "environ is not a struct", enabled: true,
			input: &types.Struct{Fields: map[string]*types.Value{"environ": str("x")}},
			code:  codes.InvalidArgument,
			msg:   "run input environ must be a struct",
		},
		{
			name: "required parameter", enabled: true,
			workflow: func(w *v2.DeclarativeWorkflow) {
				w.Parameters["lr"] = &v2.ParameterSpec{Type: v2.PARAMETER_TYPE_NUMBER, Required: true}
				w.Parameters["batch"] = &v2.ParameterSpec{Type: v2.PARAMETER_TYPE_INTEGER, Required: true}
			},
			code: codes.InvalidArgument,
			msg:  "parameter batch is required; parameter overrides are not supported yet",
		},
	} {
		for _, src := range specSources {
			t.Run(tc.name+"/"+src.name, func(t *testing.T) {
				w := runWorkflow()
				if tc.workflow != nil {
					tc.workflow(w)
				}
				p := pinnedPipeline(t, testNamespace, w)
				if tc.pipeline != nil {
					tc.pipeline(p)
				}
				h, req := src.setUp(t, p, loadDeclarative(t, tc.enabled))
				req.PipelineRun.Spec.Input = tc.input
				err := h.BeforeCreate(context.Background(), req)
				if tc.code == codes.OK {
					require.NoError(t, err)
					return
				}
				st, ok := status.FromError(err)
				require.True(t, ok, "not a status error: %v", err)
				assert.Equal(t, tc.code, st.Code())
				want := tc.msg
				if src.name == "explicit revision" && tc.revisionMsg != "" {
					want = tc.revisionMsg
				}
				assert.Equal(t, want, st.Message())
			})
		}
	}
}

// The SourcePipeline actor reads a pipeline by name from the run namespace,
// so the hook must check that pipeline, not the one in the named namespace.
func TestBeforeCreate_DeclarativeRunChecksPipelineInRunNamespace(t *testing.T) {
	other := &v2.Pipeline{
		ObjectMeta: metav1.ObjectMeta{Name: "decl", Namespace: "other-namespace"},
		Spec: v2.PipelineSpec{Manifest: &v2.PipelineManifest{
			Type:       v2.PIPELINE_MANIFEST_TYPE_YAML,
			UniflowTar: "s3://bucket/tar.gz",
		}},
	}
	local := pinnedPipeline(t, testNamespace, runWorkflow())
	h := hookWith(t, loadDeclarative(t, false), other, local)
	req := runRequest(v2.PipelineRunSpec{Pipeline: &apipb.ResourceIdentifier{Name: "decl", Namespace: "other-namespace"}})
	err := h.BeforeCreate(context.Background(), req)
	st, ok := status.FromError(err)
	require.True(t, ok, "not a status error: %v", err)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Equal(t, `declarative pipeline run must name a pipeline in its own namespace "test-namespace", not "other-namespace"`, st.Message())
}

// A run by name in another namespace would run a pipeline of that name in the
// run namespace, or fail, so it never runs the declarative pipeline it names.
func TestBeforeCreate_DeclarativeRunNamesPipelineInOtherNamespace(t *testing.T) {
	other := pinnedPipeline(t, "other-namespace", runWorkflow())
	h := hookWith(t, loadDeclarative(t, true), other)
	req := runRequest(v2.PipelineRunSpec{Pipeline: &apipb.ResourceIdentifier{Name: "decl", Namespace: "other-namespace"}})
	err := h.BeforeCreate(context.Background(), req)
	st, ok := status.FromError(err)
	require.True(t, ok, "not a status error: %v", err)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Equal(t, `declarative pipeline run must name a pipeline in its own namespace "test-namespace", not "other-namespace"`, st.Message())
}

func TestBeforeCreate_NonDeclarativeRunInOtherNamespaceIsUnchecked(t *testing.T) {
	other := &v2.Pipeline{
		ObjectMeta: metav1.ObjectMeta{Name: "yaml", Namespace: "other-namespace"},
		Spec: v2.PipelineSpec{Manifest: &v2.PipelineManifest{
			Type:       v2.PIPELINE_MANIFEST_TYPE_YAML,
			UniflowTar: "s3://bucket/tar.gz",
		}},
	}
	h := hookWith(t, loadDeclarative(t, false), other)
	req := runRequest(v2.PipelineRunSpec{Pipeline: &apipb.ResourceIdentifier{Name: "yaml", Namespace: "other-namespace"}})
	require.NoError(t, h.BeforeCreate(context.Background(), req))
}

func TestBeforeCreate_NonDeclarativeRunIsUnchecked(t *testing.T) {
	live := &v2.Pipeline{
		ObjectMeta: metav1.ObjectMeta{Name: "uniflow", Namespace: testNamespace},
		Spec: v2.PipelineSpec{Manifest: &v2.PipelineManifest{
			Type:       v2.PIPELINE_MANIFEST_TYPE_UNIFLOW,
			UniflowTar: "s3://bucket/tar.gz",
		}},
	}
	h := hookWith(t, loadDeclarative(t, false), live)
	req := runRequest(v2.PipelineRunSpec{Pipeline: &apipb.ResourceIdentifier{Name: "uniflow", Namespace: testNamespace}})
	input := &types.Struct{Fields: map[string]*types.Value{"environ": structOf(map[string]*types.Value{"UF_TASK_IMAGE": str("img")})}}
	req.PipelineRun.Spec.Input = input
	require.NoError(t, h.BeforeCreate(context.Background(), req))
	assert.Equal(t, input, req.PipelineRun.Spec.Input)
	assert.Nil(t, req.PipelineRun.Spec.PipelineSpec)
}

func TestBeforeCreate_LatestRevisionTakesTheLivePin(t *testing.T) {
	live := pinnedPipeline(t, testNamespace, runWorkflow())
	live.Status.LatestRevision = &apipb.ResourceIdentifier{Name: "rev-1", Namespace: testNamespace}
	h := hookWith(t, loadDeclarative(t, true), live, snapshotRevision(t, "rev-1", withStalePin(live)))
	req := runRequest(v2.PipelineRunSpec{Pipeline: &apipb.ResourceIdentifier{Name: live.Name, Namespace: testNamespace}})

	require.NoError(t, h.BeforeCreate(context.Background(), req))
	manifest := req.PipelineRun.Spec.PipelineSpec.Manifest
	assert.Equal(t, &v2.InterpreterPin{Version: testRelease, Artifact: testArtifact, Digest: testDigest}, manifest.InterpreterPin)
	assert.Equal(t, testArtifact, manifest.UniflowTar)
	assert.Equal(t, "rev-1", req.PipelineRun.Spec.Revision.Name)
}

func TestBeforeCreate_ExplicitRevisionKeepsTheSnapshotPin(t *testing.T) {
	live := pinnedPipeline(t, testNamespace, runWorkflow())
	h := hookWith(t, loadDeclarative(t, true), live, snapshotRevision(t, "rev-1", withStalePin(live)))
	req := runRequest(v2.PipelineRunSpec{Revision: &apipb.ResourceIdentifier{Name: "rev-1", Namespace: testNamespace}})

	err := h.BeforeCreate(context.Background(), req)
	st, ok := status.FromError(err)
	require.True(t, ok, "not a status error: %v", err)
	assert.Equal(t, codes.FailedPrecondition, st.Code())
	assert.Equal(t, `revision "rev-1" pins interpreter "core-v0", which is not loaded on this apiserver; `+
		"apply the pipeline from a new commit to create a new revision", st.Message())
}
