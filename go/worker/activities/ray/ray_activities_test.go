package ray

import (
	"testing"

	"github.com/cadence-workflow/starlark-worker/service"
	"github.com/cadence-workflow/starlark-worker/test/types"
	"github.com/stretchr/testify/suite"

	"github.com/golang/mock/gomock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"mock/github.com/michelangelo-ai/michelangelo/proto-go/api/v2/v2mock"

	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"github.com/stretchr/testify/assert"
)

type Suite struct {
	suite.Suite
	activitySuite  types.StarTestActivitySuite
	t              *testing.T
	mockRayJob     *v2mock.MockRayJobServiceYARPCClient
	mockRayCluster *v2mock.MockRayClusterServiceYARPCClient
	act            *activities
}

func TestITCadence(t *testing.T) {
	suite.Run(t, &Suite{
		activitySuite: service.NewCadTestActivitySuite(),
		t:             t,
	})
}

func TestITTemporal(t *testing.T) {
	suite.Run(t, &Suite{
		activitySuite: service.NewTempTestActivitySuite(),
		t:             t})
}

func (r *Suite) SetupSuite() {
	ctrl := gomock.NewController(r.t)
	r.mockRayJob = v2mock.NewMockRayJobServiceYARPCClient(ctrl)
	r.mockRayCluster = v2mock.NewMockRayClusterServiceYARPCClient(ctrl)
	r.act = &activities{
		rayJobService:     r.mockRayJob,
		rayClusterService: r.mockRayCluster,
	}
	r.activitySuite.RegisterActivity(r.act)
}
func (r *Suite) TearDownSuite() {}

func (r *Suite) BeforeTest(_, _ string) {

}

func (r *Suite) Test_CreateRayJob() {
	rayJob := &v2pb.RayJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test",
		},
		Spec: v2pb.RayJobSpec{},
	}
	request := &v2pb.CreateRayJobRequest{
		RayJob:        rayJob,
		CreateOptions: &metav1.CreateOptions{},
	}
	r.mockRayJob.EXPECT().CreateRayJob(gomock.Any(), request).Return(&v2pb.CreateRayJobResponse{
		RayJob: rayJob,
	}, nil)
	res, err := r.activitySuite.ExecuteActivity(Activities.CreateRayJob, *request)
	var resp CreateRayJobActivityResponse
	res.Get(&resp)
	assert.Nil(r.t, err)
	assert.NotNil(r.t, resp)
	assert.Equal(r.t, rayJob.Name, resp.RayJob.Name)
	assert.Equal(r.t, rayJob.Namespace, resp.RayJob.Namespace)
	assert.NotEmpty(r.t, resp.ActivityID)
}

func (r *Suite) Test_CreateRayCluster() {
	rayCluster := &v2pb.RayCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test",
		},
		Spec: v2pb.RayClusterSpec{},
	}
	request := &v2pb.CreateRayClusterRequest{
		RayCluster:    rayCluster,
		CreateOptions: &metav1.CreateOptions{},
	}
	r.mockRayCluster.EXPECT().CreateRayCluster(gomock.Any(), request).Return(&v2pb.CreateRayClusterResponse{
		RayCluster: rayCluster,
	}, nil)
	res, err := r.activitySuite.ExecuteActivity(Activities.CreateRayCluster, *request)
	var resp CreateRayClusterActivityResponse
	res.Get(&resp)
	assert.Nil(r.t, err)
	assert.NotNil(r.t, resp)
	assert.NotNil(r.t, resp.RayCluster)
	assert.Equal(r.t, rayCluster.Name, resp.RayCluster.Name)
	assert.Equal(r.t, rayCluster.Namespace, resp.RayCluster.Namespace)
}

func (r *Suite) Test_TerminateCluster() {
	reason := "job failed"
	rayCluster := &v2pb.RayCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test",
		},
		Spec: v2pb.RayClusterSpec{},
	}
	request := &v2pb.UpdateRayClusterRequest{
		RayCluster:    rayCluster,
		UpdateOptions: &metav1.UpdateOptions{},
	}

	r.mockRayCluster.EXPECT().GetRayCluster(gomock.Any(), gomock.Any()).Return(&v2pb.GetRayClusterResponse{
		RayCluster: rayCluster,
	}, nil)
	r.mockRayCluster.EXPECT().UpdateRayCluster(gomock.Any(), request).Return(&v2pb.UpdateRayClusterResponse{
		RayCluster: rayCluster,
	}, nil)

	res, err := r.activitySuite.ExecuteActivity(Activities.TerminateCluster, TerminateClusterRequest{
		Name:      rayCluster.Name,
		Namespace: rayCluster.Namespace,
		Type:      v2pb.TERMINATION_TYPE_SUCCEEDED.String(),
		Reason:    reason,
	})

	var resp v2pb.UpdateRayClusterResponse
	res.Get(&res)

	assert.Nil(r.t, err)
	assert.NotNil(r.t, resp)
	assert.Equal(r.t, v2pb.TERMINATION_TYPE_SUCCEEDED, request.RayCluster.Spec.Termination.Type)
	assert.Equal(r.t, reason, request.RayCluster.Spec.Termination.Reason)
}
