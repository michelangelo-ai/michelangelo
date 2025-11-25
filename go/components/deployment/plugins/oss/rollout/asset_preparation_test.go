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

func TestAssetPreparationRetrieve(t *testing.T) {
	tests := []struct {
		name                     string
		deployment               *v2pb.Deployment
		expectedConditionStatus  api.ConditionStatus
		expectedConditionReason  string
		expectedConditionMessage string
	}{
		{
			name: "assets available when desired revision specified",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "bert_cola"},
				},
			},
			expectedConditionStatus:  api.CONDITION_STATUS_TRUE,
			expectedConditionReason:  "AssetsAvailable",
			expectedConditionMessage: "Assets for model bert_cola are available and prepared",
		},
		{
			name: "assets available for different model",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v2"},
				},
			},
			expectedConditionStatus:  api.CONDITION_STATUS_TRUE,
			expectedConditionReason:  "AssetsAvailable",
			expectedConditionMessage: "Assets for model model-v2 are available and prepared",
		},
		{
			name: "no desired revision specified",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: nil,
				},
			},
			expectedConditionStatus:  api.CONDITION_STATUS_FALSE,
			expectedConditionReason:  "NoDesiredRevision",
			expectedConditionMessage: "No desired revision specified for asset preparation",
		},
		{
			name: "assets available with complex deployment configuration",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "complex-deployment",
					Namespace: "production",
					Annotations: map[string]string{
						"rollout.michelangelo.ai/strategy": "rolling",
					},
				},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "gpt-model-v1"},
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "triton-server"},
					},
				},
			},
			expectedConditionStatus:  api.CONDITION_STATUS_TRUE,
			expectedConditionReason:  "AssetsAvailable",
			expectedConditionMessage: "Assets for model gpt-model-v1 are available and prepared",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actor := &AssetPreparationActor{
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

func TestAssetPreparationRun(t *testing.T) {
	tests := []struct {
		name                     string
		deployment               *v2pb.Deployment
		expectedConditionStatus  api.ConditionStatus
		expectedConditionReason  string
		expectedConditionMessage string
	}{
		{
			name: "asset preparation succeeds with valid deployment",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
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
		},
		{
			name: "asset preparation succeeds with different model",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "prod-deployment", Namespace: "production"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v3"},
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "triton-prod"},
					},
				},
			},
			expectedConditionStatus:  api.CONDITION_STATUS_TRUE,
			expectedConditionReason:  "Success",
			expectedConditionMessage: "Operation completed successfully",
		},
		{
			name: "asset preparation succeeds without desired revision",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: nil,
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
			},
			expectedConditionStatus:  api.CONDITION_STATUS_TRUE,
			expectedConditionReason:  "Success",
			expectedConditionMessage: "Operation completed successfully",
		},
		{
			name: "asset preparation with annotations",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "annotated-deployment",
					Namespace: "staging",
					Annotations: map[string]string{
						"rollout.michelangelo.ai/strategy":             "rolling",
						"rollout.michelangelo.ai/increment-percentage": "20",
					},
				},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "llm-model"},
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "triton-staging"},
					},
				},
			},
			expectedConditionStatus:  api.CONDITION_STATUS_TRUE,
			expectedConditionReason:  "Success",
			expectedConditionMessage: "Operation completed successfully",
		},
		{
			name: "asset preparation with minimal deployment",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "minimal-deployment"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "simple-model"},
				},
			},
			expectedConditionStatus:  api.CONDITION_STATUS_TRUE,
			expectedConditionReason:  "Success",
			expectedConditionMessage: "Operation completed successfully",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actor := &AssetPreparationActor{
				logger: zap.NewNop(),
			}

			condition, err := actor.Run(context.Background(), tt.deployment, nil)

			assert.NoError(t, err)
			assert.NotNil(t, condition)
			assert.Equal(t, tt.expectedConditionStatus, condition.Status)
			assert.Equal(t, tt.expectedConditionReason, condition.Reason)
			assert.Equal(t, tt.expectedConditionMessage, condition.Message)
		})
	}
}
