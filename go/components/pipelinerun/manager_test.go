package pipelinerun

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

func newTestManager(t *testing.T, objects ...client.Object) (*managerImpl, client.Client) {
	scheme := runtime.NewScheme()
	require.NoError(t, v2pb.AddToScheme(scheme))
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objects...).
		WithStatusSubresource(objects...).
		Build()
	mgr := &managerImpl{k8sClient: k8sClient, logger: zaptest.NewLogger(t)}
	return mgr, k8sClient
}

// newErroringManager builds a manager whose underlying controller-runtime
// client returns the configured errors for List/Update. Use a nil error to
// fall through to the real fake-client behavior.
func newErroringManager(t *testing.T, listErr, updateErr error, objects ...client.Object) *managerImpl {
	return newErroringManagerFull(t, listErr, updateErr, nil, objects...)
}

// newErroringManagerFull additionally allows injecting a Delete error.
func newErroringManagerFull(t *testing.T, listErr, updateErr, deleteErr error, objects ...client.Object) *managerImpl {
	scheme := runtime.NewScheme()
	require.NoError(t, v2pb.AddToScheme(scheme))
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objects...).
		WithStatusSubresource(objects...).
		WithInterceptorFuncs(interceptor.Funcs{
			List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
				if listErr != nil {
					return listErr
				}
				return c.List(ctx, list, opts...)
			},
			Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
				if updateErr != nil {
					return updateErr
				}
				return c.Update(ctx, obj, opts...)
			},
			Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
				if deleteErr != nil {
					return deleteErr
				}
				return c.Delete(ctx, obj, opts...)
			},
		}).
		Build()
	return &managerImpl{k8sClient: k8sClient, logger: zaptest.NewLogger(t)}
}

func makePipelineRun(name, namespace, pipelineName, pipelineNamespace string, state v2pb.PipelineRunState) *v2pb.PipelineRun {
	return &v2pb.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: v2pb.PipelineRunSpec{
			Pipeline: &apipb.ResourceIdentifier{
				Name:      pipelineName,
				Namespace: pipelineNamespace,
			},
		},
		Status: v2pb.PipelineRunStatus{
			State: state,
		},
	}
}

func TestListPipelineRunsForPipeline(t *testing.T) {
	pr1 := makePipelineRun("pr-1", "ns", "my-pipeline", "ns", v2pb.PIPELINE_RUN_STATE_RUNNING)
	pr2 := makePipelineRun("pr-2", "ns", "my-pipeline", "ns", v2pb.PIPELINE_RUN_STATE_SUCCEEDED)
	prOther := makePipelineRun("pr-other", "ns", "other-pipeline", "ns", v2pb.PIPELINE_RUN_STATE_RUNNING)

	mgr, _ := newTestManager(t, pr1, pr2, prOther)
	result, err := mgr.ListPipelineRunsForPipeline(context.Background(), "ns", "my-pipeline")
	require.NoError(t, err)
	require.Len(t, result, 2)
	names := []string{result[0].Name, result[1].Name}
	require.ElementsMatch(t, []string{"pr-1", "pr-2"}, names)
}

func TestListPipelineRunsForPipeline_Empty(t *testing.T) {
	mgr, _ := newTestManager(t)
	result, err := mgr.ListPipelineRunsForPipeline(context.Background(), "ns", "my-pipeline")
	require.NoError(t, err)
	require.Empty(t, result)
}

func TestListPipelineRunsForPipeline_IgnoresOtherPipelines(t *testing.T) {
	mine := makePipelineRun("pr-mine", "ns", "my-pipeline", "ns", v2pb.PIPELINE_RUN_STATE_RUNNING)
	other := makePipelineRun("pr-other", "ns", "other-pipeline", "ns", v2pb.PIPELINE_RUN_STATE_RUNNING)
	crossNs := makePipelineRun("pr-cross-ns", "ns", "my-pipeline", "other-ns", v2pb.PIPELINE_RUN_STATE_RUNNING)

	mgr, _ := newTestManager(t, mine, other, crossNs)
	result, err := mgr.ListPipelineRunsForPipeline(context.Background(), "ns", "my-pipeline")
	require.NoError(t, err)
	require.Len(t, result, 1)
	require.Equal(t, "pr-mine", result[0].Name)
}

func TestListPipelineRunsForPipeline_IgnoresNilPipeline(t *testing.T) {
	valid := makePipelineRun("pr-valid", "ns", "my-pipeline", "ns", v2pb.PIPELINE_RUN_STATE_RUNNING)
	nilPipeline := &v2pb.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{Name: "pr-nil", Namespace: "ns"},
		Status:     v2pb.PipelineRunStatus{State: v2pb.PIPELINE_RUN_STATE_RUNNING},
	}

	mgr, _ := newTestManager(t, valid, nilPipeline)
	result, err := mgr.ListPipelineRunsForPipeline(context.Background(), "ns", "my-pipeline")
	require.NoError(t, err)
	require.Len(t, result, 1)
	require.Equal(t, "pr-valid", result[0].Name)
}

func TestListActivePipelineRunsForPipeline(t *testing.T) {
	pr1 := makePipelineRun("pr-1", "ns", "my-pipeline", "ns", v2pb.PIPELINE_RUN_STATE_RUNNING)
	pr2 := makePipelineRun("pr-2", "ns", "my-pipeline", "ns", v2pb.PIPELINE_RUN_STATE_SUCCEEDED)
	pr3 := makePipelineRun("pr-3", "ns", "my-pipeline", "ns", v2pb.PIPELINE_RUN_STATE_KILLED)
	pr4 := makePipelineRun("pr-4", "ns", "my-pipeline", "ns", v2pb.PIPELINE_RUN_STATE_FAILED)

	mgr, _ := newTestManager(t, pr1, pr2, pr3, pr4)
	result, err := mgr.ListActivePipelineRunsForPipeline(context.Background(), "ns", "my-pipeline")
	require.NoError(t, err)
	for _, pr := range result {
		require.False(t, IsTerminalState(pr.Status.State), "expected only active PRs, got %s in state %s", pr.Name, pr.Status.State)
	}
}

func TestKillPipelineRun_Running(t *testing.T) {
	pr := makePipelineRun("pr-1", "ns", "my-pipeline", "ns", v2pb.PIPELINE_RUN_STATE_RUNNING)
	mgr, k8sClient := newTestManager(t, pr)

	err := mgr.KillPipelineRun(context.Background(), pr)
	require.NoError(t, err)

	updated := &v2pb.PipelineRun{}
	require.NoError(t, k8sClient.Get(context.Background(), client.ObjectKeyFromObject(pr), updated))
	require.True(t, updated.Spec.Kill)
}

func TestKillPipelineRun_AlreadyTerminal(t *testing.T) {
	for _, state := range []v2pb.PipelineRunState{
		v2pb.PIPELINE_RUN_STATE_SUCCEEDED,
		v2pb.PIPELINE_RUN_STATE_FAILED,
		v2pb.PIPELINE_RUN_STATE_KILLED,
	} {
		t.Run(state.String(), func(t *testing.T) {
			pr := makePipelineRun("pr-1", "ns", "my-pipeline", "ns", state)
			mgr, _ := newTestManager(t, pr)

			err := mgr.KillPipelineRun(context.Background(), pr)
			require.NoError(t, err)
			require.False(t, pr.Spec.Kill)
		})
	}
}

func TestDeleteAllPipelineRuns(t *testing.T) {
	pr1 := makePipelineRun("pr-1", "ns", "my-pipeline", "ns", v2pb.PIPELINE_RUN_STATE_SUCCEEDED)
	pr2 := makePipelineRun("pr-2", "ns", "my-pipeline", "ns", v2pb.PIPELINE_RUN_STATE_KILLED)

	mgr, k8sClient := newTestManager(t, pr1, pr2)
	err := mgr.DeleteAllPipelineRuns(context.Background(), "ns", "my-pipeline")
	require.NoError(t, err)

	list := &v2pb.PipelineRunList{}
	require.NoError(t, k8sClient.List(context.Background(), list, &client.ListOptions{Namespace: "ns"}))
	require.Empty(t, list.Items)
}

func TestDeleteAllPipelineRuns_Empty(t *testing.T) {
	mgr, _ := newTestManager(t)
	err := mgr.DeleteAllPipelineRuns(context.Background(), "ns", "my-pipeline")
	require.NoError(t, err)
}

func TestDeleteAllPipelineRuns_ListError(t *testing.T) {
	listErr := errors.New("list boom")
	mgr := newErroringManagerFull(t, listErr, nil, nil)

	err := mgr.DeleteAllPipelineRuns(context.Background(), "ns", "my-pipeline")
	require.Error(t, err)
	require.ErrorIs(t, err, listErr)
}

func TestDeleteAllPipelineRuns_DeleteNotFoundContinues(t *testing.T) {
	pr1 := makePipelineRun("pr-1", "ns", "my-pipeline", "ns", v2pb.PIPELINE_RUN_STATE_SUCCEEDED)
	notFound := apiErrors.NewNotFound(schema.GroupResource{Resource: "pipelineruns"}, "pr-1")
	mgr := newErroringManagerFull(t, nil, nil, notFound, pr1)

	err := mgr.DeleteAllPipelineRuns(context.Background(), "ns", "my-pipeline")
	require.NoError(t, err)
}

func TestDeleteAllPipelineRuns_DeleteError(t *testing.T) {
	pr1 := makePipelineRun("pr-1", "ns", "my-pipeline", "ns", v2pb.PIPELINE_RUN_STATE_SUCCEEDED)
	deleteErr := errors.New("delete boom")
	mgr := newErroringManagerFull(t, nil, nil, deleteErr, pr1)

	err := mgr.DeleteAllPipelineRuns(context.Background(), "ns", "my-pipeline")
	require.Error(t, err)
	require.ErrorIs(t, err, deleteErr)
	require.Contains(t, err.Error(), "delete pipeline run ns/pr-1")
}

func TestIsTerminalState(t *testing.T) {
	tests := []struct {
		state    v2pb.PipelineRunState
		terminal bool
	}{
		{v2pb.PIPELINE_RUN_STATE_INVALID, false},
		{v2pb.PIPELINE_RUN_STATE_PENDING, false},
		{v2pb.PIPELINE_RUN_STATE_RUNNING, false},
		{v2pb.PIPELINE_RUN_STATE_SUCCEEDED, true},
		{v2pb.PIPELINE_RUN_STATE_FAILED, true},
		{v2pb.PIPELINE_RUN_STATE_KILLED, true},
		{v2pb.PIPELINE_RUN_STATE_SKIPPED, true},
	}
	for _, tc := range tests {
		t.Run(tc.state.String(), func(t *testing.T) {
			require.Equal(t, tc.terminal, IsTerminalState(tc.state))
		})
	}
}

func TestNewManager(t *testing.T) {
	// Exercise the public constructor to ensure it wires dependencies correctly.
	scheme := runtime.NewScheme()
	require.NoError(t, v2pb.AddToScheme(scheme))
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	mgr := NewManager(k8sClient, zaptest.NewLogger(t))
	require.NotNil(t, mgr)

	result, err := mgr.ListPipelineRunsForPipeline(context.Background(), "ns", "my-pipeline")
	require.NoError(t, err)
	require.Empty(t, result)
}

func TestListPipelineRunsForPipeline_ListError(t *testing.T) {
	listErr := errors.New("boom")
	mgr := newErroringManager(t, listErr, nil)

	result, err := mgr.ListPipelineRunsForPipeline(context.Background(), "ns", "my-pipeline")
	require.Error(t, err)
	require.Nil(t, result)
	require.ErrorIs(t, err, listErr)
	require.Contains(t, err.Error(), "list pipeline runs for pipeline ns/my-pipeline")
}

func TestListActivePipelineRunsForPipeline_PropagatesListError(t *testing.T) {
	listErr := errors.New("list boom")
	mgr := newErroringManager(t, listErr, nil)

	result, err := mgr.ListActivePipelineRunsForPipeline(context.Background(), "ns", "my-pipeline")
	require.Error(t, err)
	require.Nil(t, result)
	require.ErrorIs(t, err, listErr)
}

func TestKillPipelineRun_UpdateError(t *testing.T) {
	pr := makePipelineRun("pr-1", "ns", "my-pipeline", "ns", v2pb.PIPELINE_RUN_STATE_RUNNING)
	updateErr := errors.New("update boom")
	mgr := newErroringManager(t, nil, updateErr, pr)

	err := mgr.KillPipelineRun(context.Background(), pr)
	require.Error(t, err)
	require.ErrorIs(t, err, updateErr)
	require.Contains(t, err.Error(), "kill pipeline run ns/pr-1")
}
