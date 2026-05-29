package revision

import (
	"context"

	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"go.uber.org/yarpc"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Manager handles the Revision CR lifecycle: create-or-update with immutability
// semantics, lookup, and bulk cleanup. Controllers that produce revisions depend
// on this interface rather than calling the API handler directly.
//
// Both the OSS implementation and the internal (Uber) implementation satisfy
// this interface. At mergeback the internal binary swaps the implementation
// via fx.Decorate; the call-sites in each controller remain unchanged.
//
// UpsertRevision follows the controller-runtime caller-owns-type pattern:
// callers build a fully populated Revision (in whatever API version they use)
// and pass it as a client.Object. The Manager handles the get-or-create state
// machine without inspecting version-specific fields.
type Manager interface {
	// UpsertRevision creates or updates a Revision. The caller builds the
	// complete Revision object; the Manager orchestrates the state machine
	// (get existing → create if absent → check immutability → update if mutable).
	// Returns (true, nil) on create, (false, nil) on dedup or update.
	UpsertRevision(ctx context.Context, rev client.Object, opts UpsertOpts, options ...yarpc.CallOption) (bool, error)

	// CheckRevision reports whether a Revision with the given namespace and
	// name exists. It does not log an error when the Revision is absent.
	CheckRevision(ctx context.Context, namespace, name string, options ...yarpc.CallOption) (bool, error)

	// GetRevision retrieves a single Revision by namespace and name.
	GetRevision(ctx context.Context, namespace, name string, options ...yarpc.CallOption) (*v2pb.Revision, error)

	// FetchRevisionID returns the RevisionID (spec.revision_id) for the named
	// Revision, or an error if the Revision does not exist.
	FetchRevisionID(ctx context.Context, namespace, name string, options ...yarpc.CallOption) (string, error)

	// DeleteAllRevisions removes every Revision owned by the given base
	// resource (identified by namespace, base resource name, and base type kind).
	DeleteAllRevisions(ctx context.Context, namespace, name, kind string, options ...yarpc.CallOption) error
}

// UpsertOpts carries state-machine knobs for UpsertRevision. The Revision
// content itself lives in the client.Object the caller passes directly.
type UpsertOpts struct {
	Immutable bool
}
