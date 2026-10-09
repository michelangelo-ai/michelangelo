package deployment

import (
	"testing"

	"github.com/stretchr/testify/assert"

	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

func TestRolloutFailedNeedsRollback(t *testing.T) {
	v1 := &apipb.ResourceIdentifier{Name: "model-v1"}
	v2 := &apipb.ResourceIdentifier{Name: "model-v2"}

	tests := []struct {
		name      string
		stage     v2pb.DeploymentStage
		current   *apipb.ResourceIdentifier
		candidate *apipb.ResourceIdentifier
		want      bool
	}{
		{
			name:      "failed rollout of a new revision rolls back to the previous one",
			stage:     v2pb.DEPLOYMENT_STAGE_ROLLOUT_FAILED,
			current:   v1,
			candidate: v2,
			want:      true,
		},
		{
			// There is nothing to fall back to, but the candidate still has to be torn
			// down so the deployment ends in a clean, empty state.
			name:      "failed first rollout still rolls back",
			stage:     v2pb.DEPLOYMENT_STAGE_ROLLOUT_FAILED,
			current:   nil,
			candidate: v2,
			want:      true,
		},
		{
			name:      "failed rollout without a candidate has nothing to undo",
			stage:     v2pb.DEPLOYMENT_STAGE_ROLLOUT_FAILED,
			current:   v1,
			candidate: nil,
			want:      false,
		},
		{
			name:      "candidate equal to current is already the serving revision",
			stage:     v2pb.DEPLOYMENT_STAGE_ROLLOUT_FAILED,
			current:   v2,
			candidate: v2,
			want:      false,
		},
		{
			name:      "an in-progress rollout is left alone",
			stage:     v2pb.DEPLOYMENT_STAGE_RESOURCE_ACQUISITION,
			current:   v1,
			candidate: v2,
			want:      false,
		},
		{
			name:      "a rollback that already failed is not retried in a loop",
			stage:     v2pb.DEPLOYMENT_STAGE_ROLLBACK_FAILED,
			current:   v1,
			candidate: v2,
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deployment := v2pb.Deployment{
				Status: v2pb.DeploymentStatus{
					Stage:             tt.stage,
					CurrentRevision:   tt.current,
					CandidateRevision: tt.candidate,
				},
			}
			assert.Equal(t, tt.want, RolloutFailedNeedsRollback(deployment))
		})
	}
}

func TestDescribeRevision(t *testing.T) {
	assert.Equal(t, "no previous revision", describeRevision(nil))
	assert.Contains(t, describeRevision(&apipb.ResourceIdentifier{Name: "model-v1"}), "model-v1")
}
