package pipeline

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	"github.com/michelangelo-ai/michelangelo/go/base/env"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"go.uber.org/zap/zaptest"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
)

func TestReconcile(t *testing.T) {
	testCases := []struct {
		name                         string
		initialObjects               []runtime.Object
		env                          env.Context
		expectedResult               ctrl.Result
		expectedError                string
		expectedStatusState          v2pb.PipelineState
		expectedStatusLatestRevision *apipb.ResourceIdentifier
	}{
		{
			name: "Invalid -> READY",
			initialObjects: []runtime.Object{
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
			expectedResult:               ctrl.Result{},
			expectedError:                "",
			expectedStatusState:          v2pb.PIPELINE_STATE_READY,
			expectedStatusLatestRevision: &apipb.ResourceIdentifier{
				Name:      "pipeline-test-pipeline-1234556",
				Namespace: "test-namespace",
			},
		},
		{
			name: "Ready -> should not reconcile",
			initialObjects: []runtime.Object{
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
			initialObjects: []runtime.Object{
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

func setUpReconciler(t *testing.T, initialObjects []runtime.Object, env env.Context) *Reconciler {
	scheme := runtime.NewScheme()
	err := v2pb.AddToScheme(scheme)
	require.NoError(t, err)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(initialObjects...).Build()
	reconciler := &Reconciler{
		Handler: apiHandler.NewFakeAPIHandler(k8sClient),
		logger:  zaptest.NewLogger(t),
	}
	return reconciler
}
