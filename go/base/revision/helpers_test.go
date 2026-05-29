package revision

import (
	"testing"

	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNewRevision(t *testing.T) {
	parent := "parent-rev"
	params := UpsertRevisionParams{
		RevisionName:       "pipeline-my-pipeline-abc123",
		RevisionID:         "abc123",
		ParentRevisionName: &parent,
		Content:            &v2pb.UserInfo{Name: "test-content"},
		Owner:              &v2pb.UserInfo{Name: "owner"},
		BaseType:           &metav1.TypeMeta{Kind: "Pipeline", APIVersion: "michelangelo.api/v2"},
		BaseResource:       &apipb.ResourceIdentifier{Namespace: "test-ns", Name: "my-pipeline"},
		Source:             SourceGit,
		GitCommit:          &v2pb.CommitInfo{GitRef: "abc123"},
		Annotations:        map[string]string{"a": "1"},
	}

	rev, err := NewRevision(params)
	require.NoError(t, err)
	assert.Equal(t, "pipeline-my-pipeline-abc123", rev.Name)
	assert.Equal(t, "test-ns", rev.Namespace)
	assert.Equal(t, map[string]string{"a": "1"}, rev.Annotations)
	assert.Equal(t, "abc123", rev.Spec.RevisionId)
	assert.Equal(t, SourceGit, rev.Spec.Source)
	assert.Equal(t, "abc123", rev.Spec.GitCommit.GitRef)
	assert.Equal(t, "my-pipeline", rev.Spec.BaseResource.Name)
	assert.Equal(t, "Pipeline", rev.Spec.BaseType.Kind)
	require.NotNil(t, rev.Spec.Parent)
	assert.Equal(t, parent, rev.Spec.Parent.Name)
	require.NotNil(t, rev.Spec.Content)
}
