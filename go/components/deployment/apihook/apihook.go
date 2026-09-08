// Package apihook registers the Deployment API hook that populates
// Spec.ModelFamily from the Model named by Spec.DesiredRevision at
// create/update time. It follows the shape of go/components/model/apihook and
// go/components/pipelinerun/apihook.
//
// Spec.ModelFamily exists on the Deployment proto but nothing in the controller
// writes it, so until now every Deployment was created without it and clients
// had to re-derive the family with a second GetModel call. The Model is the
// source of truth for which family a Deployment belongs to, so the hook copies
// the Model's family onto the Deployment and rejects writes that would put the
// two out of sync.
//
// A Deployment's family is immutable once set: a rollout to a Model from a
// different family is rejected on update. Deployments created before this hook
// existed carry no family, so their first update simply backfills it.
package apihook

import (
	"context"
	"errors"
	"fmt"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/michelangelo-ai/michelangelo/go/api"
	"github.com/michelangelo-ai/michelangelo/go/api/utils"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/common"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2 "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// RegisterDeploymentAPIHook registers the API hook that populates
// Spec.ModelFamily on Deployments from their desired Model.
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
	return a.applyModelFamily(ctx, request.Deployment, nil)
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
			return a.applyModelFamily(ctx, deployment, nil)
		}
		return status.Errorf(codes.Internal,
			"failed to get existing deployment %s/%s: %v", deployment.GetNamespace(), deployment.GetName(), err)
	}
	return a.applyModelFamily(ctx, deployment, existing.Spec.GetModelFamily())
}

// applyModelFamily resolves the Deployment's desired Model and copies its
// family onto Spec.ModelFamily. lockedFamily is the family the Deployment
// already has on the server (nil on create or when unset); once set it cannot
// change.
//
// Deployments with no desired revision (retirements) are left untouched. A
// missing Model is not an error here: the controller already reports it on the
// AssetsPrepared condition, and blocking the write would hide that signal. Any
// other lookup failure is returned so a transient API-server error doesn't
// silently produce a Deployment with the wrong family.
func (a apiHook) applyModelFamily(ctx context.Context, deployment *v2.Deployment, lockedFamily *apipb.ResourceIdentifier) error {
	if deployment == nil || deployment.Spec.GetDesiredRevision().GetName() == "" {
		return nil
	}

	model, err := common.FetchModel(ctx, a.apiHandler, deployment)
	if err != nil {
		var resolutionErr *common.ModelResolutionError
		if errors.As(err, &resolutionErr) {
			a.logger.Warn("applyModelFamily: desired model not found, leaving model family unset",
				zap.String("deployment", deployment.GetName()),
				zap.String("model", deployment.Spec.GetDesiredRevision().GetName()))
			return nil
		}
		return status.Errorf(codes.Internal, "failed to resolve desired model for deployment %s/%s: %v",
			deployment.GetNamespace(), deployment.GetName(), err)
	}

	modelFamily := model.Spec.GetModelFamily()
	if modelFamily.GetName() == "" {
		a.logger.Info("applyModelFamily: desired model has no model family, leaving model family unset",
			zap.String("deployment", deployment.GetName()),
			zap.String("model", model.GetName()))
		return nil
	}

	if lockedFamily.GetName() != "" && lockedFamily.GetName() != modelFamily.GetName() {
		return status.Errorf(codes.InvalidArgument,
			"the model family of a deployment cannot be changed: deployment %s/%s belongs to model family %q but model %q belongs to %q",
			deployment.GetNamespace(), deployment.GetName(), lockedFamily.GetName(), model.GetName(), modelFamily.GetName())
	}

	requested := deployment.Spec.GetModelFamily()
	if requested.GetName() != "" && requested.GetName() != modelFamily.GetName() {
		return status.Error(codes.InvalidArgument, fmt.Sprintf(
			"spec.modelFamily %q does not match the model family %q of model %q",
			requested.GetName(), modelFamily.GetName(), model.GetName()))
	}

	namespace := modelFamily.GetNamespace()
	if namespace == "" {
		namespace = model.GetNamespace()
	}
	deployment.Spec.ModelFamily = &apipb.ResourceIdentifier{
		Namespace: namespace,
		Name:      modelFamily.GetName(),
	}
	return nil
}
