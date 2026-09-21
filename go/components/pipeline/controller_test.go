package pipeline

import (
	"context"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/client"

	pbtypes "github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/michelangelo-ai/michelangelo/go/api"
	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	"github.com/michelangelo-ai/michelangelo/go/base/env"
	"github.com/michelangelo-ai/michelangelo/go/base/revision"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"go.uber.org/zap/zaptest"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
)

func TestReconcile_RevisioningDisabled(t *testing.T) {
	now := metav1.Now()
	gracePeriod := int64(0)
	testCases := []struct {
		name                         string
		initialObjects               []client.Object
		env                          env.Context
		expectedResult               ctrl.Result
		expectedError                string
		expectedStatusState          v2pb.PipelineState
		expectedStatusLatestRevision *apipb.ResourceIdentifier
	}{
		{
			name: "Invalid -> READY, no LatestRevision set",
			initialObjects: []client.Object{
				&v2pb.Pipeline{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pipeline",
						Namespace: "test-namespace",
					},
					Spec: v2pb.PipelineSpec{
						Commit: &v2pb.CommitInfo{
							GitRef: "1234556",
							Branch: "test-git-branch",
						},
					},
				},
			},
			expectedResult:      ctrl.Result{},
			expectedStatusState: v2pb.PIPELINE_STATE_READY,
		},
		{
			// A Pipeline with a deletionTimestamp is being torn down by the
			// Kubernetes garbage collector. Reconcile must short-circuit so it
			// does not keep stamping status and requeueing during deletion.
			// The fake client only accepts a deletion timestamp when a finalizer
			// and a (zero) grace period are also set.
			name: "Being deleted -> skip reconcile",
			initialObjects: []client.Object{
				&v2pb.Pipeline{
					ObjectMeta: metav1.ObjectMeta{
						Name:                       "test-pipeline",
						Namespace:                  "test-namespace",
						DeletionTimestamp:          &now,
						DeletionGracePeriodSeconds: &gracePeriod,
						Finalizers:                 []string{"michelangelo.uber.com/pipeline"},
					},
					Spec: v2pb.PipelineSpec{
						Commit: &v2pb.CommitInfo{
							GitRef: "345678",
							Branch: "test-git-branch",
						},
					},
					Status: v2pb.PipelineStatus{
						State: v2pb.PIPELINE_STATE_CREATED,
					},
				},
			},
			// No requeue, no error, and the status is left untouched (state stays
			// CREATED and LatestRevision is never set), proving UpdateStatus and
			// the READY stamping were skipped.
			expectedResult:      ctrl.Result{},
			expectedError:       "",
			expectedStatusState: v2pb.PIPELINE_STATE_CREATED,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			reconciler := setUpReconciler(t, tc.initialObjects, tc.env, Config{RevisioningEnabled: false})
			result, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "test-pipeline", Namespace: "test-namespace"}})
			require.NoError(t, err)
			require.Equal(t, tc.expectedResult, result)
			pipeline := &v2pb.Pipeline{}
			require.NoError(t, reconciler.Get(context.Background(), "test-namespace", "test-pipeline", &metav1.GetOptions{}, pipeline))
			require.Equal(t, tc.expectedStatusState, pipeline.Status.State)
			assert.Nil(t, pipeline.Status.LatestRevision, "LatestRevision should not be set when revisioning is disabled")
		})
	}
}

func TestReconcile_RevisioningEnabled(t *testing.T) {
	pipeline := &v2pb.Pipeline{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pipeline",
			Namespace: "test-namespace",
		},
		Spec: v2pb.PipelineSpec{
			Type: v2pb.PIPELINE_TYPE_DATA_PREP,
			Commit: &v2pb.CommitInfo{
				GitRef: "abc123456789",
				Branch: "main",
			},
		},
	}

	reconciler := setUpReconciler(t, []client.Object{pipeline}, env.Context{}, Config{RevisioningEnabled: true})
	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "test-pipeline", Namespace: "test-namespace"}})
	require.NoError(t, err)

	got := &v2pb.Pipeline{}
	require.NoError(t, reconciler.Get(context.Background(), "test-namespace", "test-pipeline", &metav1.GetOptions{}, got))
	require.Equal(t, v2pb.PIPELINE_STATE_READY, got.Status.State)
	require.Equal(t, &apipb.ResourceIdentifier{
		Name:      "pipeline-test-pipeline-abc123456789",
		Namespace: "test-namespace",
	}, got.Status.LatestRevision)

	// Revision CR should have been created.
	rev := &v2pb.Revision{}
	require.NoError(t, reconciler.Get(context.Background(), "test-namespace", "pipeline-test-pipeline-abc123456789", &metav1.GetOptions{}, rev))
	assert.Equal(t, "abc123456789", rev.Spec.RevisionId)
	assert.Equal(t, "Pipeline", rev.Spec.BaseType.Kind)
	assert.Equal(t, "PIPELINE_TYPE_DATA_PREP", rev.Labels[api.PipelineTypeLabelName])
}

func TestReconcile_RevisioningEnabled_NoCommit(t *testing.T) {
	pipeline := &v2pb.Pipeline{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pipeline",
			Namespace: "test-namespace",
		},
	}

	reconciler := setUpReconciler(t, []client.Object{pipeline}, env.Context{}, Config{RevisioningEnabled: true})
	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "test-pipeline", Namespace: "test-namespace"}})
	require.NoError(t, err)

	got := &v2pb.Pipeline{}
	require.NoError(t, reconciler.Get(context.Background(), "test-namespace", "test-pipeline", &metav1.GetOptions{}, got))
	require.Equal(t, v2pb.PIPELINE_STATE_READY, got.Status.State)
	assert.Nil(t, got.Status.LatestRevision, "LatestRevision should not be set when pipeline has no commit")

	// Confirm no Revision CR was created.
	rev := &v2pb.Revision{}
	err = reconciler.Get(context.Background(), "test-namespace", "pipeline-test-pipeline-", &metav1.GetOptions{}, rev)
	assert.True(t, err != nil, "no Revision CR should exist when pipeline has no commit")
}

// status.latestRevision only advances for main/master. Feature-branch reconciles
// still snapshot a Revision CR — they just don't move the "latest" pointer.
func TestReconcile_LatestRevisionOnlyOnDefaultBranch(t *testing.T) {
	testCases := []struct {
		name                   string
		branch                 string
		existingLatestRevision *apipb.ResourceIdentifier
		expectLatestRevision   *apipb.ResourceIdentifier
	}{
		{
			name:   "main advances latestRevision",
			branch: "main",
			expectLatestRevision: &apipb.ResourceIdentifier{
				Name:      "pipeline-test-pipeline-abc123456789",
				Namespace: "test-namespace",
			},
		},
		{
			name:   "master advances latestRevision",
			branch: "master",
			expectLatestRevision: &apipb.ResourceIdentifier{
				Name:      "pipeline-test-pipeline-abc123456789",
				Namespace: "test-namespace",
			},
		},
		{
			name:                 "feature branch does not advance latestRevision",
			branch:               "feature/my-mr",
			expectLatestRevision: nil,
		},
		{
			name:   "feature branch leaves an existing latestRevision untouched",
			branch: "feature/my-mr",
			existingLatestRevision: &apipb.ResourceIdentifier{
				Name:      "pipeline-test-pipeline-earlier",
				Namespace: "test-namespace",
			},
			expectLatestRevision: &apipb.ResourceIdentifier{
				Name:      "pipeline-test-pipeline-earlier",
				Namespace: "test-namespace",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			pipeline := &v2pb.Pipeline{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pipeline",
					Namespace: "test-namespace",
				},
				Spec: v2pb.PipelineSpec{
					Commit: &v2pb.CommitInfo{
						GitRef: "abc123456789",
						Branch: tc.branch,
					},
				},
				Status: v2pb.PipelineStatus{
					LatestRevision: tc.existingLatestRevision,
				},
			}

			reconciler := setUpReconciler(t, []client.Object{pipeline}, env.Context{}, Config{RevisioningEnabled: true})
			_, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "test-pipeline", Namespace: "test-namespace"}})
			require.NoError(t, err)

			got := &v2pb.Pipeline{}
			require.NoError(t, reconciler.Get(context.Background(), "test-namespace", "test-pipeline", &metav1.GetOptions{}, got))
			require.Equal(t, v2pb.PIPELINE_STATE_READY, got.Status.State)
			assert.Equal(t, tc.expectLatestRevision, got.Status.LatestRevision)

			rev := &v2pb.Revision{}
			require.NoError(t, reconciler.Get(context.Background(), "test-namespace", "pipeline-test-pipeline-abc123456789", &metav1.GetOptions{}, rev),
				"Revision CR must still be snapshotted for non-default branches")
			assert.Equal(t, "abc123456789", rev.Spec.RevisionId)
		})
	}
}

// TestReconcile_RevisioningEnabled_SetsParentLineage exercises the
// Spec.Parent fix directly: the second reconcile's Revision must chain to
// the *first* revision's identity, sourced from the pre-mutation
// originalPipeline snapshot rather than the in-flight pipeline object (which
// has already been overwritten to point at the new revision by the time
// snapshotRevision runs, for main/master commits).
func TestReconcile_RevisioningEnabled_SetsParentLineage(t *testing.T) {
	pipeline := &v2pb.Pipeline{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pipeline",
			Namespace: "test-namespace",
		},
		Spec: v2pb.PipelineSpec{
			Type: v2pb.PIPELINE_TYPE_DATA_PREP,
			Commit: &v2pb.CommitInfo{
				GitRef: "abc123456789",
				Branch: "main",
			},
		},
	}

	reconciler := setUpReconciler(t, []client.Object{pipeline}, env.Context{}, Config{RevisioningEnabled: true})
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "test-pipeline", Namespace: "test-namespace"}}

	_, err := reconciler.Reconcile(context.Background(), req)
	require.NoError(t, err)

	firstRev := &v2pb.Revision{}
	require.NoError(t, reconciler.Get(context.Background(), "test-namespace", "pipeline-test-pipeline-abc123456789", &metav1.GetOptions{}, firstRev))
	assert.Nil(t, firstRev.Spec.Parent, "first revision has no prior revision to chain to")

	// Advance to a new commit and reconcile again.
	got := &v2pb.Pipeline{}
	require.NoError(t, reconciler.Get(context.Background(), "test-namespace", "test-pipeline", &metav1.GetOptions{}, got))
	got.Spec.Commit.GitRef = "def987654321"
	require.NoError(t, reconciler.Update(context.Background(), got, &metav1.UpdateOptions{}))

	_, err = reconciler.Reconcile(context.Background(), req)
	require.NoError(t, err)

	secondRev := &v2pb.Revision{}
	require.NoError(t, reconciler.Get(context.Background(), "test-namespace", "pipeline-test-pipeline-def987654321", &metav1.GetOptions{}, secondRev))
	require.Equal(t, &apipb.ResourceIdentifier{
		Name:      "pipeline-test-pipeline-abc123456789",
		Namespace: "test-namespace",
	}, secondRev.Spec.Parent, "second revision must chain to the first, not self-reference")
}

// TestReconcile_RevisioningEnabled_FeatureBranch_ParentStillSet covers the
// path where Reconcile never advances Status.LatestRevision (feature
// branches): Spec.Parent must still be populated from whatever
// Status.LatestRevision held going in, since originalPipeline and pipeline
// never diverge on this field for a non-default branch.
func TestReconcile_RevisioningEnabled_FeatureBranch_ParentStillSet(t *testing.T) {
	pipeline := &v2pb.Pipeline{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pipeline",
			Namespace: "test-namespace",
		},
		Spec: v2pb.PipelineSpec{
			Commit: &v2pb.CommitInfo{
				GitRef: "abc123456789",
				Branch: "feature/my-mr",
			},
		},
		Status: v2pb.PipelineStatus{
			LatestRevision: &apipb.ResourceIdentifier{
				Name:      "pipeline-test-pipeline-earlier",
				Namespace: "test-namespace",
			},
		},
	}

	reconciler := setUpReconciler(t, []client.Object{pipeline}, env.Context{}, Config{RevisioningEnabled: true})
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "test-pipeline", Namespace: "test-namespace"}}

	_, err := reconciler.Reconcile(context.Background(), req)
	require.NoError(t, err)

	rev := &v2pb.Revision{}
	require.NoError(t, reconciler.Get(context.Background(), "test-namespace", "pipeline-test-pipeline-abc123456789", &metav1.GetOptions{}, rev))
	require.Equal(t, &apipb.ResourceIdentifier{
		Name:      "pipeline-test-pipeline-earlier",
		Namespace: "test-namespace",
	}, rev.Spec.Parent)
}

// TestReconcile_RevisioningEnabled_StripsManagedFields confirms
// ObjectMeta.ManagedFields never rides along in the snapshotted content.
func TestReconcile_RevisioningEnabled_StripsManagedFields(t *testing.T) {
	pipeline := &v2pb.Pipeline{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pipeline",
			Namespace: "test-namespace",
			ManagedFields: []metav1.ManagedFieldsEntry{
				{
					Manager:    "some-field-manager",
					Operation:  metav1.ManagedFieldsOperationUpdate,
					APIVersion: pipelineAPIVersion,
					FieldsType: "FieldsV1",
				},
			},
		},
		Spec: v2pb.PipelineSpec{
			Commit: &v2pb.CommitInfo{
				GitRef: "abc123456789",
				Branch: "main",
			},
		},
	}

	reconciler := setUpReconciler(t, []client.Object{pipeline}, env.Context{}, Config{RevisioningEnabled: true})
	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "test-pipeline", Namespace: "test-namespace"}})
	require.NoError(t, err)

	rev := &v2pb.Revision{}
	require.NoError(t, reconciler.Get(context.Background(), "test-namespace", "pipeline-test-pipeline-abc123456789", &metav1.GetOptions{}, rev))

	content := &v2pb.Pipeline{}
	require.NoError(t, pbtypes.UnmarshalAny(rev.Spec.Content, content))
	assert.Empty(t, content.ObjectMeta.ManagedFields, "ManagedFields must be stripped before marshaling the snapshot content")
}

// TestReconcile_RevisioningEnabled_StampsOwnerRef confirms the created
// Revision carries a controller ownerReference back to its Pipeline, so
// Kubernetes garbage collection cleans up Revisions when the Pipeline is
// deleted.
func TestReconcile_RevisioningEnabled_StampsOwnerRef(t *testing.T) {
	pipeline := &v2pb.Pipeline{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pipeline",
			Namespace: "test-namespace",
			UID:       "test-pipeline-uid",
		},
		Spec: v2pb.PipelineSpec{
			Commit: &v2pb.CommitInfo{
				GitRef: "abc123456789",
				Branch: "main",
			},
		},
	}

	reconciler := setUpReconciler(t, []client.Object{pipeline}, env.Context{}, Config{RevisioningEnabled: true})
	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "test-pipeline", Namespace: "test-namespace"}})
	require.NoError(t, err)

	rev := &v2pb.Revision{}
	require.NoError(t, reconciler.Get(context.Background(), "test-namespace", "pipeline-test-pipeline-abc123456789", &metav1.GetOptions{}, rev))

	owner := metav1.GetControllerOf(rev)
	require.NotNil(t, owner, "Revision must have a controller ownerReference")
	assert.Equal(t, pipeline.UID, owner.UID)
	assert.Equal(t, "test-pipeline", owner.Name)
}

func TestFormatRevisionName(t *testing.T) {
	testCases := []struct {
		name           string
		pipeline       *v2pb.Pipeline
		expectedResult string
	}{
		{
			name: "Normal git ref",
			pipeline: &v2pb.Pipeline{
				ObjectMeta: metav1.ObjectMeta{
					Name: "my-pipeline",
				},
				Spec: v2pb.PipelineSpec{
					Commit: &v2pb.CommitInfo{
						GitRef: "abcdef1234567890",
					},
				},
			},
			expectedResult: "pipeline-my-pipeline-abcdef123456",
		},
		{
			name: "Short git ref",
			pipeline: &v2pb.Pipeline{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pipe",
				},
				Spec: v2pb.PipelineSpec{
					Commit: &v2pb.CommitInfo{
						GitRef: "abc123",
					},
				},
			},
			expectedResult: "pipeline-test-pipe-abc123",
		},
		{
			name: "Uppercase pipeline name",
			pipeline: &v2pb.Pipeline{
				ObjectMeta: metav1.ObjectMeta{
					Name: "MY-PIPELINE",
				},
				Spec: v2pb.PipelineSpec{
					Commit: &v2pb.CommitInfo{
						GitRef: "def456789012",
					},
				},
			},
			expectedResult: "pipeline-my-pipeline-def456789012",
		},
		{
			name: "No commit info",
			pipeline: &v2pb.Pipeline{
				ObjectMeta: metav1.ObjectMeta{
					Name: "no-commit",
				},
				Spec: v2pb.PipelineSpec{
					Commit: nil,
				},
			},
			expectedResult: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := formatRevisionName(tc.pipeline)
			require.Equal(t, tc.expectedResult, result)
		})
	}
}

func setUpReconciler(t *testing.T, initialObjects []client.Object, env env.Context, cfg Config) *Reconciler {
	scheme := runtime.NewScheme()
	require.NoError(t, v2pb.AddToScheme(scheme))
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(initialObjects...).WithStatusSubresource(initialObjects...).Build()
	handler := apiHandler.NewFakeAPIHandler(k8sClient)
	return &Reconciler{
		Handler:         handler,
		logger:          zaptest.NewLogger(t),
		revisionManager: revision.NewManager(handler, zaptest.NewLogger(t)),
		scheme:          scheme,
		config:          cfg,
	}
}
