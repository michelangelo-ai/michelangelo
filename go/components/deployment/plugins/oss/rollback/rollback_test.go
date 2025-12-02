package rollback

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

func TestRetrieve(t *testing.T) {
	tests := []struct {
		name                     string
		deployment               *v2pb.Deployment
		expectedConditionStatus  api.ConditionStatus
		expectedConditionReason  string
		expectedConditionMessage string
	}{
		{
			name: "rollback complete when current revision exists",
			deployment: &v2pb.Deployment{
				Status: v2pb.DeploymentStatus{
					CurrentRevision: &api.ResourceIdentifier{Name: "previous-model"},
				},
			},
			expectedConditionStatus:  api.CONDITION_STATUS_TRUE,
			expectedConditionReason:  "RollbackCompleted",
			expectedConditionMessage: "Rollback completed successfully",
		},
		{
			name: "rollback in progress when current revision is nil",
			deployment: &v2pb.Deployment{
				Status: v2pb.DeploymentStatus{
					CurrentRevision: nil,
				},
			},
			expectedConditionStatus:  api.CONDITION_STATUS_FALSE,
			expectedConditionReason:  "RollbackInProgress",
			expectedConditionMessage: "Rollback in progress",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actor := &RollbackActor{
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

func TestRun(t *testing.T) {
	tests := []struct {
		name                     string
		deployment               *v2pb.Deployment
		expectedConditionStatus  api.ConditionStatus
		expectedConditionReason  string
		expectedConditionMessage string
		expectedStage            v2pb.DeploymentStage
		expectedState            v2pb.DeploymentState
		expectedDesiredRevision  string
	}{
		{
			name: "successful rollback with previous revision",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "failed-model-v2"},
				},
				Status: v2pb.DeploymentStatus{
					CurrentRevision: &api.ResourceIdentifier{Name: "previous-model-v1"},
					Stage:           v2pb.DEPLOYMENT_STAGE_ROLLOUT_FAILED,
					State:           v2pb.DEPLOYMENT_STATE_UNHEALTHY,
				},
			},
			expectedConditionStatus:  api.CONDITION_STATUS_TRUE,
			expectedConditionReason:  "RollbackCompleted",
			expectedConditionMessage: "Rollback completed successfully",
			expectedStage:            v2pb.DEPLOYMENT_STAGE_ROLLBACK_COMPLETE,
			expectedState:            v2pb.DEPLOYMENT_STATE_HEALTHY,
			expectedDesiredRevision:  "previous-model-v1",
		},
		{
			name: "rollback without previous revision",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "failed-model-v1"},
				},
				Status: v2pb.DeploymentStatus{
					CurrentRevision: nil,
					Stage:           v2pb.DEPLOYMENT_STAGE_ROLLOUT_FAILED,
					State:           v2pb.DEPLOYMENT_STATE_UNHEALTHY,
				},
			},
			expectedConditionStatus:  api.CONDITION_STATUS_TRUE,
			expectedConditionReason:  "RollbackCompleted",
			expectedConditionMessage: "Rollback completed successfully",
			expectedStage:            v2pb.DEPLOYMENT_STAGE_ROLLBACK_IN_PROGRESS,
			expectedState:            v2pb.DEPLOYMENT_STATE_UNHEALTHY,
			expectedDesiredRevision:  "failed-model-v1",
		},
		{
			name: "rollback from rollback complete stage",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "new-failed-model-v3"},
				},
				Status: v2pb.DeploymentStatus{
					CurrentRevision: &api.ResourceIdentifier{Name: "stable-model-v2"},
					Stage:           v2pb.DEPLOYMENT_STAGE_ROLLBACK_COMPLETE,
					State:           v2pb.DEPLOYMENT_STATE_HEALTHY,
				},
			},
			expectedConditionStatus:  api.CONDITION_STATUS_TRUE,
			expectedConditionReason:  "RollbackCompleted",
			expectedConditionMessage: "Rollback completed successfully",
			expectedStage:            v2pb.DEPLOYMENT_STAGE_ROLLBACK_COMPLETE,
			expectedState:            v2pb.DEPLOYMENT_STATE_HEALTHY,
			expectedDesiredRevision:  "stable-model-v2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actor := &RollbackActor{
				logger: zap.NewNop(),
			}

			condition, err := actor.Run(context.Background(), tt.deployment, nil)

			assert.NoError(t, err)
			assert.NotNil(t, condition)
			assert.Equal(t, tt.expectedConditionStatus, condition.Status)
			assert.Equal(t, tt.expectedConditionReason, condition.Reason)
			assert.Equal(t, tt.expectedConditionMessage, condition.Message)
			assert.Equal(t, tt.expectedStage, tt.deployment.Status.Stage)
			assert.Equal(t, tt.expectedState, tt.deployment.Status.State)
			assert.Equal(t, tt.expectedDesiredRevision, tt.deployment.Spec.DesiredRevision.Name)
		})
	}
}
