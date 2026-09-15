package model

import (
	"context"
	"errors"
	"mock/github.com/michelangelo-ai/michelangelo/proto-go/api/v2/v2mock"
	"net/http/httptest"
	"testing"

	gogotypes "github.com/gogo/protobuf/types"

	"github.com/cadence-workflow/starlark-worker/service"
	"github.com/cadence-workflow/starlark-worker/test/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

type Suite struct {
	suite.Suite
	act                   *activities
	server                *httptest.Server
	t                     *testing.T
	activitySuite         types.StarTestActivitySuite
	mockModelService      *v2mock.MockModelServiceYARPCClient
	mockDeploymentService *v2mock.MockDeploymentServiceYARPCClient
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
	r.mockModelService = v2mock.NewMockModelServiceYARPCClient(ctrl)
	r.mockDeploymentService = v2mock.NewMockDeploymentServiceYARPCClient(ctrl)
	r.act = &activities{
		modelService:      r.mockModelService,
		deploymentService: r.mockDeploymentService,
	}
	r.activitySuite.RegisterActivity(r.act)
}
func (r *Suite) TearDownSuite() {}

func (r *Suite) BeforeTest(_, _ string) {}

func (r *Suite) Test_ModelSearch_Succeeded() {
	modelService := r.act.modelService.(*v2mock.MockModelServiceYARPCClient)
	deploymentService := r.act.deploymentService.(*v2mock.MockDeploymentServiceYARPCClient)
	model := &v2pb.Model{
		ObjectMeta: v1.ObjectMeta{Name: "model-test-123", Namespace: "ma-test-sandbox"},
		Spec: v2pb.ModelSpec{
			RevisionId: 11,
		},
	}
	gomock.InOrder(
		deploymentService.EXPECT().GetDeployment(gomock.Any(), gomock.Any()).Return(
			&v2pb.GetDeploymentResponse{
				Deployment: &v2pb.Deployment{
					ObjectMeta: v1.ObjectMeta{Name: "test-deployment", Namespace: "ma-test-sandbox"},
					Spec: v2pb.DeploymentSpec{
						DesiredRevision: &api.ResourceIdentifier{
							Namespace: "ma-test-sandbox",
							Name:      "model-test-123-1",
						},
					},
				},
			},
			nil,
		),
		modelService.EXPECT().GetModel(gomock.Any(), gomock.Any()).Return(
			&v2pb.GetModelResponse{
				Model: model,
			},
			nil,
		),
	)
	val, err := r.activitySuite.ExecuteActivity(Activities.ModelSearch, &ModelSearchRequest{
		Namespace:      "ma-test-sandbox",
		DeploymentName: "test-deployment",
	})

	var res *ModelSearchResponse
	err = val.Get(&res)
	assert.NoError(r.t, err)
	assert.Equal(r.t, "ma-test-sandbox", res.Namespace)
	assert.Equal(r.t, "model-test-123", res.ModelName)
	assert.Equal(r.t, int32(1), res.ModelRevisionID)
}

func (r *Suite) Test_ModelSearch_Failed() {
	modelService := r.act.modelService.(*v2mock.MockModelServiceYARPCClient)
	deploymentService := r.act.deploymentService.(*v2mock.MockDeploymentServiceYARPCClient)
	model := &v2pb.Model{
		ObjectMeta: v1.ObjectMeta{Name: "model-test-123", Namespace: "ma-test-sandbox"},
		Spec: v2pb.ModelSpec{
			RevisionId: 11,
		},
	}
	gomock.InOrder(
		deploymentService.EXPECT().GetDeployment(gomock.Any(), gomock.Any()).Return(
			&v2pb.GetDeploymentResponse{
				Deployment: &v2pb.Deployment{
					ObjectMeta: v1.ObjectMeta{Name: "test-deployment", Namespace: "ma-test-sandbox"},
					Spec: v2pb.DeploymentSpec{
						DesiredRevision: &api.ResourceIdentifier{
							Namespace: "ma-test-sandbox",
							Name:      "model-test-123-someWrongInfo",
						},
					},
				},
			},
			nil,
		),
		modelService.EXPECT().GetModel(gomock.Any(), gomock.Any()).Return(
			&v2pb.GetModelResponse{
				Model: model,
			},
			nil,
		),
	)
	val, err := r.activitySuite.ExecuteActivity(Activities.ModelSearch, &ModelSearchRequest{
		Namespace:      "ma-test-sandbox",
		DeploymentName: "shadow-test",
	})
	var res *ModelSearchResponse
	err = val.Get(&res)
	assert.NoError(r.t, err)
	assert.Equal(r.t, "ma-test-sandbox", res.Namespace)
	assert.Equal(r.t, "model-test-123", res.ModelName)
	assert.Equal(r.t, int32(11), res.ModelRevisionID)
}

func (r *Suite) Test_GetModelsByPipelineRun_Succeeded() {
	modelService := r.act.modelService.(*v2mock.MockModelServiceYARPCClient)
	modelService.EXPECT().ListModel(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, request *v2pb.ListModelRequest, _ ...interface{}) (*v2pb.ListModelResponse, error) {
			assert.Equal(r.t, "ma-test-sandbox", request.Namespace)
			assert.Len(r.t, request.ListOptionsExt.Operation.Criterion, 2)
			assert.Equal(r.t, "model.spec.source_pipeline_run.namespace", request.ListOptionsExt.Operation.Criterion[0].FieldName)
			assert.Equal(r.t, "ma-test-sandbox", unpackString(r.t, request.ListOptionsExt.Operation.Criterion[0].MatchValue))
			assert.Equal(r.t, "model.spec.source_pipeline_run.name", request.ListOptionsExt.Operation.Criterion[1].FieldName)
			assert.Equal(r.t, "child-run", unpackString(r.t, request.ListOptionsExt.Operation.Criterion[1].MatchValue))
			return &v2pb.ListModelResponse{
				ModelList: &v2pb.ModelList{Items: []v2pb.Model{
					{
						ObjectMeta: v1.ObjectMeta{Name: "model-z", Namespace: "ma-test-sandbox"},
						Spec: v2pb.ModelSpec{
							RevisionId:        7,
							SourcePipelineRun: &api.ResourceIdentifier{Namespace: "ma-test-sandbox", Name: "child-run"},
						},
					},
					{
						ObjectMeta: v1.ObjectMeta{Name: "model-a", Namespace: "ma-test-sandbox"},
						Spec: v2pb.ModelSpec{
							RevisionId:        3,
							SourcePipelineRun: &api.ResourceIdentifier{Namespace: "ma-test-sandbox", Name: "child-run"},
						},
					},
					{
						ObjectMeta: v1.ObjectMeta{Name: "model-a", Namespace: "ma-test-sandbox"},
						Spec: v2pb.ModelSpec{
							RevisionId:        5,
							SourcePipelineRun: &api.ResourceIdentifier{Namespace: "ma-test-sandbox", Name: "child-run"},
						},
					},
					{
						ObjectMeta: v1.ObjectMeta{Name: "wrong-model", Namespace: "ma-test-sandbox"},
						Spec: v2pb.ModelSpec{
							RevisionId:        1,
							SourcePipelineRun: &api.ResourceIdentifier{Namespace: "ma-test-sandbox", Name: "other-run"},
						},
					},
				}},
			}, nil
		},
	)

	val, err := r.activitySuite.ExecuteActivity(Activities.GetModelsByPipelineRun, &GetModelsByPipelineRunRequest{
		Namespace:       "ma-test-sandbox",
		PipelineRunName: "child-run",
	})
	assert.NoError(r.t, err)
	var response *GetModelsByPipelineRunResponse
	assert.NoError(r.t, val.Get(&response))
	assert.Equal(r.t, []PipelineRunModel{
		{Name: "model-a", Namespace: "ma-test-sandbox", RevisionID: 3},
		{Name: "model-a", Namespace: "ma-test-sandbox", RevisionID: 5},
		{Name: "model-z", Namespace: "ma-test-sandbox", RevisionID: 7},
	}, response.Models)
}

func (r *Suite) Test_GetModelsByPipelineRun_RequiresIdentifiers() {
	response, err := r.act.GetModelsByPipelineRun(context.Background(), &GetModelsByPipelineRunRequest{})
	assert.Nil(r.t, response)
	assert.EqualError(r.t, err, "both \"namespace\" and \"pipeline_run_name\" are required")
}

func (r *Suite) Test_GetModelsByPipelineRun_PropagatesListError() {
	modelService := r.act.modelService.(*v2mock.MockModelServiceYARPCClient)
	modelService.EXPECT().ListModel(gomock.Any(), gomock.Any()).Return(nil, errors.New("list failed"))

	response, err := r.act.GetModelsByPipelineRun(context.Background(), &GetModelsByPipelineRunRequest{
		Namespace:       "ma-test-sandbox",
		PipelineRunName: "child-run",
	})
	assert.Nil(r.t, response)
	assert.EqualError(r.t, err, "list failed")
}

func (r *Suite) Test_GetModelsByPipelineRun_RejectsUnrelatedServerResults() {
	modelService := r.act.modelService.(*v2mock.MockModelServiceYARPCClient)
	modelService.EXPECT().ListModel(gomock.Any(), gomock.Any()).Return(&v2pb.ListModelResponse{
		ModelList: &v2pb.ModelList{Items: []v2pb.Model{{
			ObjectMeta: v1.ObjectMeta{Name: "wrong-model", Namespace: "ma-test-sandbox"},
			Spec: v2pb.ModelSpec{SourcePipelineRun: &api.ResourceIdentifier{
				Namespace: "ma-test-sandbox",
				Name:      "other-run",
			}},
		}}},
	}, nil)

	response, err := r.act.GetModelsByPipelineRun(context.Background(), &GetModelsByPipelineRunRequest{
		Namespace:       "ma-test-sandbox",
		PipelineRunName: "child-run",
	})
	assert.Nil(r.t, response)
	assert.EqualError(r.t, err, "no models found for pipeline run ma-test-sandbox/child-run")
}

func unpackString(t *testing.T, value *gogotypes.Any) string {
	t.Helper()
	var wrapped gogotypes.StringValue
	assert.NoError(t, gogotypes.UnmarshalAny(value, &wrapped))
	return wrapped.Value
}
