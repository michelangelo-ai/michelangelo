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

func TestValidationRetrieve(t *testing.T) {
	tests := []struct {
		name                     string
		deployment               *v2pb.Deployment
		expectedConditionStatus  api.ConditionStatus
		expectedConditionReason  string
		expectedConditionMessage string
	}{
		{
			name: "validation succeeds with valid configuration",
			deployment: &v2pb.Deployment{
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "bert_cola"},
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
			},
			expectedConditionStatus:  api.CONDITION_STATUS_TRUE,
			expectedConditionReason:  "ValidationSucceeded",
			expectedConditionMessage: "Deployment validation completed successfully",
		},
		{
			name: "validation fails when no desired revision",
			deployment: &v2pb.Deployment{
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: nil,
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
			},
			expectedConditionStatus:  api.CONDITION_STATUS_FALSE,
			expectedConditionReason:  "NoDesiredRevision",
			expectedConditionMessage: "No desired revision specified for deployment",
		},
		{
			name: "validation fails when no inference server",
			deployment: &v2pb.Deployment{
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "bert_cola"},
					Target:          nil,
				},
			},
			expectedConditionStatus:  api.CONDITION_STATUS_FALSE,
			expectedConditionReason:  "NoInferenceServer",
			expectedConditionMessage: "No inference server specified for deployment",
		},
		{
			name: "validation fails when model name is empty",
			deployment: &v2pb.Deployment{
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: ""},
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
			},
			expectedConditionStatus:  api.CONDITION_STATUS_FALSE,
			expectedConditionReason:  "InvalidModelName",
			expectedConditionMessage: "Model name cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actor := &ValidationActor{
				logger: zap.NewNop(),
			}

			condition, err := actor.Retrieve(context.Background(), tt.deployment, nil)

			assert.NoError(t, err)
			assert.NotNil(t, condition)
			assert.Equal(t, tt.expectedConditionStatus, condition.Status)
			assert.Equal(t, tt.expectedConditionReason, condition.Reason)
			assert.Contains(t, condition.Message, tt.expectedConditionMessage)
		})
	}
}

func TestValidationRun(t *testing.T) {
	tests := []struct {
		name                     string
		deployment               *v2pb.Deployment
		expectedConditionStatus  api.ConditionStatus
		expectedConditionReason  string
		expectedConditionMessage string
		expectedStage            v2pb.DeploymentStage
		expectedState            v2pb.DeploymentState
	}{
		{
			name: "validation run succeeds with valid configuration",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "bert_cola"},
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
			},
			expectedConditionStatus:  api.CONDITION_STATUS_TRUE,
			expectedConditionReason:  "Success",
			expectedConditionMessage: "Operation completed successfully",
			expectedStage:            v2pb.DEPLOYMENT_STAGE_VALIDATION,
			expectedState:            v2pb.DEPLOYMENT_STATE_INITIALIZING,
		},
		{
			name: "validation run fails when no desired revision",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: nil,
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
			},
			expectedConditionStatus:  api.CONDITION_STATUS_FALSE,
			expectedConditionReason:  "NoDesiredRevision",
			expectedConditionMessage: "Validation failed: No desired revision specified",
			expectedStage:            v2pb.DEPLOYMENT_STAGE_VALIDATION,
			expectedState:            v2pb.DEPLOYMENT_STATE_UNHEALTHY,
		},
		{
			name: "validation run fails when no inference server",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "bert_cola"},
					Target:          nil,
				},
			},
			expectedConditionStatus:  api.CONDITION_STATUS_FALSE,
			expectedConditionReason:  "NoInferenceServer",
			expectedConditionMessage: "Validation failed: No inference server specified",
			expectedStage:            v2pb.DEPLOYMENT_STAGE_VALIDATION,
			expectedState:            v2pb.DEPLOYMENT_STATE_UNHEALTHY,
		},
		{
			name: "validation run fails when revision name is empty",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: ""},
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
			},
			expectedConditionStatus:  api.CONDITION_STATUS_FALSE,
			expectedConditionReason:  "EmptyRevisionName",
			expectedConditionMessage: "Validation failed: Desired revision name is empty",
			expectedStage:            v2pb.DEPLOYMENT_STAGE_VALIDATION,
			expectedState:            v2pb.DEPLOYMENT_STATE_UNHEALTHY,
		},
		{
			name: "validation run fails when inference server name is empty",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "bert_cola"},
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: ""},
					},
				},
			},
			expectedConditionStatus:  api.CONDITION_STATUS_FALSE,
			expectedConditionReason:  "EmptyInferenceServerName",
			expectedConditionMessage: "Validation failed: Inference server name is empty",
			expectedStage:            v2pb.DEPLOYMENT_STAGE_VALIDATION,
			expectedState:            v2pb.DEPLOYMENT_STATE_UNHEALTHY,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actor := &ValidationActor{
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
			if tt.expectedConditionStatus == api.CONDITION_STATUS_FALSE {
				assert.Contains(t, tt.deployment.Status.Message, "Validation failed")
			} else {
				assert.Equal(t, "Validation completed successfully", tt.deployment.Status.Message)
			}
		})
	}
}
