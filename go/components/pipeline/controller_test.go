package pipeline

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/michelangelo-ai/michelangelo/go/api"
	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	"github.com/michelangelo-ai/michelangelo/go/base/env"
	"github.com/michelangelo-ai/michelangelo/go/components/pipelinerun"
	"github.com/michelangelo-ai/michelangelo/go/components/triggerrun"
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

// updateErroringHandler wraps an api.Handler and returns configured errors
// from List/Update. Used to exercise finalizer and cascade-delete error branches.
type updateErroringHandler struct {
	api.Handler
	updateErr     error
	listErr       error
	// listErrForType, when non-nil, only fails List when the list object is of
	// the given type (e.g. "*v2pb.TriggerRunList"). This lets us assert the
	// controller surfaces the exact failure path we expect.
	listErrForType string
}

func (u *updateErroringHandler) Update(ctx context.Context, obj client.Object, opts *metav1.UpdateOptions) error {
	if u.updateErr != nil {
		return u.updateErr
	}
	return u.Handler.Update(ctx, obj, opts)
}

func (u *updateErroringHandler) List(ctx context.Context, namespace string, opts *metav1.ListOptions, listOptionsExt *apipb.ListOptionsExt, list client.ObjectList) error {
	if u.listErr != nil && (u.listErrForType == "" || u.listErrForType == fmt.Sprintf("%T", list)) {
		return u.listErr
	}
	return u.Handler.List(ctx, namespace, opts, listOptionsExt, list)
}

func setUpReconcilerWithUpdateErr(t *testing.T, initialObjects []client.Object, updateErr error) *Reconciler {
	return setUpReconcilerWithErrors(t, initialObjects, updateErr, nil, "")
}

func setUpReconcilerWithErrors(t *testing.T, initialObjects []client.Object, updateErr, listErr error, listErrForType string) *Reconciler {
	scheme := runtime.NewScheme()
	require.NoError(t, v2pb.AddToScheme(scheme))
	// Build the underlying fake client first; then wrap it with an interceptor
	// that injects the same listErr/listErrForType into manager-side List calls
	// (which now go through the controller-runtime client, not api.Handler).
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(initialObjects...).WithStatusSubresource(initialObjects...).Build()
	managerClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(initialObjects...).
		WithStatusSubresource(initialObjects...).
		WithInterceptorFuncs(interceptor.Funcs{
			List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
				if listErr != nil && (listErrForType == "" || listErrForType == fmt.Sprintf("%T", list)) {
					return listErr
				}
				return c.List(ctx, list, opts...)
			},
		}).
		Build()
	logger := zaptest.NewLogger(t)
	handler := &updateErroringHandler{
		Handler:        apiHandler.NewFakeAPIHandler(k8sClient),
		updateErr:      updateErr,
		listErr:        listErr,
		listErrForType: listErrForType,
	}
	return &Reconciler{
		Handler:            handler,
		logger:             logger,
		triggerRunManager:  triggerrun.NewManager(managerClient, logger),
		pipelineRunManager: pipelinerun.NewManager(managerClient, logger),
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

// NOTE: Error coverage for the delete path is provided by
// TestCascadeDelete_RemoveFinalizer_UpdateError below, which exercises the
// handleDeletion error wrapping introduced in this PR.

func TestCascadeDelete_NoChildren(t *testing.T) {
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
}

func TestCascadeDelete_FinalizerAbsent(t *testing.T) {
	// Pipeline with a DeletionTimestamp but not our finalizer must not be
	// cascaded. handleDeletion returns immediately without listing children.
	// The fake client requires at least one finalizer when DeletionTimestamp
	// is set, so we attach an unrelated finalizer.
	now := metav1.Now()
	pipeline := &v2pb.Pipeline{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-pipeline",
			Namespace:         "test-namespace",
			Finalizers:        []string{"unrelated/finalizer"},
			DeletionTimestamp: &now,
		},
		Spec: v2pb.PipelineSpec{
			Commit: &v2pb.CommitInfo{GitRef: "abc123", Branch: "main"},
		},
	}
	// Seed a TR that would normally be killed. handleDeletion must not touch it.
	tr := &v2pb.TriggerRun{
		ObjectMeta: metav1.ObjectMeta{Name: "tr-running", Namespace: "test-namespace"},
		Spec: v2pb.TriggerRunSpec{
			Pipeline: &apipb.ResourceIdentifier{Name: "test-pipeline", Namespace: "test-namespace"},
		},
		Status: v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_RUNNING},
	}
	reconciler := setUpReconciler(t, []client.Object{pipeline, tr}, env.Context{})

	result, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-pipeline", Namespace: "test-namespace"},
	})
	require.NoError(t, err)
	require.Equal(t, ctrl.Result{}, result)

	untouched := &v2pb.TriggerRun{}
	require.NoError(t, reconciler.Get(context.Background(), "test-namespace", "tr-running", &metav1.GetOptions{}, untouched))
	require.NotEqual(t, v2pb.TRIGGER_RUN_ACTION_KILL, untouched.Spec.Action)
	require.False(t, untouched.Spec.Kill)
}

func TestCascadeDelete_ListTriggerRunsError(t *testing.T) {
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
	listErr := errors.New("list tr boom")
	reconciler := setUpReconcilerWithErrors(t, []client.Object{pipeline}, nil, listErr, "*v2.TriggerRunList")

	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-pipeline", Namespace: "test-namespace"},
	})
	require.Error(t, err)
	require.ErrorIs(t, err, listErr)
	require.Contains(t, err.Error(), "list trigger runs for pipeline test-namespace/test-pipeline")
}

func TestCascadeDelete_ListPipelineRunsError(t *testing.T) {
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
	listErr := errors.New("list pr boom")
	reconciler := setUpReconcilerWithErrors(t, []client.Object{pipeline}, nil, listErr, "*v2.PipelineRunList")

	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-pipeline", Namespace: "test-namespace"},
	})
	require.Error(t, err)
	require.ErrorIs(t, err, listErr)
	require.Contains(t, err.Error(), "list pipeline runs for pipeline test-namespace/test-pipeline")
}

func TestCascadeDelete_RemoveFinalizer_UpdateError(t *testing.T) {
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
	reconciler := setUpReconcilerWithErrors(t, []client.Object{pipeline}, updateErr, nil, "")

	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-pipeline", Namespace: "test-namespace"},
	})
	require.Error(t, err)
	require.ErrorIs(t, err, updateErr)
	require.Contains(t, err.Error(), "remove finalizer on pipeline test-namespace/test-pipeline")
}

func TestCascadeDelete_WithChildrenRequeues(t *testing.T) {
	// When children exist, handleDeletion does not remove the finalizer; it
	// requeues after reconcileInterval so a subsequent PR can perform kill/delete.
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
	tr := &v2pb.TriggerRun{
		ObjectMeta: metav1.ObjectMeta{Name: "tr-running", Namespace: "test-namespace"},
		Spec: v2pb.TriggerRunSpec{
			Pipeline: &apipb.ResourceIdentifier{Name: "test-pipeline", Namespace: "test-namespace"},
		},
		Status: v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_RUNNING},
	}
	reconciler := setUpReconciler(t, []client.Object{pipeline, tr}, env.Context{})

	result, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-pipeline", Namespace: "test-namespace"},
	})
	require.NoError(t, err)
	require.Equal(t, ctrl.Result{RequeueAfter: reconcileInterval}, result)

	// Finalizer should NOT have been removed yet.
	updated := &v2pb.Pipeline{}
	require.NoError(t, reconciler.Get(context.Background(), "test-namespace", "test-pipeline", &metav1.GetOptions{}, updated))
	require.True(t, controllerutil.ContainsFinalizer(updated, api.PipelineFinalizer))
}

func setUpReconciler(t *testing.T, initialObjects []client.Object, env env.Context) *Reconciler {
	scheme := runtime.NewScheme()
	err := v2pb.AddToScheme(scheme)
	require.NoError(t, err)
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(initialObjects...).
		WithStatusSubresource(initialObjects...).
		Build()
	logger := zaptest.NewLogger(t)
	handler := apiHandler.NewFakeAPIHandler(k8sClient)
	reconciler := &Reconciler{
		Handler:            handler,
		logger:             logger,
		triggerRunManager:  triggerrun.NewManager(k8sClient, logger),
		pipelineRunManager: pipelinerun.NewManager(k8sClient, logger),
	}
	return reconciler
}
