package route

import (
	"context"
	"fmt"

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

var _ RouteProvider = &httpRouteManager{} // Ensure httpRouteManager implements RouteProvider interface

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

// CheckDeploymentRouteStatus checks if a deployment-specific HTTPRoute is properly configured
func (h *httpRouteManager) CheckDeploymentRouteStatus(ctx context.Context, logger *zap.Logger, client dynamic.Interface, deploymentName string, namespace string, inferenceServerName string, modelName string) (bool, error) {
	httpRouteName := addSuffixToString(deploymentName, httpRouteNameSuffix)
	httpRoute, err := h.dynamicClient.Resource(httpRouteGVR).Namespace(namespace).Get(ctx, httpRouteName, metav1.GetOptions{})
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

	// Verify the route matches the expected deployment and OpenAI-compatible API configuration
	expectedMatchPath := fmt.Sprintf("/%s", deploymentName)
	expectedRewritePath := "/v1"
	expectedBackendService := inferenceServerName

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

// EnsureDeploymentRoute ensures that a deployment-specific route is present.
// In this case, we create a new HTTPRoute for the deployment.
func (h *httpRouteManager) EnsureDeploymentRoute(ctx context.Context, logger *zap.Logger, client dynamic.Interface, deploymentName string, namespace string, inferenceServerName string, modelName string) error {
	deploymentRouteName := addSuffixToString(deploymentName, httpRouteNameSuffix)

	labels := map[string]string{
		"app.kubernetes.io/name":      "michelangelo-deployment",
		"app.kubernetes.io/component": "traffic-routing",
		"app.kubernetes.io/instance":  deploymentName,
		"michelangelo.ai/deployment":  deploymentName,
	}
	annotations := map[string]string{
		"michelangelo.ai/deployment":       deploymentName,
		"michelangelo.ai/inference-server": inferenceServerName,
	}

	// Match on deployment name prefix, rewrite to OpenAI-compatible /v1 path
	matchPath := fmt.Sprintf("/%s", deploymentName)
	rewritePath := "/v1"

	httpRoute := buildHTTPRoute(
		deploymentRouteName,
		namespace,
		labels,
		annotations,
		matchPath,
		inferenceServerName,
		nil,
		rewritePath,
	)

	existingRoute, err := h.dynamicClient.Resource(httpRouteGVR).Namespace(namespace).Get(ctx, deploymentRouteName, metav1.GetOptions{})
	if err != nil {
		// Create new HTTPRoute
		if _, err := h.dynamicClient.Resource(httpRouteGVR).Namespace(namespace).Create(ctx, httpRoute, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("failed to create HTTPRoute %s: %v", deploymentRouteName, err)
		}
		logger.Info("Created HTTPRoute for deployment",
			zap.String("httproute", deploymentRouteName),
			zap.String("deployment", deploymentName),
			zap.String("path", matchPath),
			zap.String("rewrite", rewritePath))
	} else {
		// Update existing HTTPRoute spec
		existingRoute.Object["spec"] = httpRoute.Object["spec"]
		if _, err := h.dynamicClient.Resource(httpRouteGVR).Namespace(namespace).Update(ctx, existingRoute, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("failed to update HTTPRoute %s: %v", deploymentRouteName, err)
		}
		logger.Info("Updated HTTPRoute for deployment",
			zap.String("httproute", deploymentRouteName),
			zap.String("deployment", deploymentName),
			zap.String("path", matchPath),
			zap.String("rewrite", rewritePath))
	}
	logger.Info("Deployment-specific route added successfully",
		zap.String("deploymentPath", matchPath),
		zap.String("modelName", modelName))
	return nil
}

// DeploymentRouteExists checks if a deployment-specific route exists.
func (h *httpRouteManager) DeploymentRouteExists(ctx context.Context, logger *zap.Logger, client dynamic.Interface, deploymentName string, namespace string) (bool, error) {
	deploymentRouteName := addSuffixToString(deploymentName, httpRouteNameSuffix)
	route, err := h.dynamicClient.Resource(httpRouteGVR).Namespace(namespace).Get(ctx, deploymentRouteName, metav1.GetOptions{})
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
func (h *httpRouteManager) DeleteDeploymentRoute(ctx context.Context, logger *zap.Logger, client dynamic.Interface, deploymentName string, namespace string) error {
	deploymentRouteName := addSuffixToString(deploymentName, httpRouteNameSuffix)
	if err := h.deleteHTTPRoute(ctx, logger, client, deploymentRouteName, namespace); err != nil {
		return fmt.Errorf("failed to delete deployment HTTPRoute %s: %w", deploymentRouteName, err)
	}
	return nil
}

func (h *httpRouteManager) deleteHTTPRoute(ctx context.Context, logger *zap.Logger, client dynamic.Interface, routeName, namespace string) error {
	if err := h.dynamicClient.Resource(httpRouteGVR).Namespace(namespace).Delete(ctx, routeName, metav1.DeleteOptions{}); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("HTTPRoute not found, already deleted", zap.String("httpRoute", routeName))
		} else {
			return err
		}
	}
	return nil
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
