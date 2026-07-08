package pipelinerun

import (
	"context"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/michelangelo-ai/michelangelo/go/api"
	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	"github.com/michelangelo-ai/michelangelo/go/api/utils"
	defaultEngine "github.com/michelangelo-ai/michelangelo/go/base/conditions/engine"
	interfaceMock "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/interface/interface_mock"
	"github.com/michelangelo-ai/michelangelo/go/cascadedelete"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2 "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// TestDrainFinalizerLiteral asserts the drain finalizer string is byte-identical
// (rollout safety — see the cascade-delete plan §8).
func TestDrainFinalizerLiteral(t *testing.T) {
	assert.Equal(t, "pipelineruns.michelangelo.uber.com/drain", drainFinalizer)
}

// TestMetricKindLiteral asserts the metric label value is the documented dashboard
// contract.
func TestMetricKindLiteral(t *testing.T) {
	assert.Equal(t, "pipeline_run", metricKind)
}

// cascadeReconciler builds a Reconciler over a fake api.Handler for cascade tests.
func cascadeReconciler(t *testing.T, c client.Client, scheme *runtime.Scheme, metadataStorageEnabled bool) *Reconciler {
	t.Helper()
	logger := zaptest.NewLogger(t)
	return &Reconciler{
		Handler:                apiHandler.NewFakeAPIHandler(c),
		logger:                 logger,
		engine:                 defaultEngine.NewDefaultEngine[*v2pb.PipelineRun](logger),
		scheme:                 scheme,
		metadataStorageEnabled: metadataStorageEnabled,
	}
}

func newScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, v2.AddToScheme(s))
	return s
}

// TestInvariant_FinalizerBeforeOwnerRef: a live, pre-existing run reconciled ends
// with the drain finalizer persisted; and if the ownerRef-adding Update fails, the
// finalizer still persists (it was committed first).
func TestInvariant_FinalizerBeforeOwnerRef(t *testing.T) {
	scheme := newScheme(t)
	run := &v2.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{Name: "run1", Namespace: "ns", UID: types.UID("r1")},
		Spec:       v2.PipelineRunSpec{Pipeline: &apipb.ResourceIdentifier{Name: "pl", Namespace: "ns"}},
		Status:     v2.PipelineRunStatus{State: v2.PIPELINE_RUN_STATE_RUNNING},
	}
	pipeline := &v2.Pipeline{ObjectMeta: metav1.ObjectMeta{Name: "pl", Namespace: "ns", UID: types.UID("p1")}}

	// Interceptor fails any Update that carries an ownerReference (the ownerRef
	// stamp), but allows the finalizer-only Update (ensureDrainFinalizer).
	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(run, pipeline).
		WithStatusSubresource(run).
		WithInterceptorFuncs(interceptor.Funcs{
			Update: func(ctx context.Context, cl client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
				if len(obj.GetOwnerReferences()) > 0 {
					return assert.AnError
				}
				return cl.Update(ctx, obj, opts...)
			},
		}).
		Build()

	r := cascadeReconciler(t, c, scheme, true)
	_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "run1", Namespace: "ns"}})
	require.Error(t, err, "ownerRef stamp update is expected to fail")

	got := &v2.PipelineRun{}
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "run1", Namespace: "ns"}, got))
	assert.True(t, ctrlutil.ContainsFinalizer(got, drainFinalizer), "drain finalizer must be persisted before (and despite) ownerRef failure")
	assert.Empty(t, got.GetOwnerReferences(), "ownerRef must not have been persisted")
}

// TestInvariant_ImmutableBeforeFinalizerRemoval: CompleteDrain marks immutable in
// the SAME update that removes the drain finalizer (when metadata storage is enabled).
func TestInvariant_ImmutableBeforeFinalizerRemoval(t *testing.T) {
	scheme := newScheme(t)
	run := &v2.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{
			Name: "run2", Namespace: "ns", UID: types.UID("r2"),
			Finalizers: []string{drainFinalizer},
		},
		Status: v2.PipelineRunStatus{State: v2.PIPELINE_RUN_STATE_KILLED},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(run).Build()
	r := cascadeReconciler(t, c, scheme, true)

	target := &pipelineRunDrainTarget{r: r, logger: r.logger, run: run.DeepCopy()}
	require.NoError(t, target.CompleteDrain(context.Background()))

	// The run no longer has the drain finalizer → fake client deletes it; but the
	// in-memory object reflects the single update.
	assert.True(t, utils.IsImmutable(target.run), "must be marked immutable")
	assert.False(t, ctrlutil.ContainsFinalizer(target.run, drainFinalizer), "drain finalizer must be removed")
}

// TestInvariant_MetadataStorageGuard: with metadata storage disabled, CompleteDrain
// must NOT mark immutable (no MySQL-only eviction → no data loss).
func TestInvariant_MetadataStorageGuard(t *testing.T) {
	scheme := newScheme(t)
	run := &v2.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{
			Name: "run3", Namespace: "ns", UID: types.UID("r3"),
			Finalizers: []string{drainFinalizer},
		},
		Status: v2.PipelineRunStatus{State: v2.PIPELINE_RUN_STATE_KILLED},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(run).Build()
	r := cascadeReconciler(t, c, scheme, false) // storage disabled

	target := &pipelineRunDrainTarget{r: r, logger: r.logger, run: run.DeepCopy()}
	require.NoError(t, target.CompleteDrain(context.Background()))

	assert.False(t, utils.IsImmutable(target.run), "must NOT be marked immutable when storage disabled")
	assert.False(t, ctrlutil.ContainsFinalizer(target.run, drainFinalizer), "drain finalizer still removed")
}

// TestRequestCancel_AtomicTokenAndKill: RequestCancel sets Spec.Kill and the
// drain-counted token in one persisted update.
func TestRequestCancel_AtomicTokenAndKill(t *testing.T) {
	scheme := newScheme(t)
	run := &v2.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{Name: "run4", Namespace: "ns", UID: types.UID("r4"), Finalizers: []string{drainFinalizer}},
		Status:     v2.PipelineRunStatus{State: v2.PIPELINE_RUN_STATE_RUNNING, WorkflowId: "w", WorkflowRunId: "wr"},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(run).Build()
	r := cascadeReconciler(t, c, scheme, true)
	target := &pipelineRunDrainTarget{r: r, logger: r.logger, run: run.DeepCopy()}

	require.NoError(t, target.RequestCancel(context.Background()))
	assert.True(t, target.run.Spec.Kill, "Spec.Kill must be set")

	got := &v2.PipelineRun{}
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "run4", Namespace: "ns"}, got))
	assert.True(t, got.Spec.Kill, "Spec.Kill must be persisted")
	assert.Equal(t, "true", got.GetAnnotations()["cascade.michelangelo.uber.com/drain-counted"], "drain-counted token must be persisted in the same update")
}

// TestMarkKilled_NoToken: MarkKilled drives terminal KILLED without the token.
func TestMarkKilled_NoToken(t *testing.T) {
	scheme := newScheme(t)
	run := &v2.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{Name: "run5", Namespace: "ns", UID: types.UID("r5"), Finalizers: []string{drainFinalizer}},
		Status:     v2.PipelineRunStatus{State: v2.PIPELINE_RUN_STATE_RUNNING},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(run).WithStatusSubresource(run).Build()
	r := cascadeReconciler(t, c, scheme, true)
	target := &pipelineRunDrainTarget{r: r, logger: r.logger, run: run.DeepCopy()}

	require.NoError(t, target.MarkKilled(context.Background()))
	assert.Equal(t, v2.PIPELINE_RUN_STATE_KILLED, target.run.Status.State)
	assert.Empty(t, target.run.GetAnnotations()["cascade.michelangelo.uber.com/drain-counted"], "MarkKilled must not stamp the drain-counted token")
}

// TestIngesterFinalizerConstUnchanged guards against accidental edits to the
// ingester finalizer this controller coexists with.
func TestIngesterFinalizerConstUnchanged(t *testing.T) {
	assert.Equal(t, "michelangelo/Ingester", api.IngesterFinalizer)
}

// TestPipelineRun_ForceKill_CancelsWorkflow: on the cascade safety-timeout path,
// ForceKill directly cancels the run's workflow via the workflow client (using the
// recorded WorkflowId/WorkflowRunId), so a run that never drained gracefully still
// has its Spark/Ray workflow torn down.
func TestPipelineRun_ForceKill_CancelsWorkflow(t *testing.T) {
	ctrlMock := gomock.NewController(t)
	defer ctrlMock.Finish()
	mockWF := interfaceMock.NewMockWorkflowClient(ctrlMock)
	mockWF.EXPECT().CancelWorkflow(gomock.Any(), "wid", "rid", defaultEngine.KillReason).Return(nil)

	r := &Reconciler{workflowClient: mockWF}
	run := &v2.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{Name: "run6", Namespace: "ns"},
		Status:     v2.PipelineRunStatus{WorkflowId: "wid", WorkflowRunId: "rid"},
	}
	target := &pipelineRunDrainTarget{r: r, logger: zaptest.NewLogger(t), run: run}
	require.NoError(t, target.ForceKill(context.Background()))
}

// TestPipelineRun_ForceKill_NoWorkflowNoop: ForceKill makes no client call when no
// workflow was ever started (empty WorkflowId/WorkflowRunId) — nothing to tear down.
// The gomock controller has no expectations, so any client call would fail.
func TestPipelineRun_ForceKill_NoWorkflowNoop(t *testing.T) {
	ctrlMock := gomock.NewController(t)
	defer ctrlMock.Finish()
	mockWF := interfaceMock.NewMockWorkflowClient(ctrlMock)

	r := &Reconciler{workflowClient: mockWF}
	run := &v2.PipelineRun{ObjectMeta: metav1.ObjectMeta{Name: "run7", Namespace: "ns"}}
	target := &pipelineRunDrainTarget{r: r, logger: zaptest.NewLogger(t), run: run}
	require.NoError(t, target.ForceKill(context.Background()))
}

// TestCascade_TimeoutPath_CancelsWorkflowBeforeFinalize reproduces the P1: a
// work-started run whose drain never progressed (old deletionTimestamp, no
// drain-counted token) is first reconciled past the 24h safety timeout. The driver's
// timeout branch must tear the workflow down (ForceKill → CancelWorkflow) before
// removing the drain finalizer, so the Spark/Ray workflow is not orphaned.
func TestCascade_TimeoutPath_CancelsWorkflowBeforeFinalize(t *testing.T) {
	ctrlMock := gomock.NewController(t)
	defer ctrlMock.Finish()
	mockWF := interfaceMock.NewMockWorkflowClient(ctrlMock)
	mockWF.EXPECT().CancelWorkflow(gomock.Any(), "wid", "rid", defaultEngine.KillReason).Return(nil)

	scheme := newScheme(t)
	ts := metav1.NewTime(time.Now().Add(-(cascadedelete.CascadeDrainTimeout + time.Hour)))
	run := &v2.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{
			Name: "run8", Namespace: "ns", UID: types.UID("r8"),
			Finalizers:        []string{drainFinalizer},
			DeletionTimestamp: &ts,
		},
		Status: v2.PipelineRunStatus{State: v2.PIPELINE_RUN_STATE_RUNNING, WorkflowId: "wid", WorkflowRunId: "rid"},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(run).Build()
	r := cascadeReconciler(t, c, scheme, true)
	r.workflowClient = mockWF

	// No drain-counted token, work started, deletionTimestamp older than the safety
	// timeout → the driver's timeout branch fires on the very first drain step.
	drainRun := run.DeepCopy()
	st := cascadedelete.DrainState{
		Object:      drainRun,
		Kind:        metricKind,
		Finalizer:   drainFinalizer,
		IsTerminal:  isTerminalState(drainRun.Status.State),
		WorkStarted: pipelineRunWorkStarted(drainRun),
	}
	res, err := cascadedelete.RunDrainStep(context.Background(), st, &pipelineRunDrainTarget{r: r, logger: r.logger, run: drainRun}, drainRequeueInterval)
	require.NoError(t, err)
	require.Equal(t, time.Duration(0), res.RequeueAfter, "timeout path finalizes without requeue")
	assert.False(t, ctrlutil.ContainsFinalizer(drainRun, drainFinalizer), "drain finalizer removed after teardown")
}
