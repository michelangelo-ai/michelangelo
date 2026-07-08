package cascadedelete

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

const (
	testFinalizer = "test.michelangelo.uber.com/drain"
	testKind      = "test_run"
	testRequeue   = 10 * time.Second
)

// fakeTarget is an in-memory DrainTarget that records which methods were called
// and lets each test script the Progress terminal result and per-method errors.
// (Terminal/started state lives in the DrainState the test passes to
// RunDrainStep.) Its mutating methods update the wrapped object exactly as a real
// adapter would (RequestCancel stamps the drain-counted token; CompleteDrain
// clears it + removes the finalizer), so the driver's token-driven gauge
// accounting is exercised end-to-end.
//
// The active-drain gauge is asserted behaviorally rather than by reading the
// prometheus value: prometheus testutil pulls in client_model, which is not
// wired into this repo's Bazel module graph. completeDrainSawToken records the
// token state at finish entry — the exact predicate finish() uses to decide
// whether to DecDrainActive — so "decrements only when the token was present" is
// asserted via that flag plus the per-branch token assertions.
//
// gaugeInc/gaugeDec mirror the EXACT conditions under which the driver moves the
// active-drain gauge, so a test can pin the net delta across a multi-step (and
// retried) drain without reading prometheus:
//   - drain.go increments via IncDrainActive only on the first-loop path, after
//     RequestCancel returns nil; the fake bumps gaugeInc on that same success.
//   - drain.go decrements via DecDrainActive inside finish() only when the token
//     was present at entry AND CompleteDrain commits (returns nil); the fake bumps
//     gaugeDec under that identical predicate.
//
// Because both counters are keyed off the same predicates the production gauge
// calls are keyed off, gaugeInc/gaugeDec are a faithful stand-in for the real
// gauge's increment/decrement count — including across a failed-then-retried
// finish, where CompleteDrain runs twice but only commits (and only decrements)
// once.
type fakeTarget struct {
	obj client.Object

	// progressTerminal is what Progress reports as the terminal result.
	progressTerminal bool

	// scripted errors per method.
	requestCancelErr error
	progressErr      error
	markKilledErr    error
	completeDrainErr error
	forceKillErr     error

	// call counters.
	requestCancelCalls int
	progressCalls      int
	markKilledCalls    int
	forceKillCalls     int
	completeDrainCalls int

	// completeDrainSawToken records whether the drain-counted token was present
	// when CompleteDrain was invoked (i.e. before it cleared the token).
	completeDrainSawToken bool

	// gaugeInc/gaugeDec count active-drain gauge movements under the exact
	// predicates drain.go uses to call IncDrainActive/DecDrainActive (see the
	// type doc). They let a test assert the net gauge delta over a whole drain.
	gaugeInc int
	gaugeDec int
}

func (f *fakeTarget) RequestCancel(_ context.Context) error {
	f.requestCancelCalls++
	if f.requestCancelErr != nil {
		return f.requestCancelErr
	}
	// Atomic: cancel + drain-counted token in one persisted update.
	MarkDrainCounted(f.obj)
	// Mirror drain.go: on the first loop, IncDrainActive runs exactly once,
	// immediately after RequestCancel returns nil.
	f.gaugeInc++
	return nil
}

func (f *fakeTarget) Progress(_ context.Context) (bool, error) {
	f.progressCalls++
	if f.progressErr != nil {
		return false, f.progressErr
	}
	return f.progressTerminal, nil
}

func (f *fakeTarget) MarkKilled(_ context.Context) error {
	f.markKilledCalls++
	// Must NOT stamp the drain-counted token.
	return f.markKilledErr
}

func (f *fakeTarget) ForceKill(_ context.Context) error {
	f.forceKillCalls++
	return f.forceKillErr
}

func (f *fakeTarget) CompleteDrain(_ context.Context) error {
	f.completeDrainCalls++
	f.completeDrainSawToken = IsDrainCounted(f.obj)
	if f.completeDrainErr != nil {
		return f.completeDrainErr
	}
	// One finalizing update: clear the token + remove the drain finalizer.
	ClearDrainCounted(f.obj)
	ctrlutil.RemoveFinalizer(f.obj, testFinalizer)
	// Mirror finish(): DecDrainActive runs only when the token was present at
	// entry AND CompleteDrain committed (returned nil) — so a retried finish that
	// errored the first time decrements exactly once, on the committing call.
	if f.completeDrainSawToken {
		f.gaugeDec++
	}
	return nil
}

// drainState builds the read-only snapshot a test passes to RunDrainStep,
// pinning the test finalizer/kind and the terminal/work-started predicates.
func drainState(run client.Object, terminal, workStarted bool) DrainState {
	return DrainState{
		Object:      run,
		Kind:        testKind,
		Finalizer:   testFinalizer,
		IsTerminal:  terminal,
		WorkStarted: workStarted,
	}
}

// newDrainRun builds a child object with the drain finalizer and a deletion
// timestamp set `age` ago.
func newDrainRun(age time.Duration) *v2pb.PipelineRun {
	ts := metav1.NewTime(time.Now().Add(-age))
	return &v2pb.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "run",
			Finalizers:        []string{testFinalizer},
			DeletionTimestamp: &ts,
		},
	}
}

func TestRunDrainStepNotOurs(t *testing.T) {
	// No drain finalizer of ours → no-op, no error, no requeue.
	run := &v2pb.PipelineRun{ObjectMeta: metav1.ObjectMeta{Name: "run"}}
	f := &fakeTarget{obj: run}

	res, err := RunDrainStep(context.Background(), drainState(run, false, false), f, testRequeue)
	require.NoError(t, err)
	require.Equal(t, time.Duration(0), res.RequeueAfter)
	require.Zero(t, f.completeDrainCalls)
	require.Zero(t, f.requestCancelCalls)
}

func TestRunDrainStepTerminalFinalizesImmediately(t *testing.T) {
	run := newDrainRun(time.Minute)
	f := &fakeTarget{obj: run}

	res, err := RunDrainStep(context.Background(), drainState(run, true, false), f, testRequeue)
	require.NoError(t, err)
	require.Equal(t, time.Duration(0), res.RequeueAfter)
	require.Equal(t, 1, f.completeDrainCalls)
	require.Zero(t, f.requestCancelCalls)
	require.Zero(t, f.forceKillCalls)
	// Terminal-on-arrival carried no token → finish must not decrement the gauge.
	require.False(t, f.completeDrainSawToken)
	require.False(t, ctrlutil.ContainsFinalizer(run, testFinalizer))
}

func TestRunDrainStepTimeoutForceKills(t *testing.T) {
	run := newDrainRun(CascadeDrainTimeout + time.Hour)
	f := &fakeTarget{obj: run}

	res, err := RunDrainStep(context.Background(), drainState(run, false, true), f, testRequeue)
	require.NoError(t, err)
	require.Equal(t, time.Duration(0), res.RequeueAfter)
	require.Equal(t, 1, f.forceKillCalls, "ForceKill must run on timeout")
	require.Equal(t, 1, f.completeDrainCalls)
	// No active-drain token was ever stamped on this path → no gauge decrement.
	require.False(t, IsDrainCounted(run))
	require.False(t, f.completeDrainSawToken)
}

func TestRunDrainStepTimeoutSwallowsForceKillError(t *testing.T) {
	run := newDrainRun(CascadeDrainTimeout + time.Hour)
	f := &fakeTarget{obj: run, forceKillErr: errors.New("boom")}

	// ForceKill error is swallowed; drain still force-completes.
	res, err := RunDrainStep(context.Background(), drainState(run, false, true), f, testRequeue)
	require.NoError(t, err)
	require.Equal(t, time.Duration(0), res.RequeueAfter)
	require.Equal(t, 1, f.forceKillCalls)
	require.Equal(t, 1, f.completeDrainCalls)
}

func TestRunDrainStepNeverStartedMarksKilled(t *testing.T) {
	run := newDrainRun(time.Minute)
	f := &fakeTarget{obj: run}

	res, err := RunDrainStep(context.Background(), drainState(run, false, false), f, testRequeue)
	require.NoError(t, err)
	require.Equal(t, time.Duration(0), res.RequeueAfter)
	require.Equal(t, 1, f.markKilledCalls, "never-started must MarkKilled")
	require.Zero(t, f.requestCancelCalls, "never-started must not RequestCancel")
	require.Equal(t, 1, f.completeDrainCalls)
	// MarkKilled must not stamp the token; finish must not decrement the gauge.
	require.False(t, IsDrainCounted(run))
	require.False(t, f.completeDrainSawToken)
}

func TestRunDrainStepFirstLoopRequestsCancel(t *testing.T) {
	run := newDrainRun(time.Minute)
	f := &fakeTarget{obj: run}

	res, err := RunDrainStep(context.Background(), drainState(run, false, true), f, testRequeue)
	require.NoError(t, err)
	require.Equal(t, testRequeue, res.RequeueAfter, "first loop requeues")
	require.Equal(t, 1, f.requestCancelCalls, "first loop must RequestCancel")
	require.Zero(t, f.progressCalls, "first loop must not Progress")
	require.Zero(t, f.completeDrainCalls, "first loop must not finalize")
	// RequestCancel stamped the token (the atomic cancel+count), and
	// IncDrainActive ran exactly once (the gauge now owes a matching decrement at
	// finish).
	require.True(t, IsDrainCounted(run))
	require.Equal(t, 1, f.gaugeInc, "first loop increments the gauge exactly once")
	require.Zero(t, f.gaugeDec, "first loop does not decrement")
}

func TestRunDrainStepProgressToTerminalFinalizes(t *testing.T) {
	run := newDrainRun(time.Minute)
	MarkDrainCounted(run) // token already present → subsequent loop
	f := &fakeTarget{obj: run, progressTerminal: true}

	res, err := RunDrainStep(context.Background(), drainState(run, false, true), f, testRequeue)
	require.NoError(t, err)
	require.Equal(t, time.Duration(0), res.RequeueAfter)
	require.Equal(t, 1, f.progressCalls)
	require.Zero(t, f.requestCancelCalls, "must not re-request cancel once counted")
	require.Equal(t, 1, f.completeDrainCalls)
	// Token was present at finish entry → finish decrements the gauge exactly once.
	require.True(t, f.completeDrainSawToken)
	require.False(t, IsDrainCounted(run), "CompleteDrain cleared the token")
	require.False(t, ctrlutil.ContainsFinalizer(run, testFinalizer))
}

func TestRunDrainStepProgressNotTerminalRequeues(t *testing.T) {
	run := newDrainRun(time.Minute)
	MarkDrainCounted(run)
	f := &fakeTarget{obj: run, progressTerminal: false}

	res, err := RunDrainStep(context.Background(), drainState(run, false, true), f, testRequeue)
	require.NoError(t, err)
	require.Equal(t, testRequeue, res.RequeueAfter, "not-terminal progress requeues")
	require.Equal(t, 1, f.progressCalls)
	require.Zero(t, f.completeDrainCalls, "must not finalize while non-terminal")
	require.True(t, ctrlutil.ContainsFinalizer(run, testFinalizer))
}

func TestRunDrainStepRequestCancelErrorPropagates(t *testing.T) {
	run := newDrainRun(time.Minute)
	f := &fakeTarget{obj: run, requestCancelErr: errors.New("update failed")}

	res, err := RunDrainStep(context.Background(), drainState(run, false, true), f, testRequeue)
	require.Error(t, err)
	require.Equal(t, time.Duration(0), res.RequeueAfter)
	// Failed RequestCancel → no token stamped, so no gauge increment is owed.
	require.False(t, IsDrainCounted(run))
	require.Zero(t, f.completeDrainCalls)
}

func TestRunDrainStepProgressErrorPropagates(t *testing.T) {
	run := newDrainRun(time.Minute)
	MarkDrainCounted(run)
	f := &fakeTarget{obj: run, progressErr: errors.New("engine failed")}

	res, err := RunDrainStep(context.Background(), drainState(run, false, true), f, testRequeue)
	require.Error(t, err)
	require.Equal(t, time.Duration(0), res.RequeueAfter)
	require.Zero(t, f.completeDrainCalls, "must not finalize on Progress error")
}

func TestRunDrainStepCompleteDrainErrorKeepsToken(t *testing.T) {
	run := newDrainRun(time.Minute)
	MarkDrainCounted(run)
	f := &fakeTarget{obj: run, completeDrainErr: errors.New("finalize failed")}

	// finish returns the error; the token must persist (CompleteDrain failed) so
	// a retried finish re-reads it and decrements the gauge exactly once.
	res, err := RunDrainStep(context.Background(), drainState(run, true, false), f, testRequeue)
	require.Error(t, err)
	require.Equal(t, time.Duration(0), res.RequeueAfter)
	require.True(t, f.completeDrainSawToken, "token was present when CompleteDrain ran")
	require.True(t, IsDrainCounted(run), "token must persist when CompleteDrain fails")
	// CompleteDrain saw the token but did NOT commit, so no decrement happened yet.
	require.Zero(t, f.gaugeDec, "errored finish must not decrement the gauge")
}

// TestRunDrainStepFullLifecycleRetriedFinishNetsZero drives a complete drain with
// a fail-then-retry at finish, pinning the active-drain gauge to a net delta of
// zero achieved by EXACTLY one increment and EXACTLY one decrement — even though
// finish() (and thus CompleteDrain) runs twice. This closes the gap the earlier
// keeps-token test only half-exercised: it now clears the CompleteDrain error and
// re-invokes RunDrainStep to prove the retry decrements exactly once (not zero,
// not twice).
func TestRunDrainStepFullLifecycleRetriedFinishNetsZero(t *testing.T) {
	run := newDrainRun(time.Minute)
	f := &fakeTarget{obj: run}
	st := drainState(run, false, true)

	// First loop: RequestCancel stamps the token and IncDrainActive fires once.
	res, err := RunDrainStep(context.Background(), st, f, testRequeue)
	require.NoError(t, err)
	require.Equal(t, testRequeue, res.RequeueAfter, "first loop requeues")
	require.Equal(t, 1, f.requestCancelCalls)
	require.True(t, IsDrainCounted(run), "first loop stamped the token")
	require.Equal(t, 1, f.gaugeInc, "first loop increments the gauge exactly once")
	require.Zero(t, f.gaugeDec, "no decrement until finish commits")

	// Drive toward terminal, but make the finalizing CompleteDrain error once.
	f.progressTerminal = true
	f.completeDrainErr = errors.New("finalize failed")
	res, err = RunDrainStep(context.Background(), st, f, testRequeue)
	require.Error(t, err, "finish surfaces the CompleteDrain error")
	require.Equal(t, time.Duration(0), res.RequeueAfter)
	require.Equal(t, 1, f.progressCalls, "progressed to terminal")
	require.Equal(t, 1, f.completeDrainCalls, "first finish attempted CompleteDrain")
	require.True(t, f.completeDrainSawToken, "token present when CompleteDrain ran")
	require.True(t, IsDrainCounted(run), "errored CompleteDrain leaves the token persisted")
	// Critical: the errored finish must NOT have decremented. The gauge still owes
	// exactly one decrement.
	require.Zero(t, f.gaugeDec, "errored finish decrements zero times")
	require.Equal(t, 1, f.gaugeInc, "increment count unchanged by the errored finish")

	// Retry: clear the error and re-invoke. The still-present token drives finish
	// to decrement exactly once (the token is then cleared) and the drain
	// completes.
	f.completeDrainErr = nil
	res, err = RunDrainStep(context.Background(), st, f, testRequeue)
	require.NoError(t, err, "retry completes the drain")
	require.Equal(t, time.Duration(0), res.RequeueAfter)
	// Still terminal-on-arrival (IsTerminal would be false here since terminal was
	// never set; this loop is the subsequent-loop Progress path), so Progress runs
	// again and reports terminal, then finish commits.
	require.Equal(t, 2, f.progressCalls, "retry re-progressed (idempotent) to terminal")
	require.Equal(t, 2, f.completeDrainCalls, "finish ran twice across the retry")
	require.False(t, IsDrainCounted(run), "committing CompleteDrain cleared the token")
	require.False(t, ctrlutil.ContainsFinalizer(run, testFinalizer), "drain finalizer removed")

	// The whole point: across a finish() that ran twice, the gauge moved by
	// exactly +1 then -1 — net zero, with exactly one inc and exactly one dec.
	require.Equal(t, 1, f.gaugeInc, "exactly one increment on the first loop")
	require.Equal(t, 1, f.gaugeDec, "exactly one decrement across the retried finish")
	require.Equal(t, f.gaugeInc, f.gaugeDec, "net active-drain gauge delta is zero")
}

// TestRunDrainStepNeverStartedNoGaugeMovement pins the never-started path to
// inc==0/dec==0: no token is stamped, so finish() must never touch the gauge.
func TestRunDrainStepNeverStartedNoGaugeMovement(t *testing.T) {
	run := newDrainRun(time.Minute)
	f := &fakeTarget{obj: run}

	_, err := RunDrainStep(context.Background(), drainState(run, false, false), f, testRequeue)
	require.NoError(t, err)
	require.Equal(t, 1, f.markKilledCalls)
	require.Equal(t, 1, f.completeDrainCalls)
	require.False(t, IsDrainCounted(run), "never-started stamps no token")
	require.False(t, f.completeDrainSawToken)
	// No token was ever stamped → the gauge never moved.
	require.Zero(t, f.gaugeInc, "never-started increments zero times")
	require.Zero(t, f.gaugeDec, "never-started decrements zero times")
}

// TestRunDrainStepTerminalOnArrivalNoGaugeMovement pins the terminal-on-arrival
// path to inc==0/dec==0: the run finalizes immediately with no token, so finish()
// must never touch the gauge.
func TestRunDrainStepTerminalOnArrivalNoGaugeMovement(t *testing.T) {
	run := newDrainRun(time.Minute)
	f := &fakeTarget{obj: run}

	_, err := RunDrainStep(context.Background(), drainState(run, true, false), f, testRequeue)
	require.NoError(t, err)
	require.Equal(t, 1, f.completeDrainCalls)
	require.Zero(t, f.requestCancelCalls, "terminal-on-arrival does not request cancel")
	require.False(t, f.completeDrainSawToken, "no token present on arrival")
	// No token → no gauge movement on either side.
	require.Zero(t, f.gaugeInc, "terminal-on-arrival increments zero times")
	require.Zero(t, f.gaugeDec, "terminal-on-arrival decrements zero times")
}

func TestCascadeDrainTimeout(t *testing.T) {
	require.Equal(t, 24*time.Hour, CascadeDrainTimeout)
}

func TestDrainCountedRoundTrip(t *testing.T) {
	obj := &v2pb.PipelineRun{ObjectMeta: metav1.ObjectMeta{Name: "run"}}

	// Absent by default.
	require.False(t, IsDrainCounted(obj))

	// Mark stamps the token even when annotations is nil.
	MarkDrainCounted(obj)
	require.True(t, IsDrainCounted(obj))
	require.Equal(t, "true", obj.GetAnnotations()[DrainCountedAnnotation])

	// Mark is idempotent.
	MarkDrainCounted(obj)
	require.True(t, IsDrainCounted(obj))

	// Clear removes the token.
	ClearDrainCounted(obj)
	require.False(t, IsDrainCounted(obj))
	_, ok := obj.GetAnnotations()[DrainCountedAnnotation]
	require.False(t, ok)

	// Clear on an object with no annotations is a safe no-op.
	bare := &v2pb.PipelineRun{ObjectMeta: metav1.ObjectMeta{Name: "bare"}}
	require.NotPanics(t, func() { ClearDrainCounted(bare) })
	require.False(t, IsDrainCounted(bare))
}

func TestMarkDrainCountedPreservesOtherAnnotations(t *testing.T) {
	obj := &v2pb.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "run",
			Annotations: map[string]string{"keep": "me"},
		},
	}
	MarkDrainCounted(obj)
	require.Equal(t, "me", obj.GetAnnotations()["keep"])
	require.True(t, IsDrainCounted(obj))

	ClearDrainCounted(obj)
	require.Equal(t, "me", obj.GetAnnotations()["keep"])
	require.False(t, IsDrainCounted(obj))
}
