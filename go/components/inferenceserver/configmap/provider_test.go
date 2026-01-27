package configmap

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/clientfactory/clientfactorymocks"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

var testCluster = &v2pb.ClusterTarget{ClusterId: "test-cluster"}

func TestCreateModelConfigMap(t *testing.T) {
	tests := []struct {
		name               string
		inferenceServer    string
		namespace          string
		modelConfigs       []ModelConfigEntry
		labels             map[string]string
		annotations        map[string]string
		existingConfigMaps []runtime.Object
		expectError        bool
		validateFunc       func(t *testing.T, client client.Client, inferenceServer string, namespace string, modelConfigs []ModelConfigEntry, annotations map[string]string)
	}{
		{
			name:            "create call on new configmap",
			inferenceServer: "test-server",
			namespace:       "default",
			modelConfigs: []ModelConfigEntry{
				{Name: "model1", StoragePath: "s3://bucket/model1"},
				{Name: "model2", StoragePath: "s3://bucket/model2"},
			},
			labels: map[string]string{
				"custom-label": "custom-value",
			},
			annotations: map[string]string{
				"custom-annotation": "annotation-value",
			},
			existingConfigMaps: []runtime.Object{},
			validateFunc: func(t *testing.T, c client.Client, inferenceServer string, namespace string, modelConfigs []ModelConfigEntry, annotations map[string]string) {
				configMapName := generateConfigMapName(inferenceServer)
				cm := &corev1.ConfigMap{}
				err := c.Get(context.Background(), client.ObjectKey{Name: configMapName, Namespace: namespace}, cm)
				require.NoError(t, err)

				// Verify the configmap was created with correct name and namespace
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

				// Verify model-list.json data
				modelListJSON, exists := cm.Data[modelListKey]
				assert.True(t, exists)

				var actualModels []ModelConfigEntry
				err = json.Unmarshal([]byte(modelListJSON), &actualModels)
				require.NoError(t, err)
				assert.Equal(t, modelConfigs, actualModels)
			},
		},
		{
			name:            "create call on configmap that already exists",
			inferenceServer: "existing-server",
			namespace:       "default",
			modelConfigs: []ModelConfigEntry{
				{Name: "new-model", StoragePath: "s3://bucket/new-model"},
			},
			labels:      nil,
			annotations: nil,
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
			validateFunc: func(t *testing.T, c client.Client, inferenceServer string, namespace string, modelConfigs []ModelConfigEntry, annotations map[string]string) {
				configMapName := generateConfigMapName(inferenceServer)
				cm := &corev1.ConfigMap{}
				err := c.Get(context.Background(), client.ObjectKey{Name: configMapName, Namespace: namespace}, cm)
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
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			// Create fake client with existing objects
			scheme := runtime.NewScheme()
			_ = corev1.AddToScheme(scheme)
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(tt.existingConfigMaps...).
				Build()

			mockFactory := clientfactorymocks.NewMockClientFactory(ctrl)
			mockFactory.EXPECT().GetClient(gomock.Any(), testCluster).Return(fakeClient, nil)

			provider := NewDefaultModelConfigMapProvider(fakeClient, mockFactory, zap.NewNop())

			// Execute
			err := provider.CreateModelConfigMap(context.Background(), tt.inferenceServer, tt.namespace, tt.modelConfigs, tt.labels, tt.annotations, testCluster)

			assert.NoError(t, err)
			tt.validateFunc(t, fakeClient, tt.inferenceServer, tt.namespace, tt.modelConfigs, tt.annotations)
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
			name:            "get call on existing configmap with multiple models",
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
			name:               "get call on non-existent configmap",
			inferenceServer:    "non-existent-server",
			namespace:          "default",
			existingConfigMaps: []runtime.Object{},
			expectedResponse:   nil,
			expectError:        true,
		},
		{
			name:            "get models from configmap with empty model list",
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
			name:            "get models from configmap with no model-list.json key",
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
			name:            "get models from configmap with invalid json",
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
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			// Create fake client with existing objects
			scheme := runtime.NewScheme()
			_ = corev1.AddToScheme(scheme)
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(tt.existingConfigMaps...).
				Build()

			mockFactory := clientfactorymocks.NewMockClientFactory(ctrl)
			mockFactory.EXPECT().GetClient(gomock.Any(), testCluster).Return(fakeClient, nil)

			provider := NewDefaultModelConfigMapProvider(fakeClient, mockFactory, zap.NewNop())

			// Execute
			actualResponse, err := provider.GetModelsFromConfigMap(context.Background(), tt.inferenceServer, tt.namespace, testCluster)

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
		inferenceServer    string
		namespace          string
		modelConfig        ModelConfigEntry
		existingConfigMaps []runtime.Object
		expectError        bool
		validateFunc       func(t *testing.T, client client.Client, inferenceServer string, namespace string)
	}{
		{
			name:            "add new model to existing configmap",
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
			name:            "update existing model in configmap",
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
			name:            "add model to empty configmap",
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
			name:            "add model to non-existent configmap returns error",
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
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			// Create fake client with existing objects
			scheme := runtime.NewScheme()
			_ = corev1.AddToScheme(scheme)
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(tt.existingConfigMaps...).
				Build()

			mockFactory := clientfactorymocks.NewMockClientFactory(ctrl)
			mockFactory.EXPECT().GetClient(gomock.Any(), testCluster).Return(fakeClient, nil)

			provider := NewDefaultModelConfigMapProvider(fakeClient, mockFactory, zap.NewNop())

			// Execute
			err := provider.AddModelToConfigMap(context.Background(), tt.inferenceServer, tt.namespace, tt.modelConfig, testCluster)

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
			name:            "remove existing model from configmap",
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
			name:            "remove non-existent model from configmap (no error, no change)",
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
			name:            "remove last model from configmap (results in empty list)",
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
			name:               "remove model from non-existent configmap returns error",
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
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			// Create fake client with existing objects
			scheme := runtime.NewScheme()
			_ = corev1.AddToScheme(scheme)
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(tt.existingConfigMaps...).
				Build()

			mockFactory := clientfactorymocks.NewMockClientFactory(ctrl)
			mockFactory.EXPECT().GetClient(gomock.Any(), testCluster).Return(fakeClient, nil)

			provider := NewDefaultModelConfigMapProvider(fakeClient, mockFactory, zap.NewNop())

			// Execute
			err := provider.RemoveModelFromConfigMap(context.Background(), tt.inferenceServer, tt.namespace, tt.modelName, testCluster)

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
			name:            "delete call on existing configmap",
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
			name:               "delete non-existent configmap returns error",
			inferenceServer:    "non-existent-server",
			namespace:          "default",
			existingConfigMaps: []runtime.Object{},
			expectError:        true,
			validateFunc:       nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			// Create fake client with existing objects
			scheme := runtime.NewScheme()
			_ = corev1.AddToScheme(scheme)
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(tt.existingConfigMaps...).
				Build()

			mockFactory := clientfactorymocks.NewMockClientFactory(ctrl)
			mockFactory.EXPECT().GetClient(gomock.Any(), testCluster).Return(fakeClient, nil)

			provider := NewDefaultModelConfigMapProvider(fakeClient, mockFactory, zap.NewNop())

			// Execute
			err := provider.DeleteModelConfigMap(context.Background(), tt.inferenceServer, tt.namespace, testCluster)

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
