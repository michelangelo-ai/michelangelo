package rollout

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

func TestRolloutCompletionRetrieve(t *testing.T) {
	tests := []struct {
		name                     string
		deployment               *v2pb.Deployment
		expectedConditionStatus  api.ConditionStatus
		expectedConditionReason  string
		expectedConditionMessage string
	}{
		{
			name: "completion tasks finished when rollout complete and healthy",
			deployment: &v2pb.Deployment{
				Status: v2pb.DeploymentStatus{
					Stage: v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE,
					State: v2pb.DEPLOYMENT_STATE_HEALTHY,
				},
			},
			expectedConditionStatus:  api.CONDITION_STATUS_TRUE,
			expectedConditionReason:  "CompletionTasksFinished",
			expectedConditionMessage: "All rollout completion tasks have been successfully executed",
		},
		{
			name: "completion tasks pending when stage not rollout complete",
			deployment: &v2pb.Deployment{
				Status: v2pb.DeploymentStatus{
					Stage: v2pb.DEPLOYMENT_STAGE_PLACEMENT,
					State: v2pb.DEPLOYMENT_STATE_HEALTHY,
				},
			},
			expectedConditionStatus:  api.CONDITION_STATUS_FALSE,
			expectedConditionReason:  "CompletionTasksPending",
			expectedConditionMessage: "Rollout completion tasks are pending",
		},
		{
			name: "completion tasks pending when rollout complete but unhealthy",
			deployment: &v2pb.Deployment{
				Status: v2pb.DeploymentStatus{
					Stage: v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE,
					State: v2pb.DEPLOYMENT_STATE_UNHEALTHY,
				},
			},
			expectedConditionStatus:  api.CONDITION_STATUS_FALSE,
			expectedConditionReason:  "CompletionTasksPending",
			expectedConditionMessage: "Rollout completion tasks are pending",
		},
		{
			name: "completion tasks pending when state healthy but stage not complete",
			deployment: &v2pb.Deployment{
				Status: v2pb.DeploymentStatus{
					Stage: v2pb.DEPLOYMENT_STAGE_VALIDATION,
					State: v2pb.DEPLOYMENT_STATE_HEALTHY,
				},
			},
			expectedConditionStatus:  api.CONDITION_STATUS_FALSE,
			expectedConditionReason:  "CompletionTasksPending",
			expectedConditionMessage: "Rollout completion tasks are pending",
		},
		{
			name: "completion tasks pending when rollout failed",
			deployment: &v2pb.Deployment{
				Status: v2pb.DeploymentStatus{
					Stage: v2pb.DEPLOYMENT_STAGE_ROLLOUT_FAILED,
					State: v2pb.DEPLOYMENT_STATE_UNHEALTHY,
				},
			},
			expectedConditionStatus:  api.CONDITION_STATUS_FALSE,
			expectedConditionReason:  "CompletionTasksPending",
			expectedConditionMessage: "Rollout completion tasks are pending",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actor := &RolloutCompletionActor{
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

func TestRolloutCompletionRun(t *testing.T) {
	tests := []struct {
		name                     string
		deployment               *v2pb.Deployment
		expectedConditionStatus  api.ConditionStatus
		expectedConditionReason  string
		expectedConditionMessage string
		expectedStage            v2pb.DeploymentStage
		expectedState            v2pb.DeploymentState
		expectedCurrentRevision  string
		checkAnnotations         bool
		expectedAnnotations      map[string]string
	}{
		{
			name: "rollout completion updates current revision and status",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v2"},
				},
				Status: v2pb.DeploymentStatus{
					CurrentRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Stage:           v2pb.DEPLOYMENT_STAGE_PLACEMENT,
					State:           v2pb.DEPLOYMENT_STATE_INITIALIZING,
				},
			},
			expectedConditionStatus:  api.CONDITION_STATUS_TRUE,
			expectedConditionReason:  "Success",
			expectedConditionMessage: "Operation completed successfully",
			expectedStage:            v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE,
			expectedState:            v2pb.DEPLOYMENT_STATE_HEALTHY,
			expectedCurrentRevision:  "model-v2",
		},
		{
			name: "rollout completion cleans up annotations",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-deployment",
					Annotations: map[string]string{
						"rollout.michelangelo.ai/in-progress": "true",
						"rollout.michelangelo.ai/start-time":  "2024-01-01T00:00:00Z",
						"rollout.michelangelo.ai/strategy":    "rolling",
						"other.annotation.com/keep-this":      "value",
					},
				},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "bert_cola"},
				},
				Status: v2pb.DeploymentStatus{
					CurrentRevision: &api.ResourceIdentifier{Name: "old-model"},
					Stage:           v2pb.DEPLOYMENT_STAGE_PLACEMENT,
					State:           v2pb.DEPLOYMENT_STATE_INITIALIZING,
				},
			},
			expectedConditionStatus:  api.CONDITION_STATUS_TRUE,
			expectedConditionReason:  "Success",
			expectedConditionMessage: "Operation completed successfully",
			expectedStage:            v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE,
			expectedState:            v2pb.DEPLOYMENT_STATE_HEALTHY,
			expectedCurrentRevision:  "bert_cola",
			checkAnnotations:         true,
			expectedAnnotations: map[string]string{
				"rollout.michelangelo.ai/strategy": "rolling",
				"other.annotation.com/keep-this":   "value",
			},
		},
		{
			name: "rollout completion with nil current revision",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "initial-model"},
				},
				Status: v2pb.DeploymentStatus{
					CurrentRevision: nil,
					Stage:           v2pb.DEPLOYMENT_STAGE_VALIDATION,
					State:           v2pb.DEPLOYMENT_STATE_INITIALIZING,
				},
			},
			expectedConditionStatus:  api.CONDITION_STATUS_TRUE,
			expectedConditionReason:  "Success",
			expectedConditionMessage: "Operation completed successfully",
			expectedStage:            v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE,
			expectedState:            v2pb.DEPLOYMENT_STATE_HEALTHY,
			expectedCurrentRevision:  "initial-model",
		},
		{
			name: "rollout completion with no annotations",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-deployment",
					Annotations: nil,
				},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
				},
				Status: v2pb.DeploymentStatus{
					Stage: v2pb.DEPLOYMENT_STAGE_PLACEMENT,
					State: v2pb.DEPLOYMENT_STATE_INITIALIZING,
				},
			},
			expectedConditionStatus:  api.CONDITION_STATUS_TRUE,
			expectedConditionReason:  "Success",
			expectedConditionMessage: "Operation completed successfully",
			expectedStage:            v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE,
			expectedState:            v2pb.DEPLOYMENT_STATE_HEALTHY,
			expectedCurrentRevision:  "model-v1",
		},
		{
			name: "rollout completion with empty annotations map",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-deployment",
					Annotations: map[string]string{},
				},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v3"},
				},
				Status: v2pb.DeploymentStatus{
					Stage: v2pb.DEPLOYMENT_STAGE_RESOURCE_ACQUISITION,
					State: v2pb.DEPLOYMENT_STATE_INITIALIZING,
				},
			},
			expectedConditionStatus:  api.CONDITION_STATUS_TRUE,
			expectedConditionReason:  "Success",
			expectedConditionMessage: "Operation completed successfully",
			expectedStage:            v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE,
			expectedState:            v2pb.DEPLOYMENT_STATE_HEALTHY,
			expectedCurrentRevision:  "model-v3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actor := &RolloutCompletionActor{
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
			assert.NotNil(t, tt.deployment.Status.CurrentRevision)
			assert.Equal(t, tt.expectedCurrentRevision, tt.deployment.Status.CurrentRevision.Name)
			assert.Contains(t, tt.deployment.Status.Message, "Rollout completed successfully")
			assert.Contains(t, tt.deployment.Status.Message, tt.expectedCurrentRevision)

			if tt.checkAnnotations {
				assert.NotNil(t, tt.deployment.Annotations)
				// Check that rollout-specific annotations are removed
				_, hasInProgress := tt.deployment.Annotations["rollout.michelangelo.ai/in-progress"]
				_, hasStartTime := tt.deployment.Annotations["rollout.michelangelo.ai/start-time"]
				assert.False(t, hasInProgress, "in-progress annotation should be removed")
				assert.False(t, hasStartTime, "start-time annotation should be removed")

				// Check that other annotations are preserved
				for key, expectedValue := range tt.expectedAnnotations {
					actualValue, exists := tt.deployment.Annotations[key]
					assert.True(t, exists, "annotation %s should be preserved", key)
					assert.Equal(t, expectedValue, actualValue, "annotation %s should have correct value", key)
				}
			}
		})
	}
}
