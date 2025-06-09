package istio

import (
	"context"
	"fmt"
	"github.com/michelangelo-ai/michelangelo/go/api/utils"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"strings"

	"github.com/go-logr/logr"
	"github.com/michelangelo-ai/michelangelo/go/deployment/provider/proxy"
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
	log.Info("Updating Istio VirtualService", "name", deployment.Name, "namespace", deployment.Namespace)

	// Get existing VirtualService
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
					"host": fmt.Sprintf("%s-service.%s.svc.cluster.local", name, namespace),
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
						"host": fmt.Sprintf("%s-service.%s.svc.cluster.local", inferenceServerName, namespace),
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
