package pipeline

import (
	"context"
	"errors"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

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
	"go.uber.org/yarpc"
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

// Deleting a Pipeline must cascade to its Revisions, which relies on every
// snapshotted Revision carrying a controller ownerReference to the Pipeline.
func TestReconcile_RevisionOwnedByPipeline(t *testing.T) {
	pipeline := &v2pb.Pipeline{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pipeline",
			Namespace: "test-namespace",
			UID:       types.UID("pipeline-uid"),
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
	require.NotNil(t, owner, "revision must be controller-owned by its pipeline")
	assert.Equal(t, "Pipeline", owner.Kind)
	assert.Equal(t, "test-pipeline", owner.Name)
	assert.Equal(t, types.UID("pipeline-uid"), owner.UID)
}

// Once the Revision controller marks a revision immutable, re-reconciling the same
// commit must neither fail nor rewrite it.
func TestReconcile_ImmutableRevisionIsNotResnapshotted(t *testing.T) {
	pipeline := &v2pb.Pipeline{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pipeline",
			Namespace: "test-namespace",
			UID:       "pipeline-uid",
		},
		Spec: v2pb.PipelineSpec{
			Commit: &v2pb.CommitInfo{
				GitRef: "abc123456789",
				Branch: "main",
			},
		},
	}
	existing := immutableRevision("pipeline-test-pipeline-abc123456789", "abc123456789")
	existing.OwnerReferences = []metav1.OwnerReference{pipelineControllerRef("pipeline-uid")}

	reconciler := setUpReconciler(t, []client.Object{pipeline, existing}, env.Context{}, Config{RevisioningEnabled: true})
	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "test-pipeline", Namespace: "test-namespace"}})
	require.NoError(t, err)

	rev := &v2pb.Revision{}
	require.NoError(t, reconciler.Get(context.Background(), "test-namespace", "pipeline-test-pipeline-abc123456789", &metav1.GetOptions{}, rev))
	assert.Equal(t, "abc123456789", rev.Spec.RevisionId, "an immutable revision must not be rewritten")
	assert.Equal(t, existing.Spec.Content.Value, rev.Spec.Content.Value)
}

// Immutable revisions leave etcd, so they outlive a deleted Pipeline. A re-created
// Pipeline of the same name (a new UID) must not reuse the old Pipeline's revision.
func TestReconcile_ImmutableRevisionOfDeletedPipelineIsReported(t *testing.T) {
	pipeline := &v2pb.Pipeline{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pipeline",
			Namespace: "test-namespace",
			UID:       "recreated-uid",
		},
		Spec: v2pb.PipelineSpec{
			Commit: &v2pb.CommitInfo{GitRef: "abc123456789", Branch: "main"},
		},
	}
	existing := immutableRevision("pipeline-test-pipeline-abc123456789", "abc123456789")
	existing.OwnerReferences = []metav1.OwnerReference{pipelineControllerRef("deleted-uid")}

	reconciler := setUpReconciler(t, []client.Object{pipeline, existing}, env.Context{}, Config{RevisioningEnabled: true})
	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "test-pipeline", Namespace: "test-namespace"}})
	require.ErrorContains(t, err, "belongs to a different Pipeline (uid deleted-uid, current uid recreated-uid)")

	rev := &v2pb.Revision{}
	require.NoError(t, reconciler.Get(context.Background(), "test-namespace", "pipeline-test-pipeline-abc123456789", &metav1.GetOptions{}, rev))
	assert.Equal(t, types.UID("deleted-uid"), metav1.GetControllerOf(rev).UID, "the existing revision must not be rewritten")
}

func TestOwnedByPipeline(t *testing.T) {
	pipeline := &v2pb.Pipeline{ObjectMeta: metav1.ObjectMeta{UID: "pipeline-uid"}}

	owned := &v2pb.Revision{ObjectMeta: metav1.ObjectMeta{OwnerReferences: []metav1.OwnerReference{pipelineControllerRef("pipeline-uid")}}}
	assert.True(t, ownedByPipeline(owned, pipeline))

	other := &v2pb.Revision{ObjectMeta: metav1.ObjectMeta{OwnerReferences: []metav1.OwnerReference{pipelineControllerRef("other-uid")}}}
	assert.False(t, ownedByPipeline(other, pipeline))

	// A revision written before ownerRefs were stamped is treated as owned.
	assert.True(t, ownedByPipeline(&v2pb.Revision{}, pipeline))

	// A non-controller ownerReference is not an ownership claim.
	nonController := pipelineControllerRef("other-uid")
	nonController.Controller = nil
	assert.True(t, ownedByPipeline(&v2pb.Revision{ObjectMeta: metav1.ObjectMeta{OwnerReferences: []metav1.OwnerReference{nonController}}}, pipeline))
}

func pipelineControllerRef(uid types.UID) metav1.OwnerReference {
	controller := true
	return metav1.OwnerReference{
		APIVersion: pipelineAPIVersion,
		Kind:       pipelineKind,
		Name:       "test-pipeline",
		UID:        uid,
		Controller: &controller,
	}
}

// A different commit whose ref shares the 12-character prefix maps to the same
// revision name; it must be reported, not silently skipped.
func TestReconcile_ImmutableRevisionNameCollisionIsReported(t *testing.T) {
	pipeline := &v2pb.Pipeline{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pipeline",
			Namespace: "test-namespace",
		},
		Spec: v2pb.PipelineSpec{
			Commit: &v2pb.CommitInfo{
				GitRef: "abc123456789aaaa1111",
				Branch: "main",
			},
		},
	}
	existing := immutableRevision("pipeline-test-pipeline-abc123456789", "abc123456789ffff0000")

	reconciler := setUpReconciler(t, []client.Object{pipeline, existing}, env.Context{}, Config{RevisioningEnabled: true})
	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "test-pipeline", Namespace: "test-namespace"}})
	require.ErrorContains(t, err, "git refs collide")

	rev := &v2pb.Revision{}
	require.NoError(t, reconciler.Get(context.Background(), "test-namespace", "pipeline-test-pipeline-abc123456789", &metav1.GetOptions{}, rev))
	assert.Equal(t, "abc123456789ffff0000", rev.Spec.RevisionId, "the existing revision must not be rewritten")
}

// A mutable revision of a different commit with the same name prefix (still within
// the window before the Revision controller freezes it) must not be overwritten.
func TestReconcile_MutableRevisionNameCollisionIsReported(t *testing.T) {
	pipeline := &v2pb.Pipeline{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pipeline",
			Namespace: "test-namespace",
		},
		Spec: v2pb.PipelineSpec{
			Commit: &v2pb.CommitInfo{
				GitRef: "abc123456789aaaa1111",
				Branch: "main",
			},
		},
	}
	existing := immutableRevision("pipeline-test-pipeline-abc123456789", "abc123456789ffff0000")
	existing.Annotations = nil
	existing.Status = v2pb.RevisionStatus{}

	reconciler := setUpReconciler(t, []client.Object{pipeline, existing}, env.Context{}, Config{RevisioningEnabled: true})
	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "test-pipeline", Namespace: "test-namespace"}})
	require.ErrorContains(t, err, "git refs collide")

	rev := &v2pb.Revision{}
	require.NoError(t, reconciler.Get(context.Background(), "test-namespace", "pipeline-test-pipeline-abc123456789", &metav1.GetOptions{}, rev))
	assert.Equal(t, "abc123456789ffff0000", rev.Spec.RevisionId, "the other commit's revision must not be overwritten")
	assert.Equal(t, existing.Spec.Content.Value, rev.Spec.Content.Value)
}

// An immutable revision without a recorded commit or snapshotted content cannot be
// repaired and would fail every run resolving it, so it is reported, not skipped.
func TestReconcile_MalformedImmutableRevisionIsReported(t *testing.T) {
	withoutCommit := immutableRevision("pipeline-test-pipeline-abc123456789", "")
	withoutContent := immutableRevision("pipeline-test-pipeline-abc123456789", "abc123456789")
	withoutContent.Spec.Content = nil

	for name, existing := range map[string]*v2pb.Revision{"no commit": withoutCommit, "no content": withoutContent} {
		t.Run(name, func(t *testing.T) {
			pipeline := &v2pb.Pipeline{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pipeline",
					Namespace: "test-namespace",
				},
				Spec: v2pb.PipelineSpec{
					Commit: &v2pb.CommitInfo{
						GitRef: "abc123456789",
						Branch: "main",
					},
				},
			}

			reconciler := setUpReconciler(t, []client.Object{pipeline, existing}, env.Context{}, Config{RevisioningEnabled: true})
			_, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "test-pipeline", Namespace: "test-namespace"}})
			require.ErrorContains(t, err, "no recorded commit or snapshotted content")
		})
	}
}

// A ref that is not a commit ID (here a branch name) keeps the same revision name when
// the spec changes; the frozen snapshot must not silently stand in for the new spec.
func TestReconcile_ImmutableRevisionWithChangedSpecIsReported(t *testing.T) {
	pipeline := &v2pb.Pipeline{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pipeline",
			Namespace: "test-namespace",
		},
		Spec: v2pb.PipelineSpec{
			Description: "new",
			Commit: &v2pb.CommitInfo{
				GitRef: "main",
				Branch: "main",
			},
		},
	}
	existing := immutableRevision("pipeline-test-pipeline-main", "main")
	existing.Spec.Content = mustSnapshot(&v2pb.Pipeline{Spec: v2pb.PipelineSpec{
		Description: "old",
		Commit:      &v2pb.CommitInfo{GitRef: "main", Branch: "main"},
	}})

	reconciler := setUpReconciler(t, []client.Object{pipeline, existing}, env.Context{}, Config{RevisioningEnabled: true})
	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "test-pipeline", Namespace: "test-namespace"}})
	require.ErrorContains(t, err, "snapshotted a different spec")
}

// Commit metadata may differ for the same commit; only the rest of the spec decides.
func TestSnapshotMatchesSpec(t *testing.T) {
	pipeline := &v2pb.Pipeline{Spec: v2pb.PipelineSpec{
		Description: "same",
		Commit:      &v2pb.CommitInfo{GitRef: "abc123456789", Branch: "main"},
	}}
	rev := immutableRevision("pipeline-test-pipeline-abc123456789", "abc123456789")
	rev.Spec.Content = mustSnapshot(&v2pb.Pipeline{Spec: v2pb.PipelineSpec{
		Description: "same",
		Commit:      &v2pb.CommitInfo{GitRef: "abc123456789deadbeef", Branch: "release"},
	}})

	matches, err := snapshotMatchesSpec(rev, pipeline)
	require.NoError(t, err)
	assert.True(t, matches)
	assert.Equal(t, "abc123456789", pipeline.Spec.Commit.GitRef, "the caller's pipeline must not be modified")

	rev.Spec.Content = &pbtypes.Any{TypeUrl: "type.googleapis.com/unknown", Value: []byte("x")}
	_, err = snapshotMatchesSpec(rev, pipeline)
	require.ErrorContains(t, err, "decode snapshotted pipeline")
}

// `apply` uploads a new tarball and records the caller as owner on every run, so neither
// may make a re-apply of the same commit look like a different spec.
func TestSnapshotMatchesSpecIgnoresApplyArtifacts(t *testing.T) {
	spec := func(tar, owner, description string) v2pb.PipelineSpec {
		return v2pb.PipelineSpec{
			Description: description,
			Owner:       &v2pb.UserInfo{Name: owner},
			Manifest:    &v2pb.PipelineManifest{UniflowTar: tar},
			Commit:      &v2pb.CommitInfo{GitRef: "abc123456789"},
		}
	}
	rev := immutableRevision("pipeline-test-pipeline-abc123456789", "abc123456789")
	rev.Spec.Content = mustSnapshot(&v2pb.Pipeline{Spec: spec("s3://bucket/first.tar", "alice", "same")})

	reapplied := &v2pb.Pipeline{Spec: spec("s3://bucket/second.tar", "bob", "same")}
	matches, err := snapshotMatchesSpec(rev, reapplied)
	require.NoError(t, err)
	assert.True(t, matches, "a new tarball path and owner alone must not count as a different spec")
	assert.Equal(t, "s3://bucket/second.tar", reapplied.Spec.Manifest.UniflowTar, "the caller's pipeline must not be modified")
	assert.Equal(t, "bob", reapplied.Spec.Owner.Name, "the caller's pipeline must not be modified")

	changed := &v2pb.Pipeline{Spec: spec("s3://bucket/second.tar", "bob", "different")}
	matches, err = snapshotMatchesSpec(rev, changed)
	require.NoError(t, err)
	assert.False(t, matches, "a real spec change must still be detected")
}

func mustSnapshot(pipeline *v2pb.Pipeline) *pbtypes.Any {
	content, err := pbtypes.MarshalAny(pipeline)
	if err != nil {
		panic(err)
	}
	return content
}

func TestSameCommitRef(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"abc123456789", "abc123456789", true},
		{" abc123456789 ", "abc123456789", true},
		// Refs are not resolved, so a shared prefix is not the same commit, whether
		// the refs look like abbreviated SHAs or are branch names.
		{"abc123456789", "abc123456789deadbeef", false},
		{"abcdefabcdef", "abcdefabcdef0", false},
		{"feature/abc123", "feature/abc123-long", false},
		{"main", "Main", false},
		{"", "abc123456789", false},
		{"", "", true},
	}
	for _, tc := range cases {
		assert.Equalf(t, tc.want, sameCommitRef(tc.a, tc.b), "sameCommitRef(%q, %q)", tc.a, tc.b)
	}
}

func immutableRevision(name, revisionID string) *v2pb.Revision {
	return &v2pb.Revision{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   "test-namespace",
			Annotations: map[string]string{api.ImmutableAnnotation: "true"},
		},
		Spec: v2pb.RevisionSpec{
			RevisionId: revisionID,
			Content:    mustSnapshot(&v2pb.Pipeline{Spec: v2pb.PipelineSpec{Commit: &v2pb.CommitInfo{GitRef: revisionID}}}),
		},
		Status: v2pb.RevisionStatus{
			State: v2pb.REVISION_STATE_READY,
		},
	}
}

// A failed lookup of the existing revision is surfaced so the reconcile retries,
// rather than blindly upserting.
func TestReconcile_RevisionLookupErrorIsReturned(t *testing.T) {
	pipeline := &v2pb.Pipeline{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pipeline",
			Namespace: "test-namespace",
		},
		Spec: v2pb.PipelineSpec{
			Commit: &v2pb.CommitInfo{
				GitRef: "abc123456789",
				Branch: "main",
			},
		},
	}

	scheme := runtime.NewScheme()
	require.NoError(t, v2pb.AddToScheme(scheme))
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(pipeline).
		WithStatusSubresource(pipeline).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if _, ok := obj.(*v2pb.Revision); ok {
					return errors.New("etcd unavailable")
				}
				return c.Get(ctx, key, obj, opts...)
			},
		}).
		Build()
	handler := apiHandler.NewFakeAPIHandler(k8sClient)
	reconciler := &Reconciler{
		Handler:         handler,
		logger:          zaptest.NewLogger(t),
		revisionManager: revision.NewManager(handler, zaptest.NewLogger(t)),
		config:          Config{RevisioningEnabled: true},
		scheme:          scheme,
	}

	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "test-pipeline", Namespace: "test-namespace"}})
	require.ErrorContains(t, err, "get revision test-namespace/pipeline-test-pipeline-abc123456789")
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
		config:          cfg,
		scheme:          scheme,
	}
}

// capturingRevisionManager records the Revision a snapshot upserts.
type capturingRevisionManager struct {
	upserted *v2pb.Revision
}

func (m *capturingRevisionManager) UpsertRevision(_ context.Context, rev client.Object, _ revision.UpsertOpts, _ ...yarpc.CallOption) (bool, error) {
	m.upserted = rev.(*v2pb.Revision)
	return true, nil
}

// The snapshot copies the Pipeline's CommitInfo, including its commit metadata, onto the
// Revision, so the metadata is fixed on the revision from creation.
func TestSnapshotRevision_CopiesCommitMetadata(t *testing.T) {
	commit := &v2pb.CommitInfo{
		GitRef:  "abc123456789",
		Branch:  "main",
		Message: "Raise the retry budget\n\nThe extract fails about once a week.",
		Author:  "jane@example.com",
		Url:     "https://git.example.com/repo/-/commit/abc123456789",
	}
	pipeline := &v2pb.Pipeline{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pipeline",
			Namespace: "test-namespace",
		},
		Spec: v2pb.PipelineSpec{Commit: commit},
	}
	reconciler := setUpReconciler(t, []client.Object{pipeline}, env.Context{}, Config{RevisioningEnabled: true})
	manager := &capturingRevisionManager{}
	reconciler.revisionManager = manager

	require.NoError(t, reconciler.snapshotRevision(context.Background(), pipeline))
	require.NotNil(t, manager.upserted)
	assert.Equal(t, commit, manager.upserted.Spec.GitCommit)
}
