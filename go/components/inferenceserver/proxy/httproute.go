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
func (h *httpRouteManager) EnsureInferenceServerRoute(ctx context.Context, logger *zap.Logger, request EnsureInferenceServerRouteRequest) error {
	logger.Info("Configuring proxy with Gateway API HTTPRoute", zap.String("server", request.InferenceServer), zap.String("model", request.ModelName))
	_, err := h.getOrCreateHTTPRoute(ctx, logger, request)
	if err != nil {
		logger.Error("failed to get or create HTTPRoute",
			zap.Error(err),
			zap.String("operation", "configure_proxy"),
			zap.String("namespace", request.Namespace),
			zap.String("inferenceServer", request.InferenceServer))
		return fmt.Errorf("failed to get or create HTTPRoute for %s/%s: %w",
			request.Namespace, request.InferenceServer, err)
	}
	return nil
}

func (h *httpRouteManager) getOrCreateHTTPRoute(ctx context.Context, logger *zap.Logger, request EnsureInferenceServerRouteRequest) (*unstructured.Unstructured, error) {
	httpRouteName := addSuffixToString(request.InferenceServer, httpRouteNameSuffix)

	// Try to get existing HTTPRoute
	httpRoute, err := h.dynamicClient.Resource(httpRouteGVR).Namespace(request.Namespace).Get(ctx, httpRouteName, metav1.GetOptions{})
	if err == nil {
		return httpRoute, nil
	}

	// Create new HTTPRoute if it doesn't exist
	logger.Info("Creating new HTTPRoute with baseline routing", zap.String("name", httpRouteName))

	environment := h.extractEnvironment(request.Namespace)
	labels := map[string]string{
		"app":                        "inference-server",
		"inference-server":           request.InferenceServer,
		"environment":                environment,
		"michelangelo.ai/managed-by": "controller",
	}

	baselineWeight := int64(100)
	httpRoute = buildHTTPRoute(
		httpRouteName,
		request.Namespace,
		labels,
		nil,
		fmt.Sprintf("/%s", request.InferenceServer),
		addSuffixToString(request.InferenceServer, inferenceServiceSuffix),
		&baselineWeight,
		"/v2",
	)

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

// CheckDeploymentRouteStatus checks if a deployment-specific HTTPRoute is properly configured
func (h *httpRouteManager) CheckDeploymentRouteStatus(ctx context.Context, logger *zap.Logger, request CheckDeploymentRouteStatusRequest) (bool, error) {
	httpRouteName := addSuffixToString(request.DeploymentName, httpRouteNameSuffix)
	httpRoute, err := h.dynamicClient.Resource(httpRouteGVR).Namespace(request.Namespace).Get(ctx, httpRouteName, metav1.GetOptions{})
	if err != nil {
		return false, fmt.Errorf("failed to get HTTPRoute %s: %v", httpRouteName, err)
	}

	// Validate HTTPRoute configuration
	spec, found, err := unstructured.NestedMap(httpRoute.Object, "spec")
	if err != nil || !found {
		return false, fmt.Errorf("HTTPRoute spec not found: %v", err)
	}

	rules, found, err := unstructured.NestedSlice(spec, "rules")
	if err != nil || !found || len(rules) == 0 {
		return false, fmt.Errorf("HTTPRoute has no routing rules configured: %v", err)
	}

	// Verify the route matches the expected model and inference server configuration
	expectedMatchPath := fmt.Sprintf("/%s/%s", request.InferenceServer, request.DeploymentName)
	expectedRewritePath := fmt.Sprintf("/v2/models/%s", request.ModelName)
	expectedBackendService := addSuffixToString(request.InferenceServer, inferenceServiceSuffix)

	// Check if the route configuration matches expectations
	for _, rule := range rules {
		ruleMap, ok := rule.(map[string]interface{})
		if !ok {
			continue
		}

		// Check matches (path)
		matches, matchesFound, _ := unstructured.NestedSlice(ruleMap, "matches")
		if matchesFound && len(matches) > 0 {
			for _, match := range matches {
				matchMap, ok := match.(map[string]interface{})
				if !ok {
					continue
				}

				path, pathFound, _ := unstructured.NestedMap(matchMap, "path")
				if pathFound {
					pathValue, _, _ := unstructured.NestedString(path, "value")
					if pathValue != expectedMatchPath {
						return false, fmt.Errorf("HTTPRoute path mismatch: expected %s, got %s", expectedMatchPath, pathValue)
					}
				}
			}
		}

		// Check backend refs (service)
		backendRefs, backendRefsFound, _ := unstructured.NestedSlice(ruleMap, "backendRefs")
		if backendRefsFound && len(backendRefs) > 0 {
			for _, backendRef := range backendRefs {
				backendMap, ok := backendRef.(map[string]interface{})
				if !ok {
					continue
				}

				serviceName, _, _ := unstructured.NestedString(backendMap, "name")
				if serviceName != expectedBackendService {
					return false, fmt.Errorf("HTTPRoute backend service mismatch: expected %s, got %s", expectedBackendService, serviceName)
				}
			}
		}

		// Check filters (path rewrite)
		filters, filtersFound, _ := unstructured.NestedSlice(ruleMap, "filters")
		if filtersFound && len(filters) > 0 {
			for _, filter := range filters {
				filterMap, ok := filter.(map[string]interface{})
				if !ok {
					continue
				}

				filterType, _, _ := unstructured.NestedString(filterMap, "type")
				if filterType == "URLRewrite" {
					path, urlRewritePathFound, _ := unstructured.NestedMap(filterMap, "urlRewrite", "path")
					if urlRewritePathFound {
						rewriteValue, _, _ := unstructured.NestedString(path, "replacePrefixMatch")
						if rewriteValue != expectedRewritePath {
							return false, fmt.Errorf("HTTPRoute rewrite path mismatch: expected %s, got %s", expectedRewritePath, rewriteValue)
						}
					}
				}
			}
		}
	}

	logger.Info("HTTPRoute is properly configured",
		zap.String("httpRoute", httpRouteName),
		zap.String("matchPath", expectedMatchPath),
		zap.String("backendService", expectedBackendService))

	return true, nil
}

// GetProxyStatus gets the proxy status from the HTTPRoute
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

// EnsureDeploymentRoute ensures that a deployment-specific route is present.
// In this case, we create a new HTTPRoute for the deployment.
func (h *httpRouteManager) EnsureDeploymentRoute(ctx context.Context, logger *zap.Logger, request EnsureDeploymentRouteRequest) error {
	deploymentRouteName := addSuffixToString(request.DeploymentName, httpRouteNameSuffix)
	inferenceServerName := request.InferenceServer

	labels := map[string]string{
		"app.kubernetes.io/name":      "michelangelo-deployment",
		"app.kubernetes.io/component": "traffic-routing",
		"app.kubernetes.io/instance":  request.DeploymentName,
		"michelangelo.ai/deployment":  request.DeploymentName,
	}
	annotations := map[string]string{
		"michelangelo.ai/deployment":       request.DeploymentName,
		"michelangelo.ai/inference-server": inferenceServerName,
	}

	matchPath := fmt.Sprintf("/%s/%s", inferenceServerName, request.DeploymentName)
	rewritePath := fmt.Sprintf("/v2/models/%s", request.ModelName)

	httpRoute := buildHTTPRoute(
		deploymentRouteName,
		request.Namespace,
		labels,
		annotations,
		matchPath,
		addSuffixToString(inferenceServerName, inferenceServiceSuffix),
		nil,
		rewritePath,
	)

	existingRoute, err := h.dynamicClient.Resource(httpRouteGVR).Namespace(request.Namespace).Get(ctx, deploymentRouteName, metav1.GetOptions{})
	if err != nil {
		// Create new HTTPRoute
		if _, err := h.dynamicClient.Resource(httpRouteGVR).Namespace(request.Namespace).Create(ctx, httpRoute, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("failed to create HTTPRoute %s: %v", deploymentRouteName, err)
		}
		logger.Info("Created HTTPRoute for deployment",
			zap.String("httproute", deploymentRouteName),
			zap.String("deployment", request.DeploymentName),
			zap.String("path", matchPath),
			zap.String("rewrite", rewritePath))
	} else {
		// Update existing HTTPRoute spec
		existingRoute.Object["spec"] = httpRoute.Object["spec"]
		if _, err := h.dynamicClient.Resource(httpRouteGVR).Namespace(request.Namespace).Update(ctx, existingRoute, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("failed to update HTTPRoute %s: %v", deploymentRouteName, err)
		}
		logger.Info("Updated HTTPRoute for deployment",
			zap.String("httproute", deploymentRouteName),
			zap.String("deployment", request.DeploymentName),
			zap.String("path", matchPath),
			zap.String("rewrite", rewritePath))
	}
	logger.Info("Deployment-specific route added successfully",
		zap.String("deploymentPath", matchPath),
		zap.String("modelName", request.ModelName))
	return nil
}

// DeploymentRouteExists checks if a deployment-specific route exists.
func (h *httpRouteManager) DeploymentRouteExists(ctx context.Context, logger *zap.Logger, request DeploymentRouteExistsRequest) (bool, error) {
	deploymentRouteName := addSuffixToString(request.DeploymentName, httpRouteNameSuffix)
	route, err := h.dynamicClient.Resource(httpRouteGVR).Namespace(request.Namespace).Get(ctx, deploymentRouteName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		} else {
			return false, fmt.Errorf("failed to get deployment HTTPRoute %s: %w", deploymentRouteName, err)
		}
	}
	return route != nil, nil
}

// DeleteDeploymentRoute deletes a deployment-specific route from the HTTPRoute.
// In this case, since each deployment has its own HTTPRoute, we can just delete the HTTPRoute for the deployment.
func (h *httpRouteManager) DeleteDeploymentRoute(ctx context.Context, logger *zap.Logger, request DeleteDeploymentRouteRequest) error {
	deploymentRouteName := addSuffixToString(request.DeploymentName, httpRouteNameSuffix)
	if err := h.deleteHTTPRoute(ctx, logger, deploymentRouteName, request.Namespace); err != nil {
		return fmt.Errorf("failed to delete deployment HTTPRoute %s: %w", deploymentRouteName, err)
	}
	return nil
}

// DeleteInferenceServerRoute deletes an HTTPRoute for an inference server.
func (h *httpRouteManager) DeleteInferenceServerRoute(ctx context.Context, logger *zap.Logger, request DeleteInferenceServerRouteRequest) error {
	httpRouteName := addSuffixToString(request.InferenceServer, httpRouteNameSuffix)
	if err := h.deleteHTTPRoute(ctx, logger, httpRouteName, request.Namespace); err != nil {
		return fmt.Errorf("failed to delete HTTPRoute %s: %w", httpRouteName, err)
	}
	return nil
}

func (h *httpRouteManager) deleteHTTPRoute(ctx context.Context, logger *zap.Logger, routeName, namespace string) error {
	if err := h.dynamicClient.Resource(httpRouteGVR).Namespace(namespace).Delete(ctx, routeName, metav1.DeleteOptions{}); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("HTTPRoute not found, already deleted", zap.String("httpRoute", routeName))
		} else {
			return err
		}
	}
	return nil
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

func buildHTTPRoute(name, namespace string, labels, annotations map[string]string, pathValue, backendService string, backendWeight *int64, rewritePath string) *unstructured.Unstructured {
	metadata := map[string]interface{}{
		"name":      name,
		"namespace": namespace,
	}
	if lbls := stringMapToInterface(labels); lbls != nil {
		metadata["labels"] = lbls
	}
	if ann := stringMapToInterface(annotations); ann != nil {
		metadata["annotations"] = ann
	}

	backendRef := map[string]interface{}{
		"group": "",
		"kind":  "Service",
		"name":  backendService,
		"port":  int64(80),
	}
	if backendWeight != nil {
		backendRef["weight"] = *backendWeight
	}

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": fmt.Sprintf("%s/%s", gatewayAPIGroup, gatewayAPIVersion),
			"kind":       "HTTPRoute",
			"metadata":   metadata,
			"spec": map[string]interface{}{
				"parentRefs": []interface{}{
					map[string]interface{}{
						"group":     gatewayAPIGroup,
						"kind":      gatewayKind,
						"name":      gatewayName,
						"namespace": namespace,
					},
				},
				"rules": []interface{}{
					map[string]interface{}{
						"matches": []interface{}{
							map[string]interface{}{
								"path": map[string]interface{}{
									"type":  "PathPrefix",
									"value": pathValue,
								},
							},
						},
						"backendRefs": []interface{}{backendRef},
						"filters": []interface{}{
							map[string]interface{}{
								"type": "URLRewrite",
								"urlRewrite": map[string]interface{}{
									"path": map[string]interface{}{
										"type":               "ReplacePrefixMatch",
										"replacePrefixMatch": rewritePath,
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func stringMapToInterface(input map[string]string) map[string]interface{} {
	if len(input) == 0 {
		return nil
	}
	output := make(map[string]interface{}, len(input))
	for k, v := range input {
		output[k] = v
	}
	return output
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
