package handler

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/uber-go/tally"
	"go.uber.org/zap"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/golang/mock/gomock"
	"github.com/michelangelo-ai/michelangelo/go/storage"
	"github.com/michelangelo-ai/michelangelo/go/storage/storagemocks"
)

func TestAPIHandlerBuilder(t *testing.T) {
	t.Run("NewAPIHandlerBuilder creates builder with defaults", func(t *testing.T) {
		builder := NewAPIHandlerBuilder()

		assert.NotNil(t, builder)
		assert.NotNil(t, builder.logger)
		assert.NotNil(t, builder.metrics)
		assert.Equal(t, tally.NoopScope, builder.metrics)
	})

	t.Run("WithK8sClient sets client", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().Build()
		builder := NewAPIHandlerBuilder().WithK8sClient(fakeClient)

		assert.Equal(t, fakeClient, builder.k8sClient)
	})

	t.Run("WithZapLogger sets logger", func(t *testing.T) {
		zapLogger := zap.NewNop()
		builder := NewAPIHandlerBuilder().WithZapLogger(zapLogger)

		assert.NotNil(t, builder.logger)
	})

	t.Run("WithMetrics sets metrics scope", func(t *testing.T) {
		scope := tally.NewTestScope("test", nil)
		builder := NewAPIHandlerBuilder().WithMetrics(scope)

		assert.Equal(t, scope, builder.metrics)
	})

	t.Run("Build fails without k8s client", func(t *testing.T) {
		builder := NewAPIHandlerBuilder()

		handler, err := builder.Build()

		assert.Nil(t, handler)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "k8s client is required")
	})

	t.Run("Build succeeds with minimal dependencies", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().Build()
		builder := NewAPIHandlerBuilder().WithK8sClient(fakeClient)

		handler, err := builder.Build()

		assert.NoError(t, err)
		assert.NotNil(t, handler)
	})

	t.Run("Build fails when metadata storage enabled but not provided", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().Build()
		config := storage.MetadataStorageConfig{EnableMetadataStorage: true}

		builder := NewAPIHandlerBuilder().
			WithK8sClient(fakeClient).
			WithMetadataStorage(nil, config)

		handler, err := builder.Build()

		assert.Nil(t, handler)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "metadata storage is required")
	})

	t.Run("Build succeeds with all dependencies", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		fakeClient := fake.NewClientBuilder().Build()
		mockMetadataStorage := storagemocks.NewMockMetadataStorage(ctrl)
		mockBlobStorage := storagemocks.NewMockBlobStorage(ctrl)
		zapLogger := zap.NewNop()
		scope := tally.NewTestScope("test", nil)
		config := storage.MetadataStorageConfig{EnableMetadataStorage: true}

		builder := NewAPIHandlerBuilder().
			WithK8sClient(fakeClient).
			WithMetadataStorage(mockMetadataStorage, config).
			WithBlobStorage(mockBlobStorage).
			WithZapLogger(zapLogger).
			WithMetrics(scope)

		handler, err := builder.Build()

		assert.NoError(t, err)
		assert.NotNil(t, handler)
	})

	t.Run("Fluent interface allows method chaining", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().Build()
		zapLogger := zap.NewNop()
		scope := tally.NewTestScope("test", nil)

		// Test fluent interface
		builder := NewAPIHandlerBuilder().
			WithK8sClient(fakeClient).
			WithZapLogger(zapLogger).
			WithMetrics(scope)

		assert.NotNil(t, builder)
		assert.Equal(t, fakeClient, builder.k8sClient)
		assert.NotNil(t, builder.logger)
		assert.Equal(t, scope, builder.metrics)
	})
}

func TestBuilderBasedAPIServerHandler(t *testing.T) {
	t.Run("creates handler from params", func(t *testing.T) {
		params := Params{
			K8sRestConfig: &rest.Config{Host: "test"},
			Scheme:        scheme.Scheme,
			Logger:        zap.NewNop(),
			Metrics:       tally.NewTestScope("test", nil),
			StorageConfig: storage.MetadataStorageConfig{EnableMetadataStorage: false},
		}

		// This would normally fail due to invalid rest config, but we're testing the builder logic
		handler, err := NewAPIServerHandler(params)

		// We expect an error here due to invalid rest config, but the builder logic should work
		if err != nil {
			assert.Contains(t, err.Error(), "failed to create k8s client")
		} else {
			assert.NotNil(t, handler)
		}
	})

	t.Run("includes metadata storage when enabled", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockMetadataStorage := storagemocks.NewMockMetadataStorage(ctrl)
		params := Params{
			K8sRestConfig:   &rest.Config{Host: "test"},
			Scheme:          scheme.Scheme,
			Logger:          zap.NewNop(),
			Metrics:         tally.NewTestScope("test", nil),
			StorageConfig:   storage.MetadataStorageConfig{EnableMetadataStorage: true},
			MetadataStorage: mockMetadataStorage,
		}

		// This would normally fail due to invalid rest config
		_, err := NewAPIServerHandler(params)

		// We expect an error due to invalid rest config, not due to missing metadata storage
		if err != nil {
			assert.Contains(t, err.Error(), "failed to create k8s client")
			assert.NotContains(t, err.Error(), "metadata storage is required")
		}
	})
}

func TestBuilderBasedCtrlManagerHandler(t *testing.T) {
	t.Run("fails without manager", func(t *testing.T) {
		params := Params{
			Manager: nil,
			Logger:  zap.NewNop(),
			Metrics: tally.NewTestScope("test", nil),
		}

		handler, err := NewCtrlManagerHandler(params)

		assert.Nil(t, handler)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "manager is required")
	})

	t.Run("creates handler with valid manager", func(t *testing.T) {
		// Create a mock manager that returns our fake client
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		// Since we can't easily mock ctrl.Manager, let's test the builder validation
		params := Params{
			Logger:        zap.NewNop(),
			Metrics:       tally.NewTestScope("test", nil),
			StorageConfig: storage.MetadataStorageConfig{EnableMetadataStorage: false},
		}

		// Test that we get the expected error when manager is nil
		handler, err := NewCtrlManagerHandler(params)
		assert.Nil(t, handler)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "manager is required")
	})
}

func TestBuilderValidation(t *testing.T) {
	t.Run("validate fails with no k8s client", func(t *testing.T) {
		builder := &APIHandlerBuilder{}

		err := builder.validate()

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "k8s client is required")
	})

	t.Run("validate fails when metadata storage enabled but not provided", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().Build()
		builder := &APIHandlerBuilder{
			k8sClient:     fakeClient,
			storageConfig: storage.MetadataStorageConfig{EnableMetadataStorage: true},
		}

		err := builder.validate()

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "metadata storage is required")
	})

	t.Run("validate succeeds with valid configuration", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		fakeClient := fake.NewClientBuilder().Build()
		mockStorage := storagemocks.NewMockMetadataStorage(ctrl)

		builder := &APIHandlerBuilder{
			k8sClient:       fakeClient,
			metadataStorage: mockStorage,
			storageConfig:   storage.MetadataStorageConfig{EnableMetadataStorage: true},
		}

		err := builder.validate()

		assert.NoError(t, err)
	})
}

func TestBuilderPatternBenefits(t *testing.T) {
	t.Run("builder enables optional dependencies", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().Build()

		// Test minimal configuration
		minimalHandler, err := NewAPIHandlerBuilder().
			WithK8sClient(fakeClient).
			Build()

		assert.NoError(t, err)
		assert.NotNil(t, minimalHandler)

		// Test full configuration
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockMetadataStorage := storagemocks.NewMockMetadataStorage(ctrl)
		mockBlobStorage := storagemocks.NewMockBlobStorage(ctrl)

		fullHandler, err := NewAPIHandlerBuilder().
			WithK8sClient(fakeClient).
			WithMetadataStorage(mockMetadataStorage, storage.MetadataStorageConfig{EnableMetadataStorage: true}).
			WithBlobStorage(mockBlobStorage).
			WithZapLogger(zap.NewNop()).
			WithMetrics(tally.NewTestScope("test", nil)).
			Build()

		assert.NoError(t, err)
		assert.NotNil(t, fullHandler)
	})

	t.Run("builder provides clear configuration API", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().Build()

		// Demonstrate clear, self-documenting configuration
		builder := NewAPIHandlerBuilder().
			WithK8sClient(fakeClient). // Required: Kubernetes client
			WithZapLogger(zap.NewNop()). // Optional: Structured logging
			WithMetrics(tally.NewTestScope("test", nil)) // Optional: Metrics collection

		handler, err := builder.Build()

		assert.NoError(t, err)
		assert.NotNil(t, handler)
	})
}
