package job

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"

	"github.com/golang/mock/gomock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiutils "github.com/michelangelo-ai/michelangelo/go/api/utils"
	constants "github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	"github.com/michelangelo-ai/michelangelo/go/components/spark/job/jobmocks"
	"github.com/michelangelo-ai/michelangelo/go/components/testfakes"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
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

func TestReconciler_Reconcile(t *testing.T) {
	ctx := context.Background()

	scheme := runtime.NewScheme()
	v2pb.AddToScheme(scheme)

	tests := []struct {
		name           string
		setup          func() []client.Object
		errorAssertion require.ErrorAssertionFunc
		postCheck      func(res ctrl.Result)
		setupMock      func(m *jobmocks.MockClient)
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
			setupMock: func(m *jobmocks.MockClient) {},
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
			errorAssertion: require.Error,
			postCheck: func(res ctrl.Result) {
				assert.Equal(t, requeueAfter, res.RequeueAfter)
			},
			setupMock: func(m *jobmocks.MockClient) {
				m.EXPECT().GetJobStatus(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, "", "", grpcstatus.Error(codes.NotFound, "resource not found"))
				m.EXPECT().CreateJob(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(errors.New("some error"))
			},
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
			setupMock: func(m *jobmocks.MockClient) {
				m.EXPECT().GetJobStatus(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, "", "", grpcstatus.Error(codes.NotFound, "resource not found"))
				m.EXPECT().CreateJob(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)

			objects := tc.setup()
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()
			fakeClientWrapper := testfakes.NewFakeClientWrapper(fakeClient)

			mockClient := jobmocks.NewMockClient(mockCtrl)
			tc.setupMock(mockClient)

			r := &Reconciler{
				Client:      fakeClientWrapper,
				sparkClient: mockClient,
			}

			requestSparkJob := types.NamespacedName{
				Name:      sparkJobName,
				Namespace: testNamespace,
			}

			res, err := r.Reconcile(ctx, ctrl.Request{
				NamespacedName: requestSparkJob,
			})

			tc.errorAssertion(t, err)
			tc.postCheck(res)

			var updatedSparkJob v2pb.SparkJob
			_ = r.Get(ctx, requestSparkJob, &updatedSparkJob)
		})
	}
}

// TestReconciler_Reconcile_Termination verifies that a SparkJob with
// Spec.Termination set drives the reconciler to terminate the underlying
// SparkApplication and transition the SparkJob to a terminal, immutable state.
func TestReconciler_Reconcile_Termination(t *testing.T) {
	ctx := context.Background()

	scheme := runtime.NewScheme()
	v2pb.AddToScheme(scheme)

	tests := []struct {
		name            string
		deleteJobErr    error
		terminationType v2pb.TerminationType
		wantSucceeded   apipb.ConditionStatus
	}{
		{
			name:            "failed termination deletes SparkApplication, marks killed and not-succeeded",
			deleteJobErr:    nil,
			terminationType: v2pb.TERMINATION_TYPE_FAILED,
			wantSucceeded:   apipb.CONDITION_STATUS_FALSE,
		},
		{
			name:            "succeeded termination marks killed and succeeded",
			deleteJobErr:    nil,
			terminationType: v2pb.TERMINATION_TYPE_SUCCEEDED,
			wantSucceeded:   apipb.CONDITION_STATUS_TRUE,
		},
		{
			name:            "termination tolerates already-deleted SparkApplication",
			deleteJobErr:    grpcstatus.Error(codes.NotFound, "resource not found"),
			terminationType: v2pb.TERMINATION_TYPE_FAILED,
			wantSucceeded:   apipb.CONDITION_STATUS_FALSE,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)

			sparkJob := &v2pb.SparkJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      sparkJobName,
					Namespace: testNamespace,
				},
				Spec: v2pb.SparkJobSpec{
					Termination: &v2pb.TerminationSpec{
						Type:   tc.terminationType,
						Reason: "workflow cancelled",
					},
				},
			}

			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(sparkJob).Build()
			fakeClientWrapper := testfakes.NewFakeClientWrapper(fakeClient)

			mockClient := jobmocks.NewMockClient(mockCtrl)
			mockClient.EXPECT().DeleteJob(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(tc.deleteJobErr)

			r := &Reconciler{
				Client:      fakeClientWrapper,
				sparkClient: mockClient,
			}

			requestSparkJob := types.NamespacedName{
				Name:      sparkJobName,
				Namespace: testNamespace,
			}

			res, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: requestSparkJob})

			require.NoError(t, err)
			assert.Equal(t, time.Duration(0), res.RequeueAfter, "terminal SparkJob should not be requeued")

			var updatedSparkJob v2pb.SparkJob
			require.NoError(t, r.Get(ctx, requestSparkJob, &updatedSparkJob))

			killed := findCondition(updatedSparkJob.Status.StatusConditions, constants.KilledCondition)
			require.NotNil(t, killed, "Killed condition should be set")
			assert.Equal(t, apipb.CONDITION_STATUS_TRUE, killed.Status)
			assert.Equal(t, "workflow cancelled", killed.Message)

			succeeded := findCondition(updatedSparkJob.Status.StatusConditions, constants.SucceededCondition)
			require.NotNil(t, succeeded, "Succeeded condition should be set")
			assert.Equal(t, tc.wantSucceeded, succeeded.Status)
			assert.Equal(t, "workflow cancelled", succeeded.Reason)

			assert.True(t, apiutils.IsImmutable(&updatedSparkJob), "terminated SparkJob should be marked immutable")
		})
	}
}

// findCondition returns the condition of the given type, or nil if not present.
func findCondition(conditions []*apipb.Condition, conditionType string) *apipb.Condition {
	for _, cond := range conditions {
		if cond.Type == conditionType {
			return cond
		}
	}
	return nil
}
