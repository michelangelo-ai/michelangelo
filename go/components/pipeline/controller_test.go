package pipeline

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/michelangelo-ai/michelangelo/go/base/env"
	"go.uber.org/zap/zaptest"
	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	ctrl "sigs.k8s.io/controller-runtime"
	"k8s.io/apimachinery/pkg/types"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

func TestReconcile(t *testing.T) {
	testCases := []struct {
		name string
		initialObjects []runtime.Object
		env env.Context
		expectedResult ctrl.Result
		expectedError string
		expectedStatusState v2pb.PipelineState
		expectedStatusCommit *v2pb.CommitInfo
	}{
		{
			name: "Invalid -> Created",
			initialObjects: []runtime.Object{
				&v2pb.Pipeline{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-pipeline",
						Namespace: "test-namespace",
					},
					Spec: v2pb.PipelineSpec{
						Commit: &v2pb.CommitInfo{
							GitRef: "test-git-ref",
							Branch: "test-git-branch",
						},
					},
				},
			},
			expectedResult: ctrl.Result{RequeueAfter: reconcileInterval},
			expectedError: "",
			expectedStatusState: v2pb.PIPELINE_STATE_CREATED,
			expectedStatusCommit: nil,
		},
		{
			name: "Created -> Ready",
			initialObjects: []runtime.Object{
				&v2pb.Pipeline{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-pipeline",
						Namespace: "test-namespace",
					},
					Spec: v2pb.PipelineSpec{
						Commit: &v2pb.CommitInfo{
							GitRef: "test-git-ref",
							Branch: "test-git-branch",
						},
					},
					Status: v2pb.PipelineStatus{
						State: v2pb.PIPELINE_STATE_CREATED,
					},
				},
			},
			expectedResult: ctrl.Result{},
			expectedError: "",
			expectedStatusState: v2pb.PIPELINE_STATE_READY,
			expectedStatusCommit: &v2pb.CommitInfo{GitRef: "test-git-ref", Branch: "test-git-branch"},
		},
		{
			name: "Ready -> Ready",
			initialObjects: []runtime.Object{
				&v2pb.Pipeline{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-pipeline",
						Namespace: "test-namespace",
					},
					Spec: v2pb.PipelineSpec{
						Commit: &v2pb.CommitInfo{
							GitRef: "test-git-ref",
							Branch: "test-git-branch",
						},
					},
					Status: v2pb.PipelineStatus{
						State: v2pb.PIPELINE_STATE_READY,
						Commit: &v2pb.CommitInfo{GitRef: "test-git-ref", Branch: "test-git-branch"},
					},
				},
			},
			expectedResult: ctrl.Result{},
			expectedError: "",
			expectedStatusState: v2pb.PIPELINE_STATE_READY,
			expectedStatusCommit: &v2pb.CommitInfo{GitRef: "test-git-ref", Branch: "test-git-branch"},
		},
		{
			name: "Ready -> Invalid",
			initialObjects: []runtime.Object{
				&v2pb.Pipeline{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-pipeline",
						Namespace: "test-namespace",
					},
					Spec: v2pb.PipelineSpec{
						Commit: &v2pb.CommitInfo{
							GitRef: "test-git-ref-2",
							Branch: "test-git-branch-2",
						},
					},
					Status: v2pb.PipelineStatus{
						State: v2pb.PIPELINE_STATE_READY,
						Commit: &v2pb.CommitInfo{GitRef: "test-git-ref", Branch: "test-git-branch"},
					},
				},
			},
			expectedResult: ctrl.Result{RequeueAfter: reconcileInterval},
			expectedError: "",
			expectedStatusState: v2pb.PIPELINE_STATE_INVALID,
			expectedStatusCommit: &v2pb.CommitInfo{GitRef: "test-git-ref", Branch: "test-git-branch"},
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
			if tc.expectedStatusCommit != nil {
				require.Equal(t, tc.expectedStatusCommit, pipeline.Status.Commit)
			} else {
				require.Nil(t, pipeline.Status.Commit)
			}
		})
	}
}

func setUpReconciler(t *testing.T, initialObjects []runtime.Object, env env.Context, ) *Reconciler {
	scheme := runtime.NewScheme()
	err := v2pb.AddToScheme(scheme)
	require.NoError(t, err)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(initialObjects...).Build()
	reconciler := &Reconciler{
		Handler:           apiHandler.NewFakeAPIHandler(k8sClient),
		logger:            zaptest.NewLogger(t),
	}
	return reconciler
}
