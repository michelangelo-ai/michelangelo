package triggerrun

import (
	"context"
	"testing"
	"time"

	"github.com/go-logr/zapr"
	"github.com/golang/mock/gomock"
	pbtypes "github.com/gogo/protobuf/types"
	clientInterface "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/interface"
	interfaceMock "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/interface/interface_mock"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
)

const _backfillWorkflowID = "test-namespace.backfill-run"

// newRealBackfillReconciler wires the reconciler with the REAL backfillTrigger
// runner over a mock workflow client, so these tests exercise the production
// Update/GetStatus path instead of a MockRunner that can paper over it.
func newRealBackfillReconciler(
	t *testing.T, tr *v2pb.TriggerRun, workflowStatus clientInterface.WorkflowExecutionStatus,
) Reconciler {
	t.Helper()
	gctrl := gomock.NewController(t)
	mockClient := interfaceMock.NewMockWorkflowClient(gctrl)
	mockClient.EXPECT().GetDomain().Return("test-domain").AnyTimes()
	mockClient.EXPECT().GetProvider().Return("temporal").AnyTimes()
	mockClient.EXPECT().GetWorkflowExecutionInfo(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&clientInterface.WorkflowExecutionInfo{Status: workflowStatus}, nil).AnyTimes()

	log := zapr.NewLogger(zaptest.NewLogger(t))
	return setUpReconciler(t, []runtime.Object{tr.DeepCopy()}, Params{
		Logger:          log,
		CronTrigger:     &MockRunner{},
		BackfillTrigger: NewBackfillTrigger(log, mockClient),
	})
}

func newRunningBackfill(t *testing.T, withSuccessorAnnotation bool) *v2pb.TriggerRun {
	t.Helper()
	start, err := pbtypes.TimestampProto(time.Now().Add(-48 * time.Hour))
	require.NoError(t, err)
	end, err := pbtypes.TimestampProto(time.Now().Add(-24 * time.Hour))
	require.NoError(t, err)

	tr := _triggerRun.DeepCopy()
	tr.Name = "backfill-run"
	tr.Labels = map[string]string{"michelangelo.uber.com/environment": "staging"}
	if withSuccessorAnnotation {
		tr.Annotations = map[string]string{AnnotationSuccessorTrigger: "true"}
	}
	tr.Spec.StartTimestamp = start
	tr.Spec.EndTimestamp = end
	tr.Status = v2pb.TriggerRunStatus{
		State:               v2pb.TRIGGER_RUN_STATE_RUNNING,
		ExecutionWorkflowId: _backfillWorkflowID,
		LogUrl:              "http://example/log",
	}
	return tr
}

// TestReconcile_RunningBackfillIsNotFalselyFailed is a regression test for the
// bug that broke backfill->cron sequencing: backfillTrigger.Update returned a
// status rebuilt from State alone, the reconciler assigned it wholesale, and
// the resulting empty ExecutionWorkflowId made GetStatus fail with "execution
// workflow id is empty". A healthy running backfill was marked FAILED on its
// next reconcile, so it never reached a terminal SUCCEEDED and never spawned a
// successor.
func TestReconcile_RunningBackfillIsNotFalselyFailed(t *testing.T) {
	ctx := context.Background()
	tr := newRunningBackfill(t, true)
	reconciler := newRealBackfillReconciler(t, tr, clientInterface.WorkflowExecutionStatusRunning)

	_, err := reconciler.Reconcile(ctx,
		ctrl.Request{NamespacedName: types.NamespacedName{Namespace: _namespace, Name: tr.Name}})
	assert.NoError(t, err)

	got := &v2pb.TriggerRun{}
	require.NoError(t, reconciler.Get(ctx, _namespace, tr.Name, &metav1.GetOptions{}, got))
	assert.Equal(t, v2pb.TRIGGER_RUN_STATE_RUNNING, got.Status.State,
		"a backfill whose workflow is still running must stay RUNNING")
	assert.Empty(t, got.Status.ErrorMessage)
	assert.Equal(t, _backfillWorkflowID, got.Status.ExecutionWorkflowId,
		"ExecutionWorkflowId must survive the no-op Update; GetStatus needs it")

	// The cron successor must not exist while the backfill is still running.
	successor := &v2pb.TriggerRun{}
	err = reconciler.Get(ctx, _namespace, generateSuccessorTriggerRunName(tr.Name), &metav1.GetOptions{}, successor)
	assert.Error(t, err, "cron successor must not be created before the backfill finishes")
}

// TestReconcile_BackfillSuccessorThroughRealRunner drives the full ordering
// guarantee through the production runner: only once the backfill workflow
// itself reports Completed does the recurring cron TriggerRun get created.
func TestReconcile_BackfillSuccessorThroughRealRunner(t *testing.T) {
	ctx := context.Background()
	tr := newRunningBackfill(t, true)
	reconciler := newRealBackfillReconciler(t, tr, clientInterface.WorkflowExecutionStatusCompleted)

	_, err := reconciler.Reconcile(ctx,
		ctrl.Request{NamespacedName: types.NamespacedName{Namespace: _namespace, Name: tr.Name}})
	assert.NoError(t, err)

	got := &v2pb.TriggerRun{}
	require.NoError(t, reconciler.Get(ctx, _namespace, tr.Name, &metav1.GetOptions{}, got))
	assert.Equal(t, v2pb.TRIGGER_RUN_STATE_SUCCEEDED, got.Status.State)

	successor := &v2pb.TriggerRun{}
	require.NoError(t, reconciler.Get(ctx, _namespace,
		generateSuccessorTriggerRunName(tr.Name), &metav1.GetOptions{}, successor))
	assert.Equal(t, TriggerTypeCron, GetTriggerType(successor))
	assert.Equal(t, v2pb.TRIGGER_RUN_STATE_INVALID, successor.Status.State,
		"successor starts fresh; its own reconcile arms the cron")
	assert.Equal(t, "staging", successor.Labels["michelangelo.uber.com/environment"],
		"labels feed the recurring schedule input hash and must carry over")
}

// TestReconcile_NoSuccessorAnnotationLeavesBackfillAlone confirms the feature is
// opt-in: an unannotated backfill completes normally and spawns nothing.
func TestReconcile_NoSuccessorAnnotationLeavesBackfillAlone(t *testing.T) {
	ctx := context.Background()
	tr := newRunningBackfill(t, false)
	reconciler := newRealBackfillReconciler(t, tr, clientInterface.WorkflowExecutionStatusCompleted)

	_, err := reconciler.Reconcile(ctx,
		ctrl.Request{NamespacedName: types.NamespacedName{Namespace: _namespace, Name: tr.Name}})
	assert.NoError(t, err)

	got := &v2pb.TriggerRun{}
	require.NoError(t, reconciler.Get(ctx, _namespace, tr.Name, &metav1.GetOptions{}, got))
	assert.Equal(t, v2pb.TRIGGER_RUN_STATE_SUCCEEDED, got.Status.State)

	successor := &v2pb.TriggerRun{}
	err = reconciler.Get(ctx, _namespace, generateSuccessorTriggerRunName(tr.Name), &metav1.GetOptions{}, successor)
	assert.Error(t, err, "no annotation means no successor")
}
