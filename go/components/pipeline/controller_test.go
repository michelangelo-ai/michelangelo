package pipeline

import (
	"context"
	"errors"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/michelangelo-ai/michelangelo/go/api"
	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	"github.com/michelangelo-ai/michelangelo/go/base/env"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"go.uber.org/zap/zaptest"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
)

func TestReconcile(t *testing.T) {
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
			name: "Invalid -> READY",
			initialObjects: []client.Object{
				&v2pb.Pipeline{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "test-pipeline",
						Namespace:  "test-namespace",
						Finalizers: []string{api.PipelineFinalizer},
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
			expectedError:       "",
			expectedStatusState: v2pb.PIPELINE_STATE_READY,
			expectedStatusLatestRevision: &apipb.ResourceIdentifier{
				Name:      "pipeline-test-pipeline-1234556",
				Namespace: "test-namespace",
			},
		},
		{
			name: "Ready -> should not reconcile",
			initialObjects: []client.Object{
				&v2pb.Pipeline{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "test-pipeline",
						Namespace:  "test-namespace",
						Finalizers: []string{api.PipelineFinalizer},
					},
					Spec: v2pb.PipelineSpec{
						Commit: &v2pb.CommitInfo{
							GitRef: "123456",
							Branch: "test-git-branch",
						},
					},
					Status: v2pb.PipelineStatus{
						State: v2pb.PIPELINE_STATE_READY,
						LatestRevision: &apipb.ResourceIdentifier{
							Name:      "pipeline-test-pipeline-123456",
							Namespace: "test-namespace",
						},
					},
				},
			},
			expectedResult:      ctrl.Result{},
			expectedError:       "",
			expectedStatusState: v2pb.PIPELINE_STATE_READY,
			expectedStatusLatestRevision: &apipb.ResourceIdentifier{
				Name:      "pipeline-test-pipeline-123456",
				Namespace: "test-namespace",
			},
		},
		{
			name: "Ready -> should reconcile",
			initialObjects: []client.Object{
				&v2pb.Pipeline{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "test-pipeline",
						Namespace:  "test-namespace",
						Finalizers: []string{api.PipelineFinalizer},
					},
					Spec: v2pb.PipelineSpec{
						Commit: &v2pb.CommitInfo{
							GitRef: "234567",
							Branch: "test-git-branch-2",
						},
					},
					Status: v2pb.PipelineStatus{
						State: v2pb.PIPELINE_STATE_READY,
						LatestRevision: &apipb.ResourceIdentifier{
							Name:      "pipeline-test-pipeline-123456",
							Namespace: "test-namespace",
						},
					},
				},
			},
			expectedResult:      ctrl.Result{},
			expectedError:       "",
			expectedStatusState: v2pb.PIPELINE_STATE_READY,
			expectedStatusLatestRevision: &apipb.ResourceIdentifier{
				Name:      "pipeline-test-pipeline-234567",
				Namespace: "test-namespace",
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			reconciler := setUpReconciler(t, tc.initialObjects, tc.env)
			result, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "test-pipeline", Namespace: "test-namespace"}})
			if tc.expectedError != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.expectedError)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.expectedResult, result)
			pipeline := &v2pb.Pipeline{}
			err = reconciler.Get(context.Background(), "test-namespace", "test-pipeline", &metav1.GetOptions{}, pipeline)
			require.NoError(t, err)
			require.Equal(t, tc.expectedStatusState, pipeline.Status.State)
			if tc.expectedStatusLatestRevision != nil {
				require.Equal(t, tc.expectedStatusLatestRevision, pipeline.Status.LatestRevision)
			} else {
				require.Nil(t, pipeline.Status.LatestRevision)
			}
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

func TestReconcile_AddsFinalizer(t *testing.T) {
	pipeline := &v2pb.Pipeline{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pipeline",
			Namespace: "test-namespace",
		},
		Spec: v2pb.PipelineSpec{
			Commit: &v2pb.CommitInfo{GitRef: "abc123", Branch: "main"},
		},
	}
	reconciler := setUpReconciler(t, []client.Object{pipeline}, env.Context{})

	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-pipeline", Namespace: "test-namespace"},
	})
	require.NoError(t, err)

	updated := &v2pb.Pipeline{}
	require.NoError(t, reconciler.Get(context.Background(), "test-namespace", "test-pipeline", &metav1.GetOptions{}, updated))
	require.True(t, controllerutil.ContainsFinalizer(updated, api.PipelineFinalizer))
}

func TestReconcile_RemovesFinalizerOnDelete(t *testing.T) {
	now := metav1.Now()
	pipeline := &v2pb.Pipeline{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-pipeline",
			Namespace:         "test-namespace",
			Finalizers:        []string{api.PipelineFinalizer},
			DeletionTimestamp: &now,
		},
		Spec: v2pb.PipelineSpec{
			Commit: &v2pb.CommitInfo{GitRef: "abc123", Branch: "main"},
		},
	}
	reconciler := setUpReconciler(t, []client.Object{pipeline}, env.Context{})

	result, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-pipeline", Namespace: "test-namespace"},
	})
	require.NoError(t, err)
	require.Equal(t, ctrl.Result{}, result)

	// Verify finalizer was removed (object may or may not still exist depending on fake client behavior)
	updated := &v2pb.Pipeline{}
	err = reconciler.Get(context.Background(), "test-namespace", "test-pipeline", &metav1.GetOptions{}, updated)
	if err == nil {
		require.False(t, controllerutil.ContainsFinalizer(updated, api.PipelineFinalizer))
	}
}

// updateErroringHandler wraps an api.Handler and returns a configured error
// from Update. Used to exercise finalizer Update error branches.
type updateErroringHandler struct {
	api.Handler
	updateErr error
}

func (u *updateErroringHandler) Update(ctx context.Context, obj client.Object, opts *metav1.UpdateOptions) error {
	if u.updateErr != nil {
		return u.updateErr
	}
	return u.Handler.Update(ctx, obj, opts)
}

func setUpReconcilerWithUpdateErr(t *testing.T, initialObjects []client.Object, updateErr error) *Reconciler {
	scheme := runtime.NewScheme()
	require.NoError(t, v2pb.AddToScheme(scheme))
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(initialObjects...).WithStatusSubresource(initialObjects...).Build()
	return &Reconciler{
		Handler: &updateErroringHandler{Handler: apiHandler.NewFakeAPIHandler(k8sClient), updateErr: updateErr},
		logger:  zaptest.NewLogger(t),
	}
}

func TestReconcile_AddFinalizer_UpdateError(t *testing.T) {
	pipeline := &v2pb.Pipeline{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pipeline",
			Namespace: "test-namespace",
		},
		Spec: v2pb.PipelineSpec{
			Commit: &v2pb.CommitInfo{GitRef: "abc123", Branch: "main"},
		},
	}
	updateErr := errors.New("update boom")
	reconciler := setUpReconcilerWithUpdateErr(t, []client.Object{pipeline}, updateErr)

	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-pipeline", Namespace: "test-namespace"},
	})
	require.Error(t, err)
	require.ErrorIs(t, err, updateErr)
	require.Contains(t, err.Error(), "add pipeline finalizer")
}

func TestReconcile_RemoveFinalizer_UpdateError(t *testing.T) {
	now := metav1.Now()
	pipeline := &v2pb.Pipeline{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-pipeline",
			Namespace:         "test-namespace",
			Finalizers:        []string{api.PipelineFinalizer},
			DeletionTimestamp: &now,
		},
		Spec: v2pb.PipelineSpec{
			Commit: &v2pb.CommitInfo{GitRef: "abc123", Branch: "main"},
		},
	}
	updateErr := errors.New("update boom")
	reconciler := setUpReconcilerWithUpdateErr(t, []client.Object{pipeline}, updateErr)

	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-pipeline", Namespace: "test-namespace"},
	})
	require.Error(t, err)
	require.ErrorIs(t, err, updateErr)
	require.Contains(t, err.Error(), "remove pipeline finalizer")
}

func setUpReconciler(t *testing.T, initialObjects []client.Object, env env.Context) *Reconciler {
	scheme := runtime.NewScheme()
	err := v2pb.AddToScheme(scheme)
	require.NoError(t, err)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(initialObjects...).WithStatusSubresource(initialObjects...).Build()
	reconciler := &Reconciler{
		Handler: apiHandler.NewFakeAPIHandler(k8sClient),
		logger:  zaptest.NewLogger(t),
	}
	return reconciler
}
