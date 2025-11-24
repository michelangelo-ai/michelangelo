package cleanup

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/configmap"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/gateways"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

var _ conditionInterfaces.Plugin[*v2pb.Deployment] = &conditionPlugin{}

var httpRouteGVR = schema.GroupVersionKind{
	Group:   "gateway.networking.k8s.io",
	Version: "v1",
	Kind:    "HTTPRoute",
}

type conditionPlugin struct {
	actors []conditionInterfaces.ConditionActor[*v2pb.Deployment]
}

// Params contains dependencies for cleanup plugin
type Params struct {
	Client                 client.Client
	Gateway                gateways.Gateway
	Logger                 *zap.Logger
	ModelConfigMapProvider configmap.ModelConfigMapProvider
}

// NewCleanupPlugin creates a new cleanup plugin following Uber patterns
func NewCleanupPlugin(p Params) conditionInterfaces.Plugin[*v2pb.Deployment] {
	return &conditionPlugin{actors: []conditionInterfaces.ConditionActor[*v2pb.Deployment]{
		&CleanupActor{
			client:                 p.Client,
			gateway:                p.Gateway,
			logger:                 p.Logger,
			modelConfigMapProvider: p.ModelConfigMapProvider,
		},
	}}
}

// GetActors returns all actors for this plugin
func (p *conditionPlugin) GetActors() []conditionInterfaces.ConditionActor[*v2pb.Deployment] {
	return p.actors
}

// GetConditions gets the conditions for a deployment
func (p *conditionPlugin) GetConditions(resource *v2pb.Deployment) []*apipb.Condition {
	return resource.Status.Conditions
}

// PutCondition puts a condition for a deployment
func (p *conditionPlugin) PutCondition(resource *v2pb.Deployment, condition *apipb.Condition) {
	for i, existingCondition := range resource.Status.Conditions {
		if existingCondition.Type == condition.Type {
			resource.Status.Conditions[i] = condition
			return
		}
	}
	resource.Status.Conditions = append(resource.Status.Conditions, condition)
}

// CleanupActor handles cleanup operations following Uber patterns
type CleanupActor struct {
	client                 client.Client
	gateway                gateways.Gateway
	modelConfigMapProvider configmap.ModelConfigMapProvider
	logger                 *zap.Logger
}

func (a *CleanupActor) GetType() string {
	return common.ActorTypeCleanup
}

func (a *CleanupActor) Retrieve(ctx context.Context, deployment *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	// Check if cleanup is needed
	// check if model still exists in ConfigMap
	if exists, err := modelExistsInConfig(
		ctx,
		a.modelConfigMapProvider,
		deployment.Spec.GetInferenceServer().Name,
		deployment.Namespace,
		deployment.Status.CurrentRevision.Name,
	); err != nil {
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_FALSE,
			Reason:  "UnableToCheckModelInConfigMap",
			Message: fmt.Sprintf("Unable to check if model %s exists in ConfigMap: %v", deployment.Status.CurrentRevision.Name, err),
		}, nil
	} else if exists {
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_FALSE,
			Reason:  "ModelStillExistsInConfigMap",
			Message: fmt.Sprintf("Model %s still exists in ConfigMap", deployment.Status.CurrentRevision.Name),
		}, nil
	}

	// Check if HTTPRoute already exists
	var existingRoute unstructured.Unstructured
	existingRoute.SetGroupVersionKind(httpRouteGVR)

	// TODO(GHOSH): check if htttproute still exists for the inference server
	//(DONE, CHECK)
	err := a.client.Get(ctx, client.ObjectKey{
		Name:      fmt.Sprintf("%s-httproute", deployment.Name),
		Namespace: deployment.Namespace,
	}, &existingRoute)
	if err == nil && existingRoute.GetName() == fmt.Sprintf("%s-httproute", deployment.Name) {
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_FALSE,
			Reason:  "HTTPRouteStillExists",
			Message: fmt.Sprintf("Cleanup required: HTTPRoute %s still exists: %v", fmt.Sprintf("%s-httproute", deployment.Name), err),
		}, nil
	}

	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_TRUE,
		Reason:  "CleanupCompleted",
		Message: "Cleanup not required",
	}, nil
}

func (a *CleanupActor) Run(ctx context.Context, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Running cleanup for deployment", zap.String("deployment", resource.Name))

	// Update deployment status to indicate cleanup is in progress
	resource.Status.Stage = v2pb.DEPLOYMENT_STAGE_CLEAN_UP_IN_PROGRESS

	a.logger.Info("Cleaning up model artifacts and ConfigMaps", zap.String("deployment", resource.Name))

	currentModel := resource.Status.CurrentRevision.Name
	inferenceServerName := resource.Spec.GetInferenceServer().Name
	// In Uber's implementation, cleanup involves:
	// 1. Remove model from UCS cache
	// 2. Clean up model artifacts and temporary files
	// 3. Remove ConfigMaps and other Kubernetes resources
	// 4. Update MES (Model Execution Service) records
	// 5. Clean up monitoring and logging configurations

	a.logger.Info("Starting model cleanup",
		zap.String("current_model", currentModel),
		zap.String("inference_server", inferenceServerName))

	// PHASE 1: Update ConfigMap to remove old model (following Uber's UCS pattern)

	// Get current ConfigMap and remove old model from it
	a.logger.Info("Phase 1: Removing old model from ConfigMap", zap.String("old_model", currentModel))

	// Remove old model from ConfigMap
	if err := a.modelConfigMapProvider.RemoveModelFromConfigMap(ctx, configmap.RemoveModelFromConfigMapRequest{
		InferenceServer: inferenceServerName,
		Namespace:       resource.Namespace,
		ModelName:       currentModel,
	}); err != nil {
		a.logger.Error("Failed to remove old model from ConfigMap", zap.String("model", currentModel), zap.Error(err))
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_FALSE,
			Reason:  "ConfigMapCleanupFailed",
			Message: fmt.Sprintf("Failed to remove old model %s from ConfigMap: %v", currentModel, err),
		}, nil
	}

	// PHASE 2: Directly unload old model from Triton using API
	a.logger.Info("Phase 2: Unloading old model from Triton", zap.String("old_model", currentModel))

	if err := a.gateway.UnloadModel(ctx, a.logger, gateways.UnloadModelRequest{
		ModelName:       currentModel,
		InferenceServer: inferenceServerName,
		Namespace:       resource.Namespace,
		BackendType:     v2pb.BACKEND_TYPE_TRITON,
	}); err != nil {
		a.logger.Error("Failed to unload old model from Triton", zap.String("model", currentModel), zap.Error(err))
		// ConfigMap update should eventually unload the model automatically, hence we will not fail the deployment
		a.logger.Info("ConfigMap update should eventually unload the model automatically")
	}

	// PHASE 3: Verify model is unloaded
	a.logger.Info("Phase 3: Verifying old model is unloaded", zap.String("old_model", currentModel))

	statusRequest := gateways.CheckModelStatusRequest{
		ModelName:       currentModel,
		InferenceServer: inferenceServerName,
		Namespace:       resource.Namespace,
		BackendType:     v2pb.BACKEND_TYPE_TRITON,
	}

	ready, err := a.gateway.CheckModelStatus(ctx, a.logger, statusRequest)
	if err == nil && ready {
		a.logger.Info("Old model still loaded, but ConfigMap update should unload it eventually", zap.String("model", currentModel))
	} else {
		a.logger.Info("Old model successfully unloaded", zap.String("model", currentModel))
	}

	// TODO(GHOSH): Cleanup httproutes
	//(DONE, CHECK)
	httpRoute := &unstructured.Unstructured{}
	httpRoute.SetGroupVersionKind(httpRouteGVR)
	httpRoute.SetName(fmt.Sprintf("%s-httproute", resource.Name))
	httpRoute.SetNamespace(resource.Namespace)
	fmt.Printf("DEBUG:Cleaning up httproute %+v\n", httpRoute)
	if err := a.client.Delete(ctx, httpRoute); err != nil {
		a.logger.Error("Failed to delete HTTPRoute", zap.Error(err))
		if errors.IsNotFound(err) {
			a.logger.Info("HTTPRoute not found, already deleted", zap.String("httpRoute", fmt.Sprintf("%s-httproute", resource.Name)))
		} else {
			return &apipb.Condition{
				Type:    a.GetType(),
				Status:  apipb.CONDITION_STATUS_FALSE,
				Reason:  "HTTPRouteCleanupFailed",
				Message: fmt.Sprintf("Failed to delete HTTPRoute %s: %v", fmt.Sprintf("%s-httproute", resource.Name), err),
			}, nil
		}
	}

	a.logger.Info("Model cleanup completed successfully", zap.String("current_model", currentModel))

	// Mark cleanup as complete
	resource.Status.Stage = v2pb.DEPLOYMENT_STAGE_CLEAN_UP_COMPLETE
	a.logger.Info("Cleanup completed for OSS deployment")

	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_TRUE,
		Reason:  "CleanupCompleted",
		Message: "Cleanup completed successfully",
	}, nil
}

func modelExistsInConfig(ctx context.Context, provider configmap.ModelConfigMapProvider, inferenceServerName, namespace, modelName string) (bool, error) {
	currentConfigs, err := provider.GetModelsFromConfigMap(ctx, configmap.GetModelConfigMapRequest{
		InferenceServer: inferenceServerName,
		Namespace:       namespace,
	})
	if err != nil {
		return false, fmt.Errorf("failed to get current model config: %w", err)
	}

	for _, config := range currentConfigs {
		if config.Name == modelName {
			return true, nil
		}
	}
	return false, nil
}
