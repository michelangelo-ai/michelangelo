package gateways

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Gateway API Proxy Management using HTTPRoute

func (g *gateway) configureIstioProxy(ctx context.Context, logger logr.Logger, request ProxyConfigRequest) error {
	logger.Info("Configuring proxy with Gateway API HTTPRoute", "server", request.InferenceServer, "model", request.ModelName)

	// Create or get HTTPRoute
	httpRoute, err := g.getOrCreateHTTPRoute(ctx, logger, request)
	if err != nil {
		return fmt.Errorf("failed to get or create HTTPRoute: %w", err)
	}

	// Update the production route
	err = g.updateHTTPRouteProductionRoute(ctx, logger, httpRoute, request)
	if err != nil {
		return fmt.Errorf("failed to update HTTPRoute production route: %w", err)
	}

	logger.Info("Gateway API proxy configured successfully with HTTPRoute")
	return nil
}

func (g *gateway) getIstioProxyStatus(ctx context.Context, logger logr.Logger, request ProxyStatusRequest) (*ProxyStatus, error) {
	logger.Info("Getting proxy status from HTTPRoute", "server", request.InferenceServer)

	httpRouteGvr := schema.GroupVersionResource{
		Group:    "gateway.networking.k8s.io",
		Version:  "v1",
		Resource: "httproutes",
	}

	httpRouteName := fmt.Sprintf("%s-httproute", request.InferenceServer)
	httpRoute, err := g.dynamicClient.Resource(httpRouteGvr).Namespace(request.Namespace).Get(ctx, httpRouteName, metav1.GetOptions{})
	if err != nil {
		return &ProxyStatus{
			Configured: false,
			Message:    fmt.Sprintf("HTTPRoute not found: %v", err),
		}, nil
	}

	// Extract routes from HTTPRoute
	routes, extractErr := g.extractActiveHTTPRoutes(httpRoute, request.InferenceServer)
	if extractErr != nil {
		return &ProxyStatus{
			Configured: false,
			Message:    fmt.Sprintf("Failed to extract HTTPRoute routes: %v", extractErr),
		}, nil
	}

	return &ProxyStatus{
		Configured: true,
		Routes:     routes,
		Message:    "HTTPRoute is properly configured",
	}, nil
}

func (g *gateway) getOrCreateHTTPRoute(ctx context.Context, logger logr.Logger, request ProxyConfigRequest) (*unstructured.Unstructured, error) {
	gvr := schema.GroupVersionResource{
		Group:    "gateway.networking.k8s.io",
		Version:  "v1",
		Resource: "httproutes",
	}

	httpRouteName := fmt.Sprintf("%s-httproute", request.InferenceServer)

	// Try to get existing HTTPRoute
	httpRoute, err := g.dynamicClient.Resource(gvr).Namespace(request.Namespace).Get(ctx, httpRouteName, metav1.GetOptions{})
	if err == nil {
		return httpRoute, nil
	}

	// Create new HTTPRoute if it doesn't exist
	logger.Info("Creating new HTTPRoute", "name", httpRouteName)

	// Extract environment from namespace or use default
	environment := g.extractEnvironment(request.Namespace)

	httpRoute = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "gateway.networking.k8s.io/v1",
			"kind":       "HTTPRoute",
			"metadata": map[string]interface{}{
				"name":      httpRouteName,
				"namespace": request.Namespace,
				"labels": map[string]interface{}{
					"app":                         "inference-server",
					"inference-server":            request.InferenceServer,
					"environment":                 environment,
					"michelangelo.ai/managed-by": "controller",
				},
			},
			"spec": map[string]interface{}{
				"parentRefs": []map[string]interface{}{
					{
						"group":     "gateway.networking.k8s.io",
						"kind":      "Gateway",
						"name":      "ma-gateway",
						"namespace": request.Namespace,
					},
				},
				"rules": []map[string]interface{}{
					{
						"matches": []map[string]interface{}{
							{
								"path": map[string]interface{}{
									"type":  "PathPrefix",
									"value": fmt.Sprintf("/%s/production", request.InferenceServer),
								},
							},
						},
						"backendRefs": []map[string]interface{}{
							{
								"group":  "",
								"kind":   "Service",
								"name":   fmt.Sprintf("%s-inference-service", request.InferenceServer),
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
										"replacePrefixMatch": "/",
									},
								},
							},
						},
					},
					{
						"matches": []map[string]interface{}{
							{
								"path": map[string]interface{}{
									"type":  "PathPrefix",
									"value": fmt.Sprintf("/%s/staging", request.InferenceServer),
								},
							},
						},
						"backendRefs": []map[string]interface{}{
							{
								"group":  "",
								"kind":   "Service",
								"name":   fmt.Sprintf("%s-inference-service", request.InferenceServer),
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

	return g.dynamicClient.Resource(gvr).Namespace(request.Namespace).Create(ctx, httpRoute, metav1.CreateOptions{})
}

func (g *gateway) updateHTTPRouteProductionRoute(ctx context.Context, logger logr.Logger, httpRoute *unstructured.Unstructured, request ProxyConfigRequest) error {
	rules, found, err := unstructured.NestedSlice(httpRoute.Object, "spec", "rules")
	if err != nil || !found {
		return fmt.Errorf("rules not found in HTTPRoute")
	}

	deploymentPrefix := fmt.Sprintf("/%s/production", request.InferenceServer)
	updated := false

	// Look for existing production route
	for _, rule := range rules {
		ruleMap, ok := rule.(map[string]interface{})
		if !ok {
			continue
		}

		if g.hasMatchingHTTPRoutePrefix(ruleMap, deploymentPrefix) {
			logger.Info("Updating existing HTTPRoute production route", "modelName", request.ModelName)

			// Update URLRewrite filter to route to specific model
			filters, found, _ := unstructured.NestedSlice(ruleMap, "filters")
			if found {
				for _, filter := range filters {
					filterMap, ok := filter.(map[string]interface{})
					if !ok {
						continue
					}

					if filterType, ok := filterMap["type"]; ok && filterType == "URLRewrite" {
						newPath := fmt.Sprintf("/v2/models/%s", request.ModelName)
						if err = unstructured.SetNestedField(filterMap, newPath, "urlRewrite", "path", "replacePrefixMatch"); err != nil {
							logger.Error(err, "Failed to set URLRewrite replacePrefixMatch")
							return err
						}
						break
					}
				}

				if err = unstructured.SetNestedField(ruleMap, filters, "filters"); err != nil {
					logger.Error(err, "Failed to update filters in HTTPRoute rule")
					return err
				}
			}

			updated = true
			break
		}
	}

	if !updated {
		logger.Info("Production route not found, checking if HTTPRoute already points to desired model",
			"inferenceServer", request.InferenceServer, "desiredModel", request.ModelName)

		// Check if the production route already points to the desired model
		if g.isHTTPRouteAlreadyConfiguredForModel(httpRoute, request) {
			logger.Info("HTTPRoute already configured for desired model, skipping update",
				"modelName", request.ModelName)
			return nil
		}

		return fmt.Errorf("production route not found in HTTPRoute for inference server: %s", request.InferenceServer)
	}

	// Update the HTTPRoute
	if err = unstructured.SetNestedField(httpRoute.Object, rules, "spec", "rules"); err != nil {
		logger.Error(err, "Failed to update rules in HTTPRoute")
		return err
	}

	gvr := schema.GroupVersionResource{
		Group:    "gateway.networking.k8s.io",
		Version:  "v1",
		Resource: "httproutes",
	}

	_, err = g.dynamicClient.Resource(gvr).Namespace(request.Namespace).Update(ctx, httpRoute, metav1.UpdateOptions{})
	if err != nil {
		logger.Error(err, "Failed to update HTTPRoute")
		return err
	}

	logger.Info("HTTPRoute production route updated successfully", "modelName", request.ModelName, "inferenceServer", request.InferenceServer)
	return nil
}

func (g *gateway) hasMatchingHTTPRoutePrefix(ruleMap map[string]interface{}, targetPrefix string) bool {
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

func (g *gateway) extractActiveHTTPRoutes(httpRoute *unstructured.Unstructured, inferenceServer string) ([]ActiveRoute, error) {
	rules, found, err := unstructured.NestedSlice(httpRoute.Object, "spec", "rules")
	if err != nil || !found {
		return []ActiveRoute{}, nil
	}

	var routes []ActiveRoute

	for _, rule := range rules {
		ruleMap, ok := rule.(map[string]interface{})
		if !ok {
			continue
		}

		// Extract match information
		matches, found, _ := unstructured.NestedSlice(ruleMap, "matches")
		if !found {
			continue
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

			var path string
			if value, ok := pathMap["value"]; ok {
				if valueStr, ok := value.(string); ok {
					path = valueStr
				}
			}

			// Extract rewrite information from filters
			var rewrite string
			if filters, found, _ := unstructured.NestedSlice(ruleMap, "filters"); found {
				for _, filter := range filters {
					filterMap, ok := filter.(map[string]interface{})
					if !ok {
						continue
					}

					if filterType, ok := filterMap["type"]; ok && filterType == "URLRewrite" {
						if urlRewriteMap, found, _ := unstructured.NestedMap(filterMap, "urlRewrite"); found {
							if pathMap, found, _ := unstructured.NestedMap(urlRewriteMap, "path"); found {
								if replacePrefixMatch, ok := pathMap["replacePrefixMatch"]; ok {
									if rewriteStr, ok := replacePrefixMatch.(string); ok {
										rewrite = rewriteStr
									}
								}
							}
						}
						break
					}
				}
			}

			// Extract destination information from backendRefs
			var destination string
			if backendRefs, found, _ := unstructured.NestedSlice(ruleMap, "backendRefs"); found {
				for _, ref := range backendRefs {
					if refMap, ok := ref.(map[string]interface{}); ok {
						if name, ok := refMap["name"]; ok {
							if nameStr, ok := name.(string); ok {
								destination = nameStr
							}
						}
					}
				}
			}

			if path != "" {
				routes = append(routes, ActiveRoute{
					Path:        path,
					Destination: destination,
					Rewrite:     rewrite,
					Active:      strings.Contains(path, inferenceServer),
				})
			}
		}
	}

	return routes, nil
}

// isHTTPRouteAlreadyConfiguredForModel checks if the HTTPRoute already points to the desired model
func (g *gateway) isHTTPRouteAlreadyConfiguredForModel(httpRoute *unstructured.Unstructured, request ProxyConfigRequest) bool {
	rules, found, err := unstructured.NestedSlice(httpRoute.Object, "spec", "rules")
	if err != nil || !found {
		return false
	}

	deploymentPrefix := fmt.Sprintf("/%s/production", request.InferenceServer)
	expectedRewrite := fmt.Sprintf("/v2/models/%s", request.ModelName)

	for _, rule := range rules {
		ruleMap, ok := rule.(map[string]interface{})
		if !ok {
			continue
		}

		if g.hasMatchingHTTPRoutePrefix(ruleMap, deploymentPrefix) {
			// Check if this production route already points to the desired model
			filters, found, _ := unstructured.NestedSlice(ruleMap, "filters")
			if !found {
				continue
			}

			for _, filter := range filters {
				filterMap, ok := filter.(map[string]interface{})
				if !ok {
					continue
				}

				if filterType, ok := filterMap["type"]; ok && filterType == "URLRewrite" {
					if urlRewriteMap, found, _ := unstructured.NestedMap(filterMap, "urlRewrite"); found {
						if pathMap, found, _ := unstructured.NestedMap(urlRewriteMap, "path"); found {
							if replacePrefixMatch, ok := pathMap["replacePrefixMatch"]; ok {
								if rewriteStr, ok := replacePrefixMatch.(string); ok {
									return rewriteStr == expectedRewrite
								}
							}
						}
					}
					break
				}
			}
		}
	}

	return false
}

// extractEnvironment derives environment from namespace or other context
func (g *gateway) extractEnvironment(namespace string) string {
	// Default mapping - can be enhanced based on your naming conventions
	switch {
	case strings.Contains(namespace, "prod"):
		return "production"
	case strings.Contains(namespace, "staging"):
		return "staging"
	case strings.Contains(namespace, "dev"):
		return "development"
	default:
		return "production" // Default to production
	}
}