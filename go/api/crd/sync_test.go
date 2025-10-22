package crd

import (
	"context"
	"fmt"
	"os"
	"strings"
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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
		err = upsertCRDs(ctx, logger, crdGatewayMock, []*apiextv1.CustomResourceDefinition{crd}, []string{})
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
		err = upsertCRDs(ctx, logger, crdGatewayMock, []*apiextv1.CustomResourceDefinition{crd}, []string{})
		assert.NoError(t, err)
	})

	t.Run("Test not retry on permanent error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		crdGatewayMock := crdmocks.NewMockGateway(ctrl)
		crdGatewayMock.EXPECT().ConditionalUpsert(gomock.Any(), gomock.Any(), gomock.Any()).Return(fmt.Errorf("test error"))

		ctx := context.Background()
		crd, err := readCRDFromFile(testCRDManifestDir + "/project.pb.yaml")
		assert.NotNil(t, crd)
		assert.NoError(t, err)
		err = upsertCRDs(ctx, logger, crdGatewayMock, []*apiextv1.CustomResourceDefinition{crd}, []string{})
		assert.Error(t, err)
	})
}

func TestSyncCRDsFx(t *testing.T) {
	ctx := context.Background()
	logger, err := zap.NewDevelopment()
	assert.NoError(t, err)
	crdYaml, err := os.ReadFile(testCRDManifestDir + "/project.pb.yaml")
	assert.NoError(t, err)

	apiExtClientStub := apiExtFake.NewSimpleClientset()
	crdGateway := gateway{
		logger:        logger,
		apiExtClient:  apiExtClientStub,
		dynamicClient: fakeClientNoResource,
	}

	// noop when EnableCRDUpdate is set to false
	mockGateway := crdmocks.MockGateway{} // no call is expected
	app := fx.New(fx.Options(
		fx.Provide(func() *zap.Logger {
			return logger
		}),
		fx.Provide(func() Gateway {
			return &mockGateway
		}),
		fx.Provide(func() *Configuration {
			return &Configuration{
				EnableCRDUpdate:          false,
			}
		}),
		SyncCRDs("test", []string{}, map[string]string{
			"Project": string(crdYaml),
		}),
	),
	)
	assert.NoError(t, app.Err())
	err = app.Start(ctx)
	assert.NoError(t, err)

	// create a new CRD
	app = fx.New(fx.Options(
		fx.Provide(func() *zap.Logger {
			return logger
		}),
		fx.Provide(func() Gateway {
			return &crdGateway
		}),
		fx.Provide(func() *Configuration {
			return &Configuration{
				EnableCRDUpdate:          true,
			}
		}),
		SyncCRDs("test", []string{}, map[string]string{
			"Project": string(crdYaml),
		}),
	),
	)
	assert.NoError(t, app.Err())
	err = app.Start(ctx)
	assert.NoError(t, err)
	crds, err := crdGateway.List(ctx)
	assert.NoError(t, err)
	assert.Len(t, crds.Items, 1)
	assert.Equal(t, "test", crds.Items[0].Spec.Group)
	assert.Equal(t, "v0", crds.Items[0].Spec.Versions[0].Name)
	assert.Equal(t, "Project", crds.Items[0].Spec.Names.Kind)

	// no CRD change
	app = fx.New(fx.Options(
		fx.Provide(func() *zap.Logger {
			return logger
		}),
		fx.Provide(func() Gateway {
			return &crdGateway
		}),
		fx.Provide(func() *Configuration {
			return &Configuration{
				EnableCRDUpdate:          true,
				EnableCRDDeletion:        true,
			}
		}),
		SyncCRDs("test", []string{}, map[string]string{
			"Project": string(crdYaml),
		}),
	),
	)
	assert.NoError(t, app.Err())
	err = app.Start(ctx)
	assert.NoError(t, err)
	crds, err = crdGateway.List(ctx)
	assert.NoError(t, err)
	assert.True(t, len(crds.Items) == 1)
	assert.Equal(t, "test", crds.Items[0].Spec.Group)
	assert.Equal(t, "v0", crds.Items[0].Spec.Versions[0].Name)
	assert.Equal(t, "Project", crds.Items[0].Spec.Names.Kind)

	// delete a CRD while there are instances of the CRD in the cluster
	crdGateway.dynamicClient = fakeClientWithResource

	// CRD is not deleted when EnableCRDDeletion is set to false
	app = fx.New(fx.Options(
		fx.Provide(func() *zap.Logger {
			return logger
		}),
		fx.Provide(func() Gateway {
			return &crdGateway
		}),
		fx.Provide(func() *Configuration {
			return &Configuration{
				EnableCRDUpdate:          true,
				EnableCRDDeletion:        false,
			}
		}),
		fx.Invoke(func() error {
			return nil
		}),
		SyncCRDs("test", []string{}, map[string]string{}),
	),
	)
	assert.NoError(t, app.Err())
	err = app.Start(ctx)
	assert.NoError(t, err)
	crds, err = crdGateway.List(ctx)
	assert.NoError(t, err)
	assert.Len(t, crds.Items, 1)
	assert.Equal(t, "Project", crds.Items[0].Spec.Names.Kind)

	// expect error when deleting CRD with existing instances
	app = fx.New(fx.Options(
		fx.Provide(func() *zap.Logger {
			return logger
		}),
		fx.Provide(func() Gateway {
			return &crdGateway
		}),
		fx.Provide(func() *Configuration {
			return &Configuration{
				EnableCRDUpdate:          true,
				EnableCRDDeletion:        true,
			}
		}),
		fx.Invoke(func() error {
			return nil
		}),
		SyncCRDs("test", []string{}, map[string]string{}),
	),
	)
	assert.NoError(t, app.Err())
	err = app.Start(ctx)
	assert.Error(t, err, "failed to delete CRD projects.test. There are existing resources")

	// successfully delete the CRD after the instances are removed
	crdGateway.dynamicClient = fakeClientNoResource

	app = fx.New(fx.Options(
		fx.Provide(func() *zap.Logger {
			return logger
		}),
		fx.Provide(func() Gateway {
			return &crdGateway
		}),
		fx.Provide(func() *Configuration {
			return &Configuration{
				EnableCRDUpdate:          true,
				EnableCRDDeletion:        true,
			}
		}),
		fx.Invoke(func() error {
			return nil
		}),
		SyncCRDs("test", []string{}, map[string]string{}),
	),
	)
	assert.NoError(t, app.Err())
	err = app.Start(ctx)
	assert.NoError(t, err)
	crds, err = crdGateway.List(ctx)
	assert.NoError(t, err)
	assert.True(t, len(crds.Items) == 0)
}

func TestSyncCRDs(t *testing.T) {
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
	err = syncCRDs(context.Background(), logger, "test",
		true, &crdGateway, nil, []string{}, map[string]string{"Project": string(crdYaml)})
	assert.NoError(t, err)

	// incompatible change - should be blocked with empty allowlist
	crdYaml, err = os.ReadFile(testCRDManifestDir + "/project_delete_props.pb.yaml")
	assert.NoError(t, err)
	err = syncCRDs(context.Background(), logger, "test",
		true, &crdGateway, nil, []string{}, map[string]string{"Project": string(crdYaml)})
	assert.Error(t, err, "failed to update CRD. Schema is incompatible, and there are existing instances. Abort updating CRD projects.test")

	// same incompatible change - should be allowed when CRD is in allowlist
	err = syncCRDs(context.Background(), logger, "test",
		true, &crdGateway, nil, []string{"projects.test"}, map[string]string{"Project": string(crdYaml)})
	assert.NoError(t, err, "incompatible update should be allowed when CRD is in allowlist")

	// fail to list existing CRDs
	mockGateway := crdmocks.NewMockGateway(gomock.NewController(t))
	mockGateway.EXPECT().List(gomock.Any()).Return(nil, fmt.Errorf("test error"))
	err = syncCRDs(context.Background(), logger, "test",
		true, mockGateway, nil, []string{}, map[string]string{"Project": string(crdYaml)})
	assert.Error(t, err, "failed to list existing CRDs: test error")

	// do not update CRDs that are not in the specified groups
	err = syncCRDs(context.Background(), logger, "test1",
		true, &crdGateway, nil, []string{}, map[string]string{"Project": string(crdYaml)})
	assert.Error(t, err, "CRD projects.test is not in the specified group test1")
}

func TestParseConfig(t *testing.T) {
	yamlConfStr := `
apiserver:
  crdSync:
    enableCRDUpdate: true
`
	provider, err := config.NewYAML(config.Source(strings.NewReader(yamlConfStr)))
	assert.NoError(t, err)

	conf, err := ParseConfig(provider)
	assert.NoError(t, err)
	assert.True(t, conf.EnableCRDUpdate)

	// Parse configuration with CRD versions
	yamlConfStr2 := `
apiserver:
  crdSync:
    enableCRDUpdate: true
    crdVersions:
      project:
        versions: [v1, v2]
        storageVersion: v2
`
	provider2, err := config.NewYAML(config.Source(strings.NewReader(yamlConfStr2)))
	assert.NoError(t, err)

	conf2, err := ParseConfig(provider2)
	assert.NoError(t, err)
	assert.True(t, conf2.EnableCRDUpdate)
	assert.Equal(t, conf2.CRDVersions, map[string]VersionConfig{
		"project": {
			Versions:       []string{"v1", "v2"},
			StorageVersion: "v2",
		},
	})

	invalidConfStr := `
apiserver:
  crdSync:
    enableCRDUpdat: true
`
	provider3, err := config.NewYAML(config.Source(strings.NewReader(invalidConfStr)))
	assert.NoError(t, err)
	_, err2 := ParseConfig(provider3)
	assert.Error(t, err2)
}

func TestMergeCRDVersions(t *testing.T) {
	// Helper function to create a test CRD with a specific version
	createCRD := func(name, version string, storage bool) *apiextv1.CustomResourceDefinition {
		return &apiextv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
			Spec: apiextv1.CustomResourceDefinitionSpec{
				Group: "test.example.com",
				Names: apiextv1.CustomResourceDefinitionNames{
					Kind: strings.Title(name),
				},
				Versions: []apiextv1.CustomResourceDefinitionVersion{
					{
						Name:    version,
						Storage: storage,
						Served:  true,
					},
				},
			},
		}
	}

	t.Run("Single CRD with one version and no version config", func(t *testing.T) {
		crd := createCRD("test-crd", "v1", false)
		crdList := []*apiextv1.CustomResourceDefinition{crd}

		result, err := mergeCRDVersions(crdList, nil)
		assert.NoError(t, err)
		assert.Len(t, result, 1)
		assert.Equal(t, "test-crd", result[0].Name)
		assert.Len(t, result[0].Spec.Versions, 1)
		assert.Equal(t, "v1", result[0].Spec.Versions[0].Name)
		assert.True(t, result[0].Spec.Versions[0].Storage)
	})

	t.Run("Single CRD with version config", func(t *testing.T) {
		crd := createCRD("test-crd", "v1", false)
		crdList := []*apiextv1.CustomResourceDefinition{crd}
		crdVersions := map[string]VersionConfig{
			"test-crd": {
				Versions:       []string{"v1"},
				StorageVersion: "v1",
			},
		}

		result, err := mergeCRDVersions(crdList, crdVersions)
		assert.NoError(t, err)
		assert.Len(t, result, 1)
		assert.Equal(t, "test-crd", result[0].Name)
		assert.Len(t, result[0].Spec.Versions, 1)
		assert.Equal(t, "v1", result[0].Spec.Versions[0].Name)
		assert.True(t, result[0].Spec.Versions[0].Storage)
		// Single version CRDs should not have conversion strategy set
		assert.Nil(t, result[0].Spec.Conversion)
	})

	t.Run("Multiple CRDs with different versions merged", func(t *testing.T) {
		crd1 := createCRD("test-crd", "v1", false)
		crd2 := createCRD("test-crd", "v2", false)
		crdList := []*apiextv1.CustomResourceDefinition{crd1, crd2}
		crdVersions := map[string]VersionConfig{
			"test-crd": {
				Versions:       []string{"v1", "v2"},
				StorageVersion: "v2",
			},
		}

		result, err := mergeCRDVersions(crdList, crdVersions)
		assert.NoError(t, err)
		assert.Len(t, result, 1)
		assert.Equal(t, "test-crd", result[0].Name)
		assert.Len(t, result[0].Spec.Versions, 2)

		// Check that v2 is the storage version
		v1Found := false
		v2Found := false
		for _, v := range result[0].Spec.Versions {
			if v.Name == "v1" {
				v1Found = true
				assert.False(t, v.Storage)
			}
			if v.Name == "v2" {
				v2Found = true
				assert.True(t, v.Storage)
			}
		}
		assert.True(t, v1Found)
		assert.True(t, v2Found)

		// Multiple version CRDs should have conversion strategy set
		assert.NotNil(t, result[0].Spec.Conversion)
		assert.Equal(t, apiextv1.NoneConverter, result[0].Spec.Conversion.Strategy)
	})

	t.Run("Single version with no explicit storage version in config", func(t *testing.T) {
		crd := createCRD("test-crd", "v1", false)
		crdList := []*apiextv1.CustomResourceDefinition{crd}
		crdVersions := map[string]VersionConfig{
			"test-crd": {
				Versions: []string{"v1"},
				// StorageVersion is empty
			},
		}

		result, err := mergeCRDVersions(crdList, crdVersions)
		assert.NoError(t, err)
		assert.Len(t, result, 1)
		assert.True(t, result[0].Spec.Versions[0].Storage)
	})

	t.Run("Error: CRD has multiple versions but no version config", func(t *testing.T) {
		crd1 := createCRD("test-crd", "v1", false)
		crd2 := createCRD("test-crd", "v2", false)
		crdList := []*apiextv1.CustomResourceDefinition{crd1, crd2}
		crdVersions := map[string]VersionConfig{}

		_, err := mergeCRDVersions(crdList, crdVersions)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "CRD test-crd has multiple versions but version config specified in crdVersions")
	})

	t.Run("Error: CRD has multiple versions but no storage version specified", func(t *testing.T) {
		crd1 := createCRD("test-crd", "v1", false)
		crd2 := createCRD("test-crd", "v2", false)
		crdList := []*apiextv1.CustomResourceDefinition{crd1, crd2}
		crdVersions := map[string]VersionConfig{
			"test-crd": {
				Versions: []string{"v1", "v2"},
				// StorageVersion is empty
			},
		}

		_, err := mergeCRDVersions(crdList, crdVersions)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "CRD test-crd has multiple versions, but no storageVersion specified in crdVersions")
	})

	t.Run("Error: CRD has more than one version in spec", func(t *testing.T) {
		crd := &apiextv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-crd",
			},
			Spec: apiextv1.CustomResourceDefinitionSpec{
				Group: "test.example.com",
				Names: apiextv1.CustomResourceDefinitionNames{
					Kind: "TestCrd",
				},
				Versions: []apiextv1.CustomResourceDefinitionVersion{
					{Name: "v1", Storage: true, Served: true},
					{Name: "v2", Storage: false, Served: true},
				},
			},
		}
		crdList := []*apiextv1.CustomResourceDefinition{crd}
		crdVersions := map[string]VersionConfig{}

		_, err := mergeCRDVersions(crdList, crdVersions)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "each CRD item must only have one version. CRD test-crd has 2 versions")
	})

	t.Run("Error: Duplicate version definitions", func(t *testing.T) {
		crd1 := createCRD("test-crd", "v1", false)
		crd2 := createCRD("test-crd", "v1", false) // Same version
		crdList := []*apiextv1.CustomResourceDefinition{crd1, crd2}
		crdVersions := map[string]VersionConfig{}

		_, err := mergeCRDVersions(crdList, crdVersions)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "CRD test-crd has duplicated definitions of version v1")
	})

	t.Run("Error: Version specified in config but not in CRD list", func(t *testing.T) {
		crd := createCRD("test-crd", "v1", false)
		crdList := []*apiextv1.CustomResourceDefinition{crd}
		crdVersions := map[string]VersionConfig{
			"test-crd": {
				Versions:       []string{"v1", "v2"}, // v2 doesn't exist
				StorageVersion: "v1",
			},
		}

		_, err := mergeCRDVersions(crdList, crdVersions)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "version v2 of CRD test-crd is specified in crdVersions, but does not exist not in crdList")
	})

	t.Run("Error: Storage version not found in CRD versions", func(t *testing.T) {
		crd := createCRD("test-crd", "v1", false)
		crdList := []*apiextv1.CustomResourceDefinition{crd}
		crdVersions := map[string]VersionConfig{
			"test-crd": {
				Versions:       []string{"v1"},
				StorageVersion: "v2", // v2 doesn't exist
			},
		}

		_, err := mergeCRDVersions(crdList, crdVersions)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "CRD test-crd does not have the specified storage version v2")
	})

	t.Run("Multiple different CRDs", func(t *testing.T) {
		crd1 := createCRD("first-crd", "v1", false)
		crd2 := createCRD("second-crd", "v1", false)
		crdList := []*apiextv1.CustomResourceDefinition{crd1, crd2}
		crdVersions := map[string]VersionConfig{
			"first-crd": {
				Versions:       []string{"v1"},
				StorageVersion: "v1",
			},
		}

		result, err := mergeCRDVersions(crdList, crdVersions)
		assert.NoError(t, err)
		assert.Len(t, result, 2)

		// Check both CRDs are present
		foundFirst := false
		foundSecond := false
		for _, crd := range result {
			if crd.Name == "first-crd" {
				foundFirst = true
				assert.True(t, crd.Spec.Versions[0].Storage)
				// Single version CRDs should not have conversion strategy set
				assert.Nil(t, crd.Spec.Conversion)
			}
			if crd.Name == "second-crd" {
				foundSecond = true
				assert.True(t, crd.Spec.Versions[0].Storage)
				// Single version CRDs should not have conversion strategy set
				assert.Nil(t, crd.Spec.Conversion)
			}
		}
		assert.True(t, foundFirst)
		assert.True(t, foundSecond)
	})

	t.Run("Empty CRD list", func(t *testing.T) {
		crdList := []*apiextv1.CustomResourceDefinition{}
		crdVersions := map[string]VersionConfig{}

		result, err := mergeCRDVersions(crdList, crdVersions)
		assert.NoError(t, err)
		assert.Len(t, result, 0)
	})

	t.Run("Conversion strategy only set for multiple versions", func(t *testing.T) {
		// Test that single version CRDs don't get conversion strategy
		singleVersionCRD := createCRD("single-crd", "v1", false)

		// Test that multi-version CRDs do get conversion strategy
		multiV1CRD := createCRD("multi-crd", "v1", false)
		multiV2CRD := createCRD("multi-crd", "v2", false)

		crdList := []*apiextv1.CustomResourceDefinition{singleVersionCRD, multiV1CRD, multiV2CRD}
		crdVersions := map[string]VersionConfig{
			"multi-crd": {
				Versions:       []string{"v1", "v2"},
				StorageVersion: "v2",
			},
		}

		result, err := mergeCRDVersions(crdList, crdVersions)
		assert.NoError(t, err)
		assert.Len(t, result, 2)

		for _, crd := range result {
			if crd.Name == "single-crd" {
				assert.Len(t, crd.Spec.Versions, 1)
				assert.Nil(t, crd.Spec.Conversion, "Single version CRD should not have conversion strategy")
			}
			if crd.Name == "multi-crd" {
				assert.Len(t, crd.Spec.Versions, 2)
				assert.NotNil(t, crd.Spec.Conversion, "Multi-version CRD should have conversion strategy")
				assert.Equal(t, apiextv1.NoneConverter, crd.Spec.Conversion.Strategy)
			}
		}
	})
}

func TestIsInAllowList(t *testing.T) {
	t.Run("CRD in allowlist", func(t *testing.T) {
		allowList := []string{"deployments.michelangelo.api", "agents.michelangelo.api"}
		result := isInAllowList("deployments.michelangelo.api", allowList)
		assert.True(t, result)
	})

	t.Run("CRD not in allowlist", func(t *testing.T) {
		allowList := []string{"deployments.michelangelo.api", "agents.michelangelo.api"}
		result := isInAllowList("projects.michelangelo.api", allowList)
		assert.False(t, result)
	})

	t.Run("Empty allowlist", func(t *testing.T) {
		allowList := []string{}
		result := isInAllowList("deployments.michelangelo.api", allowList)
		assert.False(t, result)
	})

	t.Run("Nil allowlist", func(t *testing.T) {
		var allowList []string
		result := isInAllowList("deployments.michelangelo.api", allowList)
		assert.False(t, result)
	})
}

func TestUpsertCRDsWithAllowList(t *testing.T) {
	logger, err := zap.NewDevelopment()
	assert.NoError(t, err)

	t.Run("CRD in allowlist enables incompatible updates", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		crdGatewayMock := crdmocks.NewMockGateway(ctrl)

		// Expect ConditionalUpsert to be called with enableIncompatibleUpdate=true
		crdGatewayMock.EXPECT().ConditionalUpsert(gomock.Any(), gomock.Any(), true).Return(nil).Times(1)

		ctx := context.Background()
		crd, err := readCRDFromFile(testCRDManifestDir + "/project.pb.yaml")
		assert.NotNil(t, crd)
		assert.NoError(t, err)

		// Set the CRD name to match our allowlist
		crd.Name = "deployments.michelangelo.api"

		allowList := []string{"deployments.michelangelo.api"}
		err = upsertCRDs(ctx, logger, crdGatewayMock, []*apiextv1.CustomResourceDefinition{crd}, allowList)
		assert.NoError(t, err)
	})

	t.Run("CRD not in allowlist disables incompatible updates", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		crdGatewayMock := crdmocks.NewMockGateway(ctrl)

		// Expect ConditionalUpsert to be called with enableIncompatibleUpdate=false
		crdGatewayMock.EXPECT().ConditionalUpsert(gomock.Any(), gomock.Any(), false).Return(nil).Times(1)

		ctx := context.Background()
		crd, err := readCRDFromFile(testCRDManifestDir + "/project.pb.yaml")
		assert.NotNil(t, crd)
		assert.NoError(t, err)

		// Set the CRD name to NOT match our allowlist
		crd.Name = "projects.michelangelo.api"

		allowList := []string{"deployments.michelangelo.api"}
		err = upsertCRDs(ctx, logger, crdGatewayMock, []*apiextv1.CustomResourceDefinition{crd}, allowList)
		assert.NoError(t, err)
	})

	t.Run("Empty allowlist disables incompatible updates for all CRDs", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		crdGatewayMock := crdmocks.NewMockGateway(ctrl)

		// Expect ConditionalUpsert to be called with enableIncompatibleUpdate=false
		crdGatewayMock.EXPECT().ConditionalUpsert(gomock.Any(), gomock.Any(), false).Return(nil).Times(1)

		ctx := context.Background()
		crd, err := readCRDFromFile(testCRDManifestDir + "/project.pb.yaml")
		assert.NotNil(t, crd)
		assert.NoError(t, err)

		allowList := []string{}
		err = upsertCRDs(ctx, logger, crdGatewayMock, []*apiextv1.CustomResourceDefinition{crd}, allowList)
		assert.NoError(t, err)
	})
}
