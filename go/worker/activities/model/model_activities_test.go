package model

import (
	"mock/github.com/michelangelo-ai/michelangelo/proto/api/v2/v2mock"
	"net/http/httptest"
	"testing"

	"github.com/cadence-workflow/starlark-worker/service"
	"github.com/cadence-workflow/starlark-worker/test/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
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
	// assert.NoError(r.T(), err)
	var res *ModelSearchResponse
	err = val.Get(&res)
	assert.NoError(r.t, err)
	assert.Equal(r.t, "ma-test-sandbox", res.Namespace)
	assert.Equal(r.t, "model-test-123", res.ModelName)
	assert.Equal(r.t, int32(11), res.ModelRevisionID)
}
