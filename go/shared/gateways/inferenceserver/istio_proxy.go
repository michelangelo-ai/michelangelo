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

// Gateway API Proxy Management (preferred) with Istio fallback

func (g *gateway) configureIstioProxy(ctx context.Context, logger logr.Logger, request ProxyConfigRequest) error {
	logger.Info("Configuring proxy with Gateway API (HTTPRoute preferred)", "server", request.InferenceServer, "model", request.ModelName)

	// Try HTTPRoute first
	httpRoute, err := g.getOrCreateHTTPRoute(ctx, logger, request)
	if err != nil {
		logger.Info("Failed to create HTTPRoute, falling back to VirtualService", "error", err)
		
		// Fallback to VirtualService
		vs, vsErr := g.getOrCreateVirtualService(ctx, logger, request)
		if vsErr != nil {
			return fmt.Errorf("failed to get or create VirtualService: %w", vsErr)
		}

		// Update the production route
		err = g.updateProductionRoute(ctx, logger, vs, request)
		if err != nil {
			return fmt.Errorf("failed to update production route: %w", err)
		}
		
		logger.Info("Istio proxy configured successfully with VirtualService fallback")
		return nil
	}

	// Update the HTTPRoute production route
	err = g.updateHTTPRouteProductionRoute(ctx, logger, httpRoute, request)
	if err != nil {
		return fmt.Errorf("failed to update HTTPRoute production route: %w", err)
	}

	logger.Info("Gateway API proxy configured successfully with HTTPRoute")
	return nil
}

func (g *gateway) getIstioProxyStatus(ctx context.Context, logger logr.Logger, request ProxyStatusRequest) (*ProxyStatus, error) {
	logger.Info("Getting proxy status (HTTPRoute preferred)", "server", request.InferenceServer)

	// Check HTTPRoute first
	httpRouteGvr := schema.GroupVersionResource{
		Group:    "gateway.networking.k8s.io",
		Version:  "v1",
		Resource: "httproutes",
	}

	httpRouteName := fmt.Sprintf("%s-httproute", request.InferenceServer)
	httpRoute, err := g.dynamicClient.Resource(httpRouteGvr).Namespace(request.Namespace).Get(ctx, httpRouteName, metav1.GetOptions{})
	if err == nil {
		// HTTPRoute found, extract routes from it
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

	// Fallback to VirtualService
	logger.Info("HTTPRoute not found, checking VirtualService", "error", err)
	
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
			Message:    fmt.Sprintf("Neither HTTPRoute nor VirtualService found: %v", err),
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
				"hosts": []string{"*"},
				"gateways": []string{
					"default/ma-gateway",
				},
				"http": []map[string]interface{}{
					{
						"match": []map[string]interface{}{
							{
								"uri": map[string]string{
									"prefix": fmt.Sprintf("/%s-endpoint/%s/production", request.InferenceServer, request.InferenceServer),
								},
							},
						},
						"route": []map[string]interface{}{
							{
								"destination": map[string]interface{}{
									"host": fmt.Sprintf("%s-service.%s.svc.cluster.local", request.InferenceServer, request.Namespace),
									"port": map[string]int{
										"number": 80,
									},
								},
							},
						},
					},
					{
						"match": []map[string]interface{}{
							{
								"uri": map[string]string{
									"prefix": fmt.Sprintf("/%s-endpoint/%s/canary", request.InferenceServer, request.InferenceServer),
								},
							},
						},
						"route": []map[string]interface{}{
							{
								"destination": map[string]interface{}{
									"host": fmt.Sprintf("%s-service.%s.svc.cluster.local", request.InferenceServer, request.Namespace),
									"port": map[string]int{
										"number": 80,
									},
								},
							},
						},
					},
				},
			},
		},
	}

	return g.dynamicClient.Resource(gvr).Namespace(request.Namespace).Create(ctx, vs, metav1.CreateOptions{})
}

func (g *gateway) updateProductionRoute(ctx context.Context, logger logr.Logger, vs *unstructured.Unstructured, request ProxyConfigRequest) error {
	httpRoutes, found, err := unstructured.NestedSlice(vs.Object, "spec", "http")
	if err != nil || !found {
		return fmt.Errorf("http routes not found in VirtualService")
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
			logger.Info("Updating existing production route", "modelName", request.ModelName)
			
			// Add rewrite URI to route to specific model
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
		return fmt.Errorf("production route not found in VirtualService for inference server: %s", request.InferenceServer)
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

	logger.Info("Production route updated successfully", "modelName", request.ModelName, "inferenceServer", request.InferenceServer)
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

// HTTPRoute functions for Gateway API

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
									"value": fmt.Sprintf("/%s-endpoint/%s/production", request.InferenceServer, request.InferenceServer),
								},
							},
						},
						"backendRefs": []map[string]interface{}{
							{
								"group":  "",
								"kind":   "Service",
								"name":   fmt.Sprintf("%s-service", request.InferenceServer),
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
									"value": fmt.Sprintf("/%s-endpoint/%s/canary", request.InferenceServer, request.InferenceServer),
								},
							},
						},
						"backendRefs": []map[string]interface{}{
							{
								"group":  "",
								"kind":   "Service",
								"name":   fmt.Sprintf("%s-service", request.InferenceServer),
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

	productionPrefix := fmt.Sprintf("/%s-endpoint/%s/production", request.InferenceServer, request.InferenceServer)
	updated := false

	// Look for existing production route
	for _, rule := range rules {
		ruleMap, ok := rule.(map[string]interface{})
		if !ok {
			continue
		}

		if g.hasMatchingHTTPRoutePrefix(ruleMap, productionPrefix) {
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