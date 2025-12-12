package deployment

import (
	"context"
	"fmt"

	"go.uber.org/cadence"
	"go.uber.org/yarpc/yarpcerrors"

	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

var Activities = (*activities)(nil)

type (
	// activities struct encapsulates the YARPC client for Deployment service.
	activities struct {
		deploymentService v2pb.DeploymentServiceYARPCClient
	}

	// SensorDeploymentRequest contains parameters for the SensorDeployment activity.
	SensorDeploymentRequest struct {
		Namespace              string `json:"namespace,omitempty"`
		DeploymentName         string `json:"deploymentName,omitempty"`
		ExpectedModelRevision  string `json:"expectedModelRevision,omitempty"`
	}
)

// GetDeployment retrieves a deployment.
func (r *activities) GetDeployment(ctx context.Context, req *v2pb.GetDeploymentRequest) (*v2pb.Deployment, error) {
	resp, err := r.deploymentService.GetDeployment(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp.Deployment, nil
}

// CreateDeployment creates a new deployment.
func (r *activities) CreateDeployment(ctx context.Context, req *v2pb.CreateDeploymentRequest) (*v2pb.Deployment, error) {
	resp, err := r.deploymentService.CreateDeployment(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp.Deployment, nil
}

// UpdateDeployment updates an existing deployment.
func (r *activities) UpdateDeployment(ctx context.Context, req *v2pb.UpdateDeploymentRequest) (*v2pb.Deployment, error) {
	resp, err := r.deploymentService.UpdateDeployment(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp.Deployment, nil
}

// SensorDeployment polls the live Deployment resource until it reaches a terminal state.
// Follows the same pattern as PipelineRunSensor, SensorSparkJob, SensorRayJob.
// This ensures each workflow tracks its intended model revision, preventing race conditions
// when multiple workflows update the same deployment concurrently.
func (r *activities) SensorDeployment(ctx context.Context, req SensorDeploymentRequest) (*v2pb.Deployment, error) {
	deployment, err := r.GetDeployment(ctx, &v2pb.GetDeploymentRequest{
		Namespace: req.Namespace,
		Name:      req.DeploymentName,
	})
	if err != nil {
		return nil, err
	}

	stage := deployment.Status.GetStage()
	desiredRev := ""
	if deployment.Spec.GetDesiredRevision() != nil {
		desiredRev = deployment.Spec.GetDesiredRevision().GetName()
	}
	currentRev := ""
	if deployment.Status.GetCurrentRevision() != nil {
		currentRev = deployment.Status.GetCurrentRevision().GetName()
	}

	// Check if deployment was updated by another workflow - fail immediately if expected revision doesn't match
	// This error is non-retriable since retrying won't change the fact that another workflow updated the deployment
	if req.ExpectedModelRevision != "" && desiredRev != req.ExpectedModelRevision {
		return nil, cadence.NewCustomError(
			"cadenceInternal:Generic",
			fmt.Sprintf("deployment was updated by another workflow: expected model revision %s, but deployment now targets %s", req.ExpectedModelRevision, desiredRev))
	}

	// Check if deployment reached terminal state (success or failure)
	if stage == v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE ||
		stage == v2pb.DEPLOYMENT_STAGE_ROLLOUT_FAILED ||
		stage == v2pb.DEPLOYMENT_STAGE_ROLLBACK_COMPLETE ||
		stage == v2pb.DEPLOYMENT_STAGE_ROLLBACK_FAILED ||
		stage == v2pb.DEPLOYMENT_STAGE_CLEAN_UP_COMPLETE ||
		stage == v2pb.DEPLOYMENT_STAGE_CLEAN_UP_FAILED {
		// Terminal state reached - return deployment and let caller handle success/failure
		return deployment, nil
	}

	// Non-terminal state - return error to trigger retry
	return nil, cadence.NewCustomError(
		yarpcerrors.CodeFailedPrecondition.String(),
		fmt.Sprintf("deployment stage %v not terminal (current revision: %s, desired revision: %s)", stage, currentRev, desiredRev))
}