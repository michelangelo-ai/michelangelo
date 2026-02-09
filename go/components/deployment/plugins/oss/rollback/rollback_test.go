package rollback

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	modelconfig "github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/modelconfig"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/modelconfig/modelconfigmocks"
	"github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

func TestRetrieve(t *testing.T) {
	tests := []struct {
		name                    string
		deployment              *v2pb.Deployment
		setupMocks              func(*modelconfigmocks.MockModelConfigProvider)
		expectedConditionStatus api.ConditionStatus
		expectedConditionReason string
	}{
		{
			name: "rollback complete when candidate revision is empty",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
				Status: v2pb.DeploymentStatus{
					CandidateRevision: nil,
				},
			},
			setupMocks:              func(gw *modelconfigmocks.MockModelConfigProvider) {},
			expectedConditionStatus: api.CONDITION_STATUS_TRUE,
			expectedConditionReason: "",
		},
		{
			name: "rollback complete when candidate model no longer exists",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
				Status: v2pb.DeploymentStatus{
					CandidateRevision: &api.ResourceIdentifier{Name: "failed-model"},
				},
			},
			setupMocks: func(gw *modelconfigmocks.MockModelConfigProvider) {
				gw.EXPECT().GetModelsFromConfig(gomock.Any(), gomock.Any(), gomock.Any()).Return([]modelconfig.ModelConfigEntry{
					{
						Name: "failed-model",
					},
				}, nil)
			},
			expectedConditionStatus: api.CONDITION_STATUS_TRUE,
			expectedConditionReason: "",
		},
		{
			name: "rollback in progress when candidate model still exists",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
				Status: v2pb.DeploymentStatus{
					CandidateRevision: &api.ResourceIdentifier{Name: "failed-model"},
				},
			},
			setupMocks: func(gw *modelconfigmocks.MockModelConfigProvider) {
				gw.EXPECT().GetModelsFromConfig(gomock.Any(), gomock.Any(), gomock.Any()).Return([]modelconfig.ModelConfigEntry{
					{
						Name: "failed-model",
					},
				}, nil)
			},
			expectedConditionStatus: api.CONDITION_STATUS_FALSE,
			expectedConditionReason: "Candidate Model failed-model still exists in model config",
		},
		{
			name: "unable to check model exists error",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
				Status: v2pb.DeploymentStatus{
					CandidateRevision: &api.ResourceIdentifier{Name: "failed-model"},
				},
			},
			setupMocks: func(gw *modelconfigmocks.MockModelConfigProvider) {
				gw.EXPECT().GetModelsFromConfig(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, errors.New("api error"))
			},
			expectedConditionStatus: api.CONDITION_STATUS_FALSE,
			expectedConditionReason: "Unable to check if model failed-model exists in model config: api error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockModelConfigProvider := modelconfigmocks.NewMockModelConfigProvider(ctrl)
			tt.setupMocks(mockModelConfigProvider)

			actor := &RollbackActor{
				logger:              zap.NewNop(),
				modelConfigProvider: mockModelConfigProvider,
			}

			condition, err := actor.Retrieve(context.Background(), tt.deployment, &api.Condition{})

			assert.NoError(t, err)
			assert.NotNil(t, condition)
			assert.Equal(t, tt.expectedConditionStatus, condition.Status)
			assert.Equal(t, tt.expectedConditionReason, condition.Reason)
		})
	}
}

func TestRun(t *testing.T) {
	tests := []struct {
		name                    string
		deployment              *v2pb.Deployment
		setupMocks              func(*modelconfigmocks.MockModelConfigProvider)
		expectedConditionStatus api.ConditionStatus
		expectedConditionReason string
	}{
		{
			name: "successful rollback unloads candidate model",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
				Status: v2pb.DeploymentStatus{
					CandidateRevision: &api.ResourceIdentifier{Name: "failed-model"},
				},
			},
			setupMocks: func(gw *modelconfigmocks.MockModelConfigProvider) {
				gw.EXPECT().RemoveModelFromConfig(gomock.Any(), gomock.Any(), "failed-model", "test-server").Return(nil)
				gw.EXPECT().GetModelsFromConfig(gomock.Any(), gomock.Any(), gomock.Any()).Return([]modelconfig.ModelConfigEntry{
					{
						Name: "failed-model",
					},
				}, nil)
			},
			expectedConditionStatus: api.CONDITION_STATUS_TRUE,
			expectedConditionReason: "",
		},
		{
			name: "rollback fails when unload fails",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
				Status: v2pb.DeploymentStatus{
					CandidateRevision: &api.ResourceIdentifier{Name: "failed-model"},
				},
			},
			setupMocks: func(gw *modelconfigmocks.MockModelConfigProvider) {
				gw.EXPECT().RemoveModelFromConfig(gomock.Any(), gomock.Any(), "failed-model", "test-server").Return(errors.New("unload failed"))
				gw.EXPECT().GetModelsFromConfig(gomock.Any(), gomock.Any(), gomock.Any()).Return([]modelconfig.ModelConfigEntry{
					{
						Name: "failed-model",
					},
				}, nil)
			},
			expectedConditionStatus: api.CONDITION_STATUS_FALSE,
			expectedConditionReason: "Failed to rollback deployment: unload failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockModelConfigProvider := modelconfigmocks.NewMockModelConfigProvider(ctrl)
			tt.setupMocks(mockModelConfigProvider)

			actor := &RollbackActor{
				logger:              zap.NewNop(),
				modelConfigProvider: mockModelConfigProvider,
			}

			condition, err := actor.Run(context.Background(), tt.deployment, &api.Condition{})

			assert.NoError(t, err)
			assert.NotNil(t, condition)
			assert.Equal(t, tt.expectedConditionStatus, condition.Status)
			assert.Equal(t, tt.expectedConditionReason, condition.Reason)
		})
	}
}
