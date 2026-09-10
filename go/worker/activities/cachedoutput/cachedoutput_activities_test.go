package cachedoutput

import (
	"context"
	"mock/github.com/michelangelo-ai/michelangelo/proto-go/api/v2/v2mock"
	"net/http/httptest"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/cadence-workflow/starlark-worker/service"
	"github.com/cadence-workflow/starlark-worker/test/types"
	"github.com/golang/mock/gomock"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"github.com/stretchr/testify/suite"
)

type Suite struct {
	suite.Suite
	act                    *activities
	server                 *httptest.Server
	t                      *testing.T
	activitySuite          types.StarTestActivitySuite
	mockCachedOutput       *v2mock.MockCachedOutputServiceYARPCClient
	mockPipelineRunService *v2mock.MockPipelineRunServiceYARPCClient
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
		t:             t,
	})
}

func (r *Suite) SetupSuite() {
	ctrl := gomock.NewController(r.t)
	r.mockCachedOutput = v2mock.NewMockCachedOutputServiceYARPCClient(ctrl)
	r.mockPipelineRunService = v2mock.NewMockPipelineRunServiceYARPCClient(ctrl)
	r.act = &activities{
		cachedOutput:       r.mockCachedOutput,
		pipelineRunService: r.mockPipelineRunService,
	}
	r.activitySuite.RegisterActivity(r.act)
}
func (r *Suite) TearDownSuite() {}

func (r *Suite) BeforeTest(_, _ string) {}

func (r *Suite) Test_Get_Success() {
	request := &v2pb.GetCachedOutputRequest{
		Name:       "test",
		Namespace:  "default",
		GetOptions: nil,
	}
	r.mockCachedOutput.EXPECT().GetCachedOutput(gomock.Any(), request).Return(&v2pb.GetCachedOutputResponse{
		CachedOutput: &v2pb.CachedOutput{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test",
				Namespace: "default",
			},
		},
	}, nil)
	val, err := r.activitySuite.ExecuteActivity(Activities.GetCachedOutput, *request)
	r.Require().NoError(err)
	r.Require().True(val.HasValue())

	var res v2pb.GetCachedOutputResponse
	r.Require().NoError(val.Get(&res))
	r.Require().Equal("test", res.GetCachedOutput().Name)
	r.Require().Equal("default", res.GetCachedOutput().Namespace)
}

func (r *Suite) Test_ShouldOverrideCacheForRetry_NoRetryInfo() {
	r.mockPipelineRunService.EXPECT().GetPipelineRun(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, req *v2pb.GetPipelineRunRequest, _ ...interface{}) (*v2pb.GetPipelineRunResponse, error) {
			r.Require().Equal("default", req.Namespace)
			return &v2pb.GetPipelineRunResponse{PipelineRun: &v2pb.PipelineRun{}}, nil
		})

	request := ShouldOverrideCacheForRetryRequest{Namespace: "default", TaskPath: "a.b.task_a", TaskName: "task_a"}
	val, err := r.activitySuite.ExecuteActivity(Activities.ShouldOverrideCacheForRetry, request)
	r.Require().NoError(err)

	var res ShouldOverrideCacheForRetryResponse
	r.Require().NoError(val.Get(&res))
	r.Require().False(res.HasOverride)
}

func (r *Suite) Test_ShouldOverrideCacheForRetry_RetryTarget_KeepsCacheOff() {
	pipelineRun := &v2pb.PipelineRun{
		Spec: v2pb.PipelineRunSpec{
			RetryInfo: &v2pb.RetryInfo{ActivityId: "act-1"},
		},
		Status: v2pb.PipelineRunStatus{
			Steps: []*v2pb.PipelineRunStepInfo{
				{
					Name: "Execute Workflow",
					SubSteps: []*v2pb.PipelineRunStepInfo{
						{Name: "a.b.task_a", DisplayName: "task_a", ActivityId: "act-1"},
						{Name: "a.b.task_b", DisplayName: "task_b", ActivityId: "act-2"},
					},
				},
			},
		},
	}
	r.mockPipelineRunService.EXPECT().GetPipelineRun(gomock.Any(), gomock.Any()).Return(
		&v2pb.GetPipelineRunResponse{PipelineRun: pipelineRun}, nil)

	request := ShouldOverrideCacheForRetryRequest{Namespace: "default", TaskPath: "a.b.task_a", TaskName: "task_a"}
	val, err := r.activitySuite.ExecuteActivity(Activities.ShouldOverrideCacheForRetry, request)
	r.Require().NoError(err)

	var res ShouldOverrideCacheForRetryResponse
	r.Require().NoError(val.Get(&res))
	r.Require().True(res.HasOverride)
	r.Require().False(res.UseCache)
}

func (r *Suite) Test_ShouldOverrideCacheForRetry_Sibling_TurnsCacheOn() {
	pipelineRun := &v2pb.PipelineRun{
		Spec: v2pb.PipelineRunSpec{
			RetryInfo: &v2pb.RetryInfo{ActivityId: "act-1"},
		},
		Status: v2pb.PipelineRunStatus{
			Steps: []*v2pb.PipelineRunStepInfo{
				{
					Name: "Execute Workflow",
					SubSteps: []*v2pb.PipelineRunStepInfo{
						{Name: "a.b.task_a", DisplayName: "task_a", ActivityId: "act-1"},
						{Name: "a.b.task_b", DisplayName: "task_b", ActivityId: "act-2"},
					},
				},
			},
		},
	}
	r.mockPipelineRunService.EXPECT().GetPipelineRun(gomock.Any(), gomock.Any()).Return(
		&v2pb.GetPipelineRunResponse{PipelineRun: pipelineRun}, nil)

	request := ShouldOverrideCacheForRetryRequest{Namespace: "default", TaskPath: "a.b.task_b", TaskName: "task_b"}
	val, err := r.activitySuite.ExecuteActivity(Activities.ShouldOverrideCacheForRetry, request)
	r.Require().NoError(err)

	var res ShouldOverrideCacheForRetryResponse
	r.Require().NoError(val.Get(&res))
	r.Require().True(res.HasOverride)
	r.Require().True(res.UseCache)
}
