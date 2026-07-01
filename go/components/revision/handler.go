// Package revision implements a Kubernetes controller for managing Revision resources.
//
// The controller watches Revision custom resources and dispatches lifecycle
// reconciliation to entity-type-specific handlers registered via FX groups.
// Each handler owns the lifecycle logic for revisions produced by a particular
// resource type (e.g. Pipeline).
package revision

import (
	"context"

	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

// Handler reconciles the lifecycle of Revision CRs produced by a specific
// resource type. Implementations are registered via the FX group
// "revision-handler" and dispatched by the controller based on
// Revision.Spec.BaseType.
type Handler interface {
	// Reconcile is called for each Revision whose Spec.BaseType matches this
	// handler's TypeMeta. Implementations may mutate rev.Status; the controller
	// persists any changes after Reconcile returns. The returned Result controls
	// requeue behavior (e.g. RequeueAfter for polling).
	Reconcile(ctx context.Context, rev *v2pb.Revision) (ctrl.Result, error)

	// TypeMeta returns the dispatch key matched against Revision.Spec.BaseType
	// (e.g. {APIVersion: "michelangelo.api/v2", Kind: "Pipeline"}).
	TypeMeta() metav1.TypeMeta
}
