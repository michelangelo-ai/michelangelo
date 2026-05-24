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

	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	apiutils "github.com/michelangelo-ai/michelangelo/go/api/utils"
)

func newTestManager(t *testing.T) Manager {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, v2pb.AddToScheme(scheme))
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	return NewManager(apiHandler.NewFakeAPIHandler(k8sClient), zaptest.NewLogger(t))
}

func testParams() UpsertRevisionParams {
	return UpsertRevisionParams{
		RevisionName: "pipeline-my-pipeline-abc123456789",
		RevisionID:   "abc123456789",
		Content:      &v2pb.Pipeline{},
		Owner:        &v2pb.UserInfo{Name: "owner"},
		BaseType:     &metav1.TypeMeta{Kind: "Pipeline", APIVersion: "michelangelo.api/v2"},
		BaseResource: &apipb.ResourceIdentifier{Namespace: "test-ns", Name: "my-pipeline"},
		Source:       SourceGit,
		GitCommit:    &v2pb.CommitInfo{GitRef: "abc123456789", Branch: "main"},
	}
}

func TestUpsertRevision_Create(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()

	created, err := mgr.UpsertRevision(ctx, testParams())
	require.NoError(t, err)
	assert.True(t, created)

	rev, err := mgr.GetRevision(ctx, "test-ns", "pipeline-my-pipeline-abc123456789")
	require.NoError(t, err)
	assert.Equal(t, "abc123456789", rev.Spec.RevisionId)
	assert.Equal(t, "Pipeline", rev.Labels[LabelBaseType])
}

func TestUpsertRevision_CreateImmutable(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	params := testParams()
	params.Immutable = true

	created, err := mgr.UpsertRevision(ctx, params)
	require.NoError(t, err)
	assert.True(t, created)

	rev, err := mgr.GetRevision(ctx, "test-ns", "pipeline-my-pipeline-abc123456789")
	require.NoError(t, err)
	assert.True(t, apiutils.IsImmutable(rev))
}

func TestUpsertRevision_DedupImmutable(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	params := testParams()
	params.Immutable = true

	_, err := mgr.UpsertRevision(ctx, params)
	require.NoError(t, err)

	created, err := mgr.UpsertRevision(ctx, params)
	require.NoError(t, err)
	assert.False(t, created, "second upsert of immutable revision should be a no-op")
}

func TestUpsertRevision_RejectImmutableToMutable(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	params := testParams()
	params.Immutable = true
	_, err := mgr.UpsertRevision(ctx, params)
	require.NoError(t, err)

	params.Immutable = false
	_, err = mgr.UpsertRevision(ctx, params)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot update immutable revision")
}

func TestUpsertRevision_UpdateMutable(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()

	_, err := mgr.UpsertRevision(ctx, testParams())
	require.NoError(t, err)

	params := testParams()
	params.Labels = map[string]string{"extra": "label"}
	created, err := mgr.UpsertRevision(ctx, params)
	require.NoError(t, err)
	assert.False(t, created)

	rev, err := mgr.GetRevision(ctx, "test-ns", "pipeline-my-pipeline-abc123456789")
	require.NoError(t, err)
	assert.Equal(t, "label", rev.Labels["extra"])
}

func TestUpsertRevision_MutableThenImmutable(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()

	_, err := mgr.UpsertRevision(ctx, testParams())
	require.NoError(t, err)

	params := testParams()
	params.Immutable = true
	created, err := mgr.UpsertRevision(ctx, params)
	require.NoError(t, err)
	assert.False(t, created)

	rev, err := mgr.GetRevision(ctx, "test-ns", "pipeline-my-pipeline-abc123456789")
	require.NoError(t, err)
	assert.True(t, apiutils.IsImmutable(rev))
}

func TestGetRevision_NotFound(t *testing.T) {
	mgr := newTestManager(t)
	_, err := mgr.GetRevision(context.Background(), "test-ns", "does-not-exist")
	assert.Error(t, err)
}

func TestGetRevision_EmptyNamespace(t *testing.T) {
	mgr := newTestManager(t)
	_, err := mgr.GetRevision(context.Background(), "", "some-name")
	assert.Error(t, err)
}

func TestFetchRevisionID(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	_, err := mgr.UpsertRevision(ctx, testParams())
	require.NoError(t, err)

	id, err := mgr.FetchRevisionID(ctx, "test-ns", "pipeline-my-pipeline-abc123456789")
	require.NoError(t, err)
	assert.Equal(t, "abc123456789", id)
}

func TestDeleteAllRevisions(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()

	p1 := testParams()
	p1.RevisionName = "pipeline-my-pipeline-aaaaaaaaaaaa"
	p2 := testParams()
	p2.RevisionName = "pipeline-my-pipeline-bbbbbbbbbbbb"
	_, err := mgr.UpsertRevision(ctx, p1)
	require.NoError(t, err)
	_, err = mgr.UpsertRevision(ctx, p2)
	require.NoError(t, err)

	err = mgr.DeleteAllRevisions(ctx, "test-ns", "my-pipeline", "Pipeline")
	require.NoError(t, err)

	_, err = mgr.GetRevision(ctx, "test-ns", "pipeline-my-pipeline-aaaaaaaaaaaa")
	assert.Error(t, err)
	_, err = mgr.GetRevision(ctx, "test-ns", "pipeline-my-pipeline-bbbbbbbbbbbb")
	assert.Error(t, err)
}
