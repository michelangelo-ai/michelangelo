package triggerrun

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	apiutils "github.com/michelangelo-ai/michelangelo/go/api/utils"
	clientInterface "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/interface"
	interfaceMock "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/interface/interface_mock"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// TestDrainFinalizerLiteral asserts the drain finalizer string is byte-identical
// (rollout safety — see the cascade-delete plan §8).
func TestDrainFinalizerLiteral(t *testing.T) {
	assert.Equal(t, "triggerruns.michelangelo.uber.com/drain", drainFinalizer)
}

// TestMetricKindLiteral asserts the metric label value is the documented dashboard
// contract.
func TestMetricKindLiteral(t *testing.T) {
	assert.Equal(t, "trigger_run", metricKind)
}

func cascadeScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, v2pb.AddToScheme(s))
	return s
}

// TestTriggerRun_RequestCancel_AtomicTokenAndKill: RequestCancel kills via the
// runner and stamps the drain-counted token in one persisted update.
func TestTriggerRun_RequestCancel_AtomicTokenAndKill(t *testing.T) {
	scheme := cascadeScheme(t)
	run := &v2pb.TriggerRun{
		ObjectMeta: metav1.ObjectMeta{Name: "tr1", Namespace: "ns", UID: types.UID("t1"), Finalizers: []string{drainFinalizer}},
		Status:     v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_RUNNING},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(run).WithStatusSubresource(run).Build()

	mockRunner := &MockRunner{}
	mockRunner.On("Kill", mock.Anything, mock.Anything).
		Return(v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_KILLED}, nil)

	r := &Reconciler{Handler: apiHandler.NewFakeAPIHandler(c), scheme: scheme, cronTrigger: mockRunner}
	target := &triggerRunDrainTarget{r: r, log: logr.Discard(), run: run.DeepCopy()}
	require.NoError(t, target.RequestCancel(context.Background()))

	got := &v2pb.TriggerRun{}
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "tr1", Namespace: "ns"}, got))
	assert.Equal(t, "true", got.GetAnnotations()["cascade.michelangelo.uber.com/drain-counted"], "token persisted with the kill")
	mockRunner.AssertCalled(t, "Kill", mock.Anything, mock.Anything)
}

// TestTriggerRun_Progress_ReissuesKill: Progress re-issues the idempotent kill and
// re-checks terminal.
func TestTriggerRun_Progress_ReissuesKill(t *testing.T) {
	scheme := cascadeScheme(t)
	run := &v2pb.TriggerRun{
		ObjectMeta: metav1.ObjectMeta{Name: "tr2", Namespace: "ns", UID: types.UID("t2"), Finalizers: []string{drainFinalizer}},
		Status:     v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_RUNNING},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(run).WithStatusSubresource(run).Build()

	mockRunner := &MockRunner{}
	mockRunner.On("Kill", mock.Anything, mock.Anything).
		Return(v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_KILLED}, nil)

	r := &Reconciler{Handler: apiHandler.NewFakeAPIHandler(c), scheme: scheme, cronTrigger: mockRunner}
	target := &triggerRunDrainTarget{r: r, log: logr.Discard(), run: run.DeepCopy()}
	terminal, err := target.Progress(context.Background())
	require.NoError(t, err)
	assert.True(t, terminal, "killed → terminal")
	mockRunner.AssertCalled(t, "Kill", mock.Anything, mock.Anything)
}

// TestTriggerRun_ForceKill_DeletesSchedule: ForceKill goes through
// ForceKillWorkflow → killWorkflow → DeleteTrigger so a Temporal schedule is
// actually removed on the 24h timeout path.
func TestTriggerRun_ForceKill_DeletesSchedule(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockWF := interfaceMock.NewMockWorkflowClient(ctrl)
	mockWF.EXPECT().GetDomain().Return("default").AnyTimes()
	runID := "rid"
	mockWF.EXPECT().ListOpenWorkflow(gomock.Any(), gomock.Any()).Return(&clientInterface.ListOpenWorkflowExecutionsResponse{
		Executions: []clientInterface.WorkflowExecutionInfo{
			{Execution: &clientInterface.WorkflowExecution{RunID: runID}},
		},
	}, nil)
	mockWF.EXPECT().DeleteTrigger(gomock.Any(), "ns.tr3", runID).Return(nil)

	r := &Reconciler{workflowClient: mockWF}
	run := &v2pb.TriggerRun{ObjectMeta: metav1.ObjectMeta{Name: "tr3", Namespace: "ns"}}
	target := &triggerRunDrainTarget{r: r, log: logr.Discard(), run: run}
	require.NoError(t, target.ForceKill(context.Background()))
}

// TestTriggerRun_CompleteDrain_ImmutableAndFinalizer: one update marks immutable
// and removes the drain finalizer.
func TestTriggerRun_CompleteDrain_ImmutableAndFinalizer(t *testing.T) {
	scheme := cascadeScheme(t)
	run := &v2pb.TriggerRun{
		ObjectMeta: metav1.ObjectMeta{Name: "tr4", Namespace: "ns", UID: types.UID("t4"), Finalizers: []string{drainFinalizer}},
		Status:     v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_KILLED},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(run).Build()
	r := &Reconciler{Handler: apiHandler.NewFakeAPIHandler(c), scheme: scheme}
	target := &triggerRunDrainTarget{r: r, log: logr.Discard(), run: run.DeepCopy()}

	require.NoError(t, target.CompleteDrain(context.Background()))
	assert.True(t, apiutils.IsImmutable(target.run), "must be marked immutable")
	assert.False(t, ctrlutil.ContainsFinalizer(target.run, drainFinalizer), "drain finalizer removed")
}

// TestTriggerRun_WorkStarted: state != INVALID means work started.
func TestTriggerRun_WorkStarted(t *testing.T) {
	invalid := &v2pb.TriggerRun{Status: v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_INVALID}}
	running := &v2pb.TriggerRun{Status: v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_RUNNING}}
	assert.False(t, triggerRunWorkStarted(invalid))
	assert.True(t, triggerRunWorkStarted(running))
}
