package discovery

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

	gatewayName              = "ma-gateway"
	discoveryRouteNameSuffix = "discovery-httproute"
	endpointsServiceSuffix   = "endpoints"

	// filterTypeURLRewrite is the Gateway API HTTPRoute filter type string for URL
	// path rewriting. Named constant prevents silent breakage if the string drifts
	// from the Gateway API spec.
	filterTypeURLRewrite = "URLRewrite"
)

var httpRouteGVR = schema.GroupVersionResource{
	Group:    gatewayAPIGroup,
	Version:  gatewayAPIVersion,
	Resource: httpRouteResource,
}

var _ ModelDiscoveryProvider = &httpRouteManager{}

type httpRouteManager struct {
	dynamicClient dynamic.Interface
	logger        *zap.Logger
}

// NewHTTPRouteManager returns a ModelDiscoveryProvider backed by Gateway API HTTPRoutes.
func NewHTTPRouteManager(dynamicClient dynamic.Interface, logger *zap.Logger) *httpRouteManager {
	return &httpRouteManager{
		dynamicClient: dynamicClient,
		logger:        logger,
	}
}

// EnsureDiscoveryRoute creates or updates the discovery HTTPRoute for the deployment.
// The route matches /<inferenceServerName>/<deploymentName> on the control-plane gateway,
// rewrites the path to /v2/models/<modelName>, and forwards to the inference server's
// cross-cluster discovery Service ({inferenceServerName}-endpoints) on port 80.
func (h *httpRouteManager) EnsureDiscoveryRoute(ctx context.Context, deploymentName string, namespace string, inferenceServerName string, modelName string) error {
	routeName := discoveryRouteName(deploymentName)

	labels := map[string]string{
		"app.kubernetes.io/name":           "michelangelo-deployment",
		"app.kubernetes.io/component":      "model-discovery",
		"app.kubernetes.io/instance":       deploymentName,
		"michelangelo.ai/deployment":       deploymentName,
		"michelangelo.ai/inference-server": inferenceServerName,
	}
	annotations := map[string]string{
		"michelangelo.ai/deployment":       deploymentName,
		"michelangelo.ai/inference-server": inferenceServerName,
	}

	matchPath := fmt.Sprintf("/%s/%s", inferenceServerName, deploymentName)
	rewritePath := fmt.Sprintf("/v2/models/%s", modelName)
	backendService := endpointsServiceName(inferenceServerName)

	httpRoute := buildDiscoveryHTTPRoute(routeName, namespace, labels, annotations, matchPath, rewritePath, backendService)

	existing, err := h.dynamicClient.Resource(httpRouteGVR).Namespace(namespace).Get(ctx, routeName, metav1.GetOptions{})
	if err != nil {
		if _, createErr := h.dynamicClient.Resource(httpRouteGVR).Namespace(namespace).Create(ctx, httpRoute, metav1.CreateOptions{}); createErr != nil {
			return fmt.Errorf("failed to create discovery HTTPRoute %s: %v", routeName, createErr)
		}
		return nil
	}

	existing.Object["spec"] = httpRoute.Object["spec"]
	if _, err := h.dynamicClient.Resource(httpRouteGVR).Namespace(namespace).Update(ctx, existing, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("failed to update discovery HTTPRoute %s: %v", routeName, err)
	}
	return nil
}

// CheckDiscoveryRouteStatus reads the discovery HTTPRoute and validates that its match path,
// rewrite path, and backend service match the expected values for the deployment and model.
func (h *httpRouteManager) CheckDiscoveryRouteStatus(ctx context.Context, deploymentName string, namespace string, inferenceServerName string, modelName string) (bool, error) {
	routeName := discoveryRouteName(deploymentName)
	route, err := h.dynamicClient.Resource(httpRouteGVR).Namespace(namespace).Get(ctx, routeName, metav1.GetOptions{})
	if err != nil {
		return false, fmt.Errorf("failed to get discovery HTTPRoute %s: %v", routeName, err)
	}

	rules, found, err := unstructured.NestedSlice(route.Object, "spec", "rules")
	if err != nil || !found || len(rules) == 0 {
		return false, fmt.Errorf("discovery HTTPRoute %s has no rules: %v", routeName, err)
	}

	expectedMatchPath := fmt.Sprintf("/%s/%s", inferenceServerName, deploymentName)
	expectedRewritePath := fmt.Sprintf("/v2/models/%s", modelName)
	expectedBackend := endpointsServiceName(inferenceServerName)

	for _, rule := range rules {
		ruleMap, ok := rule.(map[string]interface{})
		if !ok {
			continue
		}

		matches, _, _ := unstructured.NestedSlice(ruleMap, "matches")
		for _, match := range matches {
			matchMap, ok := match.(map[string]interface{})
			if !ok {
				continue
			}
			pathValue, _, _ := unstructured.NestedString(matchMap, "path", "value")
			if pathValue != expectedMatchPath {
				return false, fmt.Errorf("discovery HTTPRoute path mismatch: expected %s, got %s", expectedMatchPath, pathValue)
			}
		}

		backendRefs, _, _ := unstructured.NestedSlice(ruleMap, "backendRefs")
		for _, backendRef := range backendRefs {
			backendMap, ok := backendRef.(map[string]interface{})
			if !ok {
				continue
			}
			serviceName, _, _ := unstructured.NestedString(backendMap, "name")
			if serviceName != expectedBackend {
				return false, fmt.Errorf("discovery HTTPRoute backend mismatch: expected %s, got %s", expectedBackend, serviceName)
			}
		}

		filters, _, _ := unstructured.NestedSlice(ruleMap, "filters")
		for _, filter := range filters {
			filterMap, ok := filter.(map[string]interface{})
			if !ok {
				continue
			}
			filterType, _, _ := unstructured.NestedString(filterMap, "type")
			if filterType != filterTypeURLRewrite {
				continue
			}
			rewriteValue, _, _ := unstructured.NestedString(filterMap, "urlRewrite", "path", "replacePrefixMatch")
			if rewriteValue != expectedRewritePath {
				return false, fmt.Errorf("discovery HTTPRoute rewrite mismatch: expected %s, got %s", expectedRewritePath, rewriteValue)
			}
		}
	}
	return true, nil
}

// DeleteDiscoveryRoute removes the discovery HTTPRoute for the deployment. NotFound is tolerated.
func (h *httpRouteManager) DeleteDiscoveryRoute(ctx context.Context, deploymentName string, namespace string) error {
	routeName := discoveryRouteName(deploymentName)
	if err := h.dynamicClient.Resource(httpRouteGVR).Namespace(namespace).Delete(ctx, routeName, metav1.DeleteOptions{}); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to delete discovery HTTPRoute %s: %w", routeName, err)
	}
	return nil
}

func discoveryRouteName(deploymentName string) string {
	return fmt.Sprintf("%s-%s", deploymentName, discoveryRouteNameSuffix)
}

func endpointsServiceName(inferenceServerName string) string {
	return fmt.Sprintf("%s-%s", inferenceServerName, endpointsServiceSuffix)
}

func buildDiscoveryHTTPRoute(name, namespace string, labels, annotations map[string]string, matchPath, rewritePath, backendService string) *unstructured.Unstructured {
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
									"value": matchPath,
								},
							},
						},
						"backendRefs": []interface{}{backendRef},
						"filters": []interface{}{
							map[string]interface{}{
								"type": filterTypeURLRewrite,
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
