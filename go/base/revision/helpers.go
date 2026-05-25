// Package revision provides building blocks for producing Revision CRs.
//
// Per-controller Revisioner implementations (see go/components/<controller>/revisioner.go)
// use these helpers to keep Revision CR shape — labels, source values, content format —
// consistent across producers.
package revision

import (
	"fmt"

	pbtypes "github.com/gogo/protobuf/types"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	revisionAPIVersion = "michelangelo.api/v2"
	revisionKind       = "Revision"

	// Label keys applied to every Revision CR. Used by LabelSelectorFor to query
	// revisions for a specific base resource.
	LabelBaseResourceNamespace = "base_resource_namespace"
	LabelBaseResourceName      = "base_resource_name"
	LabelBaseType              = "base_type"
)

// NewCR builds a Revision CR from the given params, applying the conventional
// TypeMeta, ObjectMeta (with cleanup labels), and Spec fields. Returns an error
// if params.Content fails to marshal.
func NewCR(params UpsertRevisionParams) (*v2pb.Revision, error) {
	content, err := pbtypes.MarshalAny(params.Content)
	if err != nil {
		return nil, fmt.Errorf("marshal revision content: %w", err)
	}

	labels := params.Labels
	if labels == nil {
		labels = map[string]string{}
	}
	labels[LabelBaseResourceNamespace] = params.BaseResource.Namespace
	labels[LabelBaseResourceName] = params.BaseResource.Name
	labels[LabelBaseType] = params.BaseType.Kind

	rev := &v2pb.Revision{
		TypeMeta: metav1.TypeMeta{
			APIVersion: revisionAPIVersion,
			Kind:       revisionKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        params.RevisionName,
			Namespace:   params.BaseResource.Namespace,
			Labels:      labels,
			Annotations: params.Annotations,
		},
		Spec: v2pb.RevisionSpec{
			BaseType:     params.BaseType,
			BaseResource: params.BaseResource,
			Content:      content,
			Owner:        params.Owner,
			RevisionId:   params.RevisionID,
			Source:       params.Source,
			GitCommit:    params.GitCommit,
		},
	}

	if params.ParentRevisionName != nil {
		rev.Spec.Parent = &apipb.ResourceIdentifier{
			Namespace: params.BaseResource.Namespace,
			Name:      *params.ParentRevisionName,
		}
	}

	return rev, nil
}

// LabelSelectorFor returns a label selector matching all Revisions for the
// given base resource. Used to clean up revisions when a base resource is deleted.
func LabelSelectorFor(namespace, resourceName, resourceKind string) string {
	return fmt.Sprintf(
		"%s=%s,%s=%s,%s=%s",
		LabelBaseResourceNamespace, namespace,
		LabelBaseResourceName, resourceName,
		LabelBaseType, resourceKind,
	)
}

// IsAlreadyExists reports whether err indicates the resource already exists,
// recognizing both grpc (codes.AlreadyExists) and k8s (k8serrors.IsAlreadyExists)
// error shapes. Used by Revisioner implementations to treat AlreadyExists as a
// no-op — Revisions are immutable, and a revision for this identity already exists.
func IsAlreadyExists(err error) bool {
	if s, ok := status.FromError(err); ok {
		return s.Code() == codes.AlreadyExists
	}
	return k8serrors.IsAlreadyExists(err)
}
