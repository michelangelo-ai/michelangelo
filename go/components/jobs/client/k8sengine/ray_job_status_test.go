package k8sengine

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
)

// TestMapLocalJobStatusToGlobal_RayJobState pins the RayJobState derived from
// each KubeRay JobStatus/JobDeploymentStatus pair. KubeRay can end a job through
// JobDeploymentStatus alone (a failed submitter, an exceeded
// activeDeadlineSeconds, a spec that failed validation) and never update
// JobStatus again, so a mapping that read only JobStatus would leave those jobs
// non-terminal forever.
func TestMapLocalJobStatusToGlobal_RayJobState(t *testing.T) {
	tests := []struct {
		jobStatus        rayv1.JobStatus
		deploymentStatus rayv1.JobDeploymentStatus
		want             v2pb.RayJobState
	}{
		// A terminal JobStatus wins over the deployment status.
		{rayv1.JobStatusSucceeded, rayv1.JobDeploymentStatusComplete, v2pb.RAY_JOB_STATE_SUCCEEDED},
		{rayv1.JobStatusSucceeded, rayv1.JobDeploymentStatusRunning, v2pb.RAY_JOB_STATE_SUCCEEDED},
		{rayv1.JobStatusFailed, rayv1.JobDeploymentStatusFailed, v2pb.RAY_JOB_STATE_FAILED},
		{rayv1.JobStatusStopped, rayv1.JobDeploymentStatusComplete, v2pb.RAY_JOB_STATE_KILLED},
		{rayv1.JobStatusStopped, rayv1.JobDeploymentStatusRunning, v2pb.RAY_JOB_STATE_KILLED},

		// Ended by the deployment status alone.
		{rayv1.JobStatusRunning, rayv1.JobDeploymentStatusFailed, v2pb.RAY_JOB_STATE_FAILED},
		{rayv1.JobStatusPending, rayv1.JobDeploymentStatusFailed, v2pb.RAY_JOB_STATE_FAILED},
		{rayv1.JobStatusNew, rayv1.JobDeploymentStatusFailed, v2pb.RAY_JOB_STATE_FAILED},
		{rayv1.JobStatusNew, rayv1.JobDeploymentStatusValidationFailed, v2pb.RAY_JOB_STATE_FAILED},

		{rayv1.JobStatusRunning, rayv1.JobDeploymentStatusRunning, v2pb.RAY_JOB_STATE_RUNNING},

		// Not running yet.
		{rayv1.JobStatusPending, rayv1.JobDeploymentStatusRunning, v2pb.RAY_JOB_STATE_INITIALIZING},
		{rayv1.JobStatusNew, rayv1.JobDeploymentStatusNew, v2pb.RAY_JOB_STATE_INITIALIZING},
		{rayv1.JobStatusNew, rayv1.JobDeploymentStatusInitializing, v2pb.RAY_JOB_STATE_INITIALIZING},
		{rayv1.JobStatusNew, rayv1.JobDeploymentStatusWaiting, v2pb.RAY_JOB_STATE_INITIALIZING},
		{rayv1.JobStatusNew, rayv1.JobDeploymentStatusRunning, v2pb.RAY_JOB_STATE_INITIALIZING},

		// Unexpected combinations.
		{rayv1.JobStatusNew, rayv1.JobDeploymentStatusComplete, v2pb.RAY_JOB_STATE_INVALID},
	}

	// Both New statuses are the empty string.
	name := func(status string) string {
		if status == "" {
			return "New"
		}
		return status
	}

	for _, tt := range tests {
		t.Run(name(string(tt.jobStatus))+"+"+name(string(tt.deploymentStatus)), func(t *testing.T) {
			local := &rayv1.RayJob{}
			local.Status.JobStatus = tt.jobStatus
			local.Status.JobDeploymentStatus = tt.deploymentStatus

			js, err := Mapper{}.MapLocalJobStatusToGlobal(local)
			require.NoError(t, err)
			require.NotNil(t, js)
			require.NotNil(t, js.Ray)
			assert.Equal(t, tt.want, js.Ray.State)
		})
	}
}
