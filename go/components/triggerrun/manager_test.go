package triggerrun

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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
// client returns the configured errors for List/Update. Used to exercise
// error branches in the manager. Use a nil error to fall through to the
// real fake-client behavior.
func newErroringManager(t *testing.T, listErr, updateErr error, objects ...client.Object) *managerImpl {
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
		}).
		Build()
	return &managerImpl{k8sClient: k8sClient, logger: zaptest.NewLogger(t)}
}

func makeTriggerRun(name, namespace, pipelineName, pipelineNamespace string, state v2pb.TriggerRunState) *v2pb.TriggerRun {
	return &v2pb.TriggerRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: v2pb.TriggerRunSpec{
			Pipeline: &apipb.ResourceIdentifier{
				Name:      pipelineName,
				Namespace: pipelineNamespace,
			},
		},
		Status: v2pb.TriggerRunStatus{
			State: state,
		},
	}
}

func TestListTriggerRunsForPipeline(t *testing.T) {
	tr1 := makeTriggerRun("tr-1", "ns", "my-pipeline", "ns", v2pb.TRIGGER_RUN_STATE_RUNNING)
	tr2 := makeTriggerRun("tr-2", "ns", "my-pipeline", "ns", v2pb.TRIGGER_RUN_STATE_SUCCEEDED)
	trOther := makeTriggerRun("tr-other", "ns", "other-pipeline", "ns", v2pb.TRIGGER_RUN_STATE_RUNNING)

	mgr, _ := newTestManager(t, tr1, tr2, trOther)
	result, err := mgr.ListTriggerRunsForPipeline(context.Background(), "ns", "my-pipeline")
	require.NoError(t, err)
	require.Len(t, result, 2)
	names := []string{result[0].Name, result[1].Name}
	require.ElementsMatch(t, []string{"tr-1", "tr-2"}, names)
}

func TestListTriggerRunsForPipeline_Empty(t *testing.T) {
	mgr, _ := newTestManager(t)
	result, err := mgr.ListTriggerRunsForPipeline(context.Background(), "ns", "my-pipeline")
	require.NoError(t, err)
	require.Empty(t, result)
}

func TestListTriggerRunsForPipeline_IgnoresOtherPipelines(t *testing.T) {
	mine := makeTriggerRun("tr-mine", "ns", "my-pipeline", "ns", v2pb.TRIGGER_RUN_STATE_RUNNING)
	other := makeTriggerRun("tr-other", "ns", "other-pipeline", "ns", v2pb.TRIGGER_RUN_STATE_RUNNING)
	crossNs := makeTriggerRun("tr-cross-ns", "ns", "my-pipeline", "other-ns", v2pb.TRIGGER_RUN_STATE_RUNNING)

	mgr, _ := newTestManager(t, mine, other, crossNs)
	result, err := mgr.ListTriggerRunsForPipeline(context.Background(), "ns", "my-pipeline")
	require.NoError(t, err)
	require.Len(t, result, 1)
	require.Equal(t, "tr-mine", result[0].Name)
}

func TestListTriggerRunsForPipeline_IgnoresNilPipeline(t *testing.T) {
	valid := makeTriggerRun("tr-valid", "ns", "my-pipeline", "ns", v2pb.TRIGGER_RUN_STATE_RUNNING)
	nilPipeline := &v2pb.TriggerRun{
		ObjectMeta: metav1.ObjectMeta{Name: "tr-nil", Namespace: "ns"},
		Status:     v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_RUNNING},
	}

	mgr, _ := newTestManager(t, valid, nilPipeline)
	result, err := mgr.ListTriggerRunsForPipeline(context.Background(), "ns", "my-pipeline")
	require.NoError(t, err)
	require.Len(t, result, 1)
	require.Equal(t, "tr-valid", result[0].Name)
}

func TestListActiveTriggerRunsForPipeline(t *testing.T) {
	tr1 := makeTriggerRun("tr-1", "ns", "my-pipeline", "ns", v2pb.TRIGGER_RUN_STATE_RUNNING)
	tr2 := makeTriggerRun("tr-2", "ns", "my-pipeline", "ns", v2pb.TRIGGER_RUN_STATE_SUCCEEDED)
	tr3 := makeTriggerRun("tr-3", "ns", "my-pipeline", "ns", v2pb.TRIGGER_RUN_STATE_KILLED)
	tr4 := makeTriggerRun("tr-4", "ns", "my-pipeline", "ns", v2pb.TRIGGER_RUN_STATE_FAILED)

	mgr, _ := newTestManager(t, tr1, tr2, tr3, tr4)
	result, err := mgr.ListActiveTriggerRunsForPipeline(context.Background(), "ns", "my-pipeline")
	require.NoError(t, err)
	// Only tr1 (RUNNING) is non-terminal
	for _, tr := range result {
		require.False(t, IsTerminateState(tr), "expected only active TRs, got %s in state %s", tr.Name, tr.Status.State)
	}
}

func TestKillTriggerRun_SetsBothKillAndAction(t *testing.T) {
	// KillTriggerRun must set both the new Spec.Action=KILL and the deprecated
	// Spec.Kill bool so the TR controller reacts regardless of state (the
	// INVALID-state branch of the TR controller only reads Spec.Kill).
	tr := makeTriggerRun("tr-1", "ns", "my-pipeline", "ns", v2pb.TRIGGER_RUN_STATE_RUNNING)
	mgr, k8sClient := newTestManager(t, tr)

	err := mgr.KillTriggerRun(context.Background(), tr)
	require.NoError(t, err)

	updated := &v2pb.TriggerRun{}
	require.NoError(t, k8sClient.Get(context.Background(), client.ObjectKeyFromObject(tr), updated))
	require.Equal(t, v2pb.TRIGGER_RUN_ACTION_KILL, updated.Spec.Action)
	require.True(t, updated.Spec.Kill)
}

func TestKillTriggerRun_AlreadyTerminal(t *testing.T) {
	for _, state := range []v2pb.TriggerRunState{
		v2pb.TRIGGER_RUN_STATE_SUCCEEDED,
		v2pb.TRIGGER_RUN_STATE_FAILED,
		v2pb.TRIGGER_RUN_STATE_KILLED,
	} {
		t.Run(state.String(), func(t *testing.T) {
			tr := makeTriggerRun("tr-1", "ns", "my-pipeline", "ns", state)
			mgr, _ := newTestManager(t, tr)

			err := mgr.KillTriggerRun(context.Background(), tr)
			require.NoError(t, err)
			require.NotEqual(t, v2pb.TRIGGER_RUN_ACTION_KILL, tr.Spec.Action)
			require.False(t, tr.Spec.Kill)
		})
	}
}

func TestIsTerminateState(t *testing.T) {
	tests := []struct {
		state    v2pb.TriggerRunState
		terminal bool
	}{
		{v2pb.TRIGGER_RUN_STATE_INVALID, false},
		{v2pb.TRIGGER_RUN_STATE_RUNNING, false},
		{v2pb.TRIGGER_RUN_STATE_PENDING_KILL, false},
		{v2pb.TRIGGER_RUN_STATE_PAUSED, false},
		{v2pb.TRIGGER_RUN_STATE_FAILED, true},
		{v2pb.TRIGGER_RUN_STATE_KILLED, true},
		{v2pb.TRIGGER_RUN_STATE_SUCCEEDED, true},
	}
	for _, tc := range tests {
		t.Run(tc.state.String(), func(t *testing.T) {
			tr := &v2pb.TriggerRun{Status: v2pb.TriggerRunStatus{State: tc.state}}
			require.Equal(t, tc.terminal, IsTerminateState(tr))
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

	// The returned Manager should be functional end-to-end.
	result, err := mgr.ListTriggerRunsForPipeline(context.Background(), "ns", "my-pipeline")
	require.NoError(t, err)
	require.Empty(t, result)
}

func TestListTriggerRunsForPipeline_ListError(t *testing.T) {
	listErr := errors.New("boom")
	mgr := newErroringManager(t, listErr, nil)

	result, err := mgr.ListTriggerRunsForPipeline(context.Background(), "ns", "my-pipeline")
	require.Error(t, err)
	require.Nil(t, result)
	require.ErrorIs(t, err, listErr)
	require.Contains(t, err.Error(), "list trigger runs for pipeline ns/my-pipeline")
}

func TestListActiveTriggerRunsForPipeline_PropagatesListError(t *testing.T) {
	listErr := errors.New("list boom")
	mgr := newErroringManager(t, listErr, nil)

	result, err := mgr.ListActiveTriggerRunsForPipeline(context.Background(), "ns", "my-pipeline")
	require.Error(t, err)
	require.Nil(t, result)
	require.ErrorIs(t, err, listErr)
}

func TestKillTriggerRun_UpdateError(t *testing.T) {
	tr := makeTriggerRun("tr-1", "ns", "my-pipeline", "ns", v2pb.TRIGGER_RUN_STATE_RUNNING)
	updateErr := errors.New("update boom")
	mgr := newErroringManager(t, nil, updateErr, tr)

	err := mgr.KillTriggerRun(context.Background(), tr)
	require.Error(t, err)
	require.ErrorIs(t, err, updateErr)
	require.Contains(t, err.Error(), "kill trigger run ns/tr-1")
}
