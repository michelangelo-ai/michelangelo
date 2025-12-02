package strategies

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

func TestRollingRolloutRetrieve(t *testing.T) {
	tests := []struct {
		name                     string
		deployment               *v2pb.Deployment
		expectedConditionStatus  api.ConditionStatus
		expectedConditionReason  string
		expectedConditionMessage string
	}{
		{
			name: "rolling rollout completed when stage is PLACEMENT and state is INITIALIZING",
			deployment: &v2pb.Deployment{
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
				},
				Status: v2pb.DeploymentStatus{
					Stage: v2pb.DEPLOYMENT_STAGE_PLACEMENT,
					State: v2pb.DEPLOYMENT_STATE_INITIALIZING,
				},
			},
			expectedConditionStatus:  api.CONDITION_STATUS_TRUE,
			expectedConditionReason:  "RollingRolloutCompleted",
			expectedConditionMessage: "Rolling rollout completed successfully across all inference servers",
		},
		{
			name: "rolling rollout pending when stage is not PLACEMENT",
			deployment: &v2pb.Deployment{
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v2"},
				},
				Status: v2pb.DeploymentStatus{
					Stage: v2pb.DEPLOYMENT_STAGE_VALIDATION,
					State: v2pb.DEPLOYMENT_STATE_INITIALIZING,
				},
			},
			expectedConditionStatus:  api.CONDITION_STATUS_FALSE,
			expectedConditionReason:  "RollingRolloutPending",
			expectedConditionMessage: "Rolling rollout has not started",
		},
		{
			name: "rolling rollout pending when state is not INITIALIZING",
			deployment: &v2pb.Deployment{
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
				},
				Status: v2pb.DeploymentStatus{
					Stage: v2pb.DEPLOYMENT_STAGE_PLACEMENT,
					State: v2pb.DEPLOYMENT_STATE_HEALTHY,
				},
			},
			expectedConditionStatus:  api.CONDITION_STATUS_FALSE,
			expectedConditionReason:  "RollingRolloutPending",
			expectedConditionMessage: "Rolling rollout has not started",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actor := &RollingRolloutActor{
				logger: zap.NewNop(),
			}

			condition, err := actor.Retrieve(context.Background(), tt.deployment, nil)

			assert.NoError(t, err)
			assert.NotNil(t, condition)
			assert.Equal(t, tt.expectedConditionStatus, condition.Status)
			assert.Equal(t, tt.expectedConditionReason, condition.Reason)
			assert.Equal(t, tt.expectedConditionMessage, condition.Message)
		})
	}
}

func TestRollingRolloutRun(t *testing.T) {
	tests := []struct {
		name                    string
		deployment              *v2pb.Deployment
		expectedConditionStatus api.ConditionStatus
		expectedConditionReason string
		expectedStage           v2pb.DeploymentStage
		expectedState           v2pb.DeploymentState
	}{
		{
			name: "rolling rollout run completes successfully",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v2"},
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
				Status: v2pb.DeploymentStatus{
					CurrentRevision: &api.ResourceIdentifier{Name: "model-v1"},
				},
			},
			expectedConditionStatus: api.CONDITION_STATUS_TRUE,
			expectedConditionReason: "Success",
			expectedStage:           v2pb.DEPLOYMENT_STAGE_PLACEMENT,
			expectedState:           v2pb.DEPLOYMENT_STATE_INITIALIZING,
		},
		{
			name: "rolling rollout run with default increment percentage",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-deployment",
					Annotations: map[string]string{},
				},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "bert_cola"},
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "triton-server"},
					},
				},
				Status: v2pb.DeploymentStatus{
					CurrentRevision: nil,
				},
			},
			expectedConditionStatus: api.CONDITION_STATUS_TRUE,
			expectedConditionReason: "Success",
			expectedStage:           v2pb.DEPLOYMENT_STAGE_PLACEMENT,
			expectedState:           v2pb.DEPLOYMENT_STATE_INITIALIZING,
		},
		{
			name: "rolling rollout run with custom increment percentage",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-deployment",
					Annotations: map[string]string{
						"rollout.michelangelo.ai/increment-percentage": "20",
					},
				},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v3"},
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "triton-prod"},
					},
				},
				Status: v2pb.DeploymentStatus{
					CurrentRevision: &api.ResourceIdentifier{Name: "model-v2"},
				},
			},
			expectedConditionStatus: api.CONDITION_STATUS_TRUE,
			expectedConditionReason: "Success",
			expectedStage:           v2pb.DEPLOYMENT_STAGE_PLACEMENT,
			expectedState:           v2pb.DEPLOYMENT_STATE_INITIALIZING,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actor := &RollingRolloutActor{
				logger: zap.NewNop(),
			}

			condition, err := actor.Run(context.Background(), tt.deployment, nil)

			assert.NoError(t, err)
			assert.NotNil(t, condition)
			assert.Equal(t, tt.expectedConditionStatus, condition.Status)
			assert.Equal(t, tt.expectedConditionReason, condition.Reason)
			assert.Equal(t, "Operation completed successfully", condition.Message)
			assert.Equal(t, tt.expectedStage, tt.deployment.Status.Stage)
			assert.Equal(t, tt.expectedState, tt.deployment.Status.State)
		})
	}
}
