package revision

import (
	"errors"
	"testing"

	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestNewCR(t *testing.T) {
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

	rev, err := NewCR(params)
	require.NoError(t, err)
	assert.Equal(t, "pipeline-my-pipeline-abc123", rev.Name)
	assert.Equal(t, "test-ns", rev.Namespace)
	assert.Equal(t, map[string]string{"a": "1"}, rev.Annotations)
	// Cleanup labels are applied automatically.
	assert.Equal(t, "test-ns", rev.Labels[LabelBaseResourceNamespace])
	assert.Equal(t, "my-pipeline", rev.Labels[LabelBaseResourceName])
	assert.Equal(t, "Pipeline", rev.Labels[LabelBaseType])
	assert.Equal(t, "abc123", rev.Spec.RevisionId)
	assert.Equal(t, SourceGit, rev.Spec.Source)
	assert.Equal(t, "abc123", rev.Spec.GitCommit.GitRef)
	require.NotNil(t, rev.Spec.Parent)
	assert.Equal(t, parent, rev.Spec.Parent.Name)
	require.NotNil(t, rev.Spec.Content)
}

func TestNewCRMergesCallerLabels(t *testing.T) {
	rev, err := NewCR(UpsertRevisionParams{
		Content:      &v2pb.UserInfo{},
		BaseType:     &metav1.TypeMeta{Kind: "Pipeline"},
		BaseResource: &apipb.ResourceIdentifier{Namespace: "ns", Name: "p"},
		Labels:       map[string]string{"caller-key": "caller-val"},
	})
	require.NoError(t, err)
	assert.Equal(t, "caller-val", rev.Labels["caller-key"])
	// Cleanup labels still applied.
	assert.Equal(t, "ns", rev.Labels[LabelBaseResourceNamespace])
}

func TestLabelSelectorFor(t *testing.T) {
	got := LabelSelectorFor("test-ns", "my-pipeline", "Pipeline")
	assert.Equal(t,
		"base_resource_namespace=test-ns,base_resource_name=my-pipeline,base_type=Pipeline",
		got)
}

func TestIsAlreadyExists(t *testing.T) {
	t.Run("grpc AlreadyExists", func(t *testing.T) {
		assert.True(t, IsAlreadyExists(status.Error(codes.AlreadyExists, "exists")))
	})
	t.Run("k8s AlreadyExists", func(t *testing.T) {
		err := k8serrors.NewAlreadyExists(schema.GroupResource{Resource: "revisions"}, "rev")
		assert.True(t, IsAlreadyExists(err))
	})
	t.Run("unrelated error", func(t *testing.T) {
		assert.False(t, IsAlreadyExists(errors.New("oops")))
	})
}
