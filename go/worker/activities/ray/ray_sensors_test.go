package ray

import (
	"errors"
	"slices"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/temporal"
	"go.uber.org/cadence"
	"go.uber.org/yarpc/yarpcerrors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/michelangelo-ai/michelangelo/go/worker/plugins/utils"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// sensorOutcomes is the state -> outcome contract of the two provisioning sensors.
var sensorOutcomes = []struct {
	state        v2pb.RayClusterState
	provisioning sensorOutcome
	admission    sensorOutcome
}{
	{v2pb.RAY_CLUSTER_STATE_INVALID, sensorPending, sensorDone},
	{v2pb.RAY_CLUSTER_STATE_PROVISIONING, sensorPending, sensorDone},
	{v2pb.RAY_CLUSTER_STATE_READY, sensorDone, sensorDone},
	{v2pb.RAY_CLUSTER_STATE_TERMINATING, sensorPending, sensorDone},
	{v2pb.RAY_CLUSTER_STATE_TERMINATED, sensorTerminal, sensorTerminal},
	{v2pb.RAY_CLUSTER_STATE_FAILED, sensorTerminal, sensorTerminal},
	{v2pb.RAY_CLUSTER_STATE_UNKNOWN, sensorPending, sensorDone},
	{v2pb.RAY_CLUSTER_STATE_UNHEALTHY, sensorPending, sensorDone},
	{v2pb.RAY_CLUSTER_STATE_SUSPENDED, sensorDone, sensorPending},
}

func TestSensorOutcomes(t *testing.T) {
	require.Len(t, sensorOutcomes, len(v2pb.RayClusterState_name), "every state has an outcome")
	for _, tc := range sensorOutcomes {
		t.Run(tc.state.String(), func(t *testing.T) {
			require.Equal(t, tc.provisioning, provisioningOutcome(tc.state), "provisioning")
			require.Equal(t, tc.admission, admissionOutcome(tc.state), "admission")
		})
	}
}

var sensorTestRequest = v2pb.GetRayClusterRequest{
	Name:       "test",
	Namespace:  "test",
	GetOptions: &metav1.GetOptions{},
}

func sensorTestCluster(state v2pb.RayClusterState) *v2pb.RayCluster {
	return &v2pb.RayCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "test"},
		Status:     v2pb.RayClusterStatus{State: state, JobUrl: "http://ray-dashboard"},
	}
}

// sensorError returns the reason of a sensor activity error and whether the sensor retry policy
// (utils.DefaultNonRetriableErrorReasons) retries it, for the error types the Cadence and the Temporal test
// environments return.
func sensorError(err error) (reason string, retryable bool) {
	var cadenceErr *cadence.CustomError
	if errors.As(err, &cadenceErr) {
		return cadenceErr.Reason(), !slices.Contains(utils.DefaultNonRetriableErrorReasons, cadenceErr.Reason())
	}
	var temporalErr *temporal.ApplicationError
	if errors.As(err, &temporalErr) {
		return temporalErr.Message(), !temporalErr.NonRetryable() &&
			!slices.Contains(utils.DefaultNonRetriableErrorReasons, temporalErr.Type())
	}
	return err.Error(), true
}

func (r *Suite) requireSensorError(err error, outcome sensorOutcome) {
	r.Require().Error(err)
	reason, retryable := sensorError(err)
	switch outcome {
	case sensorPending:
		r.Equal(yarpcerrors.CodeFailedPrecondition.String(), reason)
		r.True(retryable, "pending states are polled again")
	case sensorTerminal:
		r.Equal(yarpcerrors.CodeInternal.String(), reason)
		r.False(retryable, "terminal states end the wait")
	default:
		r.Failf("unexpected error", "outcome %d: %v", outcome, err)
	}
}

func (r *Suite) Test_SensorRayClusterProvisioning() {
	for _, tc := range sensorOutcomes {
		r.Run(tc.state.String(), func() {
			cluster := sensorTestCluster(tc.state)
			r.mockRayCluster.EXPECT().GetRayCluster(gomock.Any(), gomock.Any()).Return(&v2pb.GetRayClusterResponse{RayCluster: cluster}, nil)

			res, err := r.activitySuite.ExecuteActivity(Activities.SensorRayClusterProvisioning, sensorTestRequest)
			if tc.provisioning != sensorDone {
				r.requireSensorError(err, tc.provisioning)
				return
			}
			r.Require().NoError(err)
			var resp SensorRayClusterProvisioningResponse
			r.Require().NoError(res.Get(&resp))
			r.Equal(tc.state == v2pb.RAY_CLUSTER_STATE_READY, resp.Ready)
			r.Equal("http://ray-dashboard", resp.JobURL)
			r.Require().NotNil(resp.RayCluster)
			r.Equal(tc.state, resp.RayCluster.Status.State)
		})
	}
}

func (r *Suite) Test_SensorRayClusterAdmission() {
	for _, tc := range sensorOutcomes {
		r.Run(tc.state.String(), func() {
			cluster := sensorTestCluster(tc.state)
			r.mockRayCluster.EXPECT().GetRayCluster(gomock.Any(), gomock.Any()).Return(&v2pb.GetRayClusterResponse{RayCluster: cluster}, nil)

			res, err := r.activitySuite.ExecuteActivity(Activities.SensorRayClusterAdmission, sensorTestRequest)
			if tc.admission != sensorDone {
				r.requireSensorError(err, tc.admission)
				return
			}
			r.Require().NoError(err)
			var resp v2pb.GetRayClusterResponse
			r.Require().NoError(res.Get(&resp))
			r.Require().NotNil(resp.RayCluster)
			r.Equal(tc.state, resp.RayCluster.Status.State)
		})
	}
}

func (r *Suite) Test_SensorRayClusterProvisioning_ReadFailureIsRetried() {
	r.mockRayCluster.EXPECT().GetRayCluster(gomock.Any(), gomock.Any()).Return(nil, yarpcerrors.UnavailableErrorf("apiserver unavailable"))
	_, err := r.activitySuite.ExecuteActivity(Activities.SensorRayClusterProvisioning, sensorTestRequest)
	r.Require().Error(err)
	_, retryable := sensorError(err)
	r.True(retryable)

	r.mockRayCluster.EXPECT().GetRayCluster(gomock.Any(), gomock.Any()).Return(&v2pb.GetRayClusterResponse{}, nil)
	_, err = r.activitySuite.ExecuteActivity(Activities.SensorRayClusterAdmission, sensorTestRequest)
	r.Require().Error(err)
	_, retryable = sensorError(err)
	r.True(retryable)
}
