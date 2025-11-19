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

	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
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
				BackendType:     v2pb.BACKEND_TYPE_TRITON,
				ModelConfigs: []ModelConfigEntry{
					{Name: "model1", S3Path: "s3://bucket/model1"},
					{Name: "model2", S3Path: "s3://bucket/model2"},
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
					"michelangelo.ai/backend-type":     request.BackendType.String(),
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
				BackendType:     v2pb.BACKEND_TYPE_TRITON,
				ModelConfigs: []ModelConfigEntry{
					{Name: "new-model", S3Path: "s3://bucket/new-model"},
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
						modelListKey: `[{"name":"old-model","s3_path":"s3://bucket/old-model"}]`,
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
					{Name: "old-model", S3Path: "s3://bucket/old-model"},
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

func TestUpdateModelConfigMap(t *testing.T) {
	tests := []struct {
		name               string
		request            UpdateModelConfigMapRequest
		existingConfigMaps []runtime.Object
		expectError        bool
		validateFunc       func(t *testing.T, client client.Client, request UpdateModelConfigMapRequest)
	}{
		{
			name: "update call on non-existent configmap",
			request: UpdateModelConfigMapRequest{
				InferenceServer: "non-existent-server",
				Namespace:       "default",
				BackendType:     v2pb.BACKEND_TYPE_TRITON,
				ModelConfigs: []ModelConfigEntry{
					{Name: "model", S3Path: "s3://bucket/model"},
				},
				Labels:      nil,
				Annotations: nil,
			},
			existingConfigMaps: []runtime.Object{},
			expectError:        true,
			validateFunc:       nil,
		},
		{
			name: "update call on configmap without labels and model list",
			request: UpdateModelConfigMapRequest{
				InferenceServer: "no-update-server",
				Namespace:       "default",
				BackendType:     v2pb.BACKEND_TYPE_TRITON,
				ModelConfigs:    nil, // No model configs provided
				Labels:          nil, // No labels provided
				Annotations:     nil,
			},
			existingConfigMaps: []runtime.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "no-update-server-model-config",
						Namespace: "default",
						Labels: map[string]string{
							"app.kubernetes.io/component": "model-config",
							"original-label":              "original-value",
						},
					},
					Data: map[string]string{
						modelListKey: `[{"name":"existing-model","s3_path":"s3://bucket/existing"}]`,
					},
				},
			},
			expectError: false,
			validateFunc: func(t *testing.T, c client.Client, request UpdateModelConfigMapRequest) {
				configMapName := addSuffixToString(request.InferenceServer, modelConfigSuffix)
				cm := &corev1.ConfigMap{}
				err := c.Get(context.Background(), client.ObjectKey{Name: configMapName, Namespace: request.Namespace}, cm)
				require.NoError(t, err)

				// Verify labels unchanged
				assert.Equal(t, "original-value", cm.Labels["original-label"])
				assert.Equal(t, "model-config", cm.Labels["app.kubernetes.io/component"])

				// Verify model list unchanged (kept existing models)
				modelListJSON, exists := cm.Data[modelListKey]
				assert.True(t, exists)

				var actualModels []ModelConfigEntry
				err = json.Unmarshal([]byte(modelListJSON), &actualModels)
				require.NoError(t, err)
				expectedModels := []ModelConfigEntry{
					{Name: "existing-model", S3Path: "s3://bucket/existing"},
				}
				assert.Equal(t, expectedModels, actualModels)
			},
		},
		{
			name: "update call on configmap with labels only",
			request: UpdateModelConfigMapRequest{
				InferenceServer: "label-update-server",
				Namespace:       "default",
				BackendType:     v2pb.BACKEND_TYPE_TRITON,
				ModelConfigs:    nil, // No model configs provided
				Labels: map[string]string{
					"new-label":     "new-value",
					"another-label": "another-value",
				},
				Annotations: nil,
			},
			existingConfigMaps: []runtime.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "label-update-server-model-config",
						Namespace: "default",
						Labels: map[string]string{
							"app.kubernetes.io/component": "model-config",
							"old-label":                   "old-value",
						},
					},
					Data: map[string]string{
						modelListKey: `[{"name":"existing-model","s3_path":"s3://bucket/existing"}]`,
					},
				},
			},
			expectError: false,
			validateFunc: func(t *testing.T, c client.Client, request UpdateModelConfigMapRequest) {
				configMapName := addSuffixToString(request.InferenceServer, modelConfigSuffix)
				cm := &corev1.ConfigMap{}
				err := c.Get(context.Background(), client.ObjectKey{Name: configMapName, Namespace: request.Namespace}, cm)
				require.NoError(t, err)

				// Verify labels were updated (new labels added, old labels preserved)
				assert.Equal(t, "new-value", cm.Labels["new-label"])
				assert.Equal(t, "another-value", cm.Labels["another-label"])
				assert.Equal(t, "old-value", cm.Labels["old-label"])
				assert.Equal(t, "model-config", cm.Labels["app.kubernetes.io/component"])

				// Verify model list unchanged (kept existing models)
				modelListJSON, exists := cm.Data[modelListKey]
				assert.True(t, exists)

				var actualModels []ModelConfigEntry
				err = json.Unmarshal([]byte(modelListJSON), &actualModels)
				require.NoError(t, err)
				expectedModels := []ModelConfigEntry{
					{Name: "existing-model", S3Path: "s3://bucket/existing"},
				}
				assert.Equal(t, expectedModels, actualModels)
			},
		},
		{
			name: "update call on configmap with model list only",
			request: UpdateModelConfigMapRequest{
				InferenceServer: "model-update-server",
				Namespace:       "default",
				BackendType:     v2pb.BACKEND_TYPE_LLM_D,
				ModelConfigs: []ModelConfigEntry{
					{Name: "updated-model1", S3Path: "s3://bucket/updated1"},
					{Name: "updated-model2", S3Path: "s3://bucket/updated2"},
				},
				Labels:      nil,
				Annotations: nil,
			},
			existingConfigMaps: []runtime.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "model-update-server-model-config",
						Namespace: "default",
						Labels: map[string]string{
							"app.kubernetes.io/component": "model-config",
							"original-label":              "original-value",
						},
					},
					Data: map[string]string{
						modelListKey: `[{"name":"old-model","s3_path":"s3://bucket/old"}]`,
					},
				},
			},
			expectError: false,
			validateFunc: func(t *testing.T, c client.Client, request UpdateModelConfigMapRequest) {
				configMapName := addSuffixToString(request.InferenceServer, modelConfigSuffix)
				cm := &corev1.ConfigMap{}
				err := c.Get(context.Background(), client.ObjectKey{Name: configMapName, Namespace: request.Namespace}, cm)
				require.NoError(t, err)

				// Verify labels unchanged
				assert.Equal(t, "original-value", cm.Labels["original-label"])
				assert.Equal(t, "model-config", cm.Labels["app.kubernetes.io/component"])

				// Verify updated model list
				modelListJSON, exists := cm.Data[modelListKey]
				assert.True(t, exists)

				var actualModels []ModelConfigEntry
				err = json.Unmarshal([]byte(modelListJSON), &actualModels)
				require.NoError(t, err)
				assert.Equal(t, request.ModelConfigs, actualModels)
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
			err := provider.UpdateModelConfigMap(context.Background(), tt.request)

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
    "s3_path": "s3://bucket/model1"
  },
  {
    "name": "model2",
    "s3_path": "s3://bucket/model2"
  }
]`,
					},
				},
			},
			expectedResponse: []ModelConfigEntry{
				{Name: "model1", S3Path: "s3://bucket/model1"},
				{Name: "model2", S3Path: "s3://bucket/model2"},
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
			actualResponse, err := provider.GetModelConfigMap(context.Background(), tt.request)

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
						modelListKey: `[{"name":"model","s3_path":"s3://bucket/model"}]`,
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
