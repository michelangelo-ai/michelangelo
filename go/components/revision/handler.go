// Package revision implements a Kubernetes controller for managing Revision resources.
//
// The controller watches Revision custom resources and dispatches lifecycle
// reconciliation to entity-type-specific handlers registered via FX groups.
// Each handler owns the lifecycle logic for revisions produced by a particular
// resource type (e.g. Pipeline).
package revision

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// Handler reconciles the lifecycle of Revision CRs produced by a specific
// resource type. Implementations are registered via the FX group
// "revision-handler" and dispatched by the controller based on
// Revision.Spec.BaseType.
type Handler interface {
	// Reconcile is called for each Revision whose Spec.BaseType matches this
	// handler's TypeMeta. Implementations may mutate rev.Status; the controller
	// persists any changes after Reconcile returns.
	Reconcile(ctx context.Context, rev *v2pb.Revision) error

	// TypeMeta returns the APIVersion and Kind of the owning resource type
	// (e.g. {APIVersion: "michelangelo.api/v2", Kind: "Pipeline"}).
	// Used as the dispatch key against Revision.Spec.BaseType.
	TypeMeta() metav1.TypeMeta
}
