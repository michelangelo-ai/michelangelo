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
		crdGatewayMock.EXPECT().ConditionalUpsert(gomock.Any(), gomock.Any(), gomock.Any()).Return(fmt.Errorf("test error"))

		ctx := context.Background()
		crd, err := readCRDFromFile(testCRDManifestDir + "/project.pb.yaml")
		assert.NotNil(t, crd)
		assert.NoError(t, err)
		err = upsertCRDs(ctx, logger, crdGatewayMock, []*apiextv1.CustomResourceDefinition{crd}, false)
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
				EnableIncompatibleUpdate: false,
			}
		}),
		SyncCRDs([]string{"test"}, map[string]string{
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
				EnableIncompatibleUpdate: false,
			}
		}),
		SyncCRDs([]string{"test"}, map[string]string{
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
				EnableIncompatibleUpdate: false,
				EnableCRDDeletion:        true,
			}
		}),
		SyncCRDs([]string{"test"}, map[string]string{
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
				EnableIncompatibleUpdate: false,
				EnableCRDDeletion:        false,
			}
		}),
		fx.Invoke(func() error {
			return nil
		}),
		SyncCRDs([]string{"test"}, map[string]string{}),
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
				EnableIncompatibleUpdate: false,
				EnableCRDDeletion:        true,
			}
		}),
		fx.Invoke(func() error {
			return nil
		}),
		SyncCRDs([]string{"test"}, map[string]string{}),
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
				EnableIncompatibleUpdate: false,
				EnableCRDDeletion:        true,
			}
		}),
		fx.Invoke(func() error {
			return nil
		}),
		SyncCRDs([]string{"test"}, map[string]string{}),
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
	err = syncCRDs(context.Background(), logger, []string{"test"},
		false, true, &crdGateway, map[string]string{"Project": string(crdYaml)})
	assert.NoError(t, err)

	// incompatible change
	crdYaml, err = os.ReadFile(testCRDManifestDir + "/project_delete_props.pb.yaml")
	assert.NoError(t, err)
	err = syncCRDs(context.Background(), logger, []string{"test"},
		false, true, &crdGateway, map[string]string{"Project": string(crdYaml)})
	assert.Error(t, err, "failed to update CRD. Schema is incompatible, and there are existing instances. Abort updating CRD projects.test")

	// fail to list existing CRDs
	mockGateway := crdmocks.NewMockGateway(gomock.NewController(t))
	mockGateway.EXPECT().List(gomock.Any()).Return(nil, fmt.Errorf("test error"))
	err = syncCRDs(context.Background(), logger, []string{"test"},
		false, true, mockGateway, map[string]string{"Project": string(crdYaml)})
	assert.Error(t, err, "failed to list existing CRDs: test error")

	// do not update CRDs that are not in the specified groups
	err = syncCRDs(context.Background(), logger, []string{"test1", "test2"},
		false, true, &crdGateway, map[string]string{"Project": string(crdYaml)})
	assert.Error(t, err, "CRD projects.test is not in the specified groups [test1, test2]")
}

func TestParseConfig(t *testing.T) {
	yamlConfStr := `
apiserver:
  crdSync:
    enableCRDUpdate: true
    enableIncompatibleUpdate: false
`
	provider, err := config.NewYAML(config.Source(strings.NewReader(yamlConfStr)))
	assert.NoError(t, err)

	conf, err := ParseConfig(provider)
	assert.NoError(t, err)
	assert.True(t, conf.EnableCRDUpdate)
	assert.False(t, conf.EnableIncompatibleUpdate)

	invalidConfStr := `
apiserver:
  crdSync:
    enableCRDUpdat: true
`
	provider2, err := config.NewYAML(config.Source(strings.NewReader(invalidConfStr)))
	assert.NoError(t, err)
	_, err2 := ParseConfig(provider2)
	assert.Error(t, err2)
}
