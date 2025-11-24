package rollout

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/gateways"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// TrafficRoutingActor handles HTTPRoute management for deployment traffic routing
type TrafficRoutingActor struct {
	client  client.Client
	gateway gateways.Gateway
	logger  *zap.Logger
}

func (a *TrafficRoutingActor) GetType() string {
	return common.ActorTypeTrafficRouting
}

func (a *TrafficRoutingActor) GetLogger() *zap.Logger {
	return a.logger
}

func (a *TrafficRoutingActor) Retrieve(ctx context.Context, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	// Check if deployment HTTPRoute exists and is properly configured
	deploymentRouteName := fmt.Sprintf("%s-httproute", resource.Name)

	// Use gateway to check HTTPRoute via dynamic client
	var httpRoute unstructured.Unstructured
	httpRoute.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "gateway.networking.k8s.io",
		Version: "v1",
		Kind:    "HTTPRoute",
	})

	err := a.client.Get(ctx, client.ObjectKey{
		Name:      deploymentRouteName,
		Namespace: resource.Namespace,
	}, &httpRoute)
	if err != nil {
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_FALSE,
			Reason:  "HTTPRouteNotFound",
			Message: fmt.Sprintf("HTTPRoute %s not found for deployment", deploymentRouteName),
		}, nil
	}

	// Validate HTTPRoute configuration
	spec, found, err := unstructured.NestedMap(httpRoute.Object, "spec")
	if err != nil || !found {
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_FALSE,
			Reason:  "HTTPRouteInvalid",
			Message: "HTTPRoute spec not found",
		}, nil
	}

	rules, found, err := unstructured.NestedSlice(spec, "rules")
	if err != nil || !found || len(rules) == 0 {
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_FALSE,
			Reason:  "HTTPRouteInvalid",
			Message: "HTTPRoute has no routing rules configured",
		}, nil
	}

	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_TRUE,
		Reason:  "TrafficRoutingConfigured",
		Message: fmt.Sprintf("HTTPRoute %s successfully configured for deployment", deploymentRouteName),
	}, nil
}

func (a *TrafficRoutingActor) Run(ctx context.Context, deployment *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Running traffic routing configuration for deployment", zap.String("deployment", deployment.Name))

	if deployment.Spec.GetInferenceServer() == nil {
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_FALSE,
			Reason:  "MissingInferenceServer",
			Message: fmt.Sprintf("inference server not specified for deployment %s", deployment.Name),
		}, nil
	}

	deploymentRouteName := fmt.Sprintf("%s-httproute", deployment.Name)
	inferenceServerName := deployment.Spec.GetInferenceServer().Name

	// Create HTTPRoute as unstructured object following existing pattern
	httpRoute := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "gateway.networking.k8s.io/v1",
			"kind":       "HTTPRoute",
			"metadata": map[string]interface{}{
				"name":      deploymentRouteName,
				"namespace": deployment.Namespace,
				"labels": map[string]interface{}{
					"app.kubernetes.io/name":      "michelangelo-deployment",
					"app.kubernetes.io/component": "traffic-routing",
					"app.kubernetes.io/instance":  deployment.Name,
					"michelangelo.ai/deployment":  deployment.Name,
				},
				"annotations": map[string]interface{}{
					"michelangelo.ai/deployment":       deployment.Name,
					"michelangelo.ai/inference-server": inferenceServerName,
				},
			},
			"spec": map[string]interface{}{
				"parentRefs": []interface{}{
					map[string]interface{}{
						"name": "ma-gateway",
						"kind": "Gateway",
					},
				},
				"rules": []interface{}{
					map[string]interface{}{
						"matches": []interface{}{
							map[string]interface{}{
								"path": map[string]interface{}{
									"type":  "PathPrefix",
									"value": fmt.Sprintf("/%s/%s", inferenceServerName, deployment.Name),
								},
							},
						},
						"backendRefs": []interface{}{
							map[string]interface{}{
								"name": fmt.Sprintf("%s-inference-service", inferenceServerName),
								"port": 80,
							},
						},
						"filters": []interface{}{
							map[string]interface{}{
								"type": "URLRewrite",
								"urlRewrite": map[string]interface{}{
									"path": map[string]interface{}{
										"type":               "ReplacePrefixMatch",
										"replacePrefixMatch": "/",
									},
								},
							},
						},
					},
				},
			},
		},
	}

	// Check if HTTPRoute already exists
	var existingRoute unstructured.Unstructured
	existingRoute.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "gateway.networking.k8s.io",
		Version: "v1",
		Kind:    "HTTPRoute",
	})

	err := a.client.Get(ctx, client.ObjectKey{
		Name:      deploymentRouteName,
		Namespace: deployment.Namespace,
	}, &existingRoute)

	if err != nil {
		// Create new HTTPRoute
		if err := a.client.Create(ctx, httpRoute); err != nil {
			return &apipb.Condition{
				Type:    a.GetType(),
				Status:  apipb.CONDITION_STATUS_FALSE,
				Reason:  "HTTPRouteCreationFailed",
				Message: fmt.Sprintf("failed to create HTTPRoute %s: %v", deploymentRouteName, err),
			}, nil
		}
		a.logger.Info("Created HTTPRoute for deployment",
			zap.String("httproute", deploymentRouteName),
			zap.String("deployment", deployment.Name),
			zap.String("path", fmt.Sprintf("/%s/%s", inferenceServerName, deployment.Name)))
	} else {
		// Update existing HTTPRoute spec
		existingRoute.Object["spec"] = httpRoute.Object["spec"]
		if err := a.client.Update(ctx, &existingRoute); err != nil {
			return &apipb.Condition{
				Type:    a.GetType(),
				Status:  apipb.CONDITION_STATUS_FALSE,
				Reason:  "HTTPRouteUpdateFailed",
				Message: fmt.Sprintf("failed to update HTTPRoute %s: %v", deploymentRouteName, err),
			}, nil
		}
		a.logger.Info("Updated HTTPRoute for deployment",
			zap.String("httproute", deploymentRouteName),
			zap.String("deployment", deployment.Name),
			zap.String("path", fmt.Sprintf("/%s/%s", inferenceServerName, deployment.Name)),
		)
	}

	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_TRUE,
		Reason:  "TrafficRoutingConfigured",
		Message: fmt.Sprintf("HTTPRoute %s successfully configured for deployment", deploymentRouteName),
	}, nil
}
