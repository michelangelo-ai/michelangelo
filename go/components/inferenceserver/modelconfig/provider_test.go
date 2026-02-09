package modelconfig

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
		inferenceServer    string
		namespace          string
		labels             map[string]string
		annotations        map[string]string
		existingConfigMaps []runtime.Object
		expectError        bool
		validateFunc       func(t *testing.T, client client.Client, inferenceServer string, namespace string, annotations map[string]string)
	}{
		{
			name:            "create call on new modelconfig",
			inferenceServer: "test-server",
			namespace:       "default",
			labels: map[string]string{
				"custom-label": "custom-value",
			},
			annotations: map[string]string{
				"custom-annotation": "annotation-value",
			},
			existingConfigMaps: []runtime.Object{},
			validateFunc: func(t *testing.T, c client.Client, inferenceServer string, namespace string, annotations map[string]string) {
				configMapName := generateConfigMapName(inferenceServer)
				cm := &corev1.ConfigMap{}
				err := c.Get(context.Background(), client.ObjectKey{Name: configMapName, Namespace: namespace}, cm)
				require.NoError(t, err)

				// Verify the modelconfig was created with correct name and namespace
				assert.Equal(t, configMapName, cm.Name)
				assert.Equal(t, namespace, cm.Namespace)

				// Verify labels
				expectedLabels := map[string]string{
					"app.kubernetes.io/component":      "model-config",
					"app.kubernetes.io/part-of":        "michelangelo",
					"michelangelo.ai/inference-server": inferenceServer,
					"custom-label":                     "custom-value",
				}
				assert.Equal(t, expectedLabels, cm.Labels)

				// Verify annotations
				assert.Equal(t, annotations, cm.Annotations)
			},
		},
		{
			name:            "create call on modelconfig that already exists",
			inferenceServer: "existing-server",
			namespace:       "default",
			labels:          nil,
			annotations:     nil,
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
			validateFunc: func(t *testing.T, c client.Client, inferenceServer string, namespace string, annotations map[string]string) {
				configMapName := generateConfigMapName(inferenceServer)
				cm := &corev1.ConfigMap{}
				err := c.Get(context.Background(), client.ObjectKey{Name: configMapName, Namespace: namespace}, cm)
				require.NoError(t, err)

				// Verify the modelconfig was NOT modified (old data should remain)
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

			provider := NewDefaultModelConfigProvider()

			// Execute
			err := provider.CreateModelConfig(context.Background(), zap.NewNop(), fakeClient, tt.inferenceServer, tt.namespace, tt.labels, tt.annotations)

			assert.NoError(t, err)
			tt.validateFunc(t, fakeClient, tt.inferenceServer, tt.namespace, tt.annotations)
		})
	}
}

func TestGetModelConfigMap(t *testing.T) {
	tests := []struct {
		name               string
		inferenceServer    string
		namespace          string
		existingConfigMaps []runtime.Object
		expectedResponse   []ModelConfigEntry
		expectError        bool
	}{
		{
			name:            "get call on existing modelconfig with multiple models",
			inferenceServer: "test-server",
			namespace:       "default",
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
			name:               "get call on non-existent modelconfig",
			inferenceServer:    "non-existent-server",
			namespace:          "default",
			existingConfigMaps: []runtime.Object{},
			expectedResponse:   nil,
			expectError:        true,
		},
		{
			name:            "get models from modelconfig with empty model list",
			inferenceServer: "test-server",
			namespace:       "default",
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
			name:            "get models from modelconfig with no model-list.json key",
			inferenceServer: "test-server",
			namespace:       "default",
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
			name:            "get models from modelconfig with invalid json",
			inferenceServer: "test-server",
			namespace:       "default",
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

			provider := NewDefaultModelConfigProvider()

			// Execute
			actualResponse, err := provider.GetModelsFromConfig(context.Background(), zap.NewNop(), fakeClient, tt.inferenceServer, tt.namespace)

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

func TestAddModelToConfig(t *testing.T) {
	tests := []struct {
		name               string
		inferenceServer    string
		namespace          string
		modelConfig        ModelConfigEntry
		existingConfigMaps []runtime.Object
		expectError        bool
		validateFunc       func(t *testing.T, client client.Client, inferenceServer string, namespace string)
	}{
		{
			name:            "add new model to existing modelconfig",
			inferenceServer: "test-server",
			namespace:       "default",
			modelConfig: ModelConfigEntry{
				Name:        "new-model",
				StoragePath: "s3://bucket/new-model",
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
			validateFunc: func(t *testing.T, c client.Client, inferenceServer string, namespace string) {
				configMapName := generateConfigMapName(inferenceServer)
				cm := &corev1.ConfigMap{}
				err := c.Get(context.Background(), client.ObjectKey{Name: configMapName, Namespace: namespace}, cm)
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
			name:            "update existing model in modelconfig",
			inferenceServer: "test-server",
			namespace:       "default",
			modelConfig: ModelConfigEntry{
				Name:        "existing-model",
				StoragePath: "s3://bucket/updated-path",
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
			validateFunc: func(t *testing.T, c client.Client, inferenceServer string, namespace string) {
				configMapName := generateConfigMapName(inferenceServer)
				cm := &corev1.ConfigMap{}
				err := c.Get(context.Background(), client.ObjectKey{Name: configMapName, Namespace: namespace}, cm)
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
			name:            "add model to empty modelconfig",
			inferenceServer: "test-server",
			namespace:       "default",
			modelConfig: ModelConfigEntry{
				Name:        "first-model",
				StoragePath: "s3://bucket/first-model",
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
			validateFunc: func(t *testing.T, c client.Client, inferenceServer string, namespace string) {
				configMapName := generateConfigMapName(inferenceServer)
				cm := &corev1.ConfigMap{}
				err := c.Get(context.Background(), client.ObjectKey{Name: configMapName, Namespace: namespace}, cm)
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
			name:            "add model to non-existent modelconfig returns error",
			inferenceServer: "non-existent-server",
			namespace:       "default",
			modelConfig: ModelConfigEntry{
				Name:        "model",
				StoragePath: "s3://bucket/model",
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

			provider := NewDefaultModelConfigProvider()

			// Execute
			err := provider.AddModelToConfig(context.Background(), zap.NewNop(), fakeClient, tt.inferenceServer, tt.namespace, tt.modelConfig)

			// Assert
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.validateFunc != nil {
					tt.validateFunc(t, fakeClient, tt.inferenceServer, tt.namespace)
				}
			}
		})
	}
}

func TestRemoveModelFromConfigMap(t *testing.T) {
	tests := []struct {
		name               string
		inferenceServer    string
		namespace          string
		modelName          string
		existingConfigMaps []runtime.Object
		expectError        bool
		validateFunc       func(t *testing.T, client client.Client, inferenceServer string, namespace string)
	}{
		{
			name:            "remove existing model from modelconfig",
			inferenceServer: "test-server",
			namespace:       "default",
			modelName:       "model-to-remove",
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
			validateFunc: func(t *testing.T, c client.Client, inferenceServer string, namespace string) {
				configMapName := generateConfigMapName(inferenceServer)
				cm := &corev1.ConfigMap{}
				err := c.Get(context.Background(), client.ObjectKey{Name: configMapName, Namespace: namespace}, cm)
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
			name:            "remove non-existent model from modelconfig (no error, no change)",
			inferenceServer: "test-server",
			namespace:       "default",
			modelName:       "non-existent-model",
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
			validateFunc: func(t *testing.T, c client.Client, inferenceServer string, namespace string) {
				configMapName := generateConfigMapName(inferenceServer)
				cm := &corev1.ConfigMap{}
				err := c.Get(context.Background(), client.ObjectKey{Name: configMapName, Namespace: namespace}, cm)
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
			name:            "remove last model from modelconfig (results in empty list)",
			inferenceServer: "test-server",
			namespace:       "default",
			modelName:       "only-model",
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
			validateFunc: func(t *testing.T, c client.Client, inferenceServer string, namespace string) {
				configMapName := generateConfigMapName(inferenceServer)
				cm := &corev1.ConfigMap{}
				err := c.Get(context.Background(), client.ObjectKey{Name: configMapName, Namespace: namespace}, cm)
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
			name:               "remove model from non-existent modelconfig returns error",
			inferenceServer:    "non-existent-server",
			namespace:          "default",
			modelName:          "model",
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

			provider := NewDefaultModelConfigProvider()

			// Execute
			err := provider.RemoveModelFromConfig(context.Background(), zap.NewNop(), fakeClient, tt.inferenceServer, tt.namespace, tt.modelName)

			// Assert
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.validateFunc != nil {
					tt.validateFunc(t, fakeClient, tt.inferenceServer, tt.namespace)
				}
			}
		})
	}
}

func TestDeleteModelConfigMap(t *testing.T) {
	tests := []struct {
		name               string
		inferenceServer    string
		namespace          string
		existingConfigMaps []runtime.Object
		expectError        bool
		validateFunc       func(t *testing.T, client client.Client, inferenceServer string, namespace string)
	}{
		{
			name:            "delete call on existing modelconfig",
			inferenceServer: "test-server",
			namespace:       "default",
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
			validateFunc: func(t *testing.T, c client.Client, inferenceServer string, namespace string) {
				configMapName := generateConfigMapName(inferenceServer)
				cm := &corev1.ConfigMap{}
				err := c.Get(context.Background(), client.ObjectKey{Name: configMapName, Namespace: namespace}, cm)
				// ConfigMap should not exist after deletion
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "not found")
			},
		},
		{
			name:               "delete non-existent modelconfig returns error",
			inferenceServer:    "non-existent-server",
			namespace:          "default",
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

			provider := NewDefaultModelConfigProvider()

			// Execute
			err := provider.DeleteModelConfig(context.Background(), zap.NewNop(), fakeClient, tt.inferenceServer, tt.namespace)

			// Assert
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.validateFunc != nil {
					tt.validateFunc(t, fakeClient, tt.inferenceServer, tt.namespace)
				}
			}
		})
	}
}
