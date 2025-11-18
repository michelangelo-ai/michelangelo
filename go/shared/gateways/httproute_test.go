package gateways

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/scheme"

	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

func TestGetOrCreateHTTPRoute(t *testing.T) {
	tests := []struct {
		name              string
		request           ConfigureProxyRequest
		existingHTTPRoute *unstructured.Unstructured
		validateFunc      func(t *testing.T, result *unstructured.Unstructured, err error)
	}{
		{
			name: "get existing httproute",
			request: ConfigureProxyRequest{
				InferenceServer: "test-server",
				Namespace:       "default",
				ModelName:       "test-model",
				DeploymentName:  "test-deployment",
				BackendType:     v2pb.BACKEND_TYPE_TRITON,
			},
			existingHTTPRoute: createHTTPRoute("test-server-httproute", "default", "/test-server"),
			validateFunc: func(t *testing.T, result *unstructured.Unstructured, err error) {
				require.NoError(t, err)
				require.NotNil(t, result)
				name, _, _ := unstructured.NestedString(result.Object, "metadata", "name")
				assert.Equal(t, "test-server-httproute", name)
			},
		},
		{
			name: "create new httproute when it doesn't exist",
			request: ConfigureProxyRequest{
				InferenceServer: "new-server",
				Namespace:       "default",
				ModelName:       "new-model",
				DeploymentName:  "new-deployment",
				BackendType:     v2pb.BACKEND_TYPE_TRITON,
			},
			existingHTTPRoute: nil,
			validateFunc: func(t *testing.T, result *unstructured.Unstructured, err error) {
				require.NoError(t, err)
				require.NotNil(t, result)

				// Verify metadata
				name, _, _ := unstructured.NestedString(result.Object, "metadata", "name")
				assert.Equal(t, "new-server-httproute", name)

				namespace, _, _ := unstructured.NestedString(result.Object, "metadata", "namespace")
				assert.Equal(t, "default", namespace)

				// Verify labels
				labels, _, _ := unstructured.NestedMap(result.Object, "metadata", "labels")
				assert.Equal(t, "inference-server", labels["app"])
				assert.Equal(t, "new-server", labels["inference-server"])
				assert.Equal(t, "controller", labels["michelangelo.ai/managed-by"])

				// Verify parentRefs
				parentRefs, _, _ := unstructured.NestedSlice(result.Object, "spec", "parentRefs")
				assert.Len(t, parentRefs, 1)

				// Verify rules
				rules, _, _ := unstructured.NestedSlice(result.Object, "spec", "rules")
				assert.Len(t, rules, 1)
			},
		},
		{
			name: "create httproute in namespace with prod, environment should be production",
			request: ConfigureProxyRequest{
				InferenceServer: "prod-server",
				Namespace:       "prod-namespace",
				ModelName:       "model",
				DeploymentName:  "deployment",
				BackendType:     v2pb.BACKEND_TYPE_TRITON,
			},
			existingHTTPRoute: nil,
			validateFunc: func(t *testing.T, result *unstructured.Unstructured, err error) {
				require.NoError(t, err)
				labels, _, _ := unstructured.NestedMap(result.Object, "metadata", "labels")
				assert.Equal(t, "production", labels["environment"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup fake client
			var objects []runtime.Object
			if tt.existingHTTPRoute != nil {
				objects = append(objects, tt.existingHTTPRoute)
			}
			fakeClient := fake.NewSimpleDynamicClient(scheme.Scheme, objects...)

			manager := newHTTPRouteManager(fakeClient, zap.NewNop())

			// Execute
			result, err := manager.getOrCreateHTTPRoute(context.Background(), zap.NewNop(), tt.request)

			// Validate
			if tt.validateFunc != nil {
				tt.validateFunc(t, result, err)
			}
		})
	}
}

func TestUpdateProductionRoute(t *testing.T) {
	tests := []struct {
		name         string
		request      ConfigureProxyRequest
		httpRoute    *unstructured.Unstructured
		expectError  bool
		validateFunc func(t *testing.T, fakeClient *fake.FakeDynamicClient, err error)
	}{
		{
			name: "update existing production route successfully",
			request: ConfigureProxyRequest{
				InferenceServer: "test-server",
				Namespace:       "default",
				ModelName:       "updated-model",
				DeploymentName:  "test-deployment",
				BackendType:     v2pb.BACKEND_TYPE_TRITON,
			},
			httpRoute:   createHTTPRouteWithProductionRoute("test-server-httproute", "default", "/test-server/test-deployment/production", "/v2/models/old-model"),
			expectError: false,
			validateFunc: func(t *testing.T, fakeClient *fake.FakeDynamicClient, err error) {
				require.NoError(t, err)

				// Get the updated HTTPRoute
				updated, getErr := fakeClient.Resource(httpRouteGVR).Namespace("default").Get(
					context.Background(), "test-server-httproute", metav1.GetOptions{})
				require.NoError(t, getErr)

				// Verify the filter was updated
				rules, _, _ := unstructured.NestedSlice(updated.Object, "spec", "rules")
				require.Len(t, rules, 1)

				ruleMap := rules[0].(map[string]interface{})
				filters, _, _ := unstructured.NestedSlice(ruleMap, "filters")
				require.Len(t, filters, 1)

				filterMap := filters[0].(map[string]interface{})
				replacePrefixMatch, _, _ := unstructured.NestedString(filterMap, "urlRewrite", "path", "replacePrefixMatch")
				assert.Equal(t, "/v2/models/updated-model", replacePrefixMatch)
			},
		},
		{
			name: "production route not found, returns error",
			request: ConfigureProxyRequest{
				InferenceServer: "test-server",
				Namespace:       "default",
				ModelName:       "model",
				DeploymentName:  "test-deployment",
				BackendType:     v2pb.BACKEND_TYPE_TRITON,
			},
			httpRoute:   createHTTPRoute("test-server-httproute", "default", "/different-path"),
			expectError: true,
			validateFunc: func(t *testing.T, fakeClient *fake.FakeDynamicClient, err error) {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "production route not found")
			},
		},
		{
			name: "httproute already configured for desired model, no update needed",
			request: ConfigureProxyRequest{
				InferenceServer: "test-server",
				Namespace:       "default",
				ModelName:       "existing-model",
				DeploymentName:  "test-deployment",
				BackendType:     v2pb.BACKEND_TYPE_TRITON,
			},
			httpRoute:   createHTTPRouteWithProductionRoute("test-server-httproute", "default", "/test-server/test-deployment/production", "/v2/models/existing-model"),
			expectError: false,
			validateFunc: func(t *testing.T, fakeClient *fake.FakeDynamicClient, err error) {
				require.NoError(t, err)
			},
		},
		{
			name: "httproute with no rules, returns error",
			request: ConfigureProxyRequest{
				InferenceServer: "test-server",
				Namespace:       "default",
				ModelName:       "model",
				DeploymentName:  "test-deployment",
				BackendType:     v2pb.BACKEND_TYPE_TRITON,
			},
			httpRoute: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "gateway.networking.k8s.io/v1",
					"kind":       "HTTPRoute",
					"metadata": map[string]interface{}{
						"name":      "test-server-httproute",
						"namespace": "default",
					},
					"spec": map[string]interface{}{},
				},
			},
			expectError: true,
			validateFunc: func(t *testing.T, fakeClient *fake.FakeDynamicClient, err error) {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "rules not found")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup fake client
			fakeClient := fake.NewSimpleDynamicClient(scheme.Scheme, tt.httpRoute)
			manager := newHTTPRouteManager(fakeClient, zap.NewNop())

			// Execute
			err := manager.updateProductionRoute(context.Background(), zap.NewNop(), tt.httpRoute, tt.request)

			// Validate
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			if tt.validateFunc != nil {
				tt.validateFunc(t, fakeClient, err)
			}
		})
	}
}

func TestGetProxyStatus(t *testing.T) {
	tests := []struct {
		name              string
		request           GetProxyStatusRequest
		existingHTTPRoute *unstructured.Unstructured
		expectedResponse  *GetProxyStatusResponse
	}{
		{
			name: "get status for existing httproute with routes",
			request: GetProxyStatusRequest{
				InferenceServer: "test-server",
				Namespace:       "default",
			},
			existingHTTPRoute: createHTTPRouteWithBackendAndFilters("test-server-httproute", "default", "/test-server", "test-server-inference-service", "/v2/models/test-model"),
			expectedResponse: &GetProxyStatusResponse{
				Status: ProxyStatus{
					Configured: true,
					Routes: []ActiveRoute{
						{
							Path:        "/test-server",
							Destination: "test-server-inference-service",
							Rewrite:     "/v2/models/test-model",
							Active:      true,
						},
					},
					Message: "HTTPRoute is properly configured",
				},
			},
		},
		{
			name: "get status for non-existent httproute",
			request: GetProxyStatusRequest{
				InferenceServer: "non-existent-server",
				Namespace:       "default",
			},
			existingHTTPRoute: nil,
			expectedResponse: &GetProxyStatusResponse{
				Status: ProxyStatus{
					Configured: false,
					Message:    "HTTPRoute not found: httproutes.gateway.networking.k8s.io \"non-existent-server-httproute\" not found",
				},
			},
		},
		{
			name: "get status for httproute with no routes",
			request: GetProxyStatusRequest{
				InferenceServer: "empty-server",
				Namespace:       "default",
			},
			existingHTTPRoute: createEmptyHTTPRoute("empty-server-httproute", "default"),
			expectedResponse: &GetProxyStatusResponse{
				Status: ProxyStatus{
					Configured: true,
					Routes:     nil,
					Message:    "HTTPRoute is properly configured",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup fake client
			var objects []runtime.Object
			if tt.existingHTTPRoute != nil {
				objects = append(objects, tt.existingHTTPRoute)
			}
			fakeClient := fake.NewSimpleDynamicClient(scheme.Scheme, objects...)
			manager := newHTTPRouteManager(fakeClient, zap.NewNop())

			// Execute
			response, err := manager.getProxyStatus(context.Background(), zap.NewNop(), tt.request)

			// Validate
			require.NoError(t, err)
			assert.Equal(t, tt.expectedResponse.Status.Configured, response.Status.Configured)
			assert.Contains(t, response.Status.Message, tt.expectedResponse.Status.Message)
			if tt.expectedResponse.Status.Routes != nil {
				assert.Equal(t, tt.expectedResponse.Status.Routes, response.Status.Routes)
			}
		})
	}
}

func TestAddDeploymentRoute(t *testing.T) {
	tests := []struct {
		name         string
		request      AddDeploymentRouteRequest
		httpRoute    *unstructured.Unstructured
		expectError  bool
		validateFunc func(t *testing.T, fakeClient *fake.FakeDynamicClient, err error)
	}{
		{
			name: "add new deployment route successfully",
			request: AddDeploymentRouteRequest{
				InferenceServer: "test-server",
				Namespace:       "default",
				DeploymentName:  "new-deployment",
				ModelName:       "new-model",
				BackendType:     v2pb.BACKEND_TYPE_TRITON,
			},
			httpRoute:   createHTTPRoute("test-server-httproute", "default", "/test-server"),
			expectError: false,
			validateFunc: func(t *testing.T, fakeClient *fake.FakeDynamicClient, err error) {
				require.NoError(t, err)

				// Get the updated HTTPRoute
				updated, getErr := fakeClient.Resource(httpRouteGVR).Namespace("default").Get(
					context.Background(), "test-server-httproute", metav1.GetOptions{})
				require.NoError(t, getErr)

				// Verify the new route was added (should be prepended)
				rules, _, _ := unstructured.NestedSlice(updated.Object, "spec", "rules")
				assert.Len(t, rules, 2)

				// Check the first rule is the new deployment route
				firstRule := rules[0].(map[string]interface{})
				matches, _, _ := unstructured.NestedSlice(firstRule, "matches")
				firstMatch := matches[0].(map[string]interface{})
				pathValue, _, _ := unstructured.NestedString(firstMatch, "path", "value")
				assert.Equal(t, "/test-server/new-deployment", pathValue)

				// Verify the filter
				filters, _, _ := unstructured.NestedSlice(firstRule, "filters")
				filterMap := filters[0].(map[string]interface{})
				replacePrefixMatch, _, _ := unstructured.NestedString(filterMap, "urlRewrite", "path", "replacePrefixMatch")
				assert.Equal(t, "/v2/models/new-model", replacePrefixMatch)
			},
		},
		{
			name: "update existing deployment route",
			request: AddDeploymentRouteRequest{
				InferenceServer: "test-server",
				Namespace:       "default",
				DeploymentName:  "existing-deployment",
				ModelName:       "updated-model",
				BackendType:     v2pb.BACKEND_TYPE_TRITON,
			},
			httpRoute:   createHTTPRouteWithProductionRoute("test-server-httproute", "default", "/test-server/existing-deployment", "/v2/models/old-model"),
			expectError: false,
			validateFunc: func(t *testing.T, fakeClient *fake.FakeDynamicClient, err error) {
				require.NoError(t, err)

				// Get the updated HTTPRoute
				updated, getErr := fakeClient.Resource(httpRouteGVR).Namespace("default").Get(
					context.Background(), "test-server-httproute", metav1.GetOptions{})
				require.NoError(t, getErr)

				// Verify the route was updated
				rules, _, _ := unstructured.NestedSlice(updated.Object, "spec", "rules")
				assert.Len(t, rules, 1)

				ruleMap := rules[0].(map[string]interface{})
				filters, _, _ := unstructured.NestedSlice(ruleMap, "filters")
				filterMap := filters[0].(map[string]interface{})
				replacePrefixMatch, _, _ := unstructured.NestedString(filterMap, "urlRewrite", "path", "replacePrefixMatch")
				assert.Equal(t, "/v2/models/updated-model", replacePrefixMatch)
			},
		},
		{
			name: "add deployment route to non-existent httproute, returns error",
			request: AddDeploymentRouteRequest{
				InferenceServer: "non-existent-server",
				Namespace:       "default",
				DeploymentName:  "deployment",
				ModelName:       "model",
				BackendType:     v2pb.BACKEND_TYPE_TRITON,
			},
			httpRoute:   nil,
			expectError: true,
			validateFunc: func(t *testing.T, fakeClient *fake.FakeDynamicClient, err error) {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "failed to get HTTPRoute")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup fake client
			var objects []runtime.Object
			if tt.httpRoute != nil {
				objects = append(objects, tt.httpRoute)
			}
			fakeClient := fake.NewSimpleDynamicClient(scheme.Scheme, objects...)
			manager := newHTTPRouteManager(fakeClient, zap.NewNop())

			// Execute
			err := manager.addDeploymentRoute(context.Background(), zap.NewNop(), tt.request)

			// Validate
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			if tt.validateFunc != nil {
				tt.validateFunc(t, fakeClient, err)
			}
		})
	}
}

func TestDeleteHTTPRoute(t *testing.T) {
	tests := []struct {
		name              string
		httpRouteName     string
		namespace         string
		existingHTTPRoute *unstructured.Unstructured
		expectError       bool
		validateFunc      func(t *testing.T, fakeClient *fake.FakeDynamicClient, err error)
	}{
		{
			name:              "delete existing httproute successfully",
			httpRouteName:     "test-server",
			namespace:         "default",
			existingHTTPRoute: createEmptyHTTPRoute("test-server-httproute", "default"),
			expectError:       false,
			validateFunc: func(t *testing.T, fakeClient *fake.FakeDynamicClient, err error) {
				require.NoError(t, err)

				// Verify the HTTPRoute was deleted
				_, getErr := fakeClient.Resource(httpRouteGVR).Namespace("default").Get(
					context.Background(), "test-server-httproute", metav1.GetOptions{})
				assert.Error(t, getErr)
				assert.Contains(t, getErr.Error(), "not found")
			},
		},
		{
			name:              "delete non-existent httproute, does not return error",
			httpRouteName:     "non-existent-server",
			namespace:         "default",
			existingHTTPRoute: nil,
			expectError:       false,
			validateFunc: func(t *testing.T, fakeClient *fake.FakeDynamicClient, err error) {
				require.NoError(t, err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup fake client
			var objects []runtime.Object
			if tt.existingHTTPRoute != nil {
				objects = append(objects, tt.existingHTTPRoute)
			}
			fakeClient := fake.NewSimpleDynamicClient(scheme.Scheme, objects...)
			manager := newHTTPRouteManager(fakeClient, zap.NewNop())

			// Execute
			err := manager.deleteHTTPRoute(context.Background(), zap.NewNop(), tt.httpRouteName, tt.namespace)

			// Validate
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			if tt.validateFunc != nil {
				tt.validateFunc(t, fakeClient, err)
			}
		})
	}
}

// Helper function to create HTTPRoute objects for testing
func createHTTPRoute(name, namespace, pathValue string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "gateway.networking.k8s.io/v1",
			"kind":       "HTTPRoute",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
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
					},
				},
			},
		},
	}
}

// Helper function to create HTTPRoute with production route
func createHTTPRouteWithProductionRoute(name, namespace, pathValue, modelPath string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "gateway.networking.k8s.io/v1",
			"kind":       "HTTPRoute",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
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
						"filters": []interface{}{
							map[string]interface{}{
								"type": "URLRewrite",
								"urlRewrite": map[string]interface{}{
									"path": map[string]interface{}{
										"type":               "ReplacePrefixMatch",
										"replacePrefixMatch": modelPath,
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

// Helper function to create HTTPRoute with backend refs and filters
func createHTTPRouteWithBackendAndFilters(name, namespace, pathValue, backendName, rewritePath string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "gateway.networking.k8s.io/v1",
			"kind":       "HTTPRoute",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
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
						"backendRefs": []interface{}{
							map[string]interface{}{
								"name": backendName,
							},
						},
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

// Helper to create HTTPRoute with no rules
func createEmptyHTTPRoute(name, namespace string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "gateway.networking.k8s.io/v1",
			"kind":       "HTTPRoute",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"rules": []interface{}{},
			},
		},
	}
}
