package revision

import (
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// UpsertParams carries the caller-facing fields needed to create or update
// a Revision. The Manager builds the full v2 Revision proto internally —
// callers never import or construct a *v2pb.Revision directly.
//
// The caller owns Content preparation: BaseCR is marshaled as-is, so strip
// any unwanted metadata (e.g. ManagedFields) before passing it in.
type UpsertParams struct {
	// Name is the Revision object's metadata name. The caller controls the
	// naming scheme (e.g. "pipeline-foo-abc123", "my-model-42").
	Name string

	// BaseCR is the resource being revisioned. The manager derives Namespace,
	// BaseType (Kind/APIVersion), BaseResource (namespace/name), and Content
	// (proto-marshaled) from this object.
	BaseCR client.Object

	// Owner is the username recorded as the revision owner.
	Owner string

	// RevisionID is a caller-chosen identifier for this revision point
	// (git SHA, generation number, semver string, etc.).
	RevisionID string

	// Labels applied to the Revision's ObjectMeta.
	Labels map[string]string

	// Annotations applied to the Revision's ObjectMeta.
	Annotations map[string]string

	// Source indicates how the revision was produced (e.g. SourceGit).
	Source string

	// GitRef populates CommitInfo.GitRef (e.g. a commit SHA).
	GitRef string

	// GitBranch populates CommitInfo.Branch.
	GitBranch string

	// Parent points to the previous revision, if any.
	Parent *apipb.ResourceIdentifier
}
