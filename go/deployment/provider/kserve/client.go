package kserve

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	"github.com/michelangelo-ai/michelangelo/go/deployment/provider"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

type KserveProvider struct {
	DynamicClient dynamic.Interface
}

var _ provider.Provider = &KserveProvider{}

func (r KserveProvider) CreateDeployment(ctx context.Context, log logr.Logger, deployment *v2pb.Deployment, model *v2pb.Model) error {
	gvr := schema.GroupVersionResource{
		Group:    "serving.kserve.io",
		Version:  "v1beta1",
		Resource: "inferenceservices",
	}

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "serving.kserve.io/v1beta1",
			"kind":       "InferenceService",
			"metadata": map[string]interface{}{
				"name":      deployment.Name,
				"namespace": deployment.Namespace,
			},
			"spec": map[string]interface{}{
				"predictor": map[string]interface{}{
					"triton": map[string]interface{}{
						"runtimeVersion": "24.04-py3",
						"resources": map[string]interface{}{
							"requests": map[string]interface{}{
								"cpu":    "1",
								"memory": "2Gi",
							},
							"limits": map[string]interface{}{
								"cpu":    "1",
								"memory": "2Gi",
							},
						},
						"storage": map[string]interface{}{
							"key":  "localMinIO",
							"path": model.Spec.DeployableArtifactUri[0],
							"parameters": map[string]interface{}{
								"bucket": "deploy-models",
							},
						},
					},
				},
			},
		},
	}

	result, err := r.DynamicClient.Resource(gvr).Namespace(deployment.Namespace).Create(ctx, obj, metav1.CreateOptions{})
	if err != nil {
		log.Error(err, "Failed to create InferenceService")
		return err
	}

	return r.updateDeploymentStatus(result, log, deployment)
}

func (r KserveProvider) Rollout(ctx context.Context, log logr.Logger, deployment *v2pb.Deployment, model *v2pb.Model) error {
	gvr := schema.GroupVersionResource{
		Group:    "serving.kserve.io",
		Version:  "v1beta1",
		Resource: "inferenceservices",
	}

	result, err := r.get(ctx, deployment)
	if err != nil {
		return err
	}
	storage := map[string]interface{}{
		"key":  "localMinIO",
		"path": model.Spec.DeployableArtifactUri[0],
		"parameters": map[string]interface{}{
			"bucket": "deploy-models",
		},
	}
	unstructured.SetNestedMap(result.Object, storage, "spec", "predictor", "triton", "storage")

	updateRes, err := r.DynamicClient.Resource(gvr).Namespace(deployment.Namespace).Update(ctx, result, metav1.UpdateOptions{})
	if err != nil {
		log.Error(err, "Failed to create InferenceService")
		return err
	}

	return r.updateDeploymentStatus(updateRes, log, deployment)
}

func (r KserveProvider) get(ctx context.Context, deployment *v2pb.Deployment) (*unstructured.Unstructured, error) {
	gvr := schema.GroupVersionResource{
		Group:    "serving.kserve.io",
		Version:  "v1beta1",
		Resource: "inferenceservices",
	}

	return r.DynamicClient.Resource(gvr).Namespace(deployment.Namespace).Get(ctx, deployment.Name, metav1.GetOptions{})
}

func (r KserveProvider) GetStatus(ctx context.Context, logger logr.Logger, deployment *v2pb.Deployment) error {
	gvr := schema.GroupVersionResource{
		Group:    "serving.kserve.io",
		Version:  "v1beta1",
		Resource: "inferenceservices",
	}

	result, err := r.DynamicClient.Resource(gvr).Namespace(deployment.Namespace).Get(ctx, deployment.Name, metav1.GetOptions{})
	if err != nil {
		logger.Error(err, "Failed to retrieve InferenceService status")
		return err
	}

	return r.updateDeploymentStatus(result, logger, deployment)
}

func (r KserveProvider) Retire(ctx context.Context, log logr.Logger, deployment *v2pb.Deployment) error {
	gvr := schema.GroupVersionResource{
		Group:    "serving.kserve.io",
		Version:  "v1beta1",
		Resource: "inferenceservices",
	}

	err := r.DynamicClient.Resource(gvr).Namespace(deployment.Namespace).Delete(ctx, deployment.Name, metav1.DeleteOptions{})
	if err != nil {
		log.Error(err, "Failed to delete InferenceService")
		return err
	}

	log.Info("Deleted InferenceService", "name", deployment.Name)
	return nil
}

func (r KserveProvider) updateDeploymentStatus(result *unstructured.Unstructured, logger logr.Logger, deployment *v2pb.Deployment) error {
	model, foundModel, err := unstructured.NestedString(result.Object, "spec", "predictor", "model", "storage", "path")
	if err != nil {
		logger.Error(err, "Failed to retrieve InferenceService status")
		return err
	}
	if foundModel {
		deployment.Status.CurrentRevision = &apipb.ResourceIdentifier{
			Namespace: deployment.Namespace,
			Name:      model,
		}
	}
	conditions, found, _ := unstructured.NestedSlice(result.Object, "status", "conditions")
	if found {
		deployment.Status.Conditions = nil
		for _, c := range conditions {
			condition, _ := c.(map[string]interface{})
			typeStr, _, _ := unstructured.NestedString(condition, "type")
			statusStr, _, _ := unstructured.NestedString(condition, "status")
			message, _, _ := unstructured.NestedString(condition, "message")
			lastTransitionTime, _, _ := unstructured.NestedString(condition, "lastTransitionTime")
			timestamp, _ := time.Parse(time.RFC3339, lastTransitionTime)

			var conditionStatus apipb.ConditionStatus
			if statusStr == "True" {
				conditionStatus = apipb.CONDITION_STATUS_TRUE
			} else if statusStr == "False" {
				conditionStatus = apipb.CONDITION_STATUS_FALSE
			} else {
				conditionStatus = apipb.CONDITION_STATUS_UNKNOWN
			}
			deployment.Status.Conditions = append(deployment.Status.Conditions, &apipb.Condition{
				Type:                 typeStr,
				Status:               conditionStatus,
				Message:              message,
				LastUpdatedTimestamp: timestamp.Unix(),
			})

			if typeStr == "Ready" {
				if statusStr == "True" {
					deployment.Status.State = v2pb.DEPLOYMENT_STATE_HEALTHY
					deployment.Status.Stage = v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE
					deployment.Status.Message = message
				} else {
					deployment.Status.State = v2pb.DEPLOYMENT_STATE_UNHEALTHY
					deployment.Status.Stage = v2pb.DEPLOYMENT_STAGE_ROLLOUT_FAILED
					deployment.Status.Message = message
				}
			}
		}
	} else {
		deployment.Status.State = v2pb.DEPLOYMENT_STATE_INITIALIZING
		deployment.Status.Stage = v2pb.DEPLOYMENT_STAGE_VALIDATION
		deployment.Status.Message = "InferenceService conditions not yet available"
	}

	logger.Info("Updated InferenceService status", "state", deployment.Status.State, "stage", deployment.Status.Stage)
	return nil
}
