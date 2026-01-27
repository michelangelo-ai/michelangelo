package proxy

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
)

func TestEnsureDeploymentRoute(t *testing.T) {
	tests := []struct {
		name                string
		deploymentName      string
		namespace           string
		inferenceServerName string
		modelName           string
		backendServiceName  string
		httpRoute           *unstructured.Unstructured
		expectError         bool
		validateFunc        func(t *testing.T, fakeClient *fake.FakeDynamicClient, err error)
	}{
		{
			name:                "create new deployment-specific httproute successfully",
			deploymentName:      "test-deployment",
			namespace:           "default",
			inferenceServerName: "test-server",
			modelName:           "new-model",
			backendServiceName:  "test-server-svc",
			httpRoute:           nil,
			expectError:         false,
			validateFunc: func(t *testing.T, fakeClient *fake.FakeDynamicClient, err error) {
				require.NoError(t, err)

				// Get the deployment HTTPRoute (not the inference server route)
				deploymentRoute, getErr := fakeClient.Resource(httpRouteGVR).Namespace("default").Get(
					context.Background(), "test-deployment-httproute", metav1.GetOptions{})
				require.NoError(t, getErr)

				// Verify the route configuration
				rules, _, _ := unstructured.NestedSlice(deploymentRoute.Object, "spec", "rules")
				require.Len(t, rules, 1)

				// Check the path match
				firstRule := rules[0].(map[string]interface{})
				matches, _, _ := unstructured.NestedSlice(firstRule, "matches")
				firstMatch := matches[0].(map[string]interface{})
				pathValue, _, _ := unstructured.NestedString(firstMatch, "path", "value")
				assert.Equal(t, "/test-server/test-deployment", pathValue)

				// Verify the filter
				filters, _, _ := unstructured.NestedSlice(firstRule, "filters")
				filterMap := filters[0].(map[string]interface{})
				replacePrefixMatch, _, _ := unstructured.NestedString(filterMap, "urlRewrite", "path", "replacePrefixMatch")
				assert.Equal(t, "/v2/models/new-model", replacePrefixMatch)

				// Verify backend ref points to the backend service
				backendRefs, _, _ := unstructured.NestedSlice(firstRule, "backendRefs")
				require.Len(t, backendRefs, 1)
				backendMap := backendRefs[0].(map[string]interface{})
				assert.Equal(t, "test-server-svc", backendMap["name"])
			},
		},
		{
			name:                "update existing deployment httproute",
			deploymentName:      "existing-deployment",
			namespace:           "default",
			inferenceServerName: "test-server",
			modelName:           "updated-model",
			backendServiceName:  "test-server-svc",
			httpRoute:           createHTTPRouteWithBackendRef("existing-deployment-httproute", "default", "/test-server/existing-deployment", "/v2/models/old-model", "old-server-svc"),
			expectError:         false,
			validateFunc: func(t *testing.T, fakeClient *fake.FakeDynamicClient, err error) {
				require.NoError(t, err)

				// Get the updated deployment HTTPRoute
				updated, getErr := fakeClient.Resource(httpRouteGVR).Namespace("default").Get(
					context.Background(), "existing-deployment-httproute", metav1.GetOptions{})
				require.NoError(t, getErr)

				// Verify the route was updated with new model
				rules, _, _ := unstructured.NestedSlice(updated.Object, "spec", "rules")
				require.Len(t, rules, 1)

				ruleMap := rules[0].(map[string]interface{})

				// Check path was updated
				matches, _, _ := unstructured.NestedSlice(ruleMap, "matches")
				matchMap := matches[0].(map[string]interface{})
				pathValue, _, _ := unstructured.NestedString(matchMap, "path", "value")
				assert.Equal(t, "/test-server/existing-deployment", pathValue)

				// Check filter was updated
				filters, _, _ := unstructured.NestedSlice(ruleMap, "filters")
				filterMap := filters[0].(map[string]interface{})
				replacePrefixMatch, _, _ := unstructured.NestedString(filterMap, "urlRewrite", "path", "replacePrefixMatch")
				assert.Equal(t, "/v2/models/updated-model", replacePrefixMatch)

				// Check backend service was updated
				backendRefs, _, _ := unstructured.NestedSlice(ruleMap, "backendRefs")
				require.Len(t, backendRefs, 1)
				backendMap := backendRefs[0].(map[string]interface{})
				assert.Equal(t, "test-server-svc", backendMap["name"])
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
			manager := NewHTTPRouteManager(fakeClient, zap.NewNop())

			// Execute
			err := manager.EnsureDeploymentRoute(context.Background(), zap.NewNop(), tt.deploymentName, tt.namespace, tt.inferenceServerName, tt.modelName, tt.backendServiceName)

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

func TestCheckDeploymentRouteStatus(t *testing.T) {
	tests := []struct {
		name                string
		deploymentName      string
		namespace           string
		inferenceServerName string
		modelName           string
		backendServiceName  string
		httpRoute           *unstructured.Unstructured
		expectResult        bool
		expectError         bool
	}{
		{
			name:                "deployment route exists and is properly configured",
			deploymentName:      "test-deployment",
			namespace:           "default",
			inferenceServerName: "test-server",
			modelName:           "test-model",
			backendServiceName:  "test-server-svc",
			httpRoute:           createHTTPRouteWithBackendRef("test-deployment-httproute", "default", "/test-server/test-deployment", "/v2/models/test-model", "test-server-svc"),
			expectResult:        true,
			expectError:         false,
		},
		{
			name:                "deployment route does not exist",
			deploymentName:      "nonexistent-deployment",
			namespace:           "default",
			inferenceServerName: "test-server",
			modelName:           "test-model",
			backendServiceName:  "test-server-svc",
			httpRoute:           nil,
			expectResult:        false,
			expectError:         true,
		},
		{
			name:                "deployment route exists but has no rules",
			deploymentName:      "empty-deployment",
			namespace:           "default",
			inferenceServerName: "test-server",
			modelName:           "test-model",
			backendServiceName:  "test-server-svc",
			httpRoute:           createEmptyHTTPRoute("empty-deployment-httproute", "default"),
			expectResult:        false,
			expectError:         true,
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
			manager := NewHTTPRouteManager(fakeClient, zap.NewNop())

			// Execute
			result, err := manager.CheckDeploymentRouteStatus(context.Background(), zap.NewNop(), tt.deploymentName, tt.namespace, tt.inferenceServerName, tt.modelName, tt.backendServiceName)

			// Validate
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.expectResult, result)
		})
	}
}

func TestDeploymentRouteExists(t *testing.T) {
	tests := []struct {
		name           string
		deploymentName string
		namespace      string
		httpRoute      *unstructured.Unstructured
		expectResult   bool
		expectError    bool
	}{
		{
			name:           "deployment route exists",
			deploymentName: "test-deployment",
			namespace:      "default",
			httpRoute:      createEmptyHTTPRoute("test-deployment-httproute", "default"),
			expectResult:   true,
			expectError:    false,
		},
		{
			name:           "deployment route does not exist",
			deploymentName: "nonexistent-deployment",
			namespace:      "default",
			httpRoute:      nil,
			expectResult:   false,
			expectError:    false,
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
			manager := NewHTTPRouteManager(fakeClient, zap.NewNop())

			// Execute
			result, err := manager.DeploymentRouteExists(context.Background(), zap.NewNop(), tt.deploymentName, tt.namespace)

			// Validate
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.expectResult, result)
		})
	}
}

func TestDeleteDeploymentRoute(t *testing.T) {
	tests := []struct {
		name           string
		deploymentName string
		namespace      string
		httpRoute      *unstructured.Unstructured
		expectError    bool
		validateFunc   func(t *testing.T, fakeClient *fake.FakeDynamicClient, err error)
	}{
		{
			name:           "delete existing deployment httproute successfully",
			deploymentName: "test-deployment",
			namespace:      "default",
			httpRoute:      createEmptyHTTPRoute("test-deployment-httproute", "default"),
			expectError:    false,
			validateFunc: func(t *testing.T, fakeClient *fake.FakeDynamicClient, err error) {
				require.NoError(t, err)

				// Verify the HTTPRoute was deleted
				_, getErr := fakeClient.Resource(httpRouteGVR).Namespace("default").Get(
					context.Background(), "test-deployment-httproute", metav1.GetOptions{})
				assert.Error(t, getErr)
				assert.Contains(t, getErr.Error(), "not found")
			},
		},
		{
			name:           "delete non-existent deployment httproute, does not return error",
			deploymentName: "nonexistent-deployment",
			namespace:      "default",
			httpRoute:      nil,
			expectError:    false,
			validateFunc: func(t *testing.T, fakeClient *fake.FakeDynamicClient, err error) {
				require.NoError(t, err)
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
			manager := NewHTTPRouteManager(fakeClient, zap.NewNop())

			// Execute
			err := manager.DeleteDeploymentRoute(context.Background(), zap.NewNop(), tt.deploymentName, tt.namespace)

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

// Helper function to create HTTPRoute with backend ref
func createHTTPRouteWithBackendRef(name, namespace, pathValue, modelPath, backendServiceName string) *unstructured.Unstructured {
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
								"group": "",
								"kind":  "Service",
								"name":  backendServiceName,
								"port":  int64(80),
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
