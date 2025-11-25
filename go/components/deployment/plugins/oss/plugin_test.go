package oss

import (
	"testing"

	"github.com/stretchr/testify/assert"

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
			name: "new rollout needed when desired != candidate",
			deployment: &v2pb.Deployment{
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "new-model-v2"},
				},
				Status: v2pb.DeploymentStatus{
					CandidateRevision: &api.ResourceIdentifier{Name: "old-model-v1"},
					Stage:             v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE,
					Conditions: []*api.Condition{
						{Type: "RolloutCompleted", Status: api.CONDITION_STATUS_TRUE},
					},
				},
			},
			expectedStage: v2pb.DEPLOYMENT_STAGE_VALIDATION,
		},
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
			name: "validated condition is true, placement stage",
			deployment: &v2pb.Deployment{
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
				},
				Status: v2pb.DeploymentStatus{
					CandidateRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Stage:             v2pb.DEPLOYMENT_STAGE_VALIDATION,
					Conditions: []*api.Condition{
						{Type: "Validated", Status: api.CONDITION_STATUS_TRUE},
					},
				},
			},
			expectedStage: v2pb.DEPLOYMENT_STAGE_PLACEMENT,
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
						{Type: "Validated", Status: api.CONDITION_STATUS_FALSE},
					},
				},
			},
			expectedStage: v2pb.DEPLOYMENT_STAGE_VALIDATION,
		},
		{
			name: "model synced false, placement stage",
			deployment: &v2pb.Deployment{
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
				},
				Status: v2pb.DeploymentStatus{
					CandidateRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Stage:             v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE,
					Conditions: []*api.Condition{
						{Type: "Validated", Status: api.CONDITION_STATUS_TRUE},
						{Type: "ModelSynced", Status: api.CONDITION_STATUS_FALSE},
					},
				},
			},
			expectedStage: v2pb.DEPLOYMENT_STAGE_PLACEMENT,
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
						{Type: "Validated", Status: api.CONDITION_STATUS_TRUE},
						{Type: "RolloutCompleted", Status: api.CONDITION_STATUS_TRUE},
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
						{Type: "CleanupComplete", Status: api.CONDITION_STATUS_TRUE},
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
						{Type: "CleanupComplete", Status: api.CONDITION_STATUS_FALSE},
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
						{Type: "RollbackComplete", Status: api.CONDITION_STATUS_TRUE},
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
						{Type: "RollbackComplete", Status: api.CONDITION_STATUS_FALSE},
					},
				},
			},
			expectedStage: v2pb.DEPLOYMENT_STAGE_ROLLBACK_IN_PROGRESS,
		},
		{
			name: "state steady condition, returns current stage",
			deployment: &v2pb.Deployment{
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
				},
				Status: v2pb.DeploymentStatus{
					CandidateRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Stage:             v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE,
					Conditions: []*api.Condition{
						{Type: "StateSteady", Status: api.CONDITION_STATUS_TRUE},
					},
				},
			},
			expectedStage: v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE,
		},
		{
			name: "no clear progress indicators, validation stage",
			deployment: &v2pb.Deployment{
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
				},
				Status: v2pb.DeploymentStatus{
					CandidateRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Stage:             v2pb.DEPLOYMENT_STAGE_PLACEMENT,
					Conditions: []*api.Condition{
						{Type: "SomeOtherCondition", Status: api.CONDITION_STATUS_TRUE},
					},
				},
			},
			expectedStage: v2pb.DEPLOYMENT_STAGE_VALIDATION,
		},
		{
			name: "multiple conditions, rollout complete has priority",
			deployment: &v2pb.Deployment{
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
				},
				Status: v2pb.DeploymentStatus{
					CandidateRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Stage:             v2pb.DEPLOYMENT_STAGE_PLACEMENT,
					Conditions: []*api.Condition{
						{Type: "Validated", Status: api.CONDITION_STATUS_TRUE},
						{Type: "ModelSynced", Status: api.CONDITION_STATUS_TRUE},
						{Type: "RolloutCompleted", Status: api.CONDITION_STATUS_TRUE},
					},
				},
			},
			expectedStage: v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE,
		},
		{
			name: "cleanup has priority over rollout complete",
			deployment: &v2pb.Deployment{
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
				},
				Status: v2pb.DeploymentStatus{
					CandidateRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Stage:             v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE,
					Conditions: []*api.Condition{
						{Type: "RolloutCompleted", Status: api.CONDITION_STATUS_TRUE},
						{Type: "CleanupComplete", Status: api.CONDITION_STATUS_FALSE},
					},
				},
			},
			expectedStage: v2pb.DEPLOYMENT_STAGE_CLEAN_UP_IN_PROGRESS,
		},
		{
			name: "rollback has priority over other conditions",
			deployment: &v2pb.Deployment{
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
				},
				Status: v2pb.DeploymentStatus{
					CandidateRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Stage:             v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE,
					Conditions: []*api.Condition{
						{Type: "RolloutCompleted", Status: api.CONDITION_STATUS_TRUE},
						{Type: "RollbackComplete", Status: api.CONDITION_STATUS_FALSE},
					},
				},
			},
			expectedStage: v2pb.DEPLOYMENT_STAGE_ROLLBACK_IN_PROGRESS,
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
			name: "desired nil, candidate exists",
			deployment: &v2pb.Deployment{
				Spec: v2pb.DeploymentSpec{},
				Status: v2pb.DeploymentStatus{
					CandidateRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Stage:             v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE,
					Conditions: []*api.Condition{
						{Type: "RolloutCompleted", Status: api.CONDITION_STATUS_TRUE},
					},
				},
			},
			expectedStage: v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE,
		},
		{
			name: "validated true, model synced false, returns placement",
			deployment: &v2pb.Deployment{
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
				},
				Status: v2pb.DeploymentStatus{
					CandidateRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Stage:             v2pb.DEPLOYMENT_STAGE_VALIDATION,
					Conditions: []*api.Condition{
						{Type: "Validated", Status: api.CONDITION_STATUS_TRUE},
						{Type: "ModelSynced", Status: api.CONDITION_STATUS_FALSE},
						{Type: "RolloutCompleted", Status: api.CONDITION_STATUS_TRUE},
					},
				},
			},
			expectedStage: v2pb.DEPLOYMENT_STAGE_PLACEMENT,
		},
		{
			name: "state steady, returns current stage even with other conditions",
			deployment: &v2pb.Deployment{
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
				},
				Status: v2pb.DeploymentStatus{
					CandidateRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Stage:             v2pb.DEPLOYMENT_STAGE_CLEAN_UP_COMPLETE,
					Conditions: []*api.Condition{
						{Type: "RolloutCompleted", Status: api.CONDITION_STATUS_TRUE},
						{Type: "CleanupComplete", Status: api.CONDITION_STATUS_TRUE},
						{Type: "StateSteady", Status: api.CONDITION_STATUS_TRUE},
					},
				},
			},
			expectedStage: v2pb.DEPLOYMENT_STAGE_CLEAN_UP_COMPLETE,
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
