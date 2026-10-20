package triggerrun

import (
	"context"
	"testing"
	"time"

	"github.com/go-logr/logr"
	gogotypes "github.com/gogo/protobuf/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	apiutils "github.com/michelangelo-ai/michelangelo/go/api/utils"
	clientInterface "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/interface"
	interfaceMock "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/interface/interface_mock"
	"github.com/michelangelo-ai/michelangelo/go/cascadedelete"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

func childRun(name, trigger string, state v2pb.PipelineRunState, kill bool) *v2pb.PipelineRun {
	return &v2pb.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "ns",
			Labels:    map[string]string{TriggeredByLabel: trigger},
		},
		Spec:   v2pb.PipelineRunSpec{Kill: kill},
		Status: v2pb.PipelineRunStatus{State: state},
	}
}

func getChild(t *testing.T, c client.Client, name string) *v2pb.PipelineRun {
	t.Helper()
	got := &v2pb.PipelineRun{}
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: name, Namespace: "ns"}, got))
	return got
}

func getTrigger(t *testing.T, c client.Client, name string) *v2pb.TriggerRun {
	t.Helper()
	got := &v2pb.TriggerRun{}
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: name, Namespace: "ns"}, got))
	return got
}

// TestCancelActiveChildPipelineRuns: only the trigger's non-terminal children are
// flagged for kill; terminal children, already-killed children and other triggers'
// runs are left untouched, and every non-terminal child is counted as active.
func TestCancelActiveChildPipelineRuns(t *testing.T) {
	scheme := cascadeScheme(t)
	objs := []client.Object{
		childRun("pending", "tr", v2pb.PIPELINE_RUN_STATE_PENDING, false),
		childRun("running", "tr", v2pb.PIPELINE_RUN_STATE_RUNNING, false),
		childRun("invalid", "tr", v2pb.PIPELINE_RUN_STATE_INVALID, false),
		childRun("killing", "tr", v2pb.PIPELINE_RUN_STATE_RUNNING, true),
		childRun("succeeded", "tr", v2pb.PIPELINE_RUN_STATE_SUCCEEDED, false),
		childRun("other-trigger", "other", v2pb.PIPELINE_RUN_STATE_RUNNING, false),
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
	r := &Reconciler{Handler: apiHandler.NewFakeAPIHandler(c), scheme: scheme}

	trigger := &v2pb.TriggerRun{ObjectMeta: metav1.ObjectMeta{Name: "tr", Namespace: "ns"}}
	active, err := r.cancelActiveChildPipelineRuns(context.Background(), logr.Discard(), trigger)
	require.NoError(t, err)
	assert.Equal(t, 4, active, "pending, running, invalid and already-killing children are active")

	for _, name := range []string{"pending", "running", "invalid", "killing"} {
		assert.True(t, getChild(t, c, name).Spec.Kill, "%s should be flagged for kill", name)
	}
	assert.False(t, getChild(t, c, "succeeded").Spec.Kill, "terminal child is not re-killed")
	assert.False(t, getChild(t, c, "other-trigger").Spec.Kill, "another trigger's run is untouched")

	// Idempotent: a second pass issues no new kills and still counts the children.
	active, err = r.cancelActiveChildPipelineRuns(context.Background(), logr.Discard(), trigger)
	require.NoError(t, err)
	assert.Equal(t, 4, active)
}

// TestReconcile_KilledTriggerCancelsChildren: a trigger that reaches KILLED through
// the kill action cancels its in-flight children before being marked immutable.
func TestReconcile_KilledTriggerCancelsChildren(t *testing.T) {
	scheme := cascadeScheme(t)
	trigger := &v2pb.TriggerRun{
		ObjectMeta: metav1.ObjectMeta{Name: "tr", Namespace: "ns"},
		Status:     v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_KILLED},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(trigger, childRun("running", "tr", v2pb.PIPELINE_RUN_STATE_RUNNING, false)).
		WithStatusSubresource(trigger).Build()
	r := &Reconciler{Handler: apiHandler.NewFakeAPIHandler(c), scheme: scheme, log: logr.Discard()}

	_, err := r.reconcile(context.Background(), logr.Discard(), getTrigger(t, c, "tr"))
	require.NoError(t, err)

	assert.True(t, getChild(t, c, "running").Spec.Kill, "in-flight child is cancelled")
	got := getTrigger(t, c, "tr")
	assert.True(t, apiutils.IsImmutable(got), "trigger is marked immutable after the cascade")
	assert.Equal(t, v2pb.TRIGGER_RUN_STATE_KILLED, got.Status.State, "trigger stays visible as KILLED")
}

// TestReconcile_KillWaitsInPendingKill: a kill stops the workflow and holds the
// trigger in PENDING_KILL, cancelling its children, until they are all terminal;
// only then does it become KILLED and immutable.
func TestReconcile_KillWaitsInPendingKill(t *testing.T) {
	for _, state := range []v2pb.TriggerRunState{v2pb.TRIGGER_RUN_STATE_RUNNING, v2pb.TRIGGER_RUN_STATE_PAUSED} {
		t.Run(state.String(), func(t *testing.T) {
			scheme := cascadeScheme(t)
			trigger := &v2pb.TriggerRun{
				ObjectMeta: metav1.ObjectMeta{Name: "tr", Namespace: "ns"},
				Spec:       v2pb.TriggerRunSpec{Action: v2pb.TRIGGER_RUN_ACTION_KILL},
				Status:     v2pb.TriggerRunStatus{State: state},
			}
			child := childRun("running", "tr", v2pb.PIPELINE_RUN_STATE_RUNNING, false)
			c := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(trigger, child).WithStatusSubresource(trigger, child).Build()

			mockRunner := &MockRunner{}
			mockRunner.On("Update", mock.Anything, mock.Anything, mock.Anything).
				Return(v2pb.TriggerRunStatus{State: state}, false, nil).Once()
			mockRunner.On("Kill", mock.Anything, mock.Anything).
				Return(v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_KILLED}, nil).Once()
			r := &Reconciler{Handler: apiHandler.NewFakeAPIHandler(c), scheme: scheme, log: logr.Discard(), cronTrigger: mockRunner}

			// Workflow killed: the trigger waits in PENDING_KILL.
			_, err := r.reconcile(context.Background(), logr.Discard(), getTrigger(t, c, "tr"))
			require.NoError(t, err)
			got := getTrigger(t, c, "tr")
			assert.Equal(t, v2pb.TRIGGER_RUN_STATE_PENDING_KILL, got.Status.State)
			assert.Equal(t, v2pb.TRIGGER_RUN_ACTION_NO_ACTION, got.Spec.Action, "kill action consumed")

			// Child flagged for kill but still running: stay PENDING_KILL and requeue.
			res, err := r.reconcile(context.Background(), logr.Discard(), getTrigger(t, c, "tr"))
			require.NoError(t, err)
			assert.Equal(t, drainRequeueInterval, res.RequeueAfter)
			assert.True(t, getChild(t, c, "running").Spec.Kill)
			assert.Equal(t, v2pb.TRIGGER_RUN_STATE_PENDING_KILL, getTrigger(t, c, "tr").Status.State)

			// Child stopped: the trigger becomes KILLED, then immutable.
			killed := getChild(t, c, "running")
			killed.Status.State = v2pb.PIPELINE_RUN_STATE_KILLED
			require.NoError(t, c.Status().Update(context.Background(), killed))
			_, err = r.reconcile(context.Background(), logr.Discard(), getTrigger(t, c, "tr"))
			require.NoError(t, err)
			assert.Equal(t, v2pb.TRIGGER_RUN_STATE_KILLED, getTrigger(t, c, "tr").Status.State)
			_, err = r.reconcile(context.Background(), logr.Discard(), getTrigger(t, c, "tr"))
			require.NoError(t, err)
			assert.True(t, apiutils.IsImmutable(getTrigger(t, c, "tr")))
			mockRunner.AssertExpectations(t)
		})
	}
}

// TestReconcile_PendingKillRetriesFailedCancel: a failed child cancel is returned for
// retry and leaves the trigger in PENDING_KILL rather than reporting KILLED.
func TestReconcile_PendingKillRetriesFailedCancel(t *testing.T) {
	scheme := cascadeScheme(t)
	trigger := &v2pb.TriggerRun{
		ObjectMeta: metav1.ObjectMeta{Name: "tr", Namespace: "ns"},
		Status:     v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_PENDING_KILL},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(trigger).WithStatusSubresource(trigger).
		WithInterceptorFuncs(interceptor.Funcs{
			List: func(ctx context.Context, cl client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
				if _, ok := list.(*v2pb.PipelineRunList); ok {
					return assert.AnError
				}
				return cl.List(ctx, list, opts...)
			},
		}).Build()
	r := &Reconciler{Handler: apiHandler.NewFakeAPIHandler(c), scheme: scheme, log: logr.Discard(), cronTrigger: &MockRunner{}}

	_, err := r.reconcile(context.Background(), logr.Discard(), getTrigger(t, c, "tr"))
	require.Error(t, err)
	got := getTrigger(t, c, "tr")
	assert.Equal(t, v2pb.TRIGGER_RUN_STATE_PENDING_KILL, got.Status.State)
	assert.False(t, apiutils.IsImmutable(got))
}

// TestReconcile_DeletePendingKillTrigger: deleting a trigger in PENDING_KILL does not
// re-kill its stopped workflow; it moves to KILLED and the drain waits on children.
func TestReconcile_DeletePendingKillTrigger(t *testing.T) {
	scheme := cascadeScheme(t)
	now := metav1.Now()
	trigger := &v2pb.TriggerRun{
		ObjectMeta: metav1.ObjectMeta{
			Name: "tr", Namespace: "ns", UID: types.UID("t1"),
			Finalizers: []string{drainFinalizer}, DeletionTimestamp: &now,
		},
		Status: v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_PENDING_KILL},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(trigger, childRun("running", "tr", v2pb.PIPELINE_RUN_STATE_RUNNING, true)).
		WithStatusSubresource(trigger).Build()

	mockRunner := &MockRunner{}
	r := &Reconciler{Handler: apiHandler.NewFakeAPIHandler(c), scheme: scheme, log: logr.Discard(), cronTrigger: mockRunner}

	_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "tr", Namespace: "ns"}})
	require.NoError(t, err)
	got := getTrigger(t, c, "tr")
	assert.Equal(t, v2pb.TRIGGER_RUN_STATE_KILLED, got.Status.State)
	assert.True(t, ctrlutil.ContainsFinalizer(got, drainFinalizer), "finalizer kept while a child is still active")
	mockRunner.AssertNotCalled(t, "Kill", mock.Anything, mock.Anything)
}

// TestReconcile_FailedTriggerLeavesChildren: only KILLED cascades; a trigger that
// finished on its own does not cancel children still running.
func TestReconcile_FailedTriggerLeavesChildren(t *testing.T) {
	scheme := cascadeScheme(t)
	trigger := &v2pb.TriggerRun{
		ObjectMeta: metav1.ObjectMeta{Name: "tr", Namespace: "ns"},
		Status:     v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_FAILED},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(trigger, childRun("running", "tr", v2pb.PIPELINE_RUN_STATE_RUNNING, false)).
		WithStatusSubresource(trigger).Build()
	r := &Reconciler{Handler: apiHandler.NewFakeAPIHandler(c), scheme: scheme, log: logr.Discard()}

	_, err := r.reconcile(context.Background(), logr.Discard(), getTrigger(t, c, "tr"))
	require.NoError(t, err)
	assert.False(t, getChild(t, c, "running").Spec.Kill)
}

// TestReconcile_DeleteWaitsForChildren: deleting a running trigger kills it and its
// children, and keeps the drain finalizer until every child is terminal.
func TestReconcile_DeleteWaitsForChildren(t *testing.T) {
	scheme := cascadeScheme(t)
	now := metav1.Now()
	trigger := &v2pb.TriggerRun{
		ObjectMeta: metav1.ObjectMeta{
			Name: "tr", Namespace: "ns", UID: types.UID("t1"),
			Finalizers: []string{drainFinalizer}, DeletionTimestamp: &now,
		},
		Status: v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_RUNNING},
	}
	child := childRun("running", "tr", v2pb.PIPELINE_RUN_STATE_RUNNING, false)
	c := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(trigger, child).WithStatusSubresource(trigger, child).Build()

	mockRunner := &MockRunner{}
	mockRunner.On("Kill", mock.Anything, mock.Anything).
		Return(v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_KILLED}, nil).
		Once()
	r := &Reconciler{Handler: apiHandler.NewFakeAPIHandler(c), scheme: scheme, log: logr.Discard(), cronTrigger: mockRunner}
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "tr", Namespace: "ns"}}

	// First loop: trigger killed, child flagged, drain still pending.
	_, err := r.Reconcile(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, v2pb.TRIGGER_RUN_STATE_KILLED, getTrigger(t, c, "tr").Status.State)
	assert.True(t, getChild(t, c, "running").Spec.Kill)
	assert.True(t, ctrlutil.ContainsFinalizer(getTrigger(t, c, "tr"), drainFinalizer))

	// Child still running: trigger is terminal but the finalizer must stay.
	_, err = r.Reconcile(context.Background(), req)
	require.NoError(t, err)
	assert.True(t, ctrlutil.ContainsFinalizer(getTrigger(t, c, "tr"), drainFinalizer),
		"finalizer kept while a child is still active")

	// Child reaches KILLED: the drain completes.
	killed := getChild(t, c, "running")
	killed.Status.State = v2pb.PIPELINE_RUN_STATE_KILLED
	require.NoError(t, c.Status().Update(context.Background(), killed))
	_, err = r.Reconcile(context.Background(), req)
	require.NoError(t, err)

	got := &v2pb.TriggerRun{}
	err = c.Get(context.Background(), types.NamespacedName{Name: "tr", Namespace: "ns"}, got)
	if err == nil {
		assert.False(t, ctrlutil.ContainsFinalizer(got, drainFinalizer), "finalizer removed once children are terminal")
	} else {
		assert.True(t, apiutils.IsNotFoundError(err), "trigger garbage-collected once the finalizer is removed")
	}
	mockRunner.AssertExpectations(t)
}

// TestReconcile_DeleteKilledTriggerCancelsChildren: deleting a trigger that is
// already KILLED does not re-kill it, but still cancels and waits on its children.
func TestReconcile_DeleteKilledTriggerCancelsChildren(t *testing.T) {
	scheme := cascadeScheme(t)
	now := metav1.Now()
	trigger := &v2pb.TriggerRun{
		ObjectMeta: metav1.ObjectMeta{
			Name: "tr", Namespace: "ns", UID: types.UID("t1"),
			Finalizers: []string{drainFinalizer}, DeletionTimestamp: &now,
		},
		Status: v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_KILLED},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(trigger, childRun("running", "tr", v2pb.PIPELINE_RUN_STATE_RUNNING, false)).
		WithStatusSubresource(trigger).Build()

	mockRunner := &MockRunner{}
	r := &Reconciler{Handler: apiHandler.NewFakeAPIHandler(c), scheme: scheme, log: logr.Discard(), cronTrigger: mockRunner}
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "tr", Namespace: "ns"}}

	_, err := r.Reconcile(context.Background(), req)
	require.NoError(t, err)
	assert.True(t, getChild(t, c, "running").Spec.Kill)
	assert.True(t, ctrlutil.ContainsFinalizer(getTrigger(t, c, "tr"), drainFinalizer))
	mockRunner.AssertNotCalled(t, "Kill", mock.Anything, mock.Anything)
}

// TestActiveChildListOptionsExt: the metadata-storage filter repeats the triggered-by
// match (the LabelSelector string is ignored once an Operation is set) and limits
// the list to active states.
func TestActiveChildListOptionsExt(t *testing.T) {
	ext, err := activeChildListOptionsExt(&v2pb.TriggerRun{ObjectMeta: metav1.ObjectMeta{Name: "tr"}})
	require.NoError(t, err)
	require.NotNil(t, ext.Operation)
	assert.Equal(t, apipb.LOGICAL_OPERATOR_AND, ext.Operation.LogicalOperator)
	require.Len(t, ext.Operation.Criterion, 2)

	values := map[string]string{}
	for _, c := range ext.Operation.Criterion {
		var sv gogotypes.StringValue
		require.NoError(t, gogotypes.UnmarshalAny(c.MatchValue, &sv))
		values[c.FieldName] = sv.Value
	}
	assert.Equal(t, "tr", values["pipeline_run.metadata.labels."+TriggeredByLabel])
	assert.Equal(t, "PIPELINE_RUN_STATE_INVALID,PIPELINE_RUN_STATE_PENDING,PIPELINE_RUN_STATE_RUNNING",
		values["pipeline_run.state"])
}

// TestReconcile_DeleteTimeoutNotBlockedByChildProbeError: a persistent failure
// listing children must not bypass the drain timeout, or the finalizer would stick.
func TestReconcile_DeleteTimeoutNotBlockedByChildProbeError(t *testing.T) {
	scheme := cascadeScheme(t)
	expired := metav1.NewTime(time.Now().Add(-cascadedelete.CascadeDrainTimeout - time.Minute))
	trigger := &v2pb.TriggerRun{
		ObjectMeta: metav1.ObjectMeta{
			Name: "tr", Namespace: "ns", UID: types.UID("t1"),
			Finalizers: []string{drainFinalizer}, DeletionTimestamp: &expired,
		},
		Status: v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_KILLED},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(trigger).WithStatusSubresource(trigger).
		WithInterceptorFuncs(interceptor.Funcs{
			List: func(ctx context.Context, cl client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
				if _, ok := list.(*v2pb.PipelineRunList); ok {
					return assert.AnError
				}
				return cl.List(ctx, list, opts...)
			},
		}).Build()

	mockWorkflowClient := interfaceMock.NewMockWorkflowClient(gomock.NewController(t))
	mockWorkflowClient.EXPECT().GetDomain().Return("default").AnyTimes()
	mockWorkflowClient.EXPECT().ListOpenWorkflow(gomock.Any(), gomock.Any()).
		Return(&clientInterface.ListOpenWorkflowExecutionsResponse{}, nil).AnyTimes()
	mockWorkflowClient.EXPECT().DeleteTrigger(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	r := &Reconciler{Handler: apiHandler.NewFakeAPIHandler(c), scheme: scheme, log: logr.Discard(),
		cronTrigger: &MockRunner{}, workflowClient: mockWorkflowClient}

	_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "tr", Namespace: "ns"}})
	require.NoError(t, err)

	got := &v2pb.TriggerRun{}
	err = c.Get(context.Background(), types.NamespacedName{Name: "tr", Namespace: "ns"}, got)
	if err == nil {
		assert.False(t, ctrlutil.ContainsFinalizer(got, drainFinalizer), "timeout must still release the finalizer")
	} else {
		assert.True(t, apiutils.IsNotFoundError(err))
	}
}
