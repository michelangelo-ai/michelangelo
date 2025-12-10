package deployment

import (
	"context"
	"fmt"

	"github.com/gogo/protobuf/types"
	"go.uber.org/cadence"
	"go.uber.org/yarpc/yarpcerrors"

	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

var Activities = (*activities)(nil)

type (
	// activities struct encapsulates the YARPC clients for Deployment and Revision services.
	activities struct {
		deploymentService v2pb.DeploymentServiceYARPCClient
		revisionService   v2pb.RevisionServiceYARPCClient
	}

	// GetLatestDeploymentRevisionRequest contains parameters for retrieving the latest deployment revision.
	GetLatestDeploymentRevisionRequest struct {
		Namespace       string `json:"namespace,omitempty"`
		DeploymentName  string `json:"deploymentName,omitempty"`
		OldRevisionName string `json:"oldRevisionName,omitempty"`
	}

	// SensorDeploymentRevisionRequest contains parameters for the SensorDeploymentRevision activity.
	SensorDeploymentRevisionRequest struct {
		Namespace    string `json:"namespace,omitempty"`
		RevisionName string `json:"revisionName,omitempty"`
	}

	// SensorDeploymentRequest contains parameters for the SensorDeployment activity.
	SensorDeploymentRequest struct {
		Namespace      string `json:"namespace,omitempty"`
		DeploymentName string `json:"deploymentName,omitempty"`
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

// GetLatestDeploymentRevision retrieves the name of the most recent deployment revision.
// NOTE: Requires deployment revision service to be enabled. In OSS, revision creation is disabled
// via revision.NewNoOpManager() in the deployment controller.
func (r *activities) GetLatestDeploymentRevision(ctx context.Context, req GetLatestDeploymentRevisionRequest) (string, error) {
	listReq := &v2pb.ListRevisionRequest{
		Namespace: req.Namespace,
		ListOptionsExt: &apipb.ListOptionsExt{
			Operation: &apipb.CriterionOperation{
				Criterion: []*apipb.Criterion{
					{
						FieldName: "revision.spec.base_resource.name",
						Operator:  apipb.CRITERION_OPERATOR_EQUAL,
						MatchValue: &types.Any{
							Value: []byte(req.DeploymentName),
						},
					},
					{
						FieldName: "revision.spec.base_type.kind",
						Operator:  apipb.CRITERION_OPERATOR_EQUAL,
						MatchValue: &types.Any{
							Value: []byte("Deployment"),
						},
					},
				},
				LogicalOperator: apipb.LOGICAL_OPERATOR_AND,
			},
			OrderBy: []*apipb.OrderBy{{
				Field: "metadata.update_timestamp",
				Dir:   apipb.SORT_ORDER_DESC,
			}},
			Pagination: &apipb.PaginationSpec{
				Offset: 0,
				Limit:  1,
			},
		},
	}

	res, err := r.revisionService.ListRevision(ctx, listReq)
	if err != nil {
		return "", err
	}

	if res.RevisionList.Items == nil || len(res.RevisionList.Items) == 0 {
		return "", cadence.NewCustomError(yarpcerrors.CodeFailedPrecondition.String(), "no deployment revisions found")
	}

	latestRevision := res.RevisionList.Items[0]

	if req.OldRevisionName != "" && latestRevision.ObjectMeta.Name == req.OldRevisionName {
		return "", cadence.NewCustomError(yarpcerrors.CodeFailedPrecondition.String(), "latest revision is the same as the old revision")
	}

	return latestRevision.ObjectMeta.Name, nil
}

// SensorDeploymentRevision acts as a sensor, fails until deployment reaches terminal state.
// NOTE: Requires revision service - kept for backward compatibility with internal.
func (r *activities) SensorDeploymentRevision(ctx context.Context, req SensorDeploymentRevisionRequest) (*v2pb.Deployment, error) {
	res, err := r.revisionService.GetRevision(ctx, &v2pb.GetRevisionRequest{
		Namespace: req.Namespace,
		Name:      req.RevisionName,
	})
	if err != nil {
		return nil, err
	}

	deployment := &v2pb.Deployment{}
	if err := types.UnmarshalAny(res.Revision.Spec.Content, deployment); err != nil {
		return nil, err
	}

	stage := deployment.Status.Stage
	if stage != v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE &&
		stage != v2pb.DEPLOYMENT_STAGE_ROLLOUT_FAILED &&
		stage != v2pb.DEPLOYMENT_STAGE_ROLLBACK_FAILED &&
		stage != v2pb.DEPLOYMENT_STAGE_ROLLBACK_COMPLETE &&
		stage != v2pb.DEPLOYMENT_STAGE_CLEAN_UP_FAILED &&
		stage != v2pb.DEPLOYMENT_STAGE_CLEAN_UP_COMPLETE {
		return nil, cadence.NewCustomError(yarpcerrors.CodeFailedPrecondition.String(), fmt.Sprintf("deployment stage %v not terminal", stage))
	}

	return deployment, nil
}

// SensorDeployment polls the live Deployment resource until it reaches a terminal state.
// Unlike SensorDeploymentRevision, this works in OSS without revision service.
// Follows the same pattern as PipelineRunSensor, SensorSparkJob, SensorRayJob.
func (r *activities) SensorDeployment(ctx context.Context, req SensorDeploymentRequest) (*v2pb.Deployment, error) {
	// Get the live deployment resource
	deployment, err := r.GetDeployment(ctx, &v2pb.GetDeploymentRequest{
		Namespace: req.Namespace,
		Name:      req.DeploymentName,
	})
	if err != nil {
		return nil, err
	}

	// Check if current revision matches desired revision
	desiredRev := deployment.Spec.GetDesiredRevision().GetName()
	currentRev := deployment.Status.GetCurrentRevision().GetName()

	if currentRev != desiredRev {
		return nil, cadence.NewCustomError(
			yarpcerrors.CodeFailedPrecondition.String(),
			fmt.Sprintf("waiting for revision %s to be deployed, currently at %s", desiredRev, currentRev))
	}

	// Check if deployment has reached a terminal stage
	stage := deployment.Status.GetStage()
	if stage != v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE &&
		stage != v2pb.DEPLOYMENT_STAGE_ROLLOUT_FAILED &&
		stage != v2pb.DEPLOYMENT_STAGE_ROLLBACK_FAILED &&
		stage != v2pb.DEPLOYMENT_STAGE_ROLLBACK_COMPLETE &&
		stage != v2pb.DEPLOYMENT_STAGE_CLEAN_UP_FAILED &&
		stage != v2pb.DEPLOYMENT_STAGE_CLEAN_UP_COMPLETE {
		return nil, cadence.NewCustomError(
			yarpcerrors.CodeFailedPrecondition.String(),
			fmt.Sprintf("deployment stage %v not terminal (current: %s, desired: %s)", stage, currentRev, desiredRev))
	}

	return deployment, nil
}
