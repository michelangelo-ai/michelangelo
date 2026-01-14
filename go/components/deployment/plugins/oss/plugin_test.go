package oss

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/gateways/gatewaysmocks"
	"github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

func TestParseStage(t *testing.T) {
	tests := []struct {
		name          string
		deployment    *v2pb.Deployment
		expectedStage v2pb.DeploymentStage
	}{
		{
			name: "no conditions, returns current stage",
			deployment: &v2pb.Deployment{
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
				},
				Status: v2pb.DeploymentStatus{
					CandidateRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Stage:             v2pb.DEPLOYMENT_STAGE_PLACEMENT,
					Conditions:        []*api.Condition{},
				},
			},
			expectedStage: v2pb.DEPLOYMENT_STAGE_PLACEMENT,
		},
		{
			name: "validated condition is true, returns current stage",
			deployment: &v2pb.Deployment{
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
				},
				Status: v2pb.DeploymentStatus{
					CandidateRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Stage:             v2pb.DEPLOYMENT_STAGE_VALIDATION,
					Conditions: []*api.Condition{
						{Type: common.ActorTypeValidation, Status: api.CONDITION_STATUS_TRUE},
					},
				},
			},
			expectedStage: v2pb.DEPLOYMENT_STAGE_VALIDATION,
		},
		{
			name: "validated condition is false, validation stage",
			deployment: &v2pb.Deployment{
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
				},
				Status: v2pb.DeploymentStatus{
					CandidateRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Stage:             v2pb.DEPLOYMENT_STAGE_PLACEMENT,
					Conditions: []*api.Condition{
						{Type: common.ActorTypeValidation, Status: api.CONDITION_STATUS_FALSE},
					},
				},
			},
			expectedStage: v2pb.DEPLOYMENT_STAGE_VALIDATION,
		},
		{
			name: "asset preparation condition is false, validation stage",
			deployment: &v2pb.Deployment{
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
				},
				Status: v2pb.DeploymentStatus{
					CandidateRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Stage:             v2pb.DEPLOYMENT_STAGE_PLACEMENT,
					Conditions: []*api.Condition{
						{Type: common.ActorTypeAssetPreparation, Status: api.CONDITION_STATUS_FALSE},
					},
				},
			},
			expectedStage: v2pb.DEPLOYMENT_STAGE_VALIDATION,
		},
		{
			name: "resource acquisition condition is false, resource acquisition stage",
			deployment: &v2pb.Deployment{
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
				},
				Status: v2pb.DeploymentStatus{
					CandidateRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Stage:             v2pb.DEPLOYMENT_STAGE_VALIDATION,
					Conditions: []*api.Condition{
						{Type: common.ActorTypeResourceAcquisition, Status: api.CONDITION_STATUS_FALSE},
					},
				},
			},
			expectedStage: v2pb.DEPLOYMENT_STAGE_RESOURCE_ACQUISITION,
		},
		{
			name: "rollout complete condition true, rollout complete stage",
			deployment: &v2pb.Deployment{
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
				},
				Status: v2pb.DeploymentStatus{
					CandidateRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Stage:             v2pb.DEPLOYMENT_STAGE_PLACEMENT,
					Conditions: []*api.Condition{
						{Type: common.ActorTypeRolloutComplete, Status: api.CONDITION_STATUS_TRUE},
					},
				},
			},
			expectedStage: v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE,
		},
		{
			name: "cleanup complete condition true, cleanup complete stage",
			deployment: &v2pb.Deployment{
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
				},
				Status: v2pb.DeploymentStatus{
					CandidateRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Stage:             v2pb.DEPLOYMENT_STAGE_CLEAN_UP_IN_PROGRESS,
					Conditions: []*api.Condition{
						{Type: common.ActorTypeCleanup, Status: api.CONDITION_STATUS_TRUE},
					},
				},
			},
			expectedStage: v2pb.DEPLOYMENT_STAGE_CLEAN_UP_COMPLETE,
		},
		{
			name: "cleanup complete condition false, cleanup in progress stage",
			deployment: &v2pb.Deployment{
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
				},
				Status: v2pb.DeploymentStatus{
					CandidateRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Stage:             v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE,
					Conditions: []*api.Condition{
						{Type: common.ActorTypeCleanup, Status: api.CONDITION_STATUS_FALSE},
					},
				},
			},
			expectedStage: v2pb.DEPLOYMENT_STAGE_CLEAN_UP_IN_PROGRESS,
		},
		{
			name: "rollback complete condition true, rollback complete stage",
			deployment: &v2pb.Deployment{
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
				},
				Status: v2pb.DeploymentStatus{
					CandidateRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Stage:             v2pb.DEPLOYMENT_STAGE_ROLLBACK_IN_PROGRESS,
					Conditions: []*api.Condition{
						{Type: common.ActorTypeRollback, Status: api.CONDITION_STATUS_TRUE},
					},
				},
			},
			expectedStage: v2pb.DEPLOYMENT_STAGE_ROLLBACK_COMPLETE,
		},
		{
			name: "rollback complete condition false, rollback in progress stage",
			deployment: &v2pb.Deployment{
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
				},
				Status: v2pb.DeploymentStatus{
					CandidateRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Stage:             v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE,
					Conditions: []*api.Condition{
						{Type: common.ActorTypeRollback, Status: api.CONDITION_STATUS_FALSE},
					},
				},
			},
			expectedStage: v2pb.DEPLOYMENT_STAGE_ROLLBACK_IN_PROGRESS,
		},
		{
			name: "unknown condition with false status, falls through to placement",
			deployment: &v2pb.Deployment{
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
				},
				Status: v2pb.DeploymentStatus{
					CandidateRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Stage:             v2pb.DEPLOYMENT_STAGE_VALIDATION,
					Conditions: []*api.Condition{
						{Type: "SomeOtherCondition", Status: api.CONDITION_STATUS_FALSE},
					},
				},
			},
			expectedStage: v2pb.DEPLOYMENT_STAGE_PLACEMENT,
		},
		{
			name: "multiple conditions, rollout complete has priority when true",
			deployment: &v2pb.Deployment{
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
				},
				Status: v2pb.DeploymentStatus{
					CandidateRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Stage:             v2pb.DEPLOYMENT_STAGE_PLACEMENT,
					Conditions: []*api.Condition{
						{Type: common.ActorTypeRolloutComplete, Status: api.CONDITION_STATUS_TRUE},
					},
				},
			},
			expectedStage: v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE,
		},
		{
			name: "desired and candidate both nil, no conditions",
			deployment: &v2pb.Deployment{
				Spec: v2pb.DeploymentSpec{},
				Status: v2pb.DeploymentStatus{
					Stage:      v2pb.DEPLOYMENT_STAGE_INVALID,
					Conditions: []*api.Condition{},
				},
			},
			expectedStage: v2pb.DEPLOYMENT_STAGE_INVALID,
		},
		{
			name: "desired nil, candidate exists with rollout complete",
			deployment: &v2pb.Deployment{
				Spec: v2pb.DeploymentSpec{},
				Status: v2pb.DeploymentStatus{
					CandidateRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Stage:             v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE,
					Conditions: []*api.Condition{
						{Type: common.ActorTypeRolloutComplete, Status: api.CONDITION_STATUS_TRUE},
					},
				},
			},
			expectedStage: v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE,
		},
		{
			name: "first false condition determines stage",
			deployment: &v2pb.Deployment{
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
				},
				Status: v2pb.DeploymentStatus{
					CandidateRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Stage:             v2pb.DEPLOYMENT_STAGE_VALIDATION,
					Conditions: []*api.Condition{
						{Type: common.ActorTypeValidation, Status: api.CONDITION_STATUS_TRUE},
						{Type: common.ActorTypeResourceAcquisition, Status: api.CONDITION_STATUS_FALSE},
						{Type: common.ActorTypeRolloutComplete, Status: api.CONDITION_STATUS_TRUE},
					},
				},
			},
			expectedStage: v2pb.DEPLOYMENT_STAGE_RESOURCE_ACQUISITION,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plugin := &Plugin{}
			actualStage := plugin.ParseStage(tt.deployment)
			assert.Equal(t, tt.expectedStage, actualStage, "Stage mismatch for test case: %s", tt.name)
		})
	}
}

func TestGetState(t *testing.T) {
	tests := []struct {
		name          string
		deployment    *v2pb.Deployment
		setupMocks    func(*gatewaysmocks.MockGateway)
		expectedState v2pb.DeploymentState
		expectError   bool
	}{
		{
			name: "returns initializing when current revision is nil",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
				},
				Status: v2pb.DeploymentStatus{
					CurrentRevision: nil,
					Stage:           v2pb.DEPLOYMENT_STAGE_PLACEMENT,
				},
			},
			setupMocks:    func(gw *gatewaysmocks.MockGateway) {},
			expectedState: v2pb.DEPLOYMENT_STATE_INITIALIZING,
			expectError:   false,
		},
		{
			name: "returns empty when current revision is nil and stage is cleanup complete",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
				},
				Status: v2pb.DeploymentStatus{
					CurrentRevision: nil,
					Stage:           v2pb.DEPLOYMENT_STAGE_CLEAN_UP_COMPLETE,
				},
			},
			setupMocks:    func(gw *gatewaysmocks.MockGateway) {},
			expectedState: v2pb.DEPLOYMENT_STATE_EMPTY,
			expectError:   false,
		},
		{
			name: "returns invalid when inference server is nil",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Target:          nil,
				},
				Status: v2pb.DeploymentStatus{
					CurrentRevision: &api.ResourceIdentifier{Name: "model-v1"},
				},
			},
			setupMocks:    func(gw *gatewaysmocks.MockGateway) {},
			expectedState: v2pb.DEPLOYMENT_STATE_INVALID,
			expectError:   false,
		},
		{
			name: "returns invalid when inference server name is empty",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: ""},
					},
				},
				Status: v2pb.DeploymentStatus{
					CurrentRevision: &api.ResourceIdentifier{Name: "model-v1"},
				},
			},
			setupMocks:    func(gw *gatewaysmocks.MockGateway) {},
			expectedState: v2pb.DEPLOYMENT_STATE_INVALID,
			expectError:   false,
		},
		{
			name: "returns healthy when model status check succeeds",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
				Status: v2pb.DeploymentStatus{
					CurrentRevision: &api.ResourceIdentifier{Name: "model-v1"},
				},
			},
			setupMocks: func(gw *gatewaysmocks.MockGateway) {
				gw.EXPECT().CheckModelStatus(gomock.Any(), gomock.Any(), "model-v1", "test-server", "default", v2pb.BACKEND_TYPE_TRITON).Return(true, nil)
			},
			expectedState: v2pb.DEPLOYMENT_STATE_HEALTHY,
			expectError:   false,
		},
		{
			name: "returns unhealthy when model status check returns not healthy",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
				Status: v2pb.DeploymentStatus{
					CurrentRevision: &api.ResourceIdentifier{Name: "model-v1"},
				},
			},
			setupMocks: func(gw *gatewaysmocks.MockGateway) {
				gw.EXPECT().CheckModelStatus(gomock.Any(), gomock.Any(), "model-v1", "test-server", "default", v2pb.BACKEND_TYPE_TRITON).Return(false, nil)
			},
			expectedState: v2pb.DEPLOYMENT_STATE_UNHEALTHY,
			expectError:   false,
		},
		{
			name: "returns error when model status check fails",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
				Status: v2pb.DeploymentStatus{
					CurrentRevision: &api.ResourceIdentifier{Name: "model-v1"},
				},
			},
			setupMocks: func(gw *gatewaysmocks.MockGateway) {
				gw.EXPECT().CheckModelStatus(gomock.Any(), gomock.Any(), "model-v1", "test-server", "default", v2pb.BACKEND_TYPE_TRITON).Return(false, errors.New("connection error"))
			},
			expectedState: v2pb.DEPLOYMENT_STATE_INVALID,
			expectError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockGateway := gatewaysmocks.NewMockGateway(ctrl)
			tt.setupMocks(mockGateway)

			plugin := &Plugin{
				gateway: mockGateway,
				logger:  zap.NewNop(),
			}

			status, err := plugin.GetState(context.Background(), plugins.ObservabilityContext{}, tt.deployment)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedState, status.State)
			}
		})
	}
}
