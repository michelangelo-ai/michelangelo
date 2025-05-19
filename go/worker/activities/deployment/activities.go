package deployment

import (
	"context"
	"github.com/cadence-workflow/starlark-worker/workflow"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	"go.uber.org/yarpc/yarpcerrors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

var Activities = (*activities)(nil)

// activities struct encapsulates the YARPC clients for Spark cluster and job services.
type activities struct {
	deployment v2pb.DeploymentServiceYARPCClient
}

// SensorRolloutRequest DTO
type SensorRolloutRequest struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	ModelName string `json:"model_name"`
}

func (r *activities) SensorRollout(ctx context.Context, request SensorRolloutRequest) (*v2pb.GetDeploymentResponse, error) {
	deploymentRes, err := r.deployment.GetDeployment(ctx, &v2pb.GetDeploymentRequest{
		Name:       request.Name,
		Namespace:  request.Namespace,
		GetOptions: &metav1.GetOptions{},
	})
	if err != nil {
		return nil, err
	}
	if deploymentRes == nil {
		return nil, yarpcerrors.Newf(yarpcerrors.CodeNotFound, "deployment not found")
	}
	deployment := deploymentRes.GetDeployment()
	deployment.Spec.DesiredRevision = &apipb.ResourceIdentifier{
		Namespace: request.Namespace,
		Name:      request.ModelName,
	}
	updatedDeploymentRes, err := r.UpdateDeployment(ctx, v2pb.UpdateDeploymentRequest{
		Deployment:    deployment,
		UpdateOptions: &metav1.UpdateOptions{},
	})
	updatedDeployment := updatedDeploymentRes.GetDeployment()
	if err != nil {
		return nil, workflow.NewCustomError(ctx, yarpcerrors.CodeInternal.String(), "failed to update deployment")
	}
	deploymentCompleted := updatedDeployment.Status.CurrentRevision.Equal(updatedDeployment.Spec.DesiredRevision)
	if deploymentCompleted {
		return &v2pb.GetDeploymentResponse{
			Deployment: deployment,
		}, nil
	}

	return nil, workflow.NewCustomError(ctx, yarpcerrors.CodeFailedPrecondition.String(), deployment.Status.Stage)
}

func (r *activities) GetDeployment(ctx context.Context, request v2pb.GetDeploymentRequest) (*v2pb.GetDeploymentResponse, error) {
	return r.deployment.GetDeployment(ctx, &request)
}

func (r *activities) ListDeployment(ctx context.Context, request v2pb.ListDeploymentRequest) (*v2pb.ListDeploymentResponse, error) {
	return r.deployment.ListDeployment(ctx, &request)
}

func (r *activities) CreateDeployment(ctx context.Context, request v2pb.CreateDeploymentRequest) (*v2pb.CreateDeploymentResponse, error) {
	return r.deployment.CreateDeployment(ctx, &request)
}

func (r *activities) UpdateDeployment(ctx context.Context, request v2pb.UpdateDeploymentRequest) (*v2pb.UpdateDeploymentResponse, error) {
	return r.deployment.UpdateDeployment(ctx, &request)
}
