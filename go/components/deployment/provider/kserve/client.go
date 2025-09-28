package kserve

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/provider"
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

	// PLUGIN COMPATIBILITY: Only update model reference, not CurrentRevision
	// Let OSS plugins manage CurrentRevision updates through proper rollout flow
	if foundModel {
		logger.Info("KServe provider found model in InferenceService", "model", model)
		// Store model reference for plugin use but don't set CurrentRevision directly
		// Plugins will handle CurrentRevision updates at appropriate rollout stages
	}

	conditions, found, _ := unstructured.NestedSlice(result.Object, "status", "conditions")
	if found {
		// Only append InferenceService conditions, don't replace plugin conditions
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

			// Check if this InferenceService condition already exists
			conditionExists := false
			for i, existing := range deployment.Status.Conditions {
				if existing.Type == "InferenceService"+typeStr {
					// Update existing InferenceService condition
					deployment.Status.Conditions[i] = &apipb.Condition{
						Type:                 "InferenceService" + typeStr,
						Status:               conditionStatus,
						Message:              message,
						LastUpdatedTimestamp: timestamp.Unix(),
					}
					conditionExists = true
					break
				}
			}

			if !conditionExists {
				// Add new InferenceService condition with prefixed type to avoid conflicts
				deployment.Status.Conditions = append(deployment.Status.Conditions, &apipb.Condition{
					Type:                 "InferenceService" + typeStr,
					Status:               conditionStatus,
					Message:              message,
					LastUpdatedTimestamp: timestamp.Unix(),
				})
			}

			// PLUGIN COMPATIBILITY: Don't override deployment stage/state - let plugins manage rollout flow
			if typeStr == "Ready" {
				if statusStr == "True" {
					logger.Info("InferenceService is ready - plugins will handle rollout completion")
					// Don't set Stage to ROLLOUT_COMPLETE - let OSS plugins manage the rollout flow
					// Only update state if deployment is not already managing its own state
					if deployment.Status.State == v2pb.DEPLOYMENT_STATE_INVALID {
						deployment.Status.State = v2pb.DEPLOYMENT_STATE_HEALTHY
					}
				} else {
					logger.Info("InferenceService is not ready", "message", message)
					// Only update state if deployment is not already managing its own state
					if deployment.Status.State == v2pb.DEPLOYMENT_STATE_INVALID {
						deployment.Status.State = v2pb.DEPLOYMENT_STATE_UNHEALTHY
					}
				}
			}
		}
	} else {
		logger.Info("InferenceService conditions not yet available - plugins will manage initial state")
		// Don't override plugin-managed state - only set if completely uninitialized
		if deployment.Status.State == v2pb.DEPLOYMENT_STATE_INVALID {
			deployment.Status.State = v2pb.DEPLOYMENT_STATE_INITIALIZING
		}
		// Don't override plugin-managed stage
	}

	logger.Info("Updated InferenceService status", "state", deployment.Status.State, "stage", deployment.Status.Stage)
	return nil
}
