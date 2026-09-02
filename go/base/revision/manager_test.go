package revision

import (
	"context"
	"testing"

	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/michelangelo-ai/michelangelo/go/api"
	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	apiutils "github.com/michelangelo-ai/michelangelo/go/api/utils"
)

func newTestManager(t *testing.T) (Manager, api.Handler, *runtime.Scheme) {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, v2pb.AddToScheme(scheme))
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	handler := apiHandler.NewFakeAPIHandler(k8sClient)
	return NewManager(handler, scheme, zaptest.NewLogger(t)), handler, scheme
}

func testBaseCR() *v2pb.Pipeline {
	return &v2pb.Pipeline{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "michelangelo.api/v2",
			Kind:       "Pipeline",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-pipeline",
			Namespace: "test-ns",
		},
	}
}

func testInput() UpsertParams {
	return UpsertParams{
		Name:       "pipeline-my-pipeline-abc123456789",
		BaseCR:     testBaseCR(),
		Owner:      "owner",
		RevisionID: "abc123456789",
		Source:     SourceGit,
		GitRef:     "abc123456789",
		GitBranch:  "main",
	}
}

func getRevision(t *testing.T, h api.Handler, namespace, name string) *v2pb.Revision {
	t.Helper()
	rev := &v2pb.Revision{}
	require.NoError(t, h.Get(context.Background(), namespace, name, &metav1.GetOptions{}, rev))
	return rev
}

func TestBuildRevision(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v2pb.AddToScheme(scheme))

	input := testInput()
	input.Labels = map[string]string{"env": "test"}
	input.Parent = &apipb.ResourceIdentifier{Namespace: "test-ns", Name: "prev-rev"}

	rev, err := buildRevision(input, scheme)
	require.NoError(t, err)

	assert.Equal(t, "pipeline-my-pipeline-abc123456789", rev.Name)
	assert.Equal(t, "test-ns", rev.Namespace)
	assert.Equal(t, "Revision", rev.TypeMeta.Kind)
	assert.Equal(t, "michelangelo.api/v2", rev.TypeMeta.APIVersion)

	assert.Equal(t, "Pipeline", rev.Spec.BaseType.Kind)
	assert.Equal(t, "michelangelo.api/v2", rev.Spec.BaseType.APIVersion)
	assert.Equal(t, "test-ns", rev.Spec.BaseResource.Namespace)
	assert.Equal(t, "my-pipeline", rev.Spec.BaseResource.Name)
	assert.NotNil(t, rev.Spec.Content)
	assert.Equal(t, "owner", rev.Spec.Owner.Name)
	assert.Equal(t, "abc123456789", rev.Spec.RevisionId)
	assert.Equal(t, SourceGit, rev.Spec.Source)
	assert.Equal(t, "abc123456789", rev.Spec.GitCommit.GitRef)
	assert.Equal(t, "main", rev.Spec.GitCommit.Branch)
	assert.Equal(t, "prev-rev", rev.Spec.Parent.Name)
	assert.Equal(t, map[string]string{"env": "test"}, rev.Labels)
}

func TestBuildRevision_NoGitInfo(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v2pb.AddToScheme(scheme))

	input := testInput()
	input.GitRef = ""
	input.GitBranch = ""

	rev, err := buildRevision(input, scheme)
	require.NoError(t, err)
	assert.Nil(t, rev.Spec.GitCommit)
}

func TestUpsertRevision_Create(t *testing.T) {
	mgr, h, _ := newTestManager(t)
	ctx := context.Background()

	created, err := mgr.UpsertRevision(ctx, testInput(), UpsertOpts{})
	require.NoError(t, err)
	assert.True(t, created)

	rev := getRevision(t, h, "test-ns", "pipeline-my-pipeline-abc123456789")
	assert.Equal(t, "abc123456789", rev.Spec.RevisionId)
	assert.Equal(t, "Pipeline", rev.Spec.BaseType.Kind)
}

func TestUpsertRevision_CreateImmutable(t *testing.T) {
	mgr, h, _ := newTestManager(t)
	ctx := context.Background()

	created, err := mgr.UpsertRevision(ctx, testInput(), UpsertOpts{Immutable: true})
	require.NoError(t, err)
	assert.True(t, created)

	rev := getRevision(t, h, "test-ns", "pipeline-my-pipeline-abc123456789")
	assert.True(t, apiutils.IsImmutable(rev))
}

func TestUpsertRevision_DedupImmutable(t *testing.T) {
	mgr, _, _ := newTestManager(t)
	ctx := context.Background()

	_, err := mgr.UpsertRevision(ctx, testInput(), UpsertOpts{Immutable: true})
	require.NoError(t, err)

	created, err := mgr.UpsertRevision(ctx, testInput(), UpsertOpts{Immutable: true})
	require.NoError(t, err)
	assert.False(t, created, "second upsert of immutable revision should be a no-op")
}

func TestUpsertRevision_RejectImmutableToMutable(t *testing.T) {
	mgr, _, _ := newTestManager(t)
	ctx := context.Background()

	_, err := mgr.UpsertRevision(ctx, testInput(), UpsertOpts{Immutable: true})
	require.NoError(t, err)

	_, err = mgr.UpsertRevision(ctx, testInput(), UpsertOpts{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot update immutable revision")
}

func TestUpsertRevision_UpdateMutable(t *testing.T) {
	mgr, _, _ := newTestManager(t)
	ctx := context.Background()

	_, err := mgr.UpsertRevision(ctx, testInput(), UpsertOpts{})
	require.NoError(t, err)

	updated, err := mgr.UpsertRevision(ctx, testInput(), UpsertOpts{})
	require.NoError(t, err)
	assert.True(t, updated)
}

func TestUpsertRevision_MutableThenImmutable(t *testing.T) {
	mgr, h, _ := newTestManager(t)
	ctx := context.Background()

	_, err := mgr.UpsertRevision(ctx, testInput(), UpsertOpts{})
	require.NoError(t, err)

	updated, err := mgr.UpsertRevision(ctx, testInput(), UpsertOpts{Immutable: true})
	require.NoError(t, err)
	assert.True(t, updated)

	rev := getRevision(t, h, "test-ns", "pipeline-my-pipeline-abc123456789")
	assert.True(t, apiutils.IsImmutable(rev))
}
