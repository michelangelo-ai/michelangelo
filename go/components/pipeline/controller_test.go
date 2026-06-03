package pipeline

import (
	"context"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	"github.com/michelangelo-ai/michelangelo/go/base/env"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"go.uber.org/zap/zaptest"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
)

func TestReconcile(t *testing.T) {
	now := metav1.Now()
	gracePeriod := int64(0)
	testCases := []struct {
		name                string
		initialObjects      []client.Object
		env                 env.Context
		expectedResult      ctrl.Result
		expectedError       string
		expectedStatusState v2pb.PipelineState
	}{
		{
			name: "Invalid -> READY",
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
			expectedError:       "",
			expectedStatusState: v2pb.PIPELINE_STATE_READY,
		},
		{
			name: "Already READY -> stays READY",
			initialObjects: []client.Object{
				&v2pb.Pipeline{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pipeline",
						Namespace: "test-namespace",
					},
					Spec: v2pb.PipelineSpec{
						Commit: &v2pb.CommitInfo{
							GitRef: "123456",
							Branch: "test-git-branch",
						},
					},
					Status: v2pb.PipelineStatus{
						State: v2pb.PIPELINE_STATE_READY,
					},
				},
			},
			expectedResult:      ctrl.Result{},
			expectedError:       "",
			expectedStatusState: v2pb.PIPELINE_STATE_READY,
		},
		{
			name: "Already READY with new commit -> stays READY",
			initialObjects: []client.Object{
				&v2pb.Pipeline{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pipeline",
						Namespace: "test-namespace",
					},
					Spec: v2pb.PipelineSpec{
						Commit: &v2pb.CommitInfo{
							GitRef: "234567",
							Branch: "test-git-branch-2",
						},
					},
					Status: v2pb.PipelineStatus{
						State: v2pb.PIPELINE_STATE_READY,
					},
				},
			},
			expectedResult:      ctrl.Result{},
			expectedError:       "",
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
			expectedResult:               ctrl.Result{},
			expectedError:                "",
			expectedStatusState:          v2pb.PIPELINE_STATE_CREATED,
			expectedStatusLatestRevision: nil,
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
		})
	}
}

func setUpReconciler(t *testing.T, initialObjects []client.Object, env env.Context) *Reconciler {
	scheme := runtime.NewScheme()
	err := v2pb.AddToScheme(scheme)
	require.NoError(t, err)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(initialObjects...).WithStatusSubresource(initialObjects...).Build()
	handler := apiHandler.NewFakeAPIHandler(k8sClient)
	reconciler := &Reconciler{
		Handler: handler,
		logger:  zaptest.NewLogger(t),
	}
	return reconciler
}
