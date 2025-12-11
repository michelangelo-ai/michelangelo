package configmap

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestCreateModelConfigMap(t *testing.T) {
	tests := []struct {
		name               string
		request            CreateModelConfigMapRequest
		existingConfigMaps []runtime.Object
		expectError        bool
		validateFunc       func(t *testing.T, client client.Client, request CreateModelConfigMapRequest)
	}{
		{
			name: "create call on new configmap",
			request: CreateModelConfigMapRequest{
				InferenceServer: "test-server",
				Namespace:       "default",
				ModelConfigs: []ModelConfigEntry{
					{Name: "model1", StoragePath: "s3://bucket/model1"},
					{Name: "model2", StoragePath: "s3://bucket/model2"},
				},
				Labels: map[string]string{
					"custom-label": "custom-value",
				},
				Annotations: map[string]string{
					"custom-annotation": "annotation-value",
				},
			},
			existingConfigMaps: []runtime.Object{},
			validateFunc: func(t *testing.T, c client.Client, request CreateModelConfigMapRequest) {
				configMapName := addSuffixToString(request.InferenceServer, modelConfigSuffix)
				cm := &corev1.ConfigMap{}
				err := c.Get(context.Background(), client.ObjectKey{Name: configMapName, Namespace: request.Namespace}, cm)
				require.NoError(t, err)

				// Verify the configmap was created with correct name and namespace
				assert.Equal(t, configMapName, cm.Name)
				assert.Equal(t, request.Namespace, cm.Namespace)

				// Verify labels
				expectedLabels := map[string]string{
					"app.kubernetes.io/component":      "model-config",
					"app.kubernetes.io/part-of":        "michelangelo",
					"michelangelo.ai/inference-server": request.InferenceServer,
					"custom-label":                     "custom-value",
				}
				assert.Equal(t, expectedLabels, cm.Labels)

				// Verify annotations
				assert.Equal(t, request.Annotations, cm.Annotations)

				// Verify model-list.json data
				modelListJSON, exists := cm.Data[modelListKey]
				assert.True(t, exists)

				var actualModels []ModelConfigEntry
				err = json.Unmarshal([]byte(modelListJSON), &actualModels)
				require.NoError(t, err)
				assert.Equal(t, request.ModelConfigs, actualModels)
			},
		},
		{
			name: "create call on configmap that already exists",
			request: CreateModelConfigMapRequest{
				InferenceServer: "existing-server",
				Namespace:       "default",
				ModelConfigs: []ModelConfigEntry{
					{Name: "new-model", StoragePath: "s3://bucket/new-model"},
				},
				Labels:      nil,
				Annotations: nil,
			},
			existingConfigMaps: []runtime.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "existing-server-model-config",
						Namespace: "default",
						Labels: map[string]string{
							"app.kubernetes.io/component": "model-config",
							"existing-label":              "existing-value",
						},
					},
					Data: map[string]string{
						modelListKey: `[{"name":"old-model","storage_path":"s3://bucket/old-model"}]`,
					},
				},
			},
			validateFunc: func(t *testing.T, c client.Client, request CreateModelConfigMapRequest) {
				configMapName := addSuffixToString(request.InferenceServer, modelConfigSuffix)
				cm := &corev1.ConfigMap{}
				err := c.Get(context.Background(), client.ObjectKey{Name: configMapName, Namespace: request.Namespace}, cm)
				require.NoError(t, err)

				// Verify the configmap was NOT modified (old data should remain)
				modelListJSON, exists := cm.Data[modelListKey]
				assert.True(t, exists)

				var actualModels []ModelConfigEntry
				err = json.Unmarshal([]byte(modelListJSON), &actualModels)
				require.NoError(t, err)

				// Should still have the old model, not the new one
				expectedOldModels := []ModelConfigEntry{
					{Name: "old-model", StoragePath: "s3://bucket/old-model"},
				}
				assert.Equal(t, expectedOldModels, actualModels)

				// Verify old labels remain
				assert.Equal(t, "existing-value", cm.Labels["existing-label"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fake client with existing objects
			scheme := runtime.NewScheme()
			_ = corev1.AddToScheme(scheme)
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(tt.existingConfigMaps...).
				Build()

			provider := NewDefaultModelConfigMapProvider(fakeClient, zap.NewNop())

			// Execute
			err := provider.CreateModelConfigMap(context.Background(), tt.request)

			assert.NoError(t, err)
			tt.validateFunc(t, fakeClient, tt.request)
		})
	}
}

func TestGetModelConfigMap(t *testing.T) {
	tests := []struct {
		name               string
		request            GetModelConfigMapRequest
		existingConfigMaps []runtime.Object
		expectedResponse   []ModelConfigEntry
		expectError        bool
	}{
		{
			name: "get call on existing configmap with multiple models",
			request: GetModelConfigMapRequest{
				InferenceServer: "test-server",
				Namespace:       "default",
			},
			existingConfigMaps: []runtime.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-server-model-config",
						Namespace: "default",
					},
					Data: map[string]string{
						modelListKey: `[
  {
    "name": "model1",
    "storage_path": "s3://bucket/model1"
  },
  {
    "name": "model2",
    "storage_path": "s3://bucket/model2"
  }
]`,
					},
				},
			},
			expectedResponse: []ModelConfigEntry{
				{Name: "model1", StoragePath: "s3://bucket/model1"},
				{Name: "model2", StoragePath: "s3://bucket/model2"},
			},
			expectError: false,
		},
		{
			name: "get call on non-existent configmap",
			request: GetModelConfigMapRequest{
				InferenceServer: "non-existent-server",
				Namespace:       "default",
			},
			existingConfigMaps: []runtime.Object{},
			expectedResponse:   nil,
			expectError:        true,
		},
		{
			name: "get models from configmap with empty model list",
			request: GetModelConfigMapRequest{
				InferenceServer: "test-server",
				Namespace:       "default",
			},
			existingConfigMaps: []runtime.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-server-model-config",
						Namespace: "default",
					},
					Data: map[string]string{
						modelListKey: `[]`,
					},
				},
			},
			expectedResponse: []ModelConfigEntry{},
			expectError:      false,
		},
		{
			name: "get models from configmap with no model-list.json key",
			request: GetModelConfigMapRequest{
				InferenceServer: "test-server",
				Namespace:       "default",
			},
			existingConfigMaps: []runtime.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-server-model-config",
						Namespace: "default",
					},
					Data: map[string]string{
						"other-key": "other-value",
					},
				},
			},
			expectedResponse: []ModelConfigEntry{},
			expectError:      false,
		},
		{
			name: "get models from configmap with invalid json",
			request: GetModelConfigMapRequest{
				InferenceServer: "test-server",
				Namespace:       "default",
			},
			existingConfigMaps: []runtime.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-server-model-config",
						Namespace: "default",
					},
					Data: map[string]string{
						modelListKey: `invalid json`,
					},
				},
			},
			expectedResponse: nil,
			expectError:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fake client with existing objects
			scheme := runtime.NewScheme()
			_ = corev1.AddToScheme(scheme)
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(tt.existingConfigMaps...).
				Build()

			provider := NewDefaultModelConfigMapProvider(fakeClient, zap.NewNop())

			// Execute
			actualResponse, err := provider.GetModelsFromConfigMap(context.Background(), tt.request)

			// Assert
			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, actualResponse)
			} else {
				assert.NoError(t, err)
				// Compare entire response
				assert.Equal(t, tt.expectedResponse, actualResponse)
			}
		})
	}
}

func TestAddModelToConfigMap(t *testing.T) {
	tests := []struct {
		name               string
		request            AddModelToConfigMapRequest
		existingConfigMaps []runtime.Object
		expectError        bool
		validateFunc       func(t *testing.T, client client.Client, request AddModelToConfigMapRequest)
	}{
		{
			name: "add new model to existing configmap",
			request: AddModelToConfigMapRequest{
				InferenceServer: "test-server",
				Namespace:       "default",
				ModelConfig: ModelConfigEntry{
					Name:        "new-model",
					StoragePath: "s3://bucket/new-model",
				},
			},
			existingConfigMaps: []runtime.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-server-model-config",
						Namespace: "default",
					},
					Data: map[string]string{
						modelListKey: `[{"name":"existing-model","storage_path":"s3://bucket/existing-model"}]`,
					},
				},
			},
			expectError: false,
			validateFunc: func(t *testing.T, c client.Client, request AddModelToConfigMapRequest) {
				configMapName := addSuffixToString(request.InferenceServer, modelConfigSuffix)
				cm := &corev1.ConfigMap{}
				err := c.Get(context.Background(), client.ObjectKey{Name: configMapName, Namespace: request.Namespace}, cm)
				require.NoError(t, err)

				// Verify model was added
				modelListJSON, exists := cm.Data[modelListKey]
				assert.True(t, exists)

				var actualModels []ModelConfigEntry
				err = json.Unmarshal([]byte(modelListJSON), &actualModels)
				require.NoError(t, err)

				expectedModels := []ModelConfigEntry{
					{Name: "existing-model", StoragePath: "s3://bucket/existing-model"},
					{Name: "new-model", StoragePath: "s3://bucket/new-model"},
				}
				assert.Equal(t, expectedModels, actualModels)
			},
		},
		{
			name: "update existing model in configmap",
			request: AddModelToConfigMapRequest{
				InferenceServer: "test-server",
				Namespace:       "default",
				ModelConfig: ModelConfigEntry{
					Name:        "existing-model",
					StoragePath: "s3://bucket/updated-path",
				},
			},
			existingConfigMaps: []runtime.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-server-model-config",
						Namespace: "default",
					},
					Data: map[string]string{
						modelListKey: `[{"name":"existing-model","storage_path":"s3://bucket/old-path"}]`,
					},
				},
			},
			expectError: false,
			validateFunc: func(t *testing.T, c client.Client, request AddModelToConfigMapRequest) {
				configMapName := addSuffixToString(request.InferenceServer, modelConfigSuffix)
				cm := &corev1.ConfigMap{}
				err := c.Get(context.Background(), client.ObjectKey{Name: configMapName, Namespace: request.Namespace}, cm)
				require.NoError(t, err)

				// Verify model was updated (not duplicated)
				modelListJSON, exists := cm.Data[modelListKey]
				assert.True(t, exists)

				var actualModels []ModelConfigEntry
				err = json.Unmarshal([]byte(modelListJSON), &actualModels)
				require.NoError(t, err)

				expectedModels := []ModelConfigEntry{
					{Name: "existing-model", StoragePath: "s3://bucket/updated-path"},
				}
				assert.Equal(t, expectedModels, actualModels)
			},
		},
		{
			name: "add model to empty configmap",
			request: AddModelToConfigMapRequest{
				InferenceServer: "test-server",
				Namespace:       "default",
				ModelConfig: ModelConfigEntry{
					Name:        "first-model",
					StoragePath: "s3://bucket/first-model",
				},
			},
			existingConfigMaps: []runtime.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-server-model-config",
						Namespace: "default",
					},
					Data: map[string]string{
						modelListKey: `[]`,
					},
				},
			},
			expectError: false,
			validateFunc: func(t *testing.T, c client.Client, request AddModelToConfigMapRequest) {
				configMapName := addSuffixToString(request.InferenceServer, modelConfigSuffix)
				cm := &corev1.ConfigMap{}
				err := c.Get(context.Background(), client.ObjectKey{Name: configMapName, Namespace: request.Namespace}, cm)
				require.NoError(t, err)

				// Verify model was added
				modelListJSON, exists := cm.Data[modelListKey]
				assert.True(t, exists)

				var actualModels []ModelConfigEntry
				err = json.Unmarshal([]byte(modelListJSON), &actualModels)
				require.NoError(t, err)

				expectedModels := []ModelConfigEntry{
					{Name: "first-model", StoragePath: "s3://bucket/first-model"},
				}
				assert.Equal(t, expectedModels, actualModels)
			},
		},
		{
			name: "add model to non-existent configmap returns error",
			request: AddModelToConfigMapRequest{
				InferenceServer: "non-existent-server",
				Namespace:       "default",
				ModelConfig: ModelConfigEntry{
					Name:        "model",
					StoragePath: "s3://bucket/model",
				},
			},
			existingConfigMaps: []runtime.Object{},
			expectError:        true,
			validateFunc:       nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fake client with existing objects
			scheme := runtime.NewScheme()
			_ = corev1.AddToScheme(scheme)
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(tt.existingConfigMaps...).
				Build()

			provider := NewDefaultModelConfigMapProvider(fakeClient, zap.NewNop())

			// Execute
			err := provider.AddModelToConfigMap(context.Background(), tt.request)

			// Assert
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.validateFunc != nil {
					tt.validateFunc(t, fakeClient, tt.request)
				}
			}
		})
	}
}

func TestRemoveModelFromConfigMap(t *testing.T) {
	tests := []struct {
		name               string
		request            RemoveModelFromConfigMapRequest
		existingConfigMaps []runtime.Object
		expectError        bool
		validateFunc       func(t *testing.T, client client.Client, request RemoveModelFromConfigMapRequest)
	}{
		{
			name: "remove existing model from configmap",
			request: RemoveModelFromConfigMapRequest{
				InferenceServer: "test-server",
				Namespace:       "default",
				ModelName:       "model-to-remove",
			},
			existingConfigMaps: []runtime.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-server-model-config",
						Namespace: "default",
					},
					Data: map[string]string{
						modelListKey: `[
  {
    "name": "model-to-keep",
    "storage_path": "s3://bucket/model-to-keep"
  },
  {
    "name": "model-to-remove",
    "storage_path": "s3://bucket/model-to-remove"
  }
]`,
					},
				},
			},
			expectError: false,
			validateFunc: func(t *testing.T, c client.Client, request RemoveModelFromConfigMapRequest) {
				configMapName := addSuffixToString(request.InferenceServer, modelConfigSuffix)
				cm := &corev1.ConfigMap{}
				err := c.Get(context.Background(), client.ObjectKey{Name: configMapName, Namespace: request.Namespace}, cm)
				require.NoError(t, err)

				// Verify model was removed
				modelListJSON, exists := cm.Data[modelListKey]
				assert.True(t, exists)

				var actualModels []ModelConfigEntry
				err = json.Unmarshal([]byte(modelListJSON), &actualModels)
				require.NoError(t, err)

				expectedModels := []ModelConfigEntry{
					{Name: "model-to-keep", StoragePath: "s3://bucket/model-to-keep"},
				}
				assert.Equal(t, expectedModels, actualModels)
			},
		},
		{
			name: "remove non-existent model from configmap (no error, no change)",
			request: RemoveModelFromConfigMapRequest{
				InferenceServer: "test-server",
				Namespace:       "default",
				ModelName:       "non-existent-model",
			},
			existingConfigMaps: []runtime.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-server-model-config",
						Namespace: "default",
					},
					Data: map[string]string{
						modelListKey: `[{"name":"existing-model","storage_path":"s3://bucket/existing-model"}]`,
					},
				},
			},
			expectError: false,
			validateFunc: func(t *testing.T, c client.Client, request RemoveModelFromConfigMapRequest) {
				configMapName := addSuffixToString(request.InferenceServer, modelConfigSuffix)
				cm := &corev1.ConfigMap{}
				err := c.Get(context.Background(), client.ObjectKey{Name: configMapName, Namespace: request.Namespace}, cm)
				require.NoError(t, err)

				// Verify nothing was removed
				modelListJSON, exists := cm.Data[modelListKey]
				assert.True(t, exists)

				var actualModels []ModelConfigEntry
				err = json.Unmarshal([]byte(modelListJSON), &actualModels)
				require.NoError(t, err)

				expectedModels := []ModelConfigEntry{
					{Name: "existing-model", StoragePath: "s3://bucket/existing-model"},
				}
				assert.Equal(t, expectedModels, actualModels)
			},
		},
		{
			name: "remove last model from configmap (results in empty list)",
			request: RemoveModelFromConfigMapRequest{
				InferenceServer: "test-server",
				Namespace:       "default",
				ModelName:       "only-model",
			},
			existingConfigMaps: []runtime.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-server-model-config",
						Namespace: "default",
					},
					Data: map[string]string{
						modelListKey: `[{"name":"only-model","storage_path":"s3://bucket/only-model"}]`,
					},
				},
			},
			expectError: false,
			validateFunc: func(t *testing.T, c client.Client, request RemoveModelFromConfigMapRequest) {
				configMapName := addSuffixToString(request.InferenceServer, modelConfigSuffix)
				cm := &corev1.ConfigMap{}
				err := c.Get(context.Background(), client.ObjectKey{Name: configMapName, Namespace: request.Namespace}, cm)
				require.NoError(t, err)

				// Verify list is now empty
				modelListJSON, exists := cm.Data[modelListKey]
				assert.True(t, exists)

				var actualModels []ModelConfigEntry
				err = json.Unmarshal([]byte(modelListJSON), &actualModels)
				require.NoError(t, err)

				assert.Empty(t, actualModels)
			},
		},
		{
			name: "remove model from non-existent configmap returns error",
			request: RemoveModelFromConfigMapRequest{
				InferenceServer: "non-existent-server",
				Namespace:       "default",
				ModelName:       "model",
			},
			existingConfigMaps: []runtime.Object{},
			expectError:        true,
			validateFunc:       nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fake client with existing objects
			scheme := runtime.NewScheme()
			_ = corev1.AddToScheme(scheme)
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(tt.existingConfigMaps...).
				Build()

			provider := NewDefaultModelConfigMapProvider(fakeClient, zap.NewNop())

			// Execute
			err := provider.RemoveModelFromConfigMap(context.Background(), tt.request)

			// Assert
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.validateFunc != nil {
					tt.validateFunc(t, fakeClient, tt.request)
				}
			}
		})
	}
}

func TestDeleteModelConfigMap(t *testing.T) {
	tests := []struct {
		name               string
		request            DeleteModelConfigMapRequest
		existingConfigMaps []runtime.Object
		expectError        bool
		validateFunc       func(t *testing.T, client client.Client, request DeleteModelConfigMapRequest)
	}{
		{
			name: "delete call on existing configmap",
			request: DeleteModelConfigMapRequest{
				InferenceServer: "test-server",
				Namespace:       "default",
			},
			existingConfigMaps: []runtime.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-server-model-config",
						Namespace: "default",
					},
					Data: map[string]string{
						modelListKey: `[{"name":"model","storage_path":"s3://bucket/model"}]`,
					},
				},
			},
			expectError: false,
			validateFunc: func(t *testing.T, c client.Client, request DeleteModelConfigMapRequest) {
				configMapName := addSuffixToString(request.InferenceServer, modelConfigSuffix)
				cm := &corev1.ConfigMap{}
				err := c.Get(context.Background(), client.ObjectKey{Name: configMapName, Namespace: request.Namespace}, cm)
				// ConfigMap should not exist after deletion
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "not found")
			},
		},
		{
			name: "delete non-existent configmap returns error",
			request: DeleteModelConfigMapRequest{
				InferenceServer: "non-existent-server",
				Namespace:       "default",
			},
			existingConfigMaps: []runtime.Object{},
			expectError:        true,
			validateFunc:       nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fake client with existing objects
			scheme := runtime.NewScheme()
			_ = corev1.AddToScheme(scheme)
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(tt.existingConfigMaps...).
				Build()

			provider := NewDefaultModelConfigMapProvider(fakeClient, zap.NewNop())

			// Execute
			err := provider.DeleteModelConfigMap(context.Background(), tt.request)

			// Assert
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.validateFunc != nil {
					tt.validateFunc(t, fakeClient, tt.request)
				}
			}
		})
	}
}
