package controllermgr

import (
	"context"
	"testing"
	"time"

	"github.com/michelangelo-ai/michelangelo/go/api/crd/crdmocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"go.uber.org/config"
	"go.uber.org/zap"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPerformSchemaComparison(t *testing.T) {
	logger := zap.NewNop()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockGateway := crdmocks.NewMockGateway(ctrl)

	t.Run("successful schema comparison", func(t *testing.T) {
		// Mock CRD list response
		serverCRDs := &apiextv1.CustomResourceDefinitionList{
			Items: []apiextv1.CustomResourceDefinition{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "projects.test",
					},
					Spec: apiextv1.CustomResourceDefinitionSpec{
						Group: "test",
						Names: apiextv1.CustomResourceDefinitionNames{
							Kind:   "Project",
							Plural: "projects",
						},
						Versions: []apiextv1.CustomResourceDefinitionVersion{
							{
								Name:    "v2",
								Served:  true,
								Storage: true,
							},
						},
					},
				},
			},
		}

		mockGateway.EXPECT().List(gomock.Any()).Return(serverCRDs, nil)

		ctx := context.Background()
		performSchemaComparison(ctx, logger, mockGateway)
	})

	t.Run("gateway list error", func(t *testing.T) {
		mockGateway.EXPECT().List(gomock.Any()).Return(nil, assert.AnError)

		ctx := context.Background()
		performSchemaComparison(ctx, logger, mockGateway)
	})
}

func TestCompareSchemasWithServerList(t *testing.T) {
	logger := zap.NewNop()

	t.Run("compare local and server schemas", func(t *testing.T) {
		// Create local schemas
		localSchemas := map[string]*apiextv1.CustomResourceDefinition{
			"projects.test": {
				ObjectMeta: metav1.ObjectMeta{
					Name: "projects.test",
				},
				Spec: apiextv1.CustomResourceDefinitionSpec{
					Group: "test",
					Names: apiextv1.CustomResourceDefinitionNames{
						Kind:   "Project",
						Plural: "projects",
					},
					Versions: []apiextv1.CustomResourceDefinitionVersion{
						{
							Name:    "v2",
							Served:  true,
							Storage: true,
						},
					},
				},
			},
		}

		// Create server CRDs
		serverCRDs := &apiextv1.CustomResourceDefinitionList{
			Items: []apiextv1.CustomResourceDefinition{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "projects.test",
					},
					Spec: apiextv1.CustomResourceDefinitionSpec{
						Group: "test",
						Names: apiextv1.CustomResourceDefinitionNames{
							Kind:   "Project",
							Plural: "projects",
						},
						Versions: []apiextv1.CustomResourceDefinitionVersion{
							{
								Name:    "v2",
								Served:  true,
								Storage: true,
							},
						},
					},
				},
			},
		}

		ctx := context.Background()
		err := compareSchemasWithServerList(ctx, logger, localSchemas, serverCRDs)
		assert.NoError(t, err)
	})

	t.Run("missing CRD on server", func(t *testing.T) {
		localSchemas := map[string]*apiextv1.CustomResourceDefinition{
			"projects.test": {
				ObjectMeta: metav1.ObjectMeta{
					Name: "projects.test",
				},
				Spec: apiextv1.CustomResourceDefinitionSpec{
					Group: "test",
					Names: apiextv1.CustomResourceDefinitionNames{
						Kind:   "Project",
						Plural: "projects",
					},
				},
			},
		}

		serverCRDs := &apiextv1.CustomResourceDefinitionList{
			Items: []apiextv1.CustomResourceDefinition{},
		}

		ctx := context.Background()
		err := compareSchemasWithServerList(ctx, logger, localSchemas, serverCRDs)
		assert.NoError(t, err)
	})

	t.Run("extra CRD on server", func(t *testing.T) {
		localSchemas := map[string]*apiextv1.CustomResourceDefinition{}

		serverCRDs := &apiextv1.CustomResourceDefinitionList{
			Items: []apiextv1.CustomResourceDefinition{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "projects.test",
					},
					Spec: apiextv1.CustomResourceDefinitionSpec{
						Group: "test",
						Names: apiextv1.CustomResourceDefinitionNames{
							Kind:   "Project",
							Plural: "projects",
						},
					},
				},
			},
		}

		ctx := context.Background()
		err := compareSchemasWithServerList(ctx, logger, localSchemas, serverCRDs)
		assert.NoError(t, err)
	})
}

func TestCompareAndLogDifferences(t *testing.T) {
	logger := zap.NewNop()

	t.Run("identical CRDs", func(t *testing.T) {
		crd := &apiextv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name: "projects.test",
			},
			Spec: apiextv1.CustomResourceDefinitionSpec{
				Group: "test",
				Names: apiextv1.CustomResourceDefinitionNames{
					Kind:   "Project",
					Plural: "projects",
				},
				Versions: []apiextv1.CustomResourceDefinitionVersion{
					{
						Name:    "v2",
						Served:  true,
						Storage: true,
					},
				},
			},
		}

		compareAndLogDifferences(logger, "projects.test", crd, crd)
	})

	t.Run("different group", func(t *testing.T) {
		localCRD := &apiextv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name: "projects.test",
			},
			Spec: apiextv1.CustomResourceDefinitionSpec{
				Group: "test",
				Names: apiextv1.CustomResourceDefinitionNames{
					Kind:   "Project",
					Plural: "projects",
				},
			},
		}

		serverCRD := &apiextv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name: "projects.test",
			},
			Spec: apiextv1.CustomResourceDefinitionSpec{
				Group: "different",
				Names: apiextv1.CustomResourceDefinitionNames{
					Kind:   "Project",
					Plural: "projects",
				},
			},
		}

		compareAndLogDifferences(logger, "projects.test", localCRD, serverCRD)
	})
}

func TestCompareVersions(t *testing.T) {
	logger := zap.NewNop()

	t.Run("identical versions", func(t *testing.T) {
		versions := []apiextv1.CustomResourceDefinitionVersion{
			{
				Name:    "v2",
				Served:  true,
				Storage: true,
			},
		}

		hasDifferences := compareVersions(logger, "projects.test", versions, versions)
		assert.False(t, hasDifferences)
	})

	t.Run("missing version on server", func(t *testing.T) {
		localVersions := []apiextv1.CustomResourceDefinitionVersion{
			{
				Name:    "v2",
				Served:  true,
				Storage: true,
			},
		}

		serverVersions := []apiextv1.CustomResourceDefinitionVersion{}

		hasDifferences := compareVersions(logger, "projects.test", localVersions, serverVersions)
		assert.True(t, hasDifferences)
	})

	t.Run("extra version on server", func(t *testing.T) {
		localVersions := []apiextv1.CustomResourceDefinitionVersion{}

		serverVersions := []apiextv1.CustomResourceDefinitionVersion{
			{
				Name:    "v2",
				Served:  true,
				Storage: true,
			},
		}

		hasDifferences := compareVersions(logger, "projects.test", localVersions, serverVersions)
		assert.True(t, hasDifferences)
	})

	t.Run("different served property", func(t *testing.T) {
		localVersions := []apiextv1.CustomResourceDefinitionVersion{
			{
				Name:    "v2",
				Served:  true,
				Storage: true,
			},
		}

		serverVersions := []apiextv1.CustomResourceDefinitionVersion{
			{
				Name:    "v2",
				Served:  false,
				Storage: true,
			},
		}

		hasDifferences := compareVersions(logger, "projects.test", localVersions, serverVersions)
		assert.True(t, hasDifferences)
	})
}

func TestStartCRDCheck(t *testing.T) {
	logger := zap.NewNop()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockGateway := crdmocks.NewMockGateway(ctrl)

	t.Run("start CRD check service", func(t *testing.T) {
		config := &CRDCheckConfig{
			CheckInterval: 1 * time.Second,
		}

		params := CRDCheckParams{
			Config:  config,
			Logger:  logger,
			Gateway: mockGateway,
		}

		err := startCRDCheck(params)
		assert.NoError(t, err)
	})
}

func TestNewCRDCheckConfig(t *testing.T) {
	t.Run("default configuration", func(t *testing.T) {
		provider, err := config.NewYAMLProviderFromBytes([]byte(`
crdCheck:
  checkInterval: 5m
`))
		assert.NoError(t, err)

		config, err := newCRDCheckConfig(provider)
		assert.NoError(t, err)
		assert.Equal(t, 5*time.Minute, config.CheckInterval)
	})

	t.Run("custom interval", func(t *testing.T) {
		provider, err := config.NewYAMLProviderFromBytes([]byte(`
crdCheck:
  checkInterval: 10m
`))
		assert.NoError(t, err)

		config, err := newCRDCheckConfig(provider)
		assert.NoError(t, err)
		assert.Equal(t, 10*time.Minute, config.CheckInterval)
	})

	t.Run("missing configuration", func(t *testing.T) {
		provider, err := config.NewYAMLProviderFromBytes([]byte(`{}`))
		assert.NoError(t, err)

		config, err := newCRDCheckConfig(provider)
		assert.NoError(t, err)
		assert.Equal(t, 5*time.Minute, config.CheckInterval) // default value
	})
}
