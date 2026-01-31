package rollout

import (
	"context"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

func TestRolloutCompletionRetrieve(t *testing.T) {
	// Helper to create a condition with cleanupComplete metadata set
	cleanupCompletedCondition := func() *api.Condition {
		cleanupComplete := &types.BoolValue{Value: true}
		metadata, _ := types.MarshalAny(cleanupComplete)
		return &api.Condition{Metadata: metadata}
	}

	tests := []struct {
		name                     string
		deployment               *v2pb.Deployment
		inputCondition           *api.Condition
		expectedConditionStatus  api.ConditionStatus
		expectedConditionReason  string
		expectedConditionMessage string
	}{
		{
			name: "completion tasks finished when metadata indicates complete",
			deployment: &v2pb.Deployment{
				Status: v2pb.DeploymentStatus{
					Stage: v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE,
					State: v2pb.DEPLOYMENT_STATE_HEALTHY,
				},
			},
			inputCondition:           cleanupCompletedCondition(),
			expectedConditionStatus:  api.CONDITION_STATUS_TRUE,
			expectedConditionReason:  "",
			expectedConditionMessage: "",
		},
		{
			name: "completion tasks pending when metadata not set",
			deployment: &v2pb.Deployment{
				Status: v2pb.DeploymentStatus{
					Stage: v2pb.DEPLOYMENT_STAGE_PLACEMENT,
					State: v2pb.DEPLOYMENT_STATE_HEALTHY,
				},
			},
			inputCondition:           &api.Condition{},
			expectedConditionStatus:  api.CONDITION_STATUS_FALSE,
			expectedConditionReason:  "Rollout completion tasks are pending",
			expectedConditionMessage: "CompletionTasksPending",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actor := &RolloutCompletionActor{
				logger: zap.NewNop(),
			}

			condition, err := actor.Retrieve(context.Background(), tt.deployment, tt.inputCondition)

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
		checkAnnotations         bool
		expectedAnnotations      map[string]string
	}{
		{
			name: "rollout completion returns true condition",
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
			expectedConditionReason:  "",
			expectedConditionMessage: "",
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
			expectedConditionReason:  "",
			expectedConditionMessage: "",
			checkAnnotations:         true,
			expectedAnnotations: map[string]string{
				"rollout.michelangelo.ai/strategy": "rolling",
				"other.annotation.com/keep-this":   "value",
			},
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
			expectedConditionReason:  "",
			expectedConditionMessage: "",
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
			expectedConditionReason:  "",
			expectedConditionMessage: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actor := &RolloutCompletionActor{
				logger: zap.NewNop(),
			}

			condition, err := actor.Run(context.Background(), tt.deployment, &api.Condition{})

			assert.NoError(t, err)
			assert.NotNil(t, condition)
			assert.Equal(t, tt.expectedConditionStatus, condition.Status)
			assert.Equal(t, tt.expectedConditionReason, condition.Reason)
			assert.Equal(t, tt.expectedConditionMessage, condition.Message)

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
