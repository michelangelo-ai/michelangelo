package cascadedelete

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// EnsureControllerRef makes owner the controller owner of child, idempotently,
// reporting whether it changed anything.
//
// It delegates to controller-runtime's SetControllerReference, which resolves the
// owner's APIVersion/Kind from the scheme rather than hard-coding a GroupVersion —
// letting the same helper work against any registered owner type unchanged. If
// child already carries a *different* controller, SetControllerReference returns
// an AlreadyOwnedError rather than silently producing an object with two
// controllers (which the API server rejects).
func EnsureControllerRef(child, owner client.Object, scheme *runtime.Scheme) (changed bool, err error) {
	// Short-circuit if already owned: avoids a no-op Update and a spurious
	// backfill-metric increment on every reconcile once stamped.
	if controller := metav1.GetControllerOf(child); controller != nil && controller.UID == owner.GetUID() {
		return false, nil
	}
	if err := ctrlutil.SetControllerReference(owner, child, scheme); err != nil {
		return false, err
	}
	return true, nil
}
