package actors

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/gogo/protobuf/jsonpb"
	pbtypes "github.com/gogo/protobuf/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	blobstoreMock "github.com/michelangelo-ai/michelangelo/go/base/blobstore/blobstore_mocks"
	clientInterfaces "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/interface"
	workflowclientMock "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/interface/interface_mock"
	"github.com/michelangelo-ai/michelangelo/go/components/pipeline/declarative"
	pipelinerunutils "github.com/michelangelo-ai/michelangelo/go/components/pipelinerun/actors/utils"
	triggerworkflow "github.com/michelangelo-ai/michelangelo/go/worker/workflows/trigger"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2 "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

const (
	declRunName   = "decl-run"
	declNamespace = "default"
	declArtifact  = "mock://interpreters/core-v0.1.0.tar.gz"
	declImage     = "registry/image:1"
)

type declFixture struct {
	tar     []byte
	digest  string
	content *pbtypes.Any
	args    string
}

func loadDeclFixture(t *testing.T) declFixture {
	t.Helper()
	tar, err := os.ReadFile("testdata/declarative/core-v0.1.0.tar.gz")
	require.NoError(t, err)
	releaseYAML, err := os.ReadFile("testdata/declarative/core-v0.1.0.yaml")
	require.NoError(t, err)
	rel, err := declarative.ParseRelease("core-v0.1.0", releaseYAML)
	require.NoError(t, err)
	specJSON, err := os.ReadFile("testdata/declarative/diamond.spec.json")
	require.NoError(t, err)
	w := &v2.DeclarativeWorkflow{}
	require.NoError(t, jsonpb.Unmarshal(bytes.NewReader(specJSON), w))
	raw, err := declarative.Encode(w)
	require.NoError(t, err)
	args, err := os.ReadFile("testdata/declarative/diamond.json")
	require.NoError(t, err)
	return declFixture{tar: tar, digest: rel.Digest, content: &pbtypes.Any{TypeUrl: declarative.TypeURL, Value: raw}, args: string(args)}
}

func (f declFixture) run() *v2.PipelineRun {
	return &v2.PipelineRun{
		ObjectMeta: v1.ObjectMeta{Name: declRunName, Namespace: declNamespace},
		Status: v2.PipelineRunStatus{
			Steps: []*v2.PipelineRunStepInfo{{
				Name:   pipelinerunutils.ImageBuildStepName,
				State:  v2.PIPELINE_RUN_STEP_STATE_SUCCEEDED,
				Output: &pbtypes.Struct{Fields: map[string]*pbtypes.Value{pipelinerunutils.ImageBuildOutputKey: {Kind: &pbtypes.Value_StringValue{StringValue: declImage}}}},
			}},
			SourcePipeline: &v2.SourcePipeline{Pipeline: &v2.Pipeline{
				ObjectMeta: v1.ObjectMeta{Name: "decl", Namespace: declNamespace},
				Spec: v2.PipelineSpec{Manifest: &v2.PipelineManifest{
					Type:           v2.PIPELINE_MANIFEST_TYPE_DECLARATIVE,
					Content:        f.content,
					InterpreterPin: &v2.InterpreterPin{Version: "core-v0.1.0", Artifact: declArtifact, Digest: f.digest},
					UniflowTar:     "mock://never-read.tar.gz",
				}},
			}},
		},
	}
}

func declActor(t *testing.T, ctrl *gomock.Controller, setUp func(*workflowclientMock.MockWorkflowClient, *blobstoreMock.MockBlobStoreClient), objects ...client.Object) *ExecuteWorkflowActor {
	t.Helper()
	workflowClient := workflowclientMock.NewMockWorkflowClient(ctrl)
	blobStoreClient := blobstoreMock.NewMockBlobStoreClient(ctrl)
	setUp(workflowClient, blobStoreClient)
	scheme := runtime.NewScheme()
	require.NoError(t, v2.AddToScheme(scheme))
	project := &v2.Project{ObjectMeta: v1.ObjectMeta{
		Name: declNamespace, Namespace: declNamespace,
		Annotations: map[string]string{"michelangelo/worker_queue": "test-task-list"},
	}}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(append(objects, project)...).Build()
	return setUpExecuteWorkflowActor(t, workflowClient, blobStoreClient, apiHandler.NewFakeAPIHandler(k8sClient))
}

func TestDeclarativeStartFailure(t *testing.T) {
	f := loadDeclFixture(t)
	for _, tc := range []struct {
		name  string
		edit  func(run *v2.PipelineRun)
		fetch func(b *blobstoreMock.MockBlobStoreClient)
		msg   string
		// log holds the fields of the Warn log beyond the run key and the message.
		log map[string]interface{}
	}{
		{
			name: "no pin",
			edit: func(run *v2.PipelineRun) { run.Status.SourcePipeline.Pipeline.Spec.Manifest.InterpreterPin = nil },
			msg:  "declarative pipeline run has no interpreter pin; re-apply the pipeline through the API",
		},
		{
			name: "run input sets args",
			edit: func(run *v2.PipelineRun) {
				run.Spec.Input = &pbtypes.Struct{Fields: map[string]*pbtypes.Value{
					"args": {Kind: &pbtypes.Value_ListValue{ListValue: &pbtypes.ListValue{Values: []*pbtypes.Value{{Kind: &pbtypes.Value_StringValue{StringValue: "x"}}}}}},
				}}
			},
			msg: "run input args must be empty for declarative pipelines",
		},
		{
			name: "run input sets another environ key",
			edit: func(run *v2.PipelineRun) {
				run.Spec.Input = &pbtypes.Struct{Fields: map[string]*pbtypes.Value{
					"environ": {Kind: &pbtypes.Value_StructValue{StructValue: &pbtypes.Struct{Fields: map[string]*pbtypes.Value{
						"UF_TASK_IMAGE": {Kind: &pbtypes.Value_StringValue{StringValue: "other"}},
					}}}},
				}}
			},
			msg: `run input environ key "UF_TASK_IMAGE" is not supported for declarative pipelines; only LAST_SCHEDULE_TIMESTAMP is allowed`,
		},
		{
			name: "parameter has no default",
			edit: func(run *v2.PipelineRun) {
				manifest := run.Status.SourcePipeline.Pipeline.Spec.Manifest
				w, err := declarative.Decode(manifest)
				require.NoError(t, err)
				w.Parameters["batch"] = &v2.ParameterSpec{Type: v2.PARAMETER_TYPE_INTEGER, Required: true}
				raw, err := declarative.Encode(w)
				require.NoError(t, err)
				manifest.Content = &pbtypes.Any{TypeUrl: declarative.TypeURL, Value: raw}
			},
			fetch: func(b *blobstoreMock.MockBlobStoreClient) {
				b.EXPECT().Get(gomock.Any(), declArtifact).Return(f.tar, nil)
			},
			msg: "parameter batch is required; parameter overrides are not supported yet",
		},
		{
			name: "fetch fails",
			fetch: func(b *blobstoreMock.MockBlobStoreClient) {
				b.EXPECT().Get(gomock.Any(), declArtifact).Return(nil, errors.New("access denied for key"))
			},
			msg: "cannot fetch interpreter core-v0.1.0",
			log: map[string]interface{}{"artifact": declArtifact, "error": "access denied for key"},
		},
		{
			name: "too large",
			fetch: func(b *blobstoreMock.MockBlobStoreClient) {
				b.EXPECT().Get(gomock.Any(), declArtifact).Return(make([]byte, 1<<20+1), nil)
			},
			msg: "interpreter core-v0.1.0 is larger than the 1 MiB limit",
			log: map[string]interface{}{"artifact": declArtifact, "bytes": int64(1<<20 + 1)},
		},
		{
			name: "digest mismatch",
			fetch: func(b *blobstoreMock.MockBlobStoreClient) {
				b.EXPECT().Get(gomock.Any(), declArtifact).Return([]byte("other"), nil)
			},
			msg: "interpreter core-v0.1.0 does not match the digest in its pin",
			log: map[string]interface{}{
				"artifact":      declArtifact,
				"pinDigest":     f.digest,
				"fetchedDigest": "sha256:d9298a10d1b0735837dc4bd85dac641b0f3cef27a47e5d53a54f2f3f5b2fcffa",
			},
		},
		{
			name: "decode fails",
			edit: func(run *v2.PipelineRun) {
				run.Status.SourcePipeline.Pipeline.Spec.Manifest.Content = &pbtypes.Any{TypeUrl: "type.googleapis.com/other"}
			},
			fetch: func(b *blobstoreMock.MockBlobStoreClient) {
				b.EXPECT().Get(gomock.Any(), declArtifact).Return(f.tar, nil)
			},
			msg: `cannot decode declarative spec: content type URL "type.googleapis.com/other" is not "type.googleapis.com/michelangelo.api.v2.DeclarativeWorkflow"`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			actor := declActor(t, ctrl, func(_ *workflowclientMock.MockWorkflowClient, b *blobstoreMock.MockBlobStoreClient) {
				if tc.fetch != nil {
					tc.fetch(b)
				}
			})
			core, logs := observer.New(zap.WarnLevel)
			actor.logger = zap.New(core)
			run := f.run()
			if tc.edit != nil {
				tc.edit(run)
			}
			condition, err := actor.Run(context.Background(), run, nil)
			require.NoError(t, err)
			assert.Equal(t, &apipb.Condition{Type: ExecuteWorkflowType, Status: apipb.CONDITION_STATUS_FALSE}, condition)
			wantLog := map[string]interface{}{"pipelineRun": declNamespace + "/" + declRunName, "message": tc.msg}
			for k, v := range tc.log {
				wantLog[k] = v
			}
			require.Equal(t, 1, logs.Len())
			entry := logs.All()[0]
			assert.Equal(t, zap.WarnLevel, entry.Level)
			assert.Equal(t, "cannot start declarative run", entry.Message)
			assert.Equal(t, wantLog, entry.ContextMap())
			step := pipelinerunutils.GetStep(run, pipelinerunutils.ExecuteWorkflowStepName)
			assert.Equal(t, v2.PIPELINE_RUN_STEP_STATE_FAILED, step.State)
			assert.Equal(t, tc.msg, step.Message)
			assert.NotNil(t, step.EndTime)
			assert.Empty(t, run.Status.WorkflowRunId)
		})
	}
}

// startCall records the inputs of one StartWorkflow call.
type startCall struct {
	tar    []byte
	file   string
	fn     string
	args   []interface{}
	kwArgs []interface{}
	env    map[string]interface{}
}

func expectStart(t *testing.T, w *workflowclientMock.MockWorkflowClient, got *startCall) {
	w.EXPECT().StartWorkflow(gomock.Any(), gomock.Any(), UniflowCadenceWorkflowName, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, opts clientInterfaces.StartWorkflowOptions, _ string, in ...interface{}) (*clientInterfaces.WorkflowExecution, error) {
			assert.Equal(t, declRunName, opts.ID)
			assert.Equal(t, "test-task-list", opts.TaskList)
			*got = startCall{
				tar: in[0].([]byte), file: in[1].(string), fn: in[2].(string),
				args: in[3].([]interface{}), kwArgs: in[4].([]interface{}), env: in[5].(map[string]interface{}),
			}
			return &clientInterfaces.WorkflowExecution{ID: declRunName, RunID: "run-1"}, nil
		})
}

func TestDeclarativeStart(t *testing.T) {
	f := loadDeclFixture(t)
	ctrl := gomock.NewController(t)
	var got startCall
	actor := declActor(t, ctrl, func(w *workflowclientMock.MockWorkflowClient, b *blobstoreMock.MockBlobStoreClient) {
		b.EXPECT().Get(gomock.Any(), declArtifact).Return(f.tar, nil)
		expectStart(t, w, &got)
	})
	run := f.run()
	run.Labels = map[string]string{triggerworkflow.PipelineRunExecutionTimestampLabel: "1764115200"}
	run.Spec.Input = &pbtypes.Struct{Fields: map[string]*pbtypes.Value{
		"environ": {Kind: &pbtypes.Value_StructValue{StructValue: &pbtypes.Struct{Fields: map[string]*pbtypes.Value{
			"LAST_SCHEDULE_TIMESTAMP": {Kind: &pbtypes.Value_StringValue{StringValue: "1764111600"}},
		}}}},
		"args":    {Kind: &pbtypes.Value_ListValue{ListValue: &pbtypes.ListValue{}}},
		"kw_args": {Kind: &pbtypes.Value_StructValue{StructValue: &pbtypes.Struct{}}},
	}}

	condition, err := actor.Run(context.Background(), run, nil)
	require.NoError(t, err)
	assert.Equal(t, &apipb.Condition{Type: ExecuteWorkflowType, Status: apipb.CONDITION_STATUS_UNKNOWN}, condition)
	assert.Equal(t, "run-1", run.Status.WorkflowRunId)
	assert.Equal(t, v2.PIPELINE_RUN_STEP_STATE_RUNNING, pipelinerunutils.GetStep(run, pipelinerunutils.ExecuteWorkflowStepName).State)

	assert.Equal(t, f.tar, got.tar)
	assert.Equal(t, "", got.file)
	assert.Equal(t, "", got.fn)
	args, err := json.Marshal(got.args)
	require.NoError(t, err)
	assert.JSONEq(t, f.args, string(args))
	assert.Equal(t, []interface{}{}, got.kwArgs)
	assert.Equal(t, map[string]interface{}{
		"UF_STORAGE_URL":          "s3://default",
		"MA_NAMESPACE":            declNamespace,
		"MA_PIPELINE_RUN_NAME":    declRunName,
		"STARLARK_TIME":           "unix:1764115200",
		"LAST_SCHEDULE_TIMESTAMP": "1764111600",
		"UF_TASK_IMAGE":           declImage,
		"CACHE_ENABLED":           "false",
		"CACHE_VERSION":           declRunName,
	}, got.env)
}

func TestDeclarativeStartResume(t *testing.T) {
	f := loadDeclFixture(t)
	previous := &v2.PipelineRun{
		ObjectMeta: v1.ObjectMeta{Name: "prev-run", Namespace: declNamespace},
		Status: v2.PipelineRunStatus{Steps: []*v2.PipelineRunStepInfo{{
			Name: pipelinerunutils.ExecuteWorkflowStepName,
			SubSteps: []*v2.PipelineRunStepInfo{
				{Name: "extract", DisplayName: "extract", State: v2.PIPELINE_RUN_STEP_STATE_SUCCEEDED, StepCachedOutputs: &v2.PipelineRunStepCachedOutputs{}},
				{Name: "train", DisplayName: "train", State: v2.PIPELINE_RUN_STEP_STATE_SUCCEEDED, StepCachedOutputs: &v2.PipelineRunStepCachedOutputs{}},
				{Name: "evaluate", DisplayName: "evaluate", State: v2.PIPELINE_RUN_STEP_STATE_FAILED},
			},
		}}},
	}
	ctrl := gomock.NewController(t)
	var got startCall
	actor := declActor(t, ctrl, func(w *workflowclientMock.MockWorkflowClient, b *blobstoreMock.MockBlobStoreClient) {
		b.EXPECT().Get(gomock.Any(), declArtifact).Return(f.tar, nil)
		expectStart(t, w, &got)
	}, previous)
	run := f.run()
	run.Spec.Resume = &v2.Resume{
		PipelineRun: &apipb.ResourceIdentifier{Name: "prev-run", Namespace: declNamespace},
		ResumeFrom:  []string{"train"},
	}

	_, err := actor.Run(context.Background(), run, nil)
	require.NoError(t, err)
	assert.Equal(t, map[string]interface{}{
		"UF_STORAGE_URL":            "s3://default",
		"MA_NAMESPACE":              declNamespace,
		"MA_PIPELINE_RUN_NAME":      declRunName,
		"UF_TASK_IMAGE":             declImage,
		"CACHE_ENABLED":             "true",
		"CACHE_VERSION":             declRunName,
		"CACHE_VERSION_GET_extract": "prev-run",
		"CACHE_VERSION_GET_train":   "prev-run",
		"CACHE_ENABLED_train":       "false",
	}, got.env)
}

func TestDeclarativeManualRetryAfterStartFailure(t *testing.T) {
	f := loadDeclFixture(t)
	ctrl := gomock.NewController(t)
	actor := declActor(t, ctrl, func(*workflowclientMock.MockWorkflowClient, *blobstoreMock.MockBlobStoreClient) {})
	run := f.run()
	run.Spec.RetryInfo = &v2.RetryInfo{ActivityId: "7", WorkflowId: declRunName, WorkflowRunId: "run-0"}
	run.Status.Steps = append(run.Status.Steps, &v2.PipelineRunStepInfo{
		Name:    pipelinerunutils.ExecuteWorkflowStepName,
		State:   v2.PIPELINE_RUN_STEP_STATE_FAILED,
		Message: "declarative pipeline run has no interpreter pin; re-apply the pipeline through the API",
		EndTime: pbtypes.TimestampNow(),
	})

	condition, err := actor.Run(context.Background(), run, nil)
	require.NoError(t, err)
	assert.Equal(t, &apipb.Condition{Type: ExecuteWorkflowType, Status: apipb.CONDITION_STATUS_FALSE}, condition)
	step := pipelinerunutils.GetStep(run, pipelinerunutils.ExecuteWorkflowStepName)
	assert.Equal(t, v2.PIPELINE_RUN_STEP_STATE_FAILED, step.State)
	assert.Equal(t, "declarative pipeline run has no interpreter pin; re-apply the pipeline through the API", step.Message)
	assert.Empty(t, run.Status.WorkflowRunId)
}

func TestGetWorkflowInputsRunIdentityWinsOverEnviron(t *testing.T) {
	run := &v2.PipelineRun{
		ObjectMeta: v1.ObjectMeta{Name: "yaml-run", Namespace: declNamespace},
		Status: v2.PipelineRunStatus{SourcePipeline: &v2.SourcePipeline{Pipeline: &v2.Pipeline{
			Spec: v2.PipelineSpec{Manifest: &v2.PipelineManifest{
				Content: createPipelineManifestWithEnviron(map[string]interface{}{
					"UF_STORAGE_URL":       "s3://from-manifest",
					"MA_NAMESPACE":         "from-manifest",
					"MA_PIPELINE_RUN_NAME": "from-manifest",
				}),
			}},
		}}},
	}
	_, _, envs, err := getWorkflowInputs(run)
	require.NoError(t, err)
	assert.Equal(t, map[string]interface{}{
		"UF_STORAGE_URL":       "s3://from-manifest",
		"MA_NAMESPACE":         declNamespace,
		"MA_PIPELINE_RUN_NAME": "yaml-run",
	}, envs)
}
