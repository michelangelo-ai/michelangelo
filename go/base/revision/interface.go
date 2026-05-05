package revision

import (
	gogoproto "github.com/gogo/protobuf/proto"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// UpsertRevisionParams describes a Revision to create. Used by per-controller
// Revisioner implementations to build a Revision CR via NewCR.
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
	Labels      map[string]string
	Annotations map[string]string
}
