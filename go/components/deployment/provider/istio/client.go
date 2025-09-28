package istio

import (
	"context"
	"fmt"
	"strings"

	"github.com/michelangelo-ai/michelangelo/go/api/utils"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"

	"github.com/go-logr/logr"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/provider/proxy"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

type IstioProvider struct {
	DynamicClient dynamic.Interface
}

var _ proxy.ProxyProvider = &IstioProvider{}

func (r IstioProvider) UpdateProxy(ctx context.Context, log logr.Logger, deployment *v2pb.Deployment) error {
	log.Info("Updating proxy routes (HTTPRoute preferred)", "name", deployment.Name, "namespace", deployment.Namespace)

	// Try HTTPRoute first
	httpRouteGvr := schema.GroupVersionResource{
		Group:    "gateway.networking.k8s.io",
		Version:  "v1",
		Resource: "httproutes",
	}

	httpRouteName := fmt.Sprintf("%s-http-route", deployment.Spec.GetInferenceServer().Name)
	httpRoute, err := r.DynamicClient.Resource(httpRouteGvr).Namespace(deployment.Namespace).Get(ctx, httpRouteName, metav1.GetOptions{})
	if err == nil {
		// HTTPRoute found, update it
		err = r.updateHTTPRouteProductionRoute(ctx, log, httpRoute, deployment.Spec.GetInferenceServer().Name, deployment.Name, deployment.Namespace, deployment.Spec.DesiredRevision.Name)
		if err != nil {
			return err
		}
		log.Info("HTTPRoute production route updated successfully")
		return nil
	}

	// Fallback to VirtualService
	log.Info("HTTPRoute not found, falling back to VirtualService", "error", err)

	gvr := schema.GroupVersionResource{
		Group:    "networking.istio.io",
		Version:  "v1beta1",
		Resource: "virtualservices",
	}

	virtualServiceName := fmt.Sprintf("%s-virtualservice", deployment.Spec.GetInferenceServer().Name)
	vs, err := r.DynamicClient.Resource(gvr).Namespace(deployment.Namespace).Get(ctx, virtualServiceName, metav1.GetOptions{})
	if err != nil {
		log.Error(err, "Failed to fetch VirtualService for update")
		return err
	}

	// Update the production route
	err = r.updateProductionRoute(ctx, log, vs, deployment.Spec.GetInferenceServer().Name, deployment.Name, deployment.Namespace, deployment.Spec.DesiredRevision.Name)
	if err != nil {
		return err
	}

	log.Info("VirtualService production route updated successfully")
	return nil
}

func (r IstioProvider) GetProxyStatus(ctx context.Context, logger logr.Logger, deployment *v2pb.Deployment) (string, error) {
	logger.Info("Getting Istio VirtualService status", "name", deployment.Spec.GetInferenceServer().Name, "namespace", deployment.Namespace)

	// Check if VirtualService exists
	gvr := schema.GroupVersionResource{
		Group:    "networking.istio.io",
		Version:  "v1beta1",
		Resource: "virtualservices",
	}

	virtualServiceName := fmt.Sprintf("%s-virtualservice", deployment.Spec.GetInferenceServer().Name)
	vs, err := r.DynamicClient.Resource(gvr).Namespace(deployment.Namespace).Get(ctx, virtualServiceName, metav1.GetOptions{})
	if err != nil {
		if utils.IsNotFoundError(err) {
			logger.Info("VirtualService not found", "name", virtualServiceName)
			return "", nil
		}
		logger.Error(err, "Failed to get VirtualService")
		return "", err
	}

	// Get the current production route model name
	modelName, err := r.getProductionRouteModelName(vs, deployment.Spec.GetInferenceServer().Name, deployment.Name)
	if err != nil {
		logger.Error(err, "Failed to get production route model name")
		return "", err
	}

	if modelName == "" {
		logger.Info("Production route not found in VirtualService", "name", virtualServiceName)
		return "", nil
	}

	logger.Info("VirtualService and production route are properly configured", "name", virtualServiceName, "modelName", modelName)
	return modelName, nil
}

// hasProductionRoute checks if the production route already exists in the VirtualService
func (r IstioProvider) hasProductionRoute(vs *unstructured.Unstructured, name string) (bool, error) {
	httpRoutes, found, err := unstructured.NestedSlice(vs.Object, "spec", "http")
	if err != nil || !found {
		return false, err
	}

	productionPrefix := fmt.Sprintf("/%s-endpoint/%s/production", name, name)

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
				if prefixStr, ok := prefix.(string); ok && prefixStr == productionPrefix {
					return true, nil
				}
			}
		}
	}

	return false, nil
}

// getProductionRouteModelName extracts the model name from the production route's rewrite URI
func (r IstioProvider) getProductionRouteModelName(vs *unstructured.Unstructured, inferenceServerName, deploymentName string) (string, error) {
	httpRoutes, found, err := unstructured.NestedSlice(vs.Object, "spec", "http")
	if err != nil || !found {
		return "", err
	}

	productionPrefix := fmt.Sprintf("/%s-endpoint/%s/production", inferenceServerName, deploymentName)

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
				if prefixStr, ok := prefix.(string); ok && prefixStr == productionPrefix {
					// Found the production route, extract model name from rewrite URI
					rewriteMap, found, _ := unstructured.NestedMap(routeMap, "rewrite")
					if !found {
						return "", nil
					}

					if uri, ok := rewriteMap["uri"]; ok {
						if uriStr, ok := uri.(string); ok {
							// Extract model name from URI pattern /v2/models/<model-name>
							if strings.HasPrefix(uriStr, "/v2/models/") {
								modelName := strings.TrimPrefix(uriStr, "/v2/models/")
								return modelName, nil
							}
						}
					}
					return "", nil
				}
			}
		}
	}

	return "", nil
}

// addProductionRoute adds a production route to an existing VirtualService
func (r IstioProvider) addProductionRoute(ctx context.Context, log logr.Logger, vs *unstructured.Unstructured, name, namespace, modelName string) error {
	httpRoutes, found, err := unstructured.NestedSlice(vs.Object, "spec", "http")
	if err != nil || !found {
		httpRoutes = []interface{}{}
	}

	// Create new production route
	productionRoute := map[string]interface{}{
		"match": []interface{}{
			map[string]interface{}{
				"uri": map[string]interface{}{
					"prefix": fmt.Sprintf("/%s-endpoint/%s/production", name, name),
				},
			},
		},
		"rewrite": map[string]interface{}{
			"uri": fmt.Sprintf("/v2/models/%s", modelName),
		},
		"route": []interface{}{
			map[string]interface{}{
				"destination": map[string]interface{}{
					"host": fmt.Sprintf("%s-inference-service.%s.svc.cluster.local", name, namespace),
					"port": map[string]interface{}{
						"number": int64(80),
					},
				},
			},
		},
	}

	// Add the production route to the beginning of the routes array
	newRoutes := make([]interface{}, 0, len(httpRoutes)+1)
	newRoutes = append(newRoutes, productionRoute)
	newRoutes = append(newRoutes, httpRoutes...)
	httpRoutes = newRoutes

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

	_, err = r.DynamicClient.Resource(gvr).Namespace(namespace).Update(ctx, vs, metav1.UpdateOptions{})
	if err != nil {
		log.Error(err, "Failed to update VirtualService")
		return err
	}

	return nil
}

// updateProductionRoute updates the existing production route with new model name
func (r IstioProvider) updateProductionRoute(ctx context.Context, log logr.Logger, vs *unstructured.Unstructured, inferenceServerName string, deploymentName, namespace, modelName string) error {
	httpRoutes, found, err := unstructured.NestedSlice(vs.Object, "spec", "http")
	if err != nil || !found {
		return fmt.Errorf("http routes not found in VirtualService")
	}

	productionPrefix := fmt.Sprintf("/%s-endpoint/%s/production", inferenceServerName, deploymentName)
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
				if prefixStr, ok := prefix.(string); ok && prefixStr == productionPrefix {
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
		// Production route not found, add it
		log.Info("Production route not found, adding new production route", "modelName", modelName)
		productionRoute := map[string]interface{}{
			"match": []interface{}{
				map[string]interface{}{
					"uri": map[string]interface{}{
						"prefix": productionPrefix,
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

		// Add the production route to the beginning of the routes array
		newRoutes := make([]interface{}, 0, len(httpRoutes)+1)
		newRoutes = append(newRoutes, productionRoute)
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

	_, err = r.DynamicClient.Resource(gvr).Namespace(namespace).Update(ctx, vs, metav1.UpdateOptions{})
	if err != nil {
		log.Error(err, "Failed to update VirtualService")
		return err
	}

	return nil
}

// HTTPRoute functions for Gateway API

// updateHTTPRouteProductionRoute updates the existing production route in HTTPRoute with new model name
func (r IstioProvider) updateHTTPRouteProductionRoute(ctx context.Context, log logr.Logger, httpRoute *unstructured.Unstructured, inferenceServerName string, deploymentName, namespace, modelName string) error {
	rules, found, err := unstructured.NestedSlice(httpRoute.Object, "spec", "rules")
	if err != nil || !found {
		return fmt.Errorf("rules not found in HTTPRoute")
	}

	productionPrefix := fmt.Sprintf("/%s-endpoint/%s/production", inferenceServerName, deploymentName)
	updated := false

	// Look for existing production route
	for _, rule := range rules {
		ruleMap, ok := rule.(map[string]interface{})
		if !ok {
			continue
		}

		if r.hasMatchingHTTPRoutePrefix(ruleMap, productionPrefix) {
			log.Info("Updating existing HTTPRoute production route", "modelName", modelName)

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
		// Production route not found, add it
		log.Info("Production route not found in HTTPRoute, adding new production route", "modelName", modelName)
		productionRule := map[string]interface{}{
			"matches": []map[string]interface{}{
				{
					"path": map[string]interface{}{
						"type":  "PathPrefix",
						"value": productionPrefix,
					},
				},
			},
			"backendRefs": []map[string]interface{}{
				{
					"group":  "",
					"kind":   "Service",
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

		// Add the production rule to the beginning of the rules array
		newRules := make([]interface{}, 0, len(rules)+1)
		newRules = append(newRules, productionRule)
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

	_, err = r.DynamicClient.Resource(gvr).Namespace(namespace).Update(ctx, httpRoute, metav1.UpdateOptions{})
	if err != nil {
		log.Error(err, "Failed to update HTTPRoute")
		return err
	}

	log.Info("HTTPRoute production route updated successfully", "modelName", modelName, "inferenceServerName", inferenceServerName)
	return nil
}

func (r IstioProvider) hasMatchingHTTPRoutePrefix(ruleMap map[string]interface{}, targetPrefix string) bool {
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
