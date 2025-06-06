package deployment

import (
	"context"
	"encoding/json"
	"fmt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
			} else {
				if err = r.createDeployment(ctx, logger, &deployment, &model); err != nil {
					logger.Error(err, "failed to rollout")
					res.RequeueAfter = requeueAfter
				} else {
					res.RequeueAfter = requeueAfter
				}
			}
		} else {
			logger.Error(err, "failed to get status")
			res.RequeueAfter = requeueAfter
		}
	} else {
		logger.Info("Found Deployment", "state", deployment.Status.State, "stage", deployment.Status.Stage)
		if deployment.Status.CurrentRevision != nil && !deployment.Status.CurrentRevision.Equal(deployment.Spec.DesiredRevision) {
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
				_, found, configErr := r.getConfig(ctx, &deployment, model.Name)
				if configErr != nil {
					return res, fmt.Errorf("failed to get ConfigMap: %w", err)
				}
				if !found {
					logger.Info("Model not found, update the configMap")
					res.RequeueAfter = requeueAfter
					err = r.updateConfig(ctx, &deployment, &model)
					if err != nil {
						logger.Error(err, "failed to update Deployment")
					}
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
func (r *Reconciler) createDeployment(ctx context.Context, log logr.Logger, deployment *v2pb.Deployment, model *v2pb.Model) error {
	return r.servingProvider.CreateDeployment(ctx, log, deployment, model)
}

// getJobStatus retrieves the status of the Spark job
func (r *Reconciler) getStatus(ctx context.Context, logger logr.Logger, deployment *v2pb.Deployment) error {
	return r.servingProvider.GetStatus(ctx, logger, deployment)
}

func (r *Reconciler) updateConfig(ctx context.Context, deployment *v2pb.Deployment, model *v2pb.Model) error {
	logger := log.FromContext(ctx)

	modelList := []map[string]string{
		{
			"name":    model.Name,
			"s3_path": fmt.Sprintf("s3://deploy-models/%s", model.Spec.DeployableArtifactUri[0]),
		},
	}

	modelListJSON, err := json.MarshalIndent(modelList, "", "  ")
	if err != nil {
		logger.Error(err, "Failed to marshal model list")
		return err
	}

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "triton-models",
			Namespace: deployment.Namespace,
		},
		Data: map[string]string{
			"model-list.json": string(modelListJSON),
		},
	}

	if err := r.Update(ctx, cm); err != nil {
		logger.Error(err, "Failed to update ConfigMap")
		return err
	}

	logger.Info("ConfigMap updated successfully")
	return nil
}

func (r *Reconciler) getConfig(ctx context.Context, deployment *v2pb.Deployment, modelName string) ([]map[string]string, bool, error) {
	logger := log.FromContext(ctx)

	cm := &corev1.ConfigMap{}
	cmKey := client.ObjectKey{
		Name:      "triton-models",
		Namespace: deployment.Namespace,
	}

	if err := r.Get(ctx, cmKey, cm); err != nil {
		logger.Error(err, "Failed to fetch ConfigMap")
		return nil, false, err
	}

	data, exists := cm.Data["model-list.json"]
	if !exists {
		err := fmt.Errorf("model-list.json not found in ConfigMap")
		logger.Error(err, "Key missing in ConfigMap")
		return nil, false, err
	}

	var modelList []map[string]string
	if err := json.Unmarshal([]byte(data), &modelList); err != nil {
		logger.Error(err, "Failed to unmarshal model list")
		return nil, false, err
	}

	found := false
	for _, model := range modelList {
		if model["name"] == modelName {
			found = true
			break
		}
	}

	logger.Info("ConfigMap fetched and unmarshalled successfully")
	return modelList, found, nil
}
