package model

import (
	"mock/github.com/michelangelo-ai/michelangelo/proto-go/api/v2/v2mock"
	"net/http/httptest"
	"testing"

	"github.com/cadence-workflow/starlark-worker/service"
	"github.com/cadence-workflow/starlark-worker/test/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"go.uber.org/yarpc/yarpcerrors"
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

func (r *Suite) Test_DeployModel_CreatesExactOSSModelReference() {
	modelService := r.act.modelService.(*v2mock.MockModelServiceYARPCClient)
	deploymentService := r.act.deploymentService.(*v2mock.MockDeploymentServiceYARPCClient)
	request := &DeployModelRequest{
		Namespace:           "default",
		DeploymentName:      "retrain-deployment",
		PipelineRunName:     "child-run",
		InferenceServerName: "inference-server",
		Actor:               "integration-test",
	}
	model := &v2pb.Model{
		ObjectMeta: v1.ObjectMeta{Name: "model-physical-name", Namespace: "default"},
		Spec: v2pb.ModelSpec{
			// Deliberately non-zero: OSS desired_revision must still use the exact
			// immutable Model metadata name, not "model-physical-name-37".
			RevisionId: 37,
			SourcePipelineRun: &api.ResourceIdentifier{
				Namespace: "default", Name: "child-run",
			},
		},
	}
	modelService.EXPECT().ListModel(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ interface{}, listRequest *v2pb.ListModelRequest, _ ...interface{}) (*v2pb.ListModelResponse, error) {
			assert.Equal(r.t, "default", listRequest.Namespace)
			assert.Len(r.t, listRequest.ListOptionsExt.Operation.Criterion, 2)
			return &v2pb.ListModelResponse{ModelList: &v2pb.ModelList{Items: []v2pb.Model{*model}}}, nil
		},
	)
	deploymentService.EXPECT().GetDeployment(gomock.Any(), gomock.Any()).Return(nil, yarpcerrors.NotFoundErrorf("missing"))
	deploymentService.EXPECT().CreateDeployment(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ interface{}, createRequest *v2pb.CreateDeploymentRequest, _ ...interface{}) (*v2pb.CreateDeploymentResponse, error) {
			deployment := createRequest.Deployment
			assert.Equal(r.t, "model-physical-name", deployment.Spec.DesiredRevision.Name)
			assert.Equal(r.t, "default", deployment.Spec.DesiredRevision.Namespace)
			assert.Equal(r.t, "inference-server", deployment.Spec.GetInferenceServer().Name)
			assert.NotNil(r.t, deployment.Spec.Strategy.GetRolling())
			assert.Equal(r.t, "integration-test", deployment.Spec.Owner.Name)
			return &v2pb.CreateDeploymentResponse{Deployment: deployment}, nil
		},
	)

	value, err := r.activitySuite.ExecuteActivity(Activities.DeployModel, request)
	assert.NoError(r.t, err)
	var response *DeployModelResponse
	assert.NoError(r.t, value.Get(&response))
	assert.Equal(r.t, "model-physical-name", response.ModelName)
}

func (r *Suite) Test_DeployModel_AmbiguousOutputRequiresModelName() {
	modelService := r.act.modelService.(*v2mock.MockModelServiceYARPCClient)
	models := []v2pb.Model{
		{
			ObjectMeta: v1.ObjectMeta{Name: "first", Namespace: "default"},
			Spec:       v2pb.ModelSpec{SourcePipelineRun: &api.ResourceIdentifier{Namespace: "default", Name: "child-run"}},
		},
		{
			ObjectMeta: v1.ObjectMeta{Name: "second", Namespace: "default"},
			Spec:       v2pb.ModelSpec{SourcePipelineRun: &api.ResourceIdentifier{Namespace: "default", Name: "child-run"}},
		},
	}
	modelService.EXPECT().ListModel(gomock.Any(), gomock.Any()).Return(
		&v2pb.ListModelResponse{ModelList: &v2pb.ModelList{Items: models}}, nil)
	_, err := r.activitySuite.ExecuteActivity(Activities.DeployModel, &DeployModelRequest{
		Namespace: "default", DeploymentName: "deployment", PipelineRunName: "child-run", InferenceServerName: "server",
	})
	assert.ErrorContains(r.t, err, "produced multiple models [first second]; pass model_name")
}

func (r *Suite) Test_DeploymentSensor_TerminalSuccess() {
	deploymentService := r.act.deploymentService.(*v2mock.MockDeploymentServiceYARPCClient)
	deployment := &v2pb.Deployment{
		ObjectMeta: v1.ObjectMeta{Name: "deployment", Namespace: "default"},
		Status: v2pb.DeploymentStatus{
			State:           v2pb.DEPLOYMENT_STATE_HEALTHY,
			Stage:           v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE,
			CurrentRevision: &api.ResourceIdentifier{Name: "trained-model", Namespace: "default"},
		},
	}
	deploymentService.EXPECT().GetDeployment(gomock.Any(), gomock.Any()).Return(
		&v2pb.GetDeploymentResponse{Deployment: deployment}, nil)
	value, err := r.activitySuite.ExecuteActivity(Activities.DeploymentSensor, &DeploymentSensorRequest{
		Namespace: "default", DeploymentName: "deployment", ModelName: "trained-model",
	})
	assert.NoError(r.t, err)
	var response *v2pb.Deployment
	assert.NoError(r.t, value.Get(&response))
	assert.Equal(r.t, "trained-model", response.Status.CurrentRevision.Name)
}

func (r *Suite) Test_DeploymentSensor_RollbackIsTerminalForDeployCall() {
	deploymentService := r.act.deploymentService.(*v2mock.MockDeploymentServiceYARPCClient)
	deployment := &v2pb.Deployment{Status: v2pb.DeploymentStatus{
		Stage:   v2pb.DEPLOYMENT_STAGE_ROLLBACK_COMPLETE,
		Message: "candidate failed health checks",
	}}
	deploymentService.EXPECT().GetDeployment(gomock.Any(), gomock.Any()).Return(
		&v2pb.GetDeploymentResponse{Deployment: deployment}, nil)
	value, err := r.activitySuite.ExecuteActivity(Activities.DeploymentSensor, &DeploymentSensorRequest{
		Namespace: "default", DeploymentName: "deployment", ModelName: "trained-model",
	})
	assert.NoError(r.t, err)
	var response *v2pb.Deployment
	assert.NoError(r.t, value.Get(&response))
	assert.Equal(r.t, v2pb.DEPLOYMENT_STAGE_ROLLBACK_COMPLETE, response.Status.Stage)
}

func (r *Suite) Test_DeploymentSensor_StaleCompletedRevisionRetries() {
	deploymentService := r.act.deploymentService.(*v2mock.MockDeploymentServiceYARPCClient)
	deployment := &v2pb.Deployment{Status: v2pb.DeploymentStatus{
		State:           v2pb.DEPLOYMENT_STATE_HEALTHY,
		Stage:           v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE,
		CurrentRevision: &api.ResourceIdentifier{Name: "old-model", Namespace: "default"},
	}}
	deploymentService.EXPECT().GetDeployment(gomock.Any(), gomock.Any()).Return(
		&v2pb.GetDeploymentResponse{Deployment: deployment}, nil)
	_, err := r.activitySuite.ExecuteActivity(Activities.DeploymentSensor, &DeploymentSensorRequest{
		Namespace: "default", DeploymentName: "deployment", ModelName: "trained-model",
	})
	assert.ErrorContains(r.t, err, "DeploymentNotReady")
}
