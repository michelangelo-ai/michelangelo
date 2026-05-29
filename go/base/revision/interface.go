package revision

import (
	"context"

	"go.uber.org/yarpc"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Manager handles the Revision CR lifecycle with immutability semantics.
// Controllers that produce revisions depend on this interface rather than
// calling the API handler directly.
//
// Both the OSS implementation and the internal (Uber) implementation satisfy
// this interface. At mergeback the internal binary swaps the implementation
// via fx.Decorate; the call-sites in each controller remain unchanged.
//
// UpsertRevision follows the controller-runtime caller-owns-type pattern:
// callers build a fully populated Revision (in whatever API version they use)
// and pass it as a client.Object. The Manager handles the get-or-create state
// machine without inspecting version-specific fields.
//
// Read and delete operations (Get, DeleteCollection) are thin wrappers over
// handler methods, so controllers use the handler directly for those.
type Manager interface {
	// UpsertRevision creates or updates a Revision. The caller builds the
	// complete Revision object; the Manager orchestrates the state machine
	// (get existing → create if absent → check immutability → update if mutable).
	// Returns (true, nil) on create, (false, nil) on dedup or update.
	UpsertRevision(ctx context.Context, rev client.Object, opts UpsertOpts, options ...yarpc.CallOption) (bool, error)
}

// UpsertOpts carries state-machine knobs for UpsertRevision. The Revision
// content itself lives in the client.Object the caller passes directly.
type UpsertOpts struct {
	Immutable bool
}
