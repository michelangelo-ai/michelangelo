package rayjob

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
	"time"

	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	rayv1fake "github.com/ray-project/kuberay/ray-operator/pkg/client/clientset/versioned/typed/ray/v1/fake"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kubescheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	rayJobName    = "test-job"
	testNamespace = "default"
)

func TestReconciler_Reconcile(t *testing.T) {
	ctx := context.Background()

	// Mock environment
	scheme := runtime.NewScheme()
	kubescheme.AddToScheme(scheme)
	v2pb.AddToScheme(scheme)


	// Test cases
	tests := []struct {
		name            string
		setup           func() []client.Object
		expectedState   v2pb.RayJobState
		expectedMessage string
		errorAssertion  require.ErrorAssertionFunc
		postCheck         func(res ctrl.Result)
	}{
		{
			name: "No ray job",
			setup: func() []client.Object {
				objects := make([]client.Object, 0)
				return objects
			},
			expectedState:   v2pb.RAY_JOB_STATE_INVALID,
			expectedMessage: "",
			errorAssertion:  require.NoError,
			postCheck: func(res ctrl.Result) {
				assert.Equal(t, res.RequeueAfter, time.Duration(0))
			},
		},
		{
			name: "Cluster not set",
			setup: func() []client.Object {
				objects := make([]client.Object, 0)
				rayJob := &v2pb.RayJob{
					ObjectMeta: metav1.ObjectMeta{
						Name:      rayJobName,
						Namespace: testNamespace,
					},
					Spec: v2pb.RayJobSpec{
						Cluster: nil,
					},
				}
				objects = append(objects, rayJob)
				return objects
			},
			expectedState:   v2pb.RAY_JOB_STATE_FAILED,
			expectedMessage: "cluster is not set",
			errorAssertion:  require.NoError,
			postCheck: func(res ctrl.Result) {
				assert.Equal(t, res.RequeueAfter, requeueAfter)
			},
		},
		{
			name: "Cluster not found",
			setup: func() []client.Object {
				objects := make([]client.Object, 0)
				rayJob := &v2pb.RayJob{
					ObjectMeta: metav1.ObjectMeta{
						Name:      rayJobName,
						Namespace: testNamespace,
					},
					Spec: v2pb.RayJobSpec{
						Cluster: &apipb.ResourceIdentifier{
							Name:      "missing-cluster",
							Namespace: testNamespace,
						},
					},
				}
				objects = append(objects, rayJob)
				return objects
			},
			expectedState:   v2pb.RAY_JOB_STATE_FAILED,
			expectedMessage: "failed to find cluster",
			errorAssertion:  require.NoError,
			postCheck: func(res ctrl.Result) {
				assert.Equal(t, res.RequeueAfter, requeueAfter)
			},
		},
		{
			name: "Successful reconciliation",
			setup: func() []client.Object {
				objects := make([]client.Object, 0)
				rayJob := &v2pb.RayJob{
					ObjectMeta: metav1.ObjectMeta{
						Name:      rayJobName,
						Namespace: testNamespace,
					},
					Spec: v2pb.RayJobSpec{
						Cluster: &apipb.ResourceIdentifier{
							Name:      "existing-cluster",
							Namespace: testNamespace,
						},
						Entrypoint: "echo Hello World",
					},
				}
				cluster := &v2pb.RayCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "existing-cluster",
						Namespace: testNamespace,
					},
				}
				objects = append(objects, rayJob)
				objects = append(objects, cluster)
				return objects
			},
			expectedState:   v2pb.RAY_JOB_STATE_INITIALIZING,
			expectedMessage: "",
			errorAssertion:  require.NoError,
			postCheck: func(res ctrl.Result) {
				assert.Equal(t, res.RequeueAfter, time.Duration(0))
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			objects := tc.setup()
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()
			// Set up a fake RayV1 client.
			fakeRayV1Client := &rayv1fake.FakeRayV1{}
			fakeClientWrapper := NewFakeClientWrapper(fakeClient)

			r := &Reconciler{
				Client:      fakeClientWrapper,
				RayV1Interface: fakeRayV1Client,
			}

			requestRayJob := types.NamespacedName{
				Name:      rayJobName,
				Namespace: testNamespace,
			}
			res, err := r.Reconcile(ctx, ctrl.Request{
				NamespacedName: requestRayJob,
			})
			tc.errorAssertion(t, err)
			tc.postCheck(res)

			// Assert
			var updatedRayJob v2pb.RayJob
			_ = r.Get(ctx, requestRayJob, &updatedRayJob)

			assert.Equal(t, tc.expectedState, updatedRayJob.Status.State)
			assert.Contains(t, updatedRayJob.Status.Message, tc.expectedMessage)
		})
	}
}
