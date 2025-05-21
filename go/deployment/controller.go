package deployment

import (
	"context"
	"reflect"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/michelangelo-ai/michelangelo/go/api/utils"
	"github.com/michelangelo-ai/michelangelo/go/base/env"
	"github.com/michelangelo-ai/michelangelo/go/deployment/provider"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

const requeueAfter = 10 * time.Second

type Reconciler struct {
	client.Client
	servingProvider provider.Provider
	env             env.Context
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	res := ctrl.Result{}

	var deployment v2pb.Deployment
	if err := r.Get(ctx, req.NamespacedName, &deployment); err != nil {
		if utils.IsNotFoundError(err) {
			return res, nil
		}
		res.RequeueAfter = requeueAfter
		return res, err
	}
	original := deployment.DeepCopy()

	err := r.getStatus(ctx, logger, &deployment)
	if err != nil {
		deployment.Status.State = v2pb.DEPLOYMENT_STATE_INITIALIZING
		if utils.IsNotFoundError(err) {
			logger.Info("Deployment not found, starting rolling out")
			var model v2pb.Model
			modelNamespacedName := types.NamespacedName{
				Namespace: deployment.Namespace,
				Name:      deployment.Spec.DesiredRevision.Name,
			}
			err = r.Get(ctx, modelNamespacedName, &model)
			if err != nil {
				if utils.IsNotFoundError(err) {
					deployment.Status.State = v2pb.DEPLOYMENT_STATE_UNHEALTHY
					logger.Info("Model not found, skipping rollout", "model", modelNamespacedName.String())
				} else {
					logger.Error(err, "failed to get Model")
					res.RequeueAfter = requeueAfter
				}
			}
			if err = r.rollout(ctx, logger, &deployment, &model); err != nil {
				logger.Error(err, "failed to rollout")
				res.RequeueAfter = requeueAfter
			} else {
				res.RequeueAfter = requeueAfter
			}
		} else {
			logger.Error(err, "failed to get status")
			res.RequeueAfter = requeueAfter
		}
	} else {
		logger.Info("Found Deployment", "state", deployment.Status.State, "stage", deployment.Status.Stage)
		if deployment.Status.CurrentRevision != nil && deployment.Status.CurrentRevision.Equal(deployment.Spec.DesiredRevision) {
			deployment.Status.State = v2pb.DEPLOYMENT_STATE_INITIALIZING
			var model v2pb.Model
			modelNamespacedName := types.NamespacedName{
				Namespace: deployment.Namespace,
				Name:      deployment.Spec.DesiredRevision.Name,
			}
			err = r.Get(ctx, modelNamespacedName, &model)
			if err != nil {
				if utils.IsNotFoundError(err) {
					deployment.Status.State = v2pb.DEPLOYMENT_STATE_UNHEALTHY
					logger.Info("Model not found, skipping rollout")
				} else {
					logger.Error(err, "failed to get Model")
					res.RequeueAfter = requeueAfter
				}
			} else {
				res.RequeueAfter = requeueAfter
				err = r.updateDeployment(ctx, logger, &deployment, &model)
				if err != nil {
					logger.Error(err, "failed to update Deployment")
				}
			}
		}
		// When reach to healthy or unhealthy state, we don't need to requeue
		if deployment.Status.State == v2pb.DEPLOYMENT_STATE_HEALTHY {
			deployment.Status.CurrentRevision = deployment.Spec.DesiredRevision
		} else if deployment.Status.State != v2pb.DEPLOYMENT_STATE_UNHEALTHY {
			res.RequeueAfter = requeueAfter
		}
	}

	if !reflect.DeepEqual(original, deployment) {
		if err = r.Status().Update(ctx, &deployment); err != nil {
			logger.Error(err, "failed to update Deployment status")
			res.RequeueAfter = requeueAfter
			return res, err
		}
	}

	logger.Info("Deployment reconciled", "name", deployment.Name, "namespace", deployment.Namespace)

	return res, err
}

func (r *Reconciler) Register(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v2pb.Deployment{}).
		Complete(r)
}

// createJob creates a new Spark job
func (r *Reconciler) rollout(ctx context.Context, log logr.Logger, deployment *v2pb.Deployment, model *v2pb.Model) error {
	return r.servingProvider.Rollout(ctx, log, deployment, model)
}

func (r *Reconciler) updateDeployment(ctx context.Context, log logr.Logger, deployment *v2pb.Deployment, model *v2pb.Model) error {
	return r.servingProvider.Update(ctx, log, deployment, model)
}

// getJobStatus retrieves the status of the Spark job
func (r *Reconciler) getStatus(ctx context.Context, logger logr.Logger, deployment *v2pb.Deployment) error {
	return r.servingProvider.GetStatus(ctx, logger, deployment)
}
