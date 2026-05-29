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

func newTestManager(t *testing.T) (Manager, api.Handler) {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, v2pb.AddToScheme(scheme))
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	handler := apiHandler.NewFakeAPIHandler(k8sClient)
	return NewManager(handler, zaptest.NewLogger(t)), handler
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

func getRevision(t *testing.T, h api.Handler, namespace, name string) *v2pb.Revision {
	t.Helper()
	rev := &v2pb.Revision{}
	require.NoError(t, h.Get(context.Background(), namespace, name, &metav1.GetOptions{}, rev))
	return rev
}

func TestUpsertRevision_Create(t *testing.T) {
	mgr, h := newTestManager(t)
	ctx := context.Background()

	created, err := mgr.UpsertRevision(ctx, testParams())
	require.NoError(t, err)
	assert.True(t, created)

	rev := getRevision(t, h, "test-ns", "pipeline-my-pipeline-abc123456789")
	assert.Equal(t, "abc123456789", rev.Spec.RevisionId)
	assert.Equal(t, "Pipeline", rev.Spec.BaseType.Kind)
}

func TestUpsertRevision_CreateImmutable(t *testing.T) {
	mgr, h := newTestManager(t)
	ctx := context.Background()
	params := testParams()
	params.Immutable = true

	created, err := mgr.UpsertRevision(ctx, params)
	require.NoError(t, err)
	assert.True(t, created)

	rev := getRevision(t, h, "test-ns", "pipeline-my-pipeline-abc123456789")
	assert.True(t, apiutils.IsImmutable(rev))
}

func TestUpsertRevision_DedupImmutable(t *testing.T) {
	mgr, _ := newTestManager(t)
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
	mgr, _ := newTestManager(t)
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
	mgr, _ := newTestManager(t)
	ctx := context.Background()

	_, err := mgr.UpsertRevision(ctx, testParams())
	require.NoError(t, err)

	created, err := mgr.UpsertRevision(ctx, testParams())
	require.NoError(t, err)
	assert.False(t, created)
}

func TestUpsertRevision_MutableThenImmutable(t *testing.T) {
	mgr, h := newTestManager(t)
	ctx := context.Background()

	_, err := mgr.UpsertRevision(ctx, testParams())
	require.NoError(t, err)

	params := testParams()
	params.Immutable = true
	created, err := mgr.UpsertRevision(ctx, params)
	require.NoError(t, err)
	assert.False(t, created)

	rev := getRevision(t, h, "test-ns", "pipeline-my-pipeline-abc123456789")
	assert.True(t, apiutils.IsImmutable(rev))
}
