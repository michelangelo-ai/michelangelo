package deployment

import (
	"context"
	"fmt"

	"github.com/cadence-workflow/starlark-worker/workflow"
	"go.uber.org/yarpc/yarpcerrors"

	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

var Activities = (*activities)(nil)

type (
	// activities struct encapsulates the YARPC client for Deployment service.
	activities struct {
		deploymentService v2pb.DeploymentServiceYARPCClient
	}

	// SensorDeploymentRequest contains parameters for the SensorDeployment activity.
	SensorDeploymentRequest struct {
		Namespace             string `json:"namespace,omitempty"`
		DeploymentName        string `json:"deploymentName,omitempty"`
		ExpectedModelRevision string `json:"expectedModelRevision,omitempty"`
	}
)

// GetDeployment retrieves a deployment. A not-found deployment is reported as
// (nil, nil) rather than an error, so callers can use existence checks (e.g.
// deciding between create and update) without having to parse error codes.
// Any other failure (transient, auth, etc.) is returned as an error.
func (r *activities) GetDeployment(ctx context.Context, req *v2pb.GetDeploymentRequest) (*v2pb.Deployment, error) {
	resp, err := r.deploymentService.GetDeployment(ctx, req)
	if err != nil {
		if yarpcerrors.FromError(err).Code() == yarpcerrors.CodeNotFound {
			return nil, nil
		}
		return nil, workflow.NewCustomError(ctx, fmt.Sprintf("%s: %s", yarpcerrors.FromError(err).Code().String(), err.Error()))
	}
	return resp.Deployment, nil
}

// CreateDeployment creates a new deployment.
func (r *activities) CreateDeployment(ctx context.Context, req *v2pb.CreateDeploymentRequest) (*v2pb.Deployment, error) {
	resp, err := r.deploymentService.CreateDeployment(ctx, req)
	if err != nil {
		return nil, workflow.NewCustomError(ctx, fmt.Sprintf("%s: %s", yarpcerrors.FromError(err).Code().String(), err.Error()))
	}
	return resp.Deployment, nil
}

// UpdateDeployment updates an existing deployment.
func (r *activities) UpdateDeployment(ctx context.Context, req *v2pb.UpdateDeploymentRequest) (*v2pb.Deployment, error) {
	resp, err := r.deploymentService.UpdateDeployment(ctx, req)
	if err != nil {
		return nil, workflow.NewCustomError(ctx, fmt.Sprintf("%s: %s", yarpcerrors.FromError(err).Code().String(), err.Error()))
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
		// GetDeployment already wraps this with a distinguishable reason so
		// transient failures are retried by the sensor's retry policy instead
		// of being bucketed under Cadence's non-retriable generic reason.
		return nil, err
	}
	if deployment == nil {
		// The deployment disappeared entirely - retrying won't help, so this
		// is deliberately reported as non-retriable via the shared reason.
		return nil, workflow.NewCustomError(ctx, yarpcerrors.CodeNotFound.String(),
			fmt.Sprintf("deployment %s/%s not found", req.Namespace, req.DeploymentName))
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
		return nil, workflow.NewCustomError(ctx, "cadenceInternal:Generic",
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
	return nil, workflow.NewCustomError(ctx, yarpcerrors.CodeFailedPrecondition.String(),
		fmt.Sprintf("deployment stage %v not terminal (current revision: %s, desired revision: %s)", stage, currentRev, desiredRev))
}
