package crd

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/michelangelo-ai/michelangelo/go/api/crd/crdmocks"
	"github.com/stretchr/testify/assert"
	"go.uber.org/config"
	"go.uber.org/fx"
	"go.uber.org/zap"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiExtFake "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestUpsertCRDs(t *testing.T) {
	logger, err := zap.NewDevelopment()
	assert.NoError(t, err)

	t.Run("Test register new CRD", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		crdGatewayMock := crdmocks.NewMockGateway(ctrl)
		crdGatewayMock.EXPECT().ConditionalUpsert(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(1)

		ctx := context.Background()
		crd, err := readCRDFromFile(testCRDManifestDir + "/project.pb.yaml")
		assert.NotNil(t, crd)
		assert.NoError(t, err)
		err = upsertCRDs(ctx, logger, crdGatewayMock, []*apiextv1.CustomResourceDefinition{crd}, false)
		assert.NoError(t, err)
	})

	t.Run("Test retry on conflict error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		crdGatewayMock := crdmocks.NewMockGateway(ctrl)
		updateConflict := crdGatewayMock.EXPECT().ConditionalUpsert(gomock.Any(), gomock.Any(), gomock.Any()).Return(k8sErrors.NewConflict(schema.GroupResource{}, "test", nil))
		updateSuccess := crdGatewayMock.EXPECT().ConditionalUpsert(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
		gomock.InOrder(updateConflict, updateSuccess)

		ctx := context.Background()
		crd, err := readCRDFromFile(testCRDManifestDir + "/project.pb.yaml")
		assert.NotNil(t, crd)
		assert.NoError(t, err)
		err = upsertCRDs(ctx, logger, crdGatewayMock, []*apiextv1.CustomResourceDefinition{crd}, false)
		assert.NoError(t, err)
	})

	t.Run("Test not retry on permanent error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		crdGatewayMock := crdmocks.NewMockGateway(ctrl)
		crdGatewayMock.EXPECT().ConditionalUpsert(gomock.Any(), gomock.Any(), gomock.Any()).Return(fmt.Errorf("permanent error")).Times(1)

		ctx := context.Background()
		crd, err := readCRDFromFile(testCRDManifestDir + "/project.pb.yaml")
		assert.NotNil(t, crd)
		assert.NoError(t, err)
		err = upsertCRDs(ctx, logger, crdGatewayMock, []*apiextv1.CustomResourceDefinition{crd}, false)
		assert.Error(t, err)
	})
}

func TestMergeCRDVersions(t *testing.T) {
	t.Run("Test merge CRD versions with version config", func(t *testing.T) {
		crd1, err := readCRDFromFile(testCRDManifestDir + "/project.pb.yaml")
		assert.NotNil(t, crd1)
		assert.NoError(t, err)

		crd2, err := readCRDFromFile(testCRDManifestDir + "/project.pb.yaml")
		assert.NotNil(t, crd2)
		assert.NoError(t, err)
		crd2.Spec.Versions[0].Name = "v1"

		crdVersions := map[string]VersionConfig{
			"projects.test": {
				Versions:       []string{"v0", "v1"},
				StorageVersion: "v0",
			},
		}

		mergedCRDs, err := mergeCRDVersions([]*apiextv1.CustomResourceDefinition{crd1, crd2}, crdVersions)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(mergedCRDs))
		assert.Equal(t, 2, len(mergedCRDs[0].Spec.Versions))
		assert.True(t, mergedCRDs[0].Spec.Versions[0].Storage)
		assert.False(t, mergedCRDs[0].Spec.Versions[1].Storage)
	})

	t.Run("Test merge CRD versions without version config", func(t *testing.T) {
		crd, err := readCRDFromFile(testCRDManifestDir + "/project.pb.yaml")
		assert.NotNil(t, crd)
		assert.NoError(t, err)

		mergedCRDs, err := mergeCRDVersions([]*apiextv1.CustomResourceDefinition{crd}, nil)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(mergedCRDs))
		assert.Equal(t, 1, len(mergedCRDs[0].Spec.Versions))
		assert.True(t, mergedCRDs[0].Spec.Versions[0].Storage)
	})

	t.Run("Test merge CRD versions with multiple versions without storage version", func(t *testing.T) {
		crd1, err := readCRDFromFile(testCRDManifestDir + "/project.pb.yaml")
		assert.NotNil(t, crd1)
		assert.NoError(t, err)

		crd2, err := readCRDFromFile(testCRDManifestDir + "/project.pb.yaml")
		assert.NotNil(t, crd2)
		assert.NoError(t, err)
		crd2.Spec.Versions[0].Name = "v1"

		crdVersions := map[string]VersionConfig{
			"projects.test": {
				Versions: []string{"v0", "v1"},
			},
		}

		_, err = mergeCRDVersions([]*apiextv1.CustomResourceDefinition{crd1, crd2}, crdVersions)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "CRD projects.test has multiple versions, but no storageVersion specified in crdVersions")
	})

	t.Run("Test merge CRD versions with missing version", func(t *testing.T) {
		crd, err := readCRDFromFile(testCRDManifestDir + "/project.pb.yaml")
		assert.NotNil(t, crd)
		assert.NoError(t, err)

		crdVersions := map[string]VersionConfig{
			"projects.test": {
				Versions:       []string{"v0", "v1"},
				StorageVersion: "v0",
			},
		}

		_, err = mergeCRDVersions([]*apiextv1.CustomResourceDefinition{crd}, crdVersions)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "version v1 of CRD projects.test is specified in crdVersions, but does not exist not in crdList")
	})

	t.Run("Test merge CRD versions with invalid storage version", func(t *testing.T) {
		crd1, err := readCRDFromFile(testCRDManifestDir + "/project.pb.yaml")
		assert.NotNil(t, crd1)
		assert.NoError(t, err)

		crd2, err := readCRDFromFile(testCRDManifestDir + "/project.pb.yaml")
		assert.NotNil(t, crd2)
		assert.NoError(t, err)
		crd2.Spec.Versions[0].Name = "v1"

		crdVersions := map[string]VersionConfig{
			"projects.test": {
				Versions:       []string{"v0", "v1"},
				StorageVersion: "v2",
			},
		}

		_, err = mergeCRDVersions([]*apiextv1.CustomResourceDefinition{crd1, crd2}, crdVersions)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "CRD projects.test does not have the specified storage version v2")
	})

	t.Run("Test merge CRD versions with multiple versions without version config", func(t *testing.T) {
		crd1, err := readCRDFromFile(testCRDManifestDir + "/project.pb.yaml")
		assert.NotNil(t, crd1)
		assert.NoError(t, err)

		crd2, err := readCRDFromFile(testCRDManifestDir + "/project.pb.yaml")
		assert.NotNil(t, crd2)
		assert.NoError(t, err)
		crd2.Spec.Versions[0].Name = "v1"

		_, err = mergeCRDVersions([]*apiextv1.CustomResourceDefinition{crd1, crd2}, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "CRD projects.test has multiple versions but version config specified in crdVersions")
	})
}

func TestCheckCRDsFx(t *testing.T) {
	t.Run("Test CheckCRDs with valid config", func(t *testing.T) {
		app := fx.New(
			fx.Provide(func() *zap.Logger {
				return zap.NewNop()
			}),
			fx.Provide(func() *Configuration {
				return &Configuration{
					EnableCRDUpdate: true,
				}
			}),
			fx.Provide(func() Gateway {
				ctrl := gomock.NewController(t)
				mockGateway := crdmocks.NewMockGateway(ctrl)
				mockGateway.EXPECT().List(gomock.Any()).Return(&apiextv1.CustomResourceDefinitionList{}, nil).AnyTimes()
				mockGateway.EXPECT().ConditionalUpsert(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockGateway
			}),
			CheckCRDs("test", map[string]string{
				"Project": "apiVersion: apiextensions.k8s.io/v1\nkind: CustomResourceDefinition\nmetadata:\n  name: projects.test\nspec:\n  group: test\n  names:\n    kind: Project\n    plural: projects\n  scope: Namespaced\n  versions:\n  - name: v0\n    served: true\n    storage: true\n    schema:\n      openAPIV3Schema:\n        type: object\n        properties:\n          spec:\n            type: object\n            properties:\n              meta:\n                type: object\n                properties:\n                  testProperty:\n                    type: string\n",
			}),
		)
		assert.NoError(t, app.Start(context.Background()))
		assert.NoError(t, app.Stop(context.Background()))
	})

	t.Run("Test CheckCRDs with disabled CRD update", func(t *testing.T) {
		app := fx.New(
			fx.Provide(func() *zap.Logger {
				return zap.NewNop()
			}),
			fx.Provide(func() *Configuration {
				return &Configuration{
					EnableCRDUpdate: false,
				}
			}),
			fx.Provide(func() Gateway {
				ctrl := gomock.NewController(t)
				mockGateway := crdmocks.NewMockGateway(ctrl)
				mockGateway.EXPECT().List(gomock.Any()).Return(&apiextv1.CustomResourceDefinitionList{}, nil).AnyTimes()
				return mockGateway
			}),
			CheckCRDs("test", map[string]string{
				"Project": "apiVersion: apiextensions.k8s.io/v1\nkind: CustomResourceDefinition\nmetadata:\n  name: projects.test\nspec:\n  group: test\n  names:\n    kind: Project\n    plural: projects\n  scope: Namespaced\n  versions:\n  - name: v0\n    served: true\n    storage: true\n    schema:\n      openAPIV3Schema:\n        type: object\n        properties:\n          spec:\n            type: object\n            properties:\n              meta:\n                type: object\n                properties:\n                  testProperty:\n                    type: string\n",
			}),
		)
		assert.NoError(t, app.Start(context.Background()))
		assert.NoError(t, app.Stop(context.Background()))
	})

	t.Run("Test CheckCRDs with invalid YAML", func(t *testing.T) {
		app := fx.New(
			fx.Provide(func() *zap.Logger {
				return zap.NewNop()
			}),
			fx.Provide(func() *Configuration {
				return &Configuration{
					EnableCRDUpdate: true,
				}
			}),
			fx.Provide(func() Gateway {
				ctrl := gomock.NewController(t)
				mockGateway := crdmocks.NewMockGateway(ctrl)
				mockGateway.EXPECT().List(gomock.Any()).Return(&apiextv1.CustomResourceDefinitionList{}, nil).AnyTimes()
				return mockGateway
			}),
			CheckCRDs("test", map[string]string{
				"Project": "invalid yaml",
			}),
		)
		assert.NoError(t, app.Start(context.Background()))
		assert.NoError(t, app.Stop(context.Background()))
	})

	t.Run("Test CheckCRDs with wrong group", func(t *testing.T) {
		app := fx.New(
			fx.Provide(func() *zap.Logger {
				return zap.NewNop()
			}),
			fx.Provide(func() *Configuration {
				return &Configuration{
					EnableCRDUpdate: true,
				}
			}),
			fx.Provide(func() Gateway {
				ctrl := gomock.NewController(t)
				mockGateway := crdmocks.NewMockGateway(ctrl)
				mockGateway.EXPECT().List(gomock.Any()).Return(&apiextv1.CustomResourceDefinitionList{}, nil).AnyTimes()
				return mockGateway
			}),
			CheckCRDs("test", map[string]string{
				"Project": "apiVersion: apiextensions.k8s.io/v1\nkind: CustomResourceDefinition\nmetadata:\n  name: projects.wronggroup\nspec:\n  group: wronggroup\n  names:\n    kind: Project\n    plural: projects\n  scope: Namespaced\n  versions:\n  - name: v0\n    served: true\n    storage: true\n    schema:\n      openAPIV3Schema:\n        type: object\n        properties:\n          spec:\n            type: object\n            properties:\n              meta:\n                type: object\n                properties:\n                  testProperty:\n                    type: string\n",
			}),
		)
		assert.NoError(t, app.Start(context.Background()))
		assert.NoError(t, app.Stop(context.Background()))
	})

	t.Run("Test CheckCRDs with empty schemas", func(t *testing.T) {
		app := fx.New(
			fx.Provide(func() *zap.Logger {
				return zap.NewNop()
			}),
			fx.Provide(func() *Configuration {
				return &Configuration{
					EnableCRDUpdate: true,
				}
			}),
			fx.Provide(func() Gateway {
				ctrl := gomock.NewController(t)
				mockGateway := crdmocks.NewMockGateway(ctrl)
				mockGateway.EXPECT().List(gomock.Any()).Return(&apiextv1.CustomResourceDefinitionList{}, nil).AnyTimes()
				return mockGateway
			}),
			CheckCRDs("test", map[string]string{}),
		)
		assert.NoError(t, app.Start(context.Background()))
		assert.NoError(t, app.Stop(context.Background()))
	})

	t.Run("Test CheckCRDs with multiple schema maps", func(t *testing.T) {
		app := fx.New(
			fx.Provide(func() *zap.Logger {
				return zap.NewNop()
			}),
			fx.Provide(func() *Configuration {
				return &Configuration{
					EnableCRDUpdate: true,
				}
			}),
			fx.Provide(func() Gateway {
				ctrl := gomock.NewController(t)
				mockGateway := crdmocks.NewMockGateway(ctrl)
				mockGateway.EXPECT().List(gomock.Any()).Return(&apiextv1.CustomResourceDefinitionList{}, nil).AnyTimes()
				return mockGateway
			}),
			CheckCRDs("test", map[string]string{}, map[string]string{}),
		)
		assert.NoError(t, app.Start(context.Background()))
		assert.NoError(t, app.Stop(context.Background()))
	})
}

func TestCheckCRDs(t *testing.T) {
	logger := zap.Must(zap.NewDevelopment())
	apiExtClientStub := apiExtFake.NewSimpleClientset()
	crdGateway := gateway{
		logger:        logger,
		apiExtClient:  apiExtClientStub,
		dynamicClient: fakeClientWithResource,
	}

	// new CRD
	crdYaml, err := os.ReadFile(testCRDManifestDir + "/project.pb.yaml")
	assert.NoError(t, err)
	err = checkCRDs(context.Background(), logger, "test",
		false, true, &crdGateway, nil, map[string]string{"Project": string(crdYaml)})
	assert.NoError(t, err)

	// incompatible change
	crdYaml, err = os.ReadFile(testCRDManifestDir + "/project_delete_props.pb.yaml")
	assert.NoError(t, err)
	err = checkCRDs(context.Background(), logger, "test",
		false, true, &crdGateway, nil, map[string]string{"Project": string(crdYaml)})
	assert.Error(t, err, "failed to update CRD. Schema is incompatible, and there are existing instances. Abort updating CRD projects.test")

	// fail to list existing CRDs
	mockGateway := crdmocks.NewMockGateway(gomock.NewController(t))
	mockGateway.EXPECT().List(gomock.Any()).Return(nil, fmt.Errorf("test error"))
	err = checkCRDs(context.Background(), logger, "test",
		false, true, mockGateway, nil, map[string]string{"Project": string(crdYaml)})
	assert.Error(t, err, "failed to list existing CRDs: test error")

	// do not update CRDs that are not in the specified groups
	err = checkCRDs(context.Background(), logger, "test1",
		false, true, &crdGateway, nil, map[string]string{"Project": string(crdYaml)})
	assert.Error(t, err, "CRD projects.test is not in the specified group test1")
}

func TestParseConfig(t *testing.T) {
	yamlConfStr :=
		`
apiserver:
  crdCheck:
    enableCRDUpdate: true
    enableIncompatibleUpdate: false
    enableCRDDeletion: true
    crdVersions:
      project:
        versions: [v2]
        storageVersion: v2
`

	provider, err := config.NewYAMLProviderFromBytes([]byte(yamlConfStr))
	assert.NoError(t, err)

	conf, err := ParseConfig(provider)
	assert.NoError(t, err)
	assert.True(t, conf.EnableCRDUpdate)
	assert.False(t, conf.EnableIncompatibleUpdate)
	assert.True(t, conf.EnableCRDDeletion)
	assert.Equal(t, 1, len(conf.CRDVersions))
	assert.Equal(t, []string{"v2"}, conf.CRDVersions["project"].Versions)
	assert.Equal(t, "v2", conf.CRDVersions["project"].StorageVersion)
}

func TestParseConfig_Empty(t *testing.T) {
	yamlConfStr :=
		`
apiserver:
  crdCheck:
    enableCRDUpdate: false
    enableIncompatibleUpdate: false
    enableCRDDeletion: false
`

	provider, err := config.NewYAMLProviderFromBytes([]byte(yamlConfStr))
	assert.NoError(t, err)

	conf, err := ParseConfig(provider)
	assert.NoError(t, err)
	assert.False(t, conf.EnableCRDUpdate)
	assert.False(t, conf.EnableIncompatibleUpdate)
	assert.False(t, conf.EnableCRDDeletion)
	assert.Equal(t, 0, len(conf.CRDVersions))
}

func TestParseConfig_WithCRDVersions(t *testing.T) {
	yamlConfStr :=
		`
apiserver:
  crdCheck:
    enableCRDUpdate: true
    enableIncompatibleUpdate: false
    enableCRDDeletion: true
    crdVersions:
      project:
        versions: [v1, v2]
        storageVersion: v2
      pipeline:
        versions: [v1]
        storageVersion: v1
`

	provider, err := config.NewYAMLProviderFromBytes([]byte(yamlConfStr))
	assert.NoError(t, err)

	conf, err := ParseConfig(provider)
	assert.NoError(t, err)
	assert.True(t, conf.EnableCRDUpdate)
	assert.False(t, conf.EnableIncompatibleUpdate)
	assert.True(t, conf.EnableCRDDeletion)
	assert.Equal(t, 2, len(conf.CRDVersions))

	projectConfig := conf.CRDVersions["project"]
	assert.Equal(t, []string{"v1", "v2"}, projectConfig.Versions)
	assert.Equal(t, "v2", projectConfig.StorageVersion)

	pipelineConfig := conf.CRDVersions["pipeline"]
	assert.Equal(t, []string{"v1"}, pipelineConfig.Versions)
	assert.Equal(t, "v1", pipelineConfig.StorageVersion)
}
