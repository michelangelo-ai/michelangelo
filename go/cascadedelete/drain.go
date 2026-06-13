// Package cascadedelete drives graceful cascade deletion of a parent's child
// runs: it stamps owner references so Kubernetes foreground GC removes children
// with their parent, drains each child's in-flight work before removal,
// optionally retains the child's final state in MySQL, and emits drain metrics.
//
// Per-kind specifics (finalizer string, metric label, engine teardown) live in
// the consuming controllers; the set of retained kinds is injected as a
// RetainPolicy at the composition root.
package cascadedelete

import (
	"context"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// CascadeDrainTimeout caps how long a child's drain waits for its workflow to
// stop before the finalizer is removed and GC proceeds anyway. It is a
// last-resort safety valve (the graceful cancel was already requested on the
// first drain loop), hard-coded rather than configurable so a too-short value
// can't orphan live compute.
const CascadeDrainTimeout = 24 * time.Hour

// DrainCountedAnnotation is a persisted, single-use token that makes active-drain
// gauge accounting idempotent across reconcile retries: it is written in the same
// update that begins the drain and cleared in the same update that removes the
// drain finalizer, so a retried completion decrements the gauge exactly once.
const DrainCountedAnnotation = "cascade.michelangelo.uber.com/drain-counted"

// MarkDrainCounted stamps the drain-counted token. Call it in the update that
// begins the drain, then call IncDrainActive only after that update succeeds.
func MarkDrainCounted(obj client.Object) {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[DrainCountedAnnotation] = "true"
	obj.SetAnnotations(annotations)
}

// IsDrainCounted reports whether the drain-counted token is present.
func IsDrainCounted(obj client.Object) bool {
	return obj.GetAnnotations()[DrainCountedAnnotation] == "true"
}

// ClearDrainCounted removes the drain-counted token. Call DecDrainActive only
// after the containing update succeeds, so the decrement happens exactly once.
func ClearDrainCounted(obj client.Object) {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		return
	}
	delete(annotations, DrainCountedAnnotation)
	obj.SetAnnotations(annotations)
}

// DrainState is the read-only snapshot RunDrainStep needs about the child being
// drained. The controller fills it from the freshly-fetched object at call time.
type DrainState struct {
	Object      client.Object // the child being drained
	Kind        string        // metric label (a stable snake_case string)
	Finalizer   string        // this kind's drain finalizer string
	IsTerminal  bool          // run is already in a terminal state
	WorkStarted bool          // the underlying workflow actually started
}

// DrainTarget performs the kind-specific, persisted transitions of a drain. Each
// method mutates the child and persists the change via the controller's client;
// RunDrainStep itself holds no client.
type DrainTarget interface {
	// RequestCancel begins cancellation AND stamps MarkDrainCounted in ONE
	// persisted update.
	RequestCancel(ctx context.Context) error
	// Progress advances an in-flight cancellation by one step and persists
	// status, returning whether the run is now terminal. For a synchronous
	// engine it MUST re-issue the idempotent kill (not merely re-read state) so
	// a failed prior status-update cannot wedge the drain.
	Progress(ctx context.Context) (terminal bool, err error)
	// MarkKilled sets the run terminal (KILLED) without engine work and persists
	// status. It must NOT stamp the drain-counted token.
	MarkKilled(ctx context.Context) error
	// ForceKill performs best-effort engine teardown on timeout (e.g. delete a
	// Temporal schedule). Errors are logged and swallowed by the driver.
	ForceKill(ctx context.Context) error
	// CompleteDrain finalizes in ONE persisted metadata update: it clears the
	// drain-counted token and removes the drain finalizer (and, for kinds that
	// retain, marks the object immutable first).
	CompleteDrain(ctx context.Context) error
}

// RunDrainStep runs one step of the drain lifecycle for a child that GC has
// marked for deletion. It reads only from st and writes only through DrainTarget
// methods, so it holds no client.
//
// The flow is:
//   - no drain finalizer of ours: nothing to do, let GC proceed.
//   - run already terminal: no in-flight work, finalize immediately.
//   - drain has run past the safety timeout: best-effort engine teardown, then
//     finalize regardless of state so GC cannot wedge forever.
//   - run active but its workflow never started: there is nothing to cancel —
//     drive it straight to terminal KILLED (no token) and finalize.
//   - first drain loop (no drain-counted token yet): request cancellation
//     atomically with the token, count the active drain, and requeue.
//   - subsequent loop: progress the cancellation; finalize when terminal, else
//     requeue.
func RunDrainStep(ctx context.Context, st DrainState, t DrainTarget, requeue time.Duration) (ctrl.Result, error) {
	obj := st.Object

	// Not our finalizer; let GC and any other finalizers handle removal.
	if !ctrlutil.ContainsFinalizer(obj, st.Finalizer) {
		return ctrl.Result{}, nil
	}

	// Already terminal: no workflow to cancel, so drain is instantaneous.
	if st.IsTerminal {
		return ctrl.Result{}, finish(ctx, st, t)
	}

	// Safety timeout: the graceful cancel was already requested on the first
	// drain loop; after the cascade drain timeout, best-effort tear down the
	// engine and force-complete so GC can proceed.
	if ts := obj.GetDeletionTimestamp(); ts != nil && time.Since(ts.Time) >= CascadeDrainTimeout {
		IncDrainTimeout(st.Kind)
		_ = t.ForceKill(ctx) // best-effort teardown; errors are swallowed.
		return ctrl.Result{}, finish(ctx, st, t)
	}

	// Active run whose workflow never started: there is nothing to cancel, and
	// engaging the engine could start a brand-new workflow on a run GC is
	// deleting. Drive it straight to terminal KILLED and finalize. No
	// active-drain token is recorded because the gauge is never incremented.
	if !st.WorkStarted {
		if err := t.MarkKilled(ctx); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, finish(ctx, st, t)
	}

	// First drain loop: request workflow cancellation once, recording the
	// drain-counted token in the same persisted update, then count the drain as
	// active and requeue.
	if !IsDrainCounted(obj) {
		if err := t.RequestCancel(ctx); err != nil {
			return ctrl.Result{}, err
		}
		IncDrainActive(st.Kind)
		return ctrl.Result{RequeueAfter: requeue}, nil
	}

	// Cancellation already requested: progress it toward a terminal state.
	terminal, err := t.Progress(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}
	if terminal {
		return ctrl.Result{}, finish(ctx, st, t)
	}
	return ctrl.Result{RequeueAfter: requeue}, nil
}

// finish finalizes a drained child. Gauge accounting is idempotent via the
// persisted drain-counted token: counted is captured BEFORE CompleteDrain clears
// the token, and DecDrainActive runs only AFTER CompleteDrain commits — so a
// retried finish re-reads the still-present token and decrements exactly once,
// while a child terminal-on-arrival (no token) is never decremented.
func finish(ctx context.Context, st DrainState, t DrainTarget) error {
	counted := IsDrainCounted(st.Object)
	if err := t.CompleteDrain(ctx); err != nil {
		// The token (if any) is still persisted because CompleteDrain failed,
		// so a retry will re-read it and decrement exactly once on success.
		return err
	}
	if ts := st.Object.GetDeletionTimestamp(); ts != nil {
		ObserveDrainDuration(st.Kind, time.Since(ts.Time).Seconds())
	}
	if counted {
		DecDrainActive(st.Kind)
	}
	return nil
}
