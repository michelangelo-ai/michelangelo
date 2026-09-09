package apihook

import (
	"context"
	"errors"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/common"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2 "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// fulfillModelFamily resolves the Deployment's desired Model and copies its
// family onto Spec.ModelFamily. The Model is the source of truth for which
// family a Deployment belongs to, so a request whose Spec.ModelFamily disagrees
// with the Model is rejected.
//
// A Deployment's family is immutable once stored: an update that points at a
// Model from a different family is rejected. Deployments created before this
// hook existed carry no family and are backfilled on their next update.
//
// Deployments with no desired revision (retirements) are left untouched. A
// missing Model is not an error here: the controller already reports it on the
// AssetsPrepared condition, and blocking the write would hide that signal. Any
// other lookup failure is returned so a transient API-server error doesn't
// silently produce a Deployment with the wrong family.
func (a apiHook) fulfillModelFamily(ctx context.Context, deployment *v2.Deployment, existing *v2.Deployment) error {
	if deployment.Spec.GetDesiredRevision().GetName() == "" {
		return nil
	}

	model, err := common.FetchModel(ctx, a.apiHandler, deployment)
	if err != nil {
		var resolutionErr *common.ModelResolutionError
		if errors.As(err, &resolutionErr) {
			a.logger.Warn("fulfillModelFamily: desired model not found, leaving model family unset",
				zap.String("deployment", deployment.GetName()),
				zap.String("model", deployment.Spec.GetDesiredRevision().GetName()))
			return nil
		}
		return status.Errorf(codes.Internal, "failed to resolve desired model for deployment %s/%s: %v",
			deployment.GetNamespace(), deployment.GetName(), err)
	}

	modelFamily := model.Spec.GetModelFamily()
	if modelFamily.GetName() == "" {
		a.logger.Info("fulfillModelFamily: desired model has no model family, leaving model family unset",
			zap.String("deployment", deployment.GetName()),
			zap.String("model", model.GetName()))
		return nil
	}

	var lockedFamily *apipb.ResourceIdentifier
	if existing != nil {
		lockedFamily = existing.Spec.GetModelFamily()
	}
	if lockedFamily.GetName() != "" && lockedFamily.GetName() != modelFamily.GetName() {
		return status.Errorf(codes.InvalidArgument,
			"the model family of a deployment cannot be changed: deployment %s/%s belongs to model family %q but model %q belongs to %q",
			deployment.GetNamespace(), deployment.GetName(), lockedFamily.GetName(), model.GetName(), modelFamily.GetName())
	}

	requested := deployment.Spec.GetModelFamily()
	if requested.GetName() != "" && requested.GetName() != modelFamily.GetName() {
		return status.Errorf(codes.InvalidArgument,
			"spec.modelFamily %q does not match the model family %q of model %q",
			requested.GetName(), modelFamily.GetName(), model.GetName())
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
