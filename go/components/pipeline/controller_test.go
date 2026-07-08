package pipeline

import (
	"context"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

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
	}
}
