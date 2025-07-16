package config

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/michelangelo-ai/michelangelo/go/shared/gateways"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ConfigProvider manages model configurations and HTTP routes for deployments
// It does NOT create any infrastructure (deployments, services, pods)
// It does NOT trigger model loading (that's handled by the inference server)
type ConfigProvider struct {
	KubeClient    client.Client
	DynamicClient dynamic.Interface
	Gateway       gateways.Gateway
}

// UpdateModelConfig updates the model configuration ConfigMap to point to the new model version
func (c *ConfigProvider) UpdateModelConfig(ctx context.Context, log logr.Logger, deployment *v2pb.Deployment) error {
	inferenceServerName := deployment.Spec.GetInferenceServer().Name
	modelName := deployment.Spec.DesiredRevision.Name
	
	log.Info("Updating model configuration", "inferenceServer", inferenceServerName, "modelName", modelName)

	if c.Gateway == nil {
		return fmt.Errorf("gateway not configured")
	}

	// Extract model path from deployment spec or use default pattern
	modelPath := fmt.Sprintf("s3://deploy-models/%s/", modelName)
	if deployment.Spec.DesiredRevision.ModelPath != "" {
		modelPath = deployment.Spec.DesiredRevision.ModelPath
	}

	// Update model configuration via gateway
	request := gateways.ModelConfigMapRequest{
		InferenceServer: inferenceServerName,
		Namespace:       deployment.Namespace,
		BackendType:     v2pb.BACKEND_TYPE_TRITON, // TODO: Get from deployment spec
		ModelConfigs: []gateways.ModelConfigEntry{
			{
				Name:   modelName,
				S3Path: modelPath,
			},
		},
		Labels: map[string]string{
			"managed-by": "deployment-controller",
			"deployment": deployment.Name,
		},
	}

	err := c.Gateway.UpdateModelConfigMap(ctx, log, request)
	if err != nil {
		return fmt.Errorf("failed to update model config via gateway: %w", err)
	}

	log.Info("Model configuration updated successfully", "inferenceServer", inferenceServerName, "modelName", modelName, "modelPath", modelPath)
	// Note: Model loading will be triggered by the inference server when it detects config changes
	return nil
}

// UpdateHTTPRoute updates the HTTPRoute to route traffic to the new model version
func (c *ConfigProvider) UpdateHTTPRoute(ctx context.Context, log logr.Logger, deployment *v2pb.Deployment) error {
	inferenceServerName := deployment.Spec.GetInferenceServer().Name
	httpRouteName := fmt.Sprintf("%s-http-route", inferenceServerName)
	modelName := deployment.Spec.DesiredRevision.Name
	
	log.Info("Updating HTTPRoute for deployment", "httpRoute", httpRouteName, "modelName", modelName)

	// Try HTTPRoute first (Gateway API)
	httpRouteGvr := schema.GroupVersionResource{
		Group:    "gateway.networking.k8s.io",
		Version:  "v1",
		Resource: "httproutes",
	}

	httpRoute, err := c.DynamicClient.Resource(httpRouteGvr).Namespace(deployment.Namespace).Get(ctx, httpRouteName, metav1.GetOptions{})
	if err == nil {
		// HTTPRoute found, update it
		err = c.updateHTTPRouteForModel(ctx, log, httpRoute, inferenceServerName, deployment.Name, deployment.Namespace, modelName)
		if err != nil {
			return fmt.Errorf("failed to update HTTPRoute: %w", err)
		}
		log.Info("HTTPRoute updated successfully", "httpRoute", httpRouteName, "modelName", modelName)
		return nil
	}

	// Fallback to VirtualService (Istio)
	log.Info("HTTPRoute not found, falling back to VirtualService", "error", err)
	
	virtualServiceGvr := schema.GroupVersionResource{
		Group:    "networking.istio.io",
		Version:  "v1beta1",
		Resource: "virtualservices",
	}

	virtualServiceName := fmt.Sprintf("%s-virtualservice", inferenceServerName)
	vs, err := c.DynamicClient.Resource(virtualServiceGvr).Namespace(deployment.Namespace).Get(ctx, virtualServiceName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get VirtualService %s: %w", virtualServiceName, err)
	}

	// Update the VirtualService
	err = c.updateVirtualServiceForModel(ctx, log, vs, inferenceServerName, deployment.Name, deployment.Namespace, modelName)
	if err != nil {
		return fmt.Errorf("failed to update VirtualService: %w", err)
	}

	log.Info("VirtualService updated successfully", "virtualService", virtualServiceName, "modelName", modelName)
	return nil
}

// updateHTTPRouteForModel updates HTTPRoute rules to route to the specific model
func (c *ConfigProvider) updateHTTPRouteForModel(ctx context.Context, log logr.Logger, httpRoute *unstructured.Unstructured, inferenceServerName, deploymentName, namespace, modelName string) error {
	rules, found, err := unstructured.NestedSlice(httpRoute.Object, "spec", "rules")
	if err != nil || !found {
		return fmt.Errorf("rules not found in HTTPRoute")
	}

	deploymentPrefix := fmt.Sprintf("/%s-endpoint/%s", inferenceServerName, deploymentName)
	updated := false

	// Update existing deployment-specific routes
	for _, rule := range rules {
		ruleMap, ok := rule.(map[string]interface{})
		if !ok {
			continue
		}

		if c.hasMatchingHTTPRoutePrefix(ruleMap, deploymentPrefix) {
			log.Info("Updating existing HTTPRoute deployment route", "modelName", modelName, "prefix", deploymentPrefix)
			
			// Update URLRewrite filter to route to specific model
			filters, found, _ := unstructured.NestedSlice(ruleMap, "filters")
			if found {
				for _, filter := range filters {
					filterMap, ok := filter.(map[string]interface{})
					if !ok {
						continue
					}
					
					if filterType, ok := filterMap["type"]; ok && filterType == "URLRewrite" {
						newPath := fmt.Sprintf("/v2/models/%s", modelName)
						if err = unstructured.SetNestedField(filterMap, newPath, "urlRewrite", "path", "replacePrefixMatch"); err != nil {
							log.Error(err, "Failed to set URLRewrite replacePrefixMatch")
							return err
						}
						break
					}
				}
				
				if err = unstructured.SetNestedField(ruleMap, filters, "filters"); err != nil {
					log.Error(err, "Failed to update filters in HTTPRoute rule")
					return err
				}
			}
			
			updated = true
			break
		}
	}

	if !updated {
		// Deployment route not found, add it
		log.Info("Deployment route not found in HTTPRoute, adding new route", "modelName", modelName, "prefix", deploymentPrefix)
		deploymentRule := map[string]interface{}{
			"matches": []map[string]interface{}{
				{
					"path": map[string]interface{}{
						"type":  "PathPrefix",
						"value": deploymentPrefix,
					},
				},
			},
			"backendRefs": []map[string]interface{}{
				{
					"name":   fmt.Sprintf("%s-inference-service", inferenceServerName),
					"port":   80,
					"weight": 100,
				},
			},
			"filters": []map[string]interface{}{
				{
					"type": "URLRewrite",
					"urlRewrite": map[string]interface{}{
						"path": map[string]interface{}{
							"type":               "ReplacePrefixMatch",
							"replacePrefixMatch": fmt.Sprintf("/v2/models/%s", modelName),
						},
					},
				},
			},
		}

		// Add the deployment rule to the beginning of the rules array (higher priority)
		newRules := make([]interface{}, 0, len(rules)+1)
		newRules = append(newRules, deploymentRule)
		newRules = append(newRules, rules...)
		rules = newRules
	}

	// Update the HTTPRoute
	if err = unstructured.SetNestedField(httpRoute.Object, rules, "spec", "rules"); err != nil {
		log.Error(err, "Failed to update rules in HTTPRoute")
		return err
	}

	gvr := schema.GroupVersionResource{
		Group:    "gateway.networking.k8s.io",
		Version:  "v1",
		Resource: "httproutes",
	}

	_, err = c.DynamicClient.Resource(gvr).Namespace(namespace).Update(ctx, httpRoute, metav1.UpdateOptions{})
	if err != nil {
		log.Error(err, "Failed to update HTTPRoute")
		return err
	}

	return nil
}

// updateVirtualServiceForModel updates VirtualService rules to route to the specific model
func (c *ConfigProvider) updateVirtualServiceForModel(ctx context.Context, log logr.Logger, vs *unstructured.Unstructured, inferenceServerName, deploymentName, namespace, modelName string) error {
	httpRoutes, found, err := unstructured.NestedSlice(vs.Object, "spec", "http")
	if err != nil || !found {
		return fmt.Errorf("http routes not found in VirtualService")
	}

	deploymentPrefix := fmt.Sprintf("/%s-endpoint/%s", inferenceServerName, deploymentName)
	updated := false

	for _, route := range httpRoutes {
		routeMap, ok := route.(map[string]interface{})
		if !ok {
			continue
		}

		matches, found, _ := unstructured.NestedSlice(routeMap, "match")
		if !found {
			continue
		}

		for _, match := range matches {
			matchMap, ok := match.(map[string]interface{})
			if !ok {
				continue
			}

			uriMap, found, _ := unstructured.NestedMap(matchMap, "uri")
			if !found {
				continue
			}

			if prefix, ok := uriMap["prefix"]; ok {
				if prefixStr, ok := prefix.(string); ok && prefixStr == deploymentPrefix {
					// Update the rewrite URI with new model name
					newUri := fmt.Sprintf("/v2/models/%s", modelName)
					if err = unstructured.SetNestedField(routeMap, newUri, "rewrite", "uri"); err != nil {
						log.Error(err, "Failed to set rewrite uri")
						return err
					}
					updated = true
					break
				}
			}
		}
		if updated {
			break
		}
	}

	if !updated {
		// Deployment route not found, add it
		log.Info("Deployment route not found in VirtualService, adding new route", "modelName", modelName, "prefix", deploymentPrefix)
		deploymentRoute := map[string]interface{}{
			"match": []interface{}{
				map[string]interface{}{
					"uri": map[string]interface{}{
						"prefix": deploymentPrefix,
					},
				},
			},
			"rewrite": map[string]interface{}{
				"uri": fmt.Sprintf("/v2/models/%s", modelName),
			},
			"route": []interface{}{
				map[string]interface{}{
					"destination": map[string]interface{}{
						"host": fmt.Sprintf("%s-inference-service.%s.svc.cluster.local", inferenceServerName, namespace),
						"port": map[string]interface{}{
							"number": int64(80),
						},
					},
				},
			},
		}

		// Add the deployment route to the beginning of the routes array (higher priority)
		newRoutes := make([]interface{}, 0, len(httpRoutes)+1)
		newRoutes = append(newRoutes, deploymentRoute)
		newRoutes = append(newRoutes, httpRoutes...)
		httpRoutes = newRoutes
	}

	// Update the VirtualService
	if err = unstructured.SetNestedField(vs.Object, httpRoutes, "spec", "http"); err != nil {
		log.Error(err, "Failed to update http routes in VirtualService")
		return err
	}

	gvr := schema.GroupVersionResource{
		Group:    "networking.istio.io",
		Version:  "v1beta1",
		Resource: "virtualservices",
	}

	_, err = c.DynamicClient.Resource(gvr).Namespace(namespace).Update(ctx, vs, metav1.UpdateOptions{})
	if err != nil {
		log.Error(err, "Failed to update VirtualService")
		return err
	}

	return nil
}

func (c *ConfigProvider) hasMatchingHTTPRoutePrefix(ruleMap map[string]interface{}, targetPrefix string) bool {
	matches, found, _ := unstructured.NestedSlice(ruleMap, "matches")
	if !found {
		return false
	}

	for _, match := range matches {
		matchMap, ok := match.(map[string]interface{})
		if !ok {
			continue
		}

		pathMap, found, _ := unstructured.NestedMap(matchMap, "path")
		if !found {
			continue
		}

		if value, ok := pathMap["value"]; ok {
			if valueStr, ok := value.(string); ok && valueStr == targetPrefix {
				return true
			}
		}
	}

	return false
}

// GetCurrentModelName retrieves the current model name from the route configuration
func (c *ConfigProvider) GetCurrentModelName(ctx context.Context, log logr.Logger, deployment *v2pb.Deployment) (string, error) {
	inferenceServerName := deployment.Spec.GetInferenceServer().Name
	deploymentPrefix := fmt.Sprintf("/%s-endpoint/%s", inferenceServerName, deployment.Name)
	
	// Try HTTPRoute first
	httpRouteGvr := schema.GroupVersionResource{
		Group:    "gateway.networking.k8s.io",
		Version:  "v1",
		Resource: "httproutes",
	}

	httpRouteName := fmt.Sprintf("%s-http-route", inferenceServerName)
	httpRoute, err := c.DynamicClient.Resource(httpRouteGvr).Namespace(deployment.Namespace).Get(ctx, httpRouteName, metav1.GetOptions{})
	if err == nil {
		return c.getModelNameFromHTTPRoute(httpRoute, deploymentPrefix)
	}

	// Fallback to VirtualService
	virtualServiceGvr := schema.GroupVersionResource{
		Group:    "networking.istio.io",
		Version:  "v1beta1",
		Resource: "virtualservices",
	}

	virtualServiceName := fmt.Sprintf("%s-virtualservice", inferenceServerName)
	vs, err := c.DynamicClient.Resource(virtualServiceGvr).Namespace(deployment.Namespace).Get(ctx, virtualServiceName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get routing configuration: %w", err)
	}

	return c.getModelNameFromVirtualService(vs, deploymentPrefix)
}

func (c *ConfigProvider) getModelNameFromHTTPRoute(httpRoute *unstructured.Unstructured, deploymentPrefix string) (string, error) {
	rules, found, err := unstructured.NestedSlice(httpRoute.Object, "spec", "rules")
	if err != nil || !found {
		return "", fmt.Errorf("rules not found in HTTPRoute")
	}

	for _, rule := range rules {
		ruleMap, ok := rule.(map[string]interface{})
		if !ok {
			continue
		}

		if c.hasMatchingHTTPRoutePrefix(ruleMap, deploymentPrefix) {
			filters, found, _ := unstructured.NestedSlice(ruleMap, "filters")
			if found {
				for _, filter := range filters {
					filterMap, ok := filter.(map[string]interface{})
					if !ok {
						continue
					}
					
					if filterType, ok := filterMap["type"]; ok && filterType == "URLRewrite" {
						replacePrefixMatch, found, _ := unstructured.NestedString(filterMap, "urlRewrite", "path", "replacePrefixMatch")
						if found && len(replacePrefixMatch) > 11 && replacePrefixMatch[:11] == "/v2/models/" {
							return replacePrefixMatch[11:], nil
						}
					}
				}
			}
		}
	}

	return "", nil
}

func (c *ConfigProvider) getModelNameFromVirtualService(vs *unstructured.Unstructured, deploymentPrefix string) (string, error) {
	httpRoutes, found, err := unstructured.NestedSlice(vs.Object, "spec", "http")
	if err != nil || !found {
		return "", fmt.Errorf("http routes not found in VirtualService")
	}

	for _, route := range httpRoutes {
		routeMap, ok := route.(map[string]interface{})
		if !ok {
			continue
		}

		matches, found, _ := unstructured.NestedSlice(routeMap, "match")
		if !found {
			continue
		}

		for _, match := range matches {
			matchMap, ok := match.(map[string]interface{})
			if !ok {
				continue
			}

			uriMap, found, _ := unstructured.NestedMap(matchMap, "uri")
			if !found {
				continue
			}

			if prefix, ok := uriMap["prefix"]; ok {
				if prefixStr, ok := prefix.(string); ok && prefixStr == deploymentPrefix {
					rewriteUri, found, _ := unstructured.NestedString(routeMap, "rewrite", "uri")
					if found && len(rewriteUri) > 11 && rewriteUri[:11] == "/v2/models/" {
						return rewriteUri[11:], nil
					}
				}
			}
		}
	}

	return "", nil
}