// Package apihook registers the Deployment API hook. It runs in the apiserver
// before a CreateDeployment or UpdateDeployment request reaches Kubernetes and
// is the place for server-side defaulting and validation that every client
// (Studio, CLI, direct API callers) should get for free. It follows the shape
// of go/components/model/apihook and go/components/pipelinerun/apihook.
//
// Each concern lives in its own file and is called from mutate() below;
// apihook.go only owns the request plumbing.
package apihook

import (
	"context"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/michelangelo-ai/michelangelo/go/api"
	"github.com/michelangelo-ai/michelangelo/go/api/utils"
	v2 "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// RegisterDeploymentAPIHook registers the Deployment API hook with the
// generated DeploymentService handler.
func RegisterDeploymentAPIHook(logger *zap.Logger, apiHandler api.Handler) {
	v2.RegisterDeploymentAPIHook(apiHook{
		logger:     logger,
		apiHandler: apiHandler,
	})
}

type apiHook struct {
	v2.NoopDeploymentAPIHook
	logger     *zap.Logger
	apiHandler api.Handler
}

func (a apiHook) BeforeCreate(ctx context.Context, request *v2.CreateDeploymentRequest) error {
	return a.mutate(ctx, request.Deployment, nil)
}

func (a apiHook) BeforeUpdate(ctx context.Context, request *v2.UpdateDeploymentRequest) error {
	deployment := request.Deployment
	if deployment == nil {
		return nil
	}

	existing := &v2.Deployment{}
	err := a.apiHandler.Get(ctx, deployment.GetNamespace(), deployment.GetName(), &metav1.GetOptions{}, existing)
	if err != nil {
		if utils.IsNotFoundError(err) {
			// Nothing to compare against; the update itself will fail downstream.
			return a.mutate(ctx, deployment, nil)
		}
		return status.Errorf(codes.Internal,
			"failed to get existing deployment %s/%s: %v", deployment.GetNamespace(), deployment.GetName(), err)
	}
	return a.mutate(ctx, deployment, existing)
}

// mutate applies every defaulting and validation step to an incoming
// Deployment, in order. existing is the Deployment currently stored on the
// server, or nil on create (and on update when it cannot be found). Steps edit
// deployment in place; the first error rejects the whole request.
func (a apiHook) mutate(ctx context.Context, deployment *v2.Deployment, existing *v2.Deployment) error {
	if deployment == nil {
		return nil
	}
	if err := a.fulfillModelFamily(ctx, deployment, existing); err != nil {
		return err
	}
	return nil
}
