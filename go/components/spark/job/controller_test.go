package job

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stretchr/testify/require"

	"github.com/michelangelo-ai/michelangelo/go/components/testfakes"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	sparkJobName  = "test-spark-job"
	testNamespace = "default"
)

type mockSparkClient struct {
	createJobErr error
	getStatusErr error
	status       *string
	message      string
}

func (m *mockSparkClient) CreateJob(ctx context.Context, log logr.Logger, job *v2pb.SparkJob) error {
	return m.createJobErr
}

func (m *mockSparkClient) GetJobStatus(ctx context.Context, logger logr.Logger, job *v2pb.SparkJob) (*string, string, string, error) {
	return m.status, m.message, "", m.getStatusErr
}

func TestReconciler_Reconcile(t *testing.T) {
	ctx := context.Background()

	// Mock environment
	scheme := runtime.NewScheme()
	v2pb.AddToScheme(scheme)

	// Test cases
	tests := []struct {
		name           string
		setup          func() []client.Object
		errorAssertion require.ErrorAssertionFunc
		postCheck      func(res ctrl.Result)
		createErr      error
		getStatusErr   error
		getStatus      *string
		getMessage     string
	}{
		{
			name: "Spark job deleted",
			setup: func() []client.Object {
				return []client.Object{}
			},
			errorAssertion: require.NoError,
			postCheck: func(res ctrl.Result) {
				assert.Equal(t, time.Duration(0), res.RequeueAfter)
			},
		},
		{
			name: "Spark job creation fails",
			setup: func() []client.Object {
				sparkJob := &v2pb.SparkJob{
					ObjectMeta: metav1.ObjectMeta{
						Name:      sparkJobName,
						Namespace: testNamespace,
					},
				}
				return []client.Object{sparkJob}
			},
			getStatusErr:   status.Error(codes.NotFound, "resource not found"),
			createErr:      errors.New("some error"),
			errorAssertion: require.Error,
			postCheck: func(res ctrl.Result) {
				assert.Equal(t, requeueAfter, res.RequeueAfter)
			},
			getMessage: "",
			getStatus:  nil,
		},
		{
			name: "Spark job successfully created",
			setup: func() []client.Object {
				sparkJob := &v2pb.SparkJob{
					ObjectMeta: metav1.ObjectMeta{
						Name:      sparkJobName,
						Namespace: testNamespace,
					},
				}
				return []client.Object{sparkJob}
			},
			errorAssertion: require.NoError,
			postCheck: func(res ctrl.Result) {
				assert.Equal(t, requeueAfter, res.RequeueAfter)
			},
			getStatusErr: status.Error(codes.NotFound, "resource not found"),
			getMessage:   "",
			getStatus:    nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			objects := tc.setup()
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()
			fakeClientWrapper := testfakes.NewFakeClientWrapper(fakeClient)

			r := &Reconciler{
				Client: fakeClientWrapper,
				SparkClient: &mockSparkClient{
					createJobErr: tc.createErr,
					getStatusErr: tc.getStatusErr,
					status:       tc.getStatus,
					message:      tc.getMessage,
				},
			}

			requestSparkJob := types.NamespacedName{
				Name:      sparkJobName,
				Namespace: testNamespace,
			}

			// Act
			res, err := r.Reconcile(ctx, ctrl.Request{
				NamespacedName: requestSparkJob,
			})

			// Assert
			tc.errorAssertion(t, err)
			tc.postCheck(res)

			var updatedSparkJob v2pb.SparkJob
			_ = r.Get(ctx, requestSparkJob, &updatedSparkJob)
		})
	}
}
