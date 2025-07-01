package inferenceserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Istio Proxy Management

func (g *gateway) configureIstioProxy(ctx context.Context, logger logr.Logger, request ProxyConfigRequest) error {
	logger.Info("Configuring Istio proxy", "server", request.InferenceServer, "model", request.ModelName)

	// Get or create VirtualService
	vs, err := g.getOrCreateVirtualService(ctx, logger, request)
	if err != nil {
		return fmt.Errorf("failed to get or create VirtualService: %w", err)
	}

	// Update the production route
	err = g.updateProductionRoute(ctx, logger, vs, request)
	if err != nil {
		return fmt.Errorf("failed to update production route: %w", err)
	}

	logger.Info("Istio proxy configured successfully")
	return nil
}

func (g *gateway) getIstioProxyStatus(ctx context.Context, logger logr.Logger, request ProxyStatusRequest) (*ProxyStatus, error) {
	logger.Info("Getting Istio proxy status", "server", request.InferenceServer)

	// Check if VirtualService exists
	gvr := schema.GroupVersionResource{
		Group:    "networking.istio.io",
		Version:  "v1beta1",
		Resource: "virtualservices",
	}

	virtualServiceName := fmt.Sprintf("%s-virtualservice", request.InferenceServer)
	vs, err := g.dynamicClient.Resource(gvr).Namespace(request.Namespace).Get(ctx, virtualServiceName, metav1.GetOptions{})
	if err != nil {
		return &ProxyStatus{
			Configured: false,
			Message:    fmt.Sprintf("VirtualService not found: %v", err),
		}, nil
	}

	// Extract routes from VirtualService
	routes, err := g.extractActiveRoutes(vs, request.InferenceServer)
	if err != nil {
		return &ProxyStatus{
			Configured: false,
			Message:    fmt.Sprintf("Failed to extract routes: %v", err),
		}, nil
	}

	return &ProxyStatus{
		Configured: true,
		Routes:     routes,
		Message:    "VirtualService is properly configured",
	}, nil
}

func (g *gateway) getOrCreateVirtualService(ctx context.Context, logger logr.Logger, request ProxyConfigRequest) (*unstructured.Unstructured, error) {
	gvr := schema.GroupVersionResource{
		Group:    "networking.istio.io",
		Version:  "v1beta1",
		Resource: "virtualservices",
	}

	virtualServiceName := fmt.Sprintf("%s-virtualservice", request.InferenceServer)
	
	// Try to get existing VirtualService
	vs, err := g.dynamicClient.Resource(gvr).Namespace(request.Namespace).Get(ctx, virtualServiceName, metav1.GetOptions{})
	if err == nil {
		return vs, nil
	}

	// Create new VirtualService if it doesn't exist
	logger.Info("Creating new VirtualService", "name", virtualServiceName)
	
	vs = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "networking.istio.io/v1beta1",
			"kind":       "VirtualService",
			"metadata": map[string]interface{}{
				"name":      virtualServiceName,
				"namespace": request.Namespace,
			},
			"spec": map[string]interface{}{
				"hosts": []interface{}{
					fmt.Sprintf("%s-service.%s.svc.cluster.local", request.InferenceServer, request.Namespace),
				},
				"http": []interface{}{},
			},
		},
	}

	return g.dynamicClient.Resource(gvr).Namespace(request.Namespace).Create(ctx, vs, metav1.CreateOptions{})
}

func (g *gateway) updateProductionRoute(ctx context.Context, logger logr.Logger, vs *unstructured.Unstructured, request ProxyConfigRequest) error {
	httpRoutes, found, err := unstructured.NestedSlice(vs.Object, "spec", "http")
	if err != nil || !found {
		httpRoutes = []interface{}{}
	}

	productionPrefix := fmt.Sprintf("/%s-endpoint/%s/production", request.InferenceServer, request.InferenceServer)
	updated := false

	// Look for existing production route
	for _, route := range httpRoutes {
		routeMap, ok := route.(map[string]interface{})
		if !ok {
			continue
		}

		if g.hasMatchingPrefix(routeMap, productionPrefix) {
			// Update existing route
			newUri := fmt.Sprintf("/v2/models/%s", request.ModelName)
			if err = unstructured.SetNestedField(routeMap, newUri, "rewrite", "uri"); err != nil {
				logger.Error(err, "Failed to set rewrite uri")
				return err
			}
			updated = true
			break
		}
	}

	if !updated {
		// Add new production route
		logger.Info("Adding new production route", "modelName", request.ModelName)
		productionRoute := map[string]interface{}{
			"match": []interface{}{
				map[string]interface{}{
					"uri": map[string]interface{}{
						"prefix": productionPrefix,
					},
				},
			},
			"rewrite": map[string]interface{}{
				"uri": fmt.Sprintf("/v2/models/%s", request.ModelName),
			},
			"route": []interface{}{
				map[string]interface{}{
					"destination": map[string]interface{}{
						"host": fmt.Sprintf("%s-service.%s.svc.cluster.local", request.InferenceServer, request.Namespace),
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
		logger.Error(err, "Failed to update http routes in VirtualService")
		return err
	}

	gvr := schema.GroupVersionResource{
		Group:    "networking.istio.io",
		Version:  "v1beta1",
		Resource: "virtualservices",
	}

	_, err = g.dynamicClient.Resource(gvr).Namespace(request.Namespace).Update(ctx, vs, metav1.UpdateOptions{})
	if err != nil {
		logger.Error(err, "Failed to update VirtualService")
		return err
	}

	return nil
}

func (g *gateway) hasMatchingPrefix(routeMap map[string]interface{}, targetPrefix string) bool {
	matches, found, _ := unstructured.NestedSlice(routeMap, "match")
	if !found {
		return false
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
			if prefixStr, ok := prefix.(string); ok && prefixStr == targetPrefix {
				return true
			}
		}
	}

	return false
}

func (g *gateway) extractActiveRoutes(vs *unstructured.Unstructured, inferenceServer string) ([]ActiveRoute, error) {
	httpRoutes, found, err := unstructured.NestedSlice(vs.Object, "spec", "http")
	if err != nil || !found {
		return []ActiveRoute{}, nil
	}

	var routes []ActiveRoute

	for _, route := range httpRoutes {
		routeMap, ok := route.(map[string]interface{})
		if !ok {
			continue
		}

		// Extract match information
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

			var path string
			if prefix, ok := uriMap["prefix"]; ok {
				if prefixStr, ok := prefix.(string); ok {
					path = prefixStr
				}
			}

			// Extract rewrite information
			var rewrite string
			if rewriteMap, found, _ := unstructured.NestedMap(routeMap, "rewrite"); found {
				if uri, ok := rewriteMap["uri"]; ok {
					if uriStr, ok := uri.(string); ok {
						rewrite = uriStr
					}
				}
			}

			// Extract destination information
			var destination string
			if routeSlice, found, _ := unstructured.NestedSlice(routeMap, "route"); found {
				for _, r := range routeSlice {
					if rMap, ok := r.(map[string]interface{}); ok {
						if destMap, found, _ := unstructured.NestedMap(rMap, "destination"); found {
							if host, ok := destMap["host"]; ok {
								if hostStr, ok := host.(string); ok {
									destination = hostStr
								}
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