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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	revisionAPIVersion = "michelangelo.api/v2"
	revisionKind       = "Revision"

	// Label keys applied to every Revision CR.
	LabelBaseResourceNamespace = "base_resource_namespace"
	LabelBaseResourceName      = "base_resource_name"
	LabelBaseType              = "base_type"
)

// NewRevision builds a Revision CR from the given params, applying the conventional
// TypeMeta, ObjectMeta (with cleanup labels), and Spec fields. Returns an error
// if params.Content fails to marshal.
func NewRevision(params UpsertRevisionParams) (*v2pb.Revision, error) {
	content, err := pbtypes.MarshalAny(params.Content)
	if err != nil {
		return nil, fmt.Errorf("marshal revision content: %w", err)
	}

	labels := make(map[string]string, len(params.Labels)+3)
	for k, v := range params.Labels {
		labels[k] = v
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
