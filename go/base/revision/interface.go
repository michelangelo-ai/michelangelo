package revision

import (
	"context"

	gogoproto "github.com/gogo/protobuf/proto"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Manager handles the full lifecycle of Revision CRs: create-or-update with
// immutability semantics, lookup, and bulk deletion. Controllers that produce
// revisions depend on this interface rather than calling the API handler directly.
type Manager interface {
	// UpsertRevision creates or updates a Revision for the given params.
	// Returns (true, nil) on create, (false, nil) when the revision already
	// exists and is immutable (dedup), or on a successful update.
	UpsertRevision(ctx context.Context, params UpsertRevisionParams) (bool, error)

	// GetRevision retrieves a Revision by namespace and name.
	GetRevision(ctx context.Context, namespace, name string) (*v2pb.Revision, error)

	// FetchRevisionID retrieves the RevisionId field from an existing Revision.
	FetchRevisionID(ctx context.Context, namespace, name string) (string, error)

	// DeleteAllRevisions deletes all Revisions for the given base resource.
	DeleteAllRevisions(ctx context.Context, namespace, name, kind string) error
}

// UpsertRevisionParams describes a Revision to create or update.
type UpsertRevisionParams struct {
	RevisionName string
	RevisionID   string
	// ParentRevisionName is the name of the revision this one was forked from. Optional.
	ParentRevisionName *string
	// Content is the base resource being revisioned, serialized into RevisionSpec.Content.
	Content      gogoproto.Message
	Owner        *v2pb.UserInfo
	BaseType     *metav1.TypeMeta
	BaseResource *apipb.ResourceIdentifier
	// Source identifies what created this revision. Use one of the
	// Source* constants in this package or a domain-namespaced value.
	Source string
	// GitCommit holds the commit info for git-backed revisions.
	// Conventionally set when Source == SourceGit.
	GitCommit   *v2pb.CommitInfo
	Immutable   bool
	Labels      map[string]string
	Annotations map[string]string
}
