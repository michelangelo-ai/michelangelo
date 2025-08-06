package revision

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	ctrl "sigs.k8s.io/controller-runtime"
	"go.uber.org/zap/zaptest"

	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	"github.com/michelangelo-ai/michelangelo/go/base/env"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

func TestReconcile(t *testing.T) {
	testCases := []struct {
		name                string
		initialObjects      []runtime.Object
		env                 env.Context
		expectedResult      ctrl.Result
		expectedError       string
		expectedStatusState v2pb.RevisionState
	}{
		{
			name: "CREATED -> BUILDING",
			initialObjects: []runtime.Object{
				&v2pb.Revision{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-revision",
						Namespace: "test-namespace",
					},
					Spec: v2pb.RevisionSpec{
						BaseType: &metav1.TypeMeta{
							Kind:       "Pipeline",
							APIVersion: "michelangelo.api.v2/Pipeline",
						},
						RevisionId: "abc123456789",
					},
					Status: v2pb.RevisionStatus{
						State: v2pb.REVISION_STATE_CREATED,
					},
				},
			},
			expectedResult:      ctrl.Result{RequeueAfter: reconcileInterval},
			expectedError:       "",
			expectedStatusState: v2pb.REVISION_STATE_BUILDING,
		},
		{
			name: "BUILDING -> READY",
			initialObjects: []runtime.Object{
				&v2pb.Revision{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-revision",
						Namespace: "test-namespace",
					},
					Spec: v2pb.RevisionSpec{
						BaseType: &metav1.TypeMeta{
							Kind:       "Pipeline",
							APIVersion: "michelangelo.api.v2/Pipeline",
						},
						RevisionId: "def123456789",
					},
					Status: v2pb.RevisionStatus{
						State: v2pb.REVISION_STATE_BUILDING,
					},
				},
			},
			expectedResult:      ctrl.Result{},
			expectedError:       "",
			expectedStatusState: v2pb.REVISION_STATE_READY,
		},
		{
			name: "READY -> No Change (Terminal State)",
			initialObjects: []runtime.Object{
				&v2pb.Revision{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-revision",
						Namespace: "test-namespace",
					},
					Spec: v2pb.RevisionSpec{
						BaseType: &metav1.TypeMeta{
							Kind:       "Pipeline",
							APIVersion: "michelangelo.api.v2/Pipeline",
						},
						RevisionId: "ghi123456789",
					},
					Status: v2pb.RevisionStatus{
						State: v2pb.REVISION_STATE_READY,
					},
				},
			},
			expectedResult:      ctrl.Result{},
			expectedError:       "",
			expectedStatusState: v2pb.REVISION_STATE_READY,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			reconciler := setUpReconciler(t, tc.initialObjects, tc.env)
			ctx := context.Background()

			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      "test-revision",
					Namespace: "test-namespace",
				},
			})

			if tc.expectedError != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.expectedError)
			} else {
				require.NoError(t, err)
			}

			require.Equal(t, tc.expectedResult, result)

			// Check the updated revision status
			revision := &v2pb.Revision{}
			err = reconciler.Get(ctx, "test-namespace", "test-revision", &metav1.GetOptions{}, revision)
			require.NoError(t, err)
			require.Equal(t, tc.expectedStatusState, revision.Status.State)
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