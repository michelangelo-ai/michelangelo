package apihook

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	pbtypes "github.com/gogo/protobuf/types"

	"github.com/michelangelo-ai/michelangelo/go/api"
	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2 "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

const testNamespace = "test-namespace"

func setUpHook(t *testing.T, initialObjects ...client.Object) apiHook {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, v2.AddToScheme(scheme))
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(initialObjects...).Build()
	return apiHook{
		logger:     zaptest.NewLogger(t),
		apiHandler: apiHandler.NewFakeAPIHandler(k8sClient),
		scheme:     scheme,
	}
}

// snapshotRevision builds a Revision CR whose content snapshots the given
// Pipeline, mirroring what the pipeline controller's snapshotRevision produces.
func snapshotRevision(t *testing.T, name string, pipeline *v2.Pipeline) *v2.Revision {
	t.Helper()
	content, err := pbtypes.MarshalAny(pipeline)
	require.NoError(t, err)
	return &v2.Revision{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
		},
		Spec: v2.RevisionSpec{
			BaseType: &metav1.TypeMeta{
				Kind:       "Pipeline",
				APIVersion: "michelangelo.api/v2",
			},
			BaseResource: &apipb.ResourceIdentifier{
				Name:      pipeline.Name,
				Namespace: pipeline.Namespace,
			},
			Content: content,
		},
	}
}

func newCreateRequest(revisionName string) *v2.CreatePipelineRunRequest {
	return &v2.CreatePipelineRunRequest{
		PipelineRun: &v2.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-run",
				Namespace: testNamespace,
			},
			Spec: v2.PipelineRunSpec{
				Revision: &apipb.ResourceIdentifier{
					Name:      revisionName,
					Namespace: testNamespace,
				},
			},
		},
	}
}

// A run pinned to revision X must execute X's snapshotted spec even after the
// live Pipeline has been mutated to Y. The hook normalises the pinned run into
// a dev-run shape (inline PipelineSpec), which the execution path uses verbatim.
func TestBeforeCreate_ResolvesPinnedRevisionSnapshotNotLivePipeline(t *testing.T) {
	snapshotted := &v2.Pipeline{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-pipeline",
			Namespace:   testNamespace,
			Annotations: map[string]string{"michelangelo.ai/uniflow-image-id": "image-x"},
		},
		Spec: v2.PipelineSpec{
			Description: "revision X",
			Commit:      &v2.CommitInfo{GitRef: "commit-x", Branch: "main"},
		},
	}
	livePipeline := &v2.Pipeline{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pipeline",
			Namespace: testNamespace,
		},
		Spec: v2.PipelineSpec{
			Description: "mutated to Y",
			Commit:      &v2.CommitInfo{GitRef: "commit-y", Branch: "main"},
		},
	}
	hook := setUpHook(t, livePipeline, snapshotRevision(t, "pipeline-test-pipeline-commit-x", snapshotted))

	request := newCreateRequest("pipeline-test-pipeline-commit-x")
	require.NoError(t, hook.BeforeCreate(context.Background(), request))

	spec := request.PipelineRun.Spec
	require.NotNil(t, spec.PipelineSpec, "pinned run must be normalised into an inline PipelineSpec")
	assert.Equal(t, "revision X", spec.PipelineSpec.Description, "must use the snapshotted spec, not the live Pipeline")
	assert.Equal(t, "commit-x", spec.PipelineSpec.Commit.GitRef)

	// Spec.Pipeline is backfilled from the revision's base resource so ownerRef
	// stamping and notification inheritance keep working.
	require.NotNil(t, spec.Pipeline)
	assert.Equal(t, "test-pipeline", spec.Pipeline.Name)

	// Snapshot annotations are carried onto the PipelineRun, which is where the
	// dev-run path reads them from.
	assert.Equal(t, "image-x", request.PipelineRun.Annotations["michelangelo.ai/uniflow-image-id"])
}

func TestBeforeCreate_RunAnnotationsWinOverSnapshot(t *testing.T) {
	snapshotted := &v2.Pipeline{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-pipeline",
			Namespace:   testNamespace,
			Annotations: map[string]string{"michelangelo.ai/uniflow-image-id": "snapshot-image"},
		},
		Spec: v2.PipelineSpec{Description: "revision X"},
	}
	hook := setUpHook(t, snapshotRevision(t, "rev-x", snapshotted))

	request := newCreateRequest("rev-x")
	request.PipelineRun.Annotations = map[string]string{"michelangelo.ai/uniflow-image-id": "run-image"}
	require.NoError(t, hook.BeforeCreate(context.Background(), request))

	assert.Equal(t, "run-image", request.PipelineRun.Annotations["michelangelo.ai/uniflow-image-id"])
}

// Revisions snapshot several resource kinds; a non-Pipeline revision must be
// rejected loudly rather than falling back to the live Pipeline.
func TestBeforeCreate_RejectsNonPipelineRevision(t *testing.T) {
	rev := snapshotRevision(t, "rev-model", &v2.Pipeline{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pipeline", Namespace: testNamespace},
	})
	rev.Spec.BaseType.Kind = "Model"
	hook := setUpHook(t, rev)

	err := hook.BeforeCreate(context.Background(), newCreateRequest("rev-model"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), `snapshots kind "Model"`)
}

// An unresolvable revision must fail the create; silently falling back to the
// live Pipeline is exactly the reproducibility bug this hook fixes.
func TestBeforeCreate_RejectsMissingRevision(t *testing.T) {
	hook := setUpHook(t)

	err := hook.BeforeCreate(context.Background(), newCreateRequest("does-not-exist"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does-not-exist")
}

func TestBeforeCreate_RejectsRevisionWithoutContent(t *testing.T) {
	rev := snapshotRevision(t, "rev-empty", &v2.Pipeline{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pipeline", Namespace: testNamespace},
	})
	rev.Spec.Content = nil
	hook := setUpHook(t, rev)

	err := hook.BeforeCreate(context.Background(), newCreateRequest("rev-empty"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no snapshotted content")
}

func TestBeforeCreate_RejectsRevisionAndInlineSpecTogether(t *testing.T) {
	hook := setUpHook(t)

	request := newCreateRequest("rev-x")
	request.PipelineRun.Spec.PipelineSpec = &v2.PipelineSpec{Description: "inline"}

	err := hook.BeforeCreate(context.Background(), request)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "supply exactly one")
}

// A run without a pinned revision and without a pipeline reference is unchanged.
func TestBeforeCreate_NoRevisionNoPipelineIsUnchanged(t *testing.T) {
	hook := setUpHook(t)

	request := &v2.CreatePipelineRunRequest{
		PipelineRun: &v2.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{Name: "test-run", Namespace: testNamespace},
		},
	}
	require.NoError(t, hook.BeforeCreate(context.Background(), request))
	assert.Nil(t, request.PipelineRun.Spec.PipelineSpec)
	assert.Nil(t, request.PipelineRun.Spec.Revision)
}

func newPipelineRefRequest(pipelineName string) *v2.CreatePipelineRunRequest {
	return &v2.CreatePipelineRunRequest{
		PipelineRun: &v2.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-run",
				Namespace: testNamespace,
			},
			Spec: v2.PipelineRunSpec{
				Pipeline: &apipb.ResourceIdentifier{
					Name:      pipelineName,
					Namespace: testNamespace,
				},
			},
		},
	}
}

// When Spec.Revision is omitted, BeforeCreate pins Pipeline.Status.LatestRevision
// into an inline PipelineSpec (same shape as an explicit pin).
func TestBeforeCreate_ResolvesLatestRevisionWhenRevisionOmitted(t *testing.T) {
	snapshotted := &v2.Pipeline{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-pipeline",
			Namespace:   testNamespace,
			Annotations: map[string]string{"michelangelo.ai/uniflow-image-id": "image-latest"},
		},
		Spec: v2.PipelineSpec{
			Description: "latest revision snapshot",
			Commit:      &v2.CommitInfo{GitRef: "commit-latest", Branch: "master"},
		},
	}
	livePipeline := &v2.Pipeline{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pipeline",
			Namespace: testNamespace,
		},
		Spec: v2.PipelineSpec{
			Description: "live pipeline mutated",
			Commit:      &v2.CommitInfo{GitRef: "commit-live", Branch: "master"},
		},
		Status: v2.PipelineStatus{
			LatestRevision: &apipb.ResourceIdentifier{
				Name:      "pipeline-test-pipeline-master",
				Namespace: testNamespace,
			},
		},
	}
	hook := setUpHook(t, livePipeline, snapshotRevision(t, "pipeline-test-pipeline-master", snapshotted))

	request := newPipelineRefRequest("test-pipeline")
	require.NoError(t, hook.BeforeCreate(context.Background(), request))

	spec := request.PipelineRun.Spec
	require.NotNil(t, spec.Revision, "latestRevision must be recorded on the run")
	assert.Equal(t, "pipeline-test-pipeline-master", spec.Revision.Name)
	require.NotNil(t, spec.PipelineSpec)
	assert.Equal(t, "latest revision snapshot", spec.PipelineSpec.Description,
		"must use the latestRevision snapshot, not the live Pipeline")
	assert.Equal(t, "commit-latest", spec.PipelineSpec.Commit.GitRef)
	assert.Equal(t, "image-latest", request.PipelineRun.Annotations["michelangelo.ai/uniflow-image-id"])
}

// Missing status.latestRevision leaves the run on the live Pipeline path.
func TestBeforeCreate_NoLatestRevisionFallsBackToLivePipeline(t *testing.T) {
	livePipeline := &v2.Pipeline{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pipeline",
			Namespace: testNamespace,
		},
		Spec: v2.PipelineSpec{Description: "live only"},
	}
	hook := setUpHook(t, livePipeline)

	request := newPipelineRefRequest("test-pipeline")
	require.NoError(t, hook.BeforeCreate(context.Background(), request))

	assert.Nil(t, request.PipelineRun.Spec.Revision)
	assert.Nil(t, request.PipelineRun.Spec.PipelineSpec,
		"no inline spec — SourcePipelineActor will load the live Pipeline")
}

// status.latestRevision pointing at a missing Revision CR falls back to live.
func TestBeforeCreate_MissingLatestRevisionCRFallsBackToLivePipeline(t *testing.T) {
	livePipeline := &v2.Pipeline{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pipeline",
			Namespace: testNamespace,
		},
		Spec: v2.PipelineSpec{Description: "live only"},
		Status: v2.PipelineStatus{
			LatestRevision: &apipb.ResourceIdentifier{
				Name:      "pipeline-test-pipeline-gone",
				Namespace: testNamespace,
			},
		},
	}
	hook := setUpHook(t, livePipeline)

	request := newPipelineRefRequest("test-pipeline")
	require.NoError(t, hook.BeforeCreate(context.Background(), request))

	assert.Nil(t, request.PipelineRun.Spec.Revision,
		"tentative pin must be cleared after the Revision CR miss")
	assert.Nil(t, request.PipelineRun.Spec.PipelineSpec)
}

// An explicit Spec.Revision still hard-fails when missing; latestRevision
// fallback applies only when the caller omitted revision.
func TestBeforeCreate_ExplicitMissingRevisionStillRejected(t *testing.T) {
	livePipeline := &v2.Pipeline{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pipeline",
			Namespace: testNamespace,
		},
		Status: v2.PipelineStatus{
			LatestRevision: &apipb.ResourceIdentifier{
				Name:      "pipeline-test-pipeline-master",
				Namespace: testNamespace,
			},
		},
	}
	hook := setUpHook(t, livePipeline)

	request := newCreateRequest("does-not-exist")
	request.PipelineRun.Spec.Pipeline = &apipb.ResourceIdentifier{
		Name:      "test-pipeline",
		Namespace: testNamespace,
	}
	err := hook.BeforeCreate(context.Background(), request)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does-not-exist")
}

// Spec.Pipeline must identify the same Pipeline the Revision snapshotted.
// Otherwise we would execute B's spec while stamping ownerRef / notifications from A.
func TestBeforeCreate_RejectsRevisionOfDifferentPipeline(t *testing.T) {
	snapshottedB := &v2.Pipeline{
		ObjectMeta: metav1.ObjectMeta{Name: "pipeline-b", Namespace: testNamespace},
		Spec:       v2.PipelineSpec{Description: "snapshot B"},
	}
	liveA := &v2.Pipeline{
		ObjectMeta: metav1.ObjectMeta{Name: "pipeline-a", Namespace: testNamespace},
	}
	hook := setUpHook(t, liveA, snapshotRevision(t, "rev-b", snapshottedB))

	request := newCreateRequest("rev-b")
	request.PipelineRun.Spec.Pipeline = &apipb.ResourceIdentifier{
		Name:      "pipeline-a",
		Namespace: testNamespace,
	}
	err := hook.BeforeCreate(context.Background(), request)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pipeline-a")
	assert.Contains(t, err.Error(), "pipeline-b")
	assert.Nil(t, request.PipelineRun.Spec.PipelineSpec,
		"must not materialize a mismatched snapshot")
}

func TestBeforeCreate_RejectsRevisionOfDifferentPipelineNamespace(t *testing.T) {
	snapshotted := &v2.Pipeline{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pipeline", Namespace: "other-namespace"},
		Spec:       v2.PipelineSpec{Description: "other ns"},
	}
	live := &v2.Pipeline{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pipeline", Namespace: testNamespace},
	}
	hook := setUpHook(t, live, snapshotRevision(t, "rev-x", snapshotted))

	request := newCreateRequest("rev-x")
	request.PipelineRun.Spec.Pipeline = &apipb.ResourceIdentifier{
		Name:      "test-pipeline",
		Namespace: testNamespace,
	}
	err := hook.BeforeCreate(context.Background(), request)
	require.Error(t, err)
	assert.Contains(t, err.Error(), testNamespace)
	assert.Contains(t, err.Error(), "other-namespace")
	assert.Nil(t, request.PipelineRun.Spec.PipelineSpec)
}

func TestBeforeCreate_MatchingPipelineAndRevisionSucceeds(t *testing.T) {
	snapshotted := &v2.Pipeline{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pipeline", Namespace: testNamespace},
		Spec:       v2.PipelineSpec{Description: "revision X"},
	}
	live := &v2.Pipeline{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pipeline", Namespace: testNamespace},
	}
	hook := setUpHook(t, live, snapshotRevision(t, "rev-x", snapshotted))

	request := newCreateRequest("rev-x")
	request.PipelineRun.Spec.Pipeline = &apipb.ResourceIdentifier{
		Name:      "test-pipeline",
		Namespace: testNamespace,
	}
	require.NoError(t, hook.BeforeCreate(context.Background(), request))
	require.NotNil(t, request.PipelineRun.Spec.PipelineSpec)
	assert.Equal(t, "revision X", request.PipelineRun.Spec.PipelineSpec.Description)
}

// interceptGetHandler optionally fails Get for selected object kinds so tests
// can exercise the soft-fail path that the fake client cannot produce.
type interceptGetHandler struct {
	api.Handler
	failPipeline error
}

func (h interceptGetHandler) Get(ctx context.Context, namespace, name string, opts *metav1.GetOptions, obj client.Object) error {
	if h.failPipeline != nil {
		if _, ok := obj.(*v2.Pipeline); ok {
			return h.failPipeline
		}
	}
	return h.Handler.Get(ctx, namespace, name, opts, obj)
}

func TestBeforeCreate_CopiesNotificationsFromPipeline(t *testing.T) {
	live := &v2.Pipeline{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pipeline", Namespace: testNamespace},
		Spec: v2.PipelineSpec{
			Notifications: []*v2.Notification{{
				NotificationType: v2.NOTIFICATION_TYPE_EMAIL,
				Emails:           []string{"alerts@example.com"},
			}},
		},
	}
	hook := setUpHook(t, live)

	request := newPipelineRefRequest("test-pipeline")
	require.NoError(t, hook.BeforeCreate(context.Background(), request))
	require.Len(t, request.PipelineRun.Spec.Notifications, 1)
	assert.Equal(t, "alerts@example.com", request.PipelineRun.Spec.Notifications[0].Emails[0])
}

func TestBeforeCreate_InlinePipelineSpecSkipsLatestRevision(t *testing.T) {
	live := &v2.Pipeline{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pipeline", Namespace: testNamespace},
		Status: v2.PipelineStatus{
			LatestRevision: &apipb.ResourceIdentifier{
				Name:      "pipeline-test-pipeline-master",
				Namespace: testNamespace,
			},
		},
	}
	hook := setUpHook(t, live)

	request := newPipelineRefRequest("test-pipeline")
	request.PipelineRun.Spec.PipelineSpec = &v2.PipelineSpec{Description: "dev-run"}
	require.NoError(t, hook.BeforeCreate(context.Background(), request))

	assert.Nil(t, request.PipelineRun.Spec.Revision, "inline spec must not be overwritten by latestRevision")
	assert.Equal(t, "dev-run", request.PipelineRun.Spec.PipelineSpec.Description)
}

func TestBeforeCreate_LatestRevisionWrongKindRejected(t *testing.T) {
	rev := snapshotRevision(t, "rev-model", &v2.Pipeline{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pipeline", Namespace: testNamespace},
	})
	rev.Spec.BaseType.Kind = "Model"
	live := &v2.Pipeline{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pipeline", Namespace: testNamespace},
		Status: v2.PipelineStatus{
			LatestRevision: &apipb.ResourceIdentifier{
				Name:      "rev-model",
				Namespace: testNamespace,
			},
		},
	}
	hook := setUpHook(t, live, rev)

	err := hook.BeforeCreate(context.Background(), newPipelineRefRequest("test-pipeline"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), `snapshots kind "Model"`)
}

func TestBeforeCreate_RejectsRevisionWithCorruptContent(t *testing.T) {
	rev := snapshotRevision(t, "rev-bad", &v2.Pipeline{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pipeline", Namespace: testNamespace},
	})
	rev.Spec.Content = &pbtypes.Any{
		TypeUrl: "type.googleapis.com/michelangelo.api.v2.Pipeline",
		Value:   []byte("not-valid-protobuf"),
	}
	hook := setUpHook(t, rev)

	err := hook.BeforeCreate(context.Background(), newCreateRequest("rev-bad"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshal snapshotted pipeline")
}

func TestBeforeCreate_RevisionNamespaceFallsBackToPipelineRun(t *testing.T) {
	snapshotted := &v2.Pipeline{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pipeline", Namespace: testNamespace},
		Spec:       v2.PipelineSpec{Description: "revision X"},
	}
	hook := setUpHook(t, snapshotRevision(t, "rev-x", snapshotted))

	request := newCreateRequest("rev-x")
	request.PipelineRun.Spec.Revision.Namespace = ""
	require.NoError(t, hook.BeforeCreate(context.Background(), request))
	require.NotNil(t, request.PipelineRun.Spec.PipelineSpec)
	assert.Equal(t, "revision X", request.PipelineRun.Spec.PipelineSpec.Description)
}

func TestBeforeCreate_DefaultsEnvironmentLabelWhenAbsent(t *testing.T) {
	hook := setUpHook(t)
	hook.defaultEnv = "staging"

	request := &v2.CreatePipelineRunRequest{
		PipelineRun: &v2.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{Name: "test-run", Namespace: testNamespace},
		},
	}
	require.NoError(t, hook.BeforeCreate(context.Background(), request))
	assert.Equal(t, "staging", request.PipelineRun.Labels[api.EnvironmentLabel])
}

func TestBeforeCreate_DefaultsToUnspecifiedWhenUnconfigured(t *testing.T) {
	hook := setUpHook(t)

	request := &v2.CreatePipelineRunRequest{
		PipelineRun: &v2.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{Name: "test-run", Namespace: testNamespace},
		},
	}
	require.NoError(t, hook.BeforeCreate(context.Background(), request))
	assert.Equal(t, api.UnspecifiedEnvironment, request.PipelineRun.Labels[api.EnvironmentLabel])
}

func TestBeforeCreate_PreservesExplicitEnvironmentLabel(t *testing.T) {
	hook := setUpHook(t)
	hook.defaultEnv = "production"

	request := &v2.CreatePipelineRunRequest{
		PipelineRun: &v2.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-run",
				Namespace: testNamespace,
				Labels:    map[string]string{api.EnvironmentLabel: "staging"},
			},
		},
	}
	require.NoError(t, hook.BeforeCreate(context.Background(), request))
	assert.Equal(t, "staging", request.PipelineRun.Labels[api.EnvironmentLabel])
}

func TestRegisterPipelineRunAPIHook(t *testing.T) {
	RegisterPipelineRunAPIHook(zaptest.NewLogger(t), nil, runtime.NewScheme(), "production")
}

func TestBeforeCreate_PipelineGetTransientErrorIsSoft(t *testing.T) {
	live := &v2.Pipeline{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pipeline", Namespace: testNamespace},
		Status: v2.PipelineStatus{
			LatestRevision: &apipb.ResourceIdentifier{
				Name:      "pipeline-test-pipeline-master",
				Namespace: testNamespace,
			},
		},
	}
	hook := setUpHook(t, live)
	hook.apiHandler = interceptGetHandler{
		Handler:      hook.apiHandler,
		failPipeline: fmt.Errorf("apiserver unavailable"),
	}

	request := newPipelineRefRequest("test-pipeline")
	require.NoError(t, hook.BeforeCreate(context.Background(), request))
	assert.Nil(t, request.PipelineRun.Spec.Revision)
	assert.Nil(t, request.PipelineRun.Spec.PipelineSpec)
}

func TestBeforeCreate_StampsSourcePipelineTypeLabel(t *testing.T) {
	live := &v2.Pipeline{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pipeline", Namespace: testNamespace},
		Spec:       v2.PipelineSpec{Type: v2.PIPELINE_TYPE_TRAIN},
	}
	hook := setUpHook(t, live)

	request := newPipelineRefRequest("test-pipeline")
	require.NoError(t, hook.BeforeCreate(context.Background(), request))
	assert.Equal(t, "PIPELINE_TYPE_TRAIN",
		request.PipelineRun.GetLabels()[api.SourcePipelineTypeLabelName])
}
