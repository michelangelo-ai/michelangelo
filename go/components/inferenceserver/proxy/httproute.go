package proxy

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

const (
	gatewayAPIGroup   = "gateway.networking.k8s.io"
	gatewayAPIVersion = "v1"
	httpRouteResource = "httproutes"
	gatewayKind       = "Gateway"

	gatewayName            = "ma-gateway"
	httpRouteNameSuffix    = "httproute"
	inferenceServiceSuffix = "inference-service"
)

var (
	httpRouteGVR = schema.GroupVersionResource{
		Group:    gatewayAPIGroup,
		Version:  gatewayAPIVersion,
		Resource: httpRouteResource,
	}

	addSuffixToString = func(str, suffix string) string {
		return fmt.Sprintf("%s-%s", str, suffix)
	}
)

var _ ProxyProvider = &httpRouteManager{} // Ensure httpRouteManager implements ProxyProvider interface

type httpRouteManager struct {
	dynamicClient dynamic.Interface
	logger        *zap.Logger
}

func NewHTTPRouteManager(dynamicClient dynamic.Interface, logger *zap.Logger) *httpRouteManager {
	return &httpRouteManager{
		dynamicClient: dynamicClient,
		logger:        logger,
	}
}

// Gateway API Proxy Management using HTTPRoute

func (h *httpRouteManager) ConfigureProxy(ctx context.Context, logger *zap.Logger, request ConfigureProxyRequest) error {
	logger.Info("Configuring proxy with Gateway API HTTPRoute", zap.String("server", request.InferenceServer), zap.String("model", request.ModelName))
	httpRoute, err := h.getOrCreateHTTPRoute(ctx, logger, request)
	if err != nil {
		logger.Error("failed to get or create HTTPRoute",
			zap.Error(err),
			zap.String("operation", "configure_proxy"),
			zap.String("namespace", request.Namespace),
			zap.String("inferenceServer", request.InferenceServer))
		return fmt.Errorf("failed to get or create HTTPRoute for %s/%s: %w",
			request.Namespace, request.InferenceServer, err)
	}
	return h.updateProductionRoute(ctx, logger, httpRoute, request)
}

func (h *httpRouteManager) getOrCreateHTTPRoute(ctx context.Context, logger *zap.Logger, request ConfigureProxyRequest) (*unstructured.Unstructured, error) {
	httpRouteName := addSuffixToString(request.InferenceServer, httpRouteNameSuffix)

	// Try to get existing HTTPRoute
	httpRoute, err := h.dynamicClient.Resource(httpRouteGVR).Namespace(request.Namespace).Get(ctx, httpRouteName, metav1.GetOptions{})
	if err == nil {
		return httpRoute, nil
	}

	// Create new HTTPRoute if it doesn't exist
	logger.Info("Creating new HTTPRoute with baseline routing", zap.String("name", httpRouteName))

	// Extract environment from namespace or use default
	environment := h.extractEnvironment(request.Namespace)

	httpRoute = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": fmt.Sprintf("%s/%s", gatewayAPIGroup, gatewayAPIVersion),
			"kind":       "HTTPRoute",
			"metadata": map[string]interface{}{
				"name":      httpRouteName,
				"namespace": request.Namespace,
				"labels": map[string]interface{}{
					"app":                        "inference-server",
					"inference-server":           request.InferenceServer,
					"environment":                environment,
					"michelangelo.ai/managed-by": "controller",
				},
			},
			"spec": map[string]interface{}{
				"parentRefs": []interface{}{
					map[string]interface{}{
						"group":     gatewayAPIGroup,
						"kind":      gatewayKind,
						"name":      gatewayName,
						"namespace": request.Namespace,
					},
				},
				"rules": []interface{}{
					map[string]interface{}{
						// Baseline inference server endpoint - routes to whatever model is loaded in Triton
						"matches": []interface{}{
							map[string]interface{}{
								"path": map[string]interface{}{
									"type":  "PathPrefix",
									"value": fmt.Sprintf("/%s", request.InferenceServer),
								},
							},
						},
						"backendRefs": []interface{}{
							map[string]interface{}{
								"group":  "",
								"kind":   "Service",
								"name":   addSuffixToString(request.InferenceServer, inferenceServiceSuffix),
								"port":   int64(80),
								"weight": int64(100),
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

	createdRoute, err := h.dynamicClient.Resource(httpRouteGVR).Namespace(request.Namespace).Create(ctx, httpRoute, metav1.CreateOptions{})
	if err != nil {
		logger.Error("failed to create HTTPRoute",
			zap.Error(err),
			zap.String("operation", "get_or_create_httproute"),
			zap.String("namespace", request.Namespace),
			zap.String("inferenceServer", request.InferenceServer))
		return nil, fmt.Errorf("failed to create HTTPRoute %s/%s: %w",
			request.Namespace, httpRouteName, err)
	}
	return createdRoute, nil
}

func (h *httpRouteManager) updateProductionRoute(ctx context.Context, logger *zap.Logger, httpRoute *unstructured.Unstructured, request ConfigureProxyRequest) error {
	rules, found, err := unstructured.NestedSlice(httpRoute.Object, "spec", "rules")
	if err != nil || !found {
		logger.Error("rules not found in HTTPRoute",
			zap.Error(err),
			zap.String("operation", "update_production_route"),
			zap.String("namespace", request.Namespace),
			zap.String("inferenceServer", request.InferenceServer))
		return fmt.Errorf("rules not found in HTTPRoute %s/%s: %w",
			request.Namespace, request.InferenceServer, err)
	}

	deploymentPrefix := fmt.Sprintf("/%s/%s/production", request.InferenceServer, request.DeploymentName)
	updated := false

	// Look for existing production route
	for _, rule := range rules {
		ruleMap, ok := rule.(map[string]interface{})
		if !ok {
			continue
		}

		if h.hasMatchingHTTPRoutePrefix(ruleMap, deploymentPrefix) {
			logger.Info("Updating existing HTTPRoute production route", zap.String("modelName", request.ModelName))

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
							logger.Error("failed to set URLRewrite replacePrefixMatch",
								zap.Error(err),
								zap.String("operation", "update_production_route"),
								zap.String("namespace", request.Namespace),
								zap.String("inferenceServer", request.InferenceServer))
							return fmt.Errorf("failed to set URLRewrite for HTTPRoute %s/%s: %w",
								request.Namespace, request.InferenceServer, err)
						}
						break
					}
				}

				if err = unstructured.SetNestedField(ruleMap, filters, "filters"); err != nil {
					logger.Error("failed to update filters in HTTPRoute rule",
						zap.Error(err),
						zap.String("operation", "update_production_route"),
						zap.String("namespace", request.Namespace),
						zap.String("inferenceServer", request.InferenceServer))
					return fmt.Errorf("failed to update filters in HTTPRoute %s/%s: %w",
						request.Namespace, request.InferenceServer, err)
				}
			}

			updated = true
			break
		}
	}

	if !updated {
		logger.Info("Production route not found, checking if HTTPRoute already points to desired model",
			zap.String("inferenceServer", request.InferenceServer), zap.String("desiredModel", request.ModelName))

		// Check if the production route already points to the desired model
		if h.isHTTPRouteAlreadyConfiguredForModel(httpRoute, request) {
			logger.Info("HTTPRoute already configured for desired model, skipping update",
				zap.String("modelName", request.ModelName))
			return nil
		}

		logger.Error("production route not found in HTTPRoute",
			zap.String("operation", "update_production_route"),
			zap.String("namespace", request.Namespace),
			zap.String("inferenceServer", request.InferenceServer),
			zap.String("deploymentName", request.DeploymentName))
		return fmt.Errorf("production route not found in HTTPRoute %s/%s for deployment %s",
			request.Namespace, request.InferenceServer, request.DeploymentName)
	}

	// Update the HTTPRoute
	if err = unstructured.SetNestedSlice(httpRoute.Object, rules, "spec", "rules"); err != nil {
		logger.Error("failed to update rules in HTTPRoute",
			zap.Error(err),
			zap.String("operation", "update_production_route"),
			zap.String("namespace", request.Namespace),
			zap.String("inferenceServer", request.InferenceServer))
		return fmt.Errorf("failed to update rules in HTTPRoute %s/%s: %w",
			request.Namespace, request.InferenceServer, err)
	}

	_, err = h.dynamicClient.Resource(httpRouteGVR).Namespace(request.Namespace).Update(ctx, httpRoute, metav1.UpdateOptions{})
	if err != nil {
		logger.Error("failed to update HTTPRoute",
			zap.Error(err),
			zap.String("operation", "update_production_route"),
			zap.String("namespace", request.Namespace),
			zap.String("inferenceServer", request.InferenceServer))
		return fmt.Errorf("failed to update HTTPRoute %s/%s: %w",
			request.Namespace, request.InferenceServer, err)
	}

	logger.Info("HTTPRoute production route updated successfully", zap.String("modelName", request.ModelName), zap.String("inferenceServer", request.InferenceServer))
	return nil
}

func (h *httpRouteManager) GetProxyStatus(ctx context.Context, logger *zap.Logger, request GetProxyStatusRequest) (*GetProxyStatusResponse, error) {
	logger.Info("Getting proxy status from HTTPRoute", zap.String("server", request.InferenceServer))

	httpRouteName := addSuffixToString(request.InferenceServer, httpRouteNameSuffix)
	httpRoute, err := h.dynamicClient.Resource(httpRouteGVR).Namespace(request.Namespace).Get(ctx, httpRouteName, metav1.GetOptions{})
	if err != nil {
		return &GetProxyStatusResponse{
			Status: ProxyStatus{
				Configured: false,
				Message:    fmt.Sprintf("HTTPRoute not found: %v", err),
			},
		}, nil
	}

	// Extract routes from HTTPRoute
	routes, extractErr := h.extractActiveHTTPRoutes(httpRoute, request.InferenceServer)
	if extractErr != nil {
		return &GetProxyStatusResponse{
			Status: ProxyStatus{
				Configured: false,
				Message:    fmt.Sprintf("Failed to extract HTTPRoute routes: %v", extractErr),
			},
		}, nil
	}
	return &GetProxyStatusResponse{
		Status: ProxyStatus{
			Configured: true,
			Routes:     routes,
			Message:    "HTTPRoute is properly configured",
		},
	}, nil
}

// addDeploymentRoute adds a deployment-specific route to the HTTPRoute
func (h *httpRouteManager) AddDeploymentRoute(ctx context.Context, logger *zap.Logger, request AddDeploymentRouteRequest) error {
	httpRouteName := addSuffixToString(request.InferenceServer, httpRouteNameSuffix)
	fmt.Printf("DEBUG: Adding deployment route for %s/%s\n", request.InferenceServer, request.DeploymentName)
	httpRoute, err := h.dynamicClient.Resource(httpRouteGVR).Namespace(request.Namespace).Get(ctx, httpRouteName, metav1.GetOptions{})
	if err != nil {
		logger.Error("failed to get HTTPRoute",
			zap.Error(err),
			zap.String("operation", "add_deployment_route"),
			zap.String("namespace", request.Namespace),
			zap.String("inferenceServer", request.InferenceServer))
		return fmt.Errorf("failed to get HTTPRoute %s/%s: %w",
			request.Namespace, httpRouteName, err)
	}

	rules, found, err := unstructured.NestedSlice(httpRoute.Object, "spec", "rules")
	if err != nil || !found {
		logger.Error("rules not found in HTTPRoute",
			zap.Error(err),
			zap.String("operation", "add_deployment_route"),
			zap.String("namespace", request.Namespace),
			zap.String("inferenceServer", request.InferenceServer))
		return fmt.Errorf("rules not found in HTTPRoute %s/%s: %w",
			request.Namespace, httpRouteName, err)
	}

	// Create deployment-specific route path: /<inference-server-name>/<deployment-name>
	deploymentPath := fmt.Sprintf("/%s/%s", request.InferenceServer, request.DeploymentName)

	// Check if deployment-specific route already exists
	for _, rule := range rules {
		ruleMap, ok := rule.(map[string]interface{})
		if !ok {
			continue
		}

		if h.hasMatchingHTTPRoutePrefix(ruleMap, deploymentPath) {
			logger.Info("Deployment-specific route already exists, updating",
				zap.String("deploymentPath", deploymentPath), zap.String("modelName", request.ModelName))

			// Update existing route to point to new model
			return h.updateExistingDeploymentRoute(ctx, logger, httpRoute, request, deploymentPath)
		}
	}

	// Add new deployment-specific route
	logger.Info("Adding new deployment-specific route",
		zap.String("deploymentPath", deploymentPath), zap.String("modelName", request.ModelName))

	newRule := map[string]interface{}{
		"matches": []interface{}{
			map[string]interface{}{
				"path": map[string]interface{}{
					"type":  "PathPrefix",
					"value": deploymentPath,
				},
			},
		},
		"backendRefs": []interface{}{
			map[string]interface{}{
				"group":  "",
				"kind":   "Service",
				"name":   addSuffixToString(request.InferenceServer, inferenceServiceSuffix),
				"port":   int64(80),
				"weight": int64(100),
			},
		},
		"filters": []interface{}{
			map[string]interface{}{
				"type": "URLRewrite",
				"urlRewrite": map[string]interface{}{
					"path": map[string]interface{}{
						"type":               "ReplacePrefixMatch",
						"replacePrefixMatch": fmt.Sprintf("/v2/models/%s", request.ModelName),
					},
				},
			},
		},
	}

	// Prepend the new rule to ensure it matches before the baseline route
	updatedRules := make([]interface{}, 0, len(rules)+1)
	updatedRules = append(updatedRules, newRule)
	for _, rule := range rules {
		updatedRules = append(updatedRules, rule)
	}

	// Update the HTTPRoute using SetNestedField to avoid deep copy issues
	err = unstructured.SetNestedField(httpRoute.Object, updatedRules, "spec", "rules")
	if err != nil {
		logger.Error("failed to set rules in HTTPRoute spec",
			zap.Error(err),
			zap.String("operation", "add_deployment_route"),
			zap.String("namespace", request.Namespace),
			zap.String("inferenceServer", request.InferenceServer))
		return fmt.Errorf("failed to set rules in HTTPRoute %s/%s: %w",
			request.Namespace, httpRouteName, err)
	}

	_, err = h.dynamicClient.Resource(httpRouteGVR).Namespace(request.Namespace).Update(ctx, httpRoute, metav1.UpdateOptions{})
	if err != nil {
		logger.Error("failed to update HTTPRoute",
			zap.Error(err),
			zap.String("operation", "add_deployment_route"),
			zap.String("namespace", request.Namespace),
			zap.String("inferenceServer", request.InferenceServer))
		return fmt.Errorf("failed to update HTTPRoute %s/%s: %w",
			request.Namespace, httpRouteName, err)
	}

	logger.Info("Deployment-specific route added successfully",
		zap.String("deploymentPath", deploymentPath), zap.String("modelName", request.ModelName))
	return nil
}

func (h *httpRouteManager) DeleteRoute(ctx context.Context, logger *zap.Logger, request DeleteRouteRequest) error {
	httpRouteName := addSuffixToString(request.InferenceServer, httpRouteNameSuffix)
	if err := h.dynamicClient.Resource(httpRouteGVR).Namespace(request.Namespace).Delete(ctx, httpRouteName, metav1.DeleteOptions{}); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("HTTPRoute not found, already deleted", zap.String("httpRoute", httpRouteName))
		} else {
			logger.Error("failed to delete HTTPRoute",
				zap.Error(err),
				zap.String("operation", "delete_route"),
				zap.String("namespace", request.Namespace),
				zap.String("httpRoute", httpRouteName))
			return fmt.Errorf("failed to delete HTTPRoute %s/%s: %w",
				request.Namespace, httpRouteName, err)
		}
	}
	return nil
}

// updateExistingDeploymentRoute updates an existing deployment-specific route
func (h *httpRouteManager) updateExistingDeploymentRoute(ctx context.Context, logger *zap.Logger, httpRoute *unstructured.Unstructured, request AddDeploymentRouteRequest, deploymentPath string) error {
	rules, found, err := unstructured.NestedSlice(httpRoute.Object, "spec", "rules")
	if err != nil || !found {
		logger.Error("rules not found in HTTPRoute",
			zap.Error(err),
			zap.String("operation", "update_deployment_route"),
			zap.String("namespace", request.Namespace),
			zap.String("inferenceServer", request.InferenceServer),
			zap.String("deploymentPath", deploymentPath))
		return fmt.Errorf("rules not found in HTTPRoute %s/%s: %w",
			request.Namespace, request.InferenceServer, err)
	}

	for _, rule := range rules {
		ruleMap, ok := rule.(map[string]interface{})
		if !ok {
			continue
		}

		if h.hasMatchingHTTPRoutePrefix(ruleMap, deploymentPath) {
			// Update URLRewrite filter to route to new model
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
							logger.Error("failed to set URLRewrite replacePrefixMatch",
								zap.Error(err),
								zap.String("operation", "update_deployment_route"),
								zap.String("namespace", request.Namespace),
								zap.String("inferenceServer", request.InferenceServer),
								zap.String("deploymentPath", deploymentPath),
								zap.String("modelName", request.ModelName))
							return fmt.Errorf("failed to set URLRewrite replacePrefixMatch for HTTPRoute %s/%s: %w",
								request.Namespace, request.InferenceServer, err)
						}
						break
					}
				}

				if err = unstructured.SetNestedField(ruleMap, filters, "filters"); err != nil {
					logger.Error("failed to update filters in HTTPRoute rule",
						zap.Error(err),
						zap.String("operation", "update_deployment_route"),
						zap.String("namespace", request.Namespace),
						zap.String("inferenceServer", request.InferenceServer),
						zap.String("deploymentPath", deploymentPath))
					return fmt.Errorf("failed to update filters in HTTPRoute %s/%s: %w",
						request.Namespace, request.InferenceServer, err)
				}
			}
			break
		}
	}

	// Update the HTTPRoute
	if err = unstructured.SetNestedSlice(httpRoute.Object, rules, "spec", "rules"); err != nil {
		logger.Error("failed to update rules in HTTPRoute",
			zap.Error(err),
			zap.String("operation", "update_deployment_route"),
			zap.String("namespace", request.Namespace),
			zap.String("inferenceServer", request.InferenceServer),
			zap.String("deploymentPath", deploymentPath))
		return fmt.Errorf("failed to update rules in HTTPRoute %s/%s: %w",
			request.Namespace, request.InferenceServer, err)
	}

	_, err = h.dynamicClient.Resource(httpRouteGVR).Namespace(request.Namespace).Update(ctx, httpRoute, metav1.UpdateOptions{})
	if err != nil {
		logger.Error("failed to update HTTPRoute",
			zap.Error(err),
			zap.String("operation", "update_deployment_route"),
			zap.String("namespace", request.Namespace),
			zap.String("inferenceServer", request.InferenceServer),
			zap.String("deploymentPath", deploymentPath),
			zap.String("modelName", request.ModelName))
		return fmt.Errorf("failed to update HTTPRoute %s/%s: %w",
			request.Namespace, request.InferenceServer, err)
	}

	logger.Info("Deployment-specific route updated successfully",
		zap.String("deploymentPath", deploymentPath), zap.String("modelName", request.ModelName))
	return nil
}

// isHTTPRouteAlreadyConfiguredForModel checks if the HTTPRoute already points to the desired model
func (h *httpRouteManager) isHTTPRouteAlreadyConfiguredForModel(httpRoute *unstructured.Unstructured, request ConfigureProxyRequest) bool {
	rules, found, err := unstructured.NestedSlice(httpRoute.Object, "spec", "rules")
	if err != nil || !found {
		return false
	}

	deploymentPrefix := fmt.Sprintf("/%s/%s/production", request.InferenceServer, request.DeploymentName)
	expectedRewrite := fmt.Sprintf("/v2/models/%s", request.ModelName)

	for _, rule := range rules {
		ruleMap, ok := rule.(map[string]interface{})
		if !ok {
			continue
		}

		if h.hasMatchingHTTPRoutePrefix(ruleMap, deploymentPrefix) {
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

func (h *httpRouteManager) extractActiveHTTPRoutes(httpRoute *unstructured.Unstructured, inferenceServer string) ([]ActiveRoute, error) {
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

func (h *httpRouteManager) hasMatchingHTTPRoutePrefix(ruleMap map[string]interface{}, targetPrefix string) bool {
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

// extractEnvironment derives environment from namespace or other context
func (h *httpRouteManager) extractEnvironment(namespace string) string {
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
