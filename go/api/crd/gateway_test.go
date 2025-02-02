package crd

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiExtFake "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/scheme"
)

var (
	fakeProject = unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "test/v0",
			"kind":       "Project",
			"metadata": map[string]interface{}{
				"name":            "project-1",
				"namespace":       "project-1",
				"resourceVersion": "1234",
			},
		},
	}

	fakeClientWithResource = fake.NewSimpleDynamicClient(scheme.Scheme, &fakeProject)
	fakeClientNoResource   = fake.NewSimpleDynamicClientWithCustomListKinds(scheme.Scheme,
		map[schema.GroupVersionResource]string{
			{Group: "test", Version: "v0", Resource: "projects"}: "ProjectList",
		})
)

func TestDelete(t *testing.T) {
	t.Run("test delete CRD with no instance", func(t *testing.T) {
		// Prepare
		crd, err := readCRDFromFile(testCRDManifestDir + "/project.pb.yaml")
		assert.NotNil(t, crd)
		assert.NoError(t, err)
		apiExtClientStub := apiExtFake.NewClientset(crd)
		crdGateway := gateway{
			logger:        zap.NewExample(),
			apiExtClient:  apiExtClientStub,
			dynamicClient: fakeClientNoResource,
		}

		// Action
		ctx := context.Background()
		err = crdGateway.Delete(ctx, crd)
		assert.NoError(t, err)
	})

	t.Run("test delete CRD with instance", func(t *testing.T) {
		// Prepare
		apiExtClientStub := apiExtFake.NewClientset()
		crd, err := readCRDFromFile(testCRDManifestDir + "/project.pb.yaml")
		assert.NotNil(t, crd)
		assert.NoError(t, err)
		crdGateway := gateway{
			logger:        zap.NewExample(),
			apiExtClient:  apiExtClientStub,
			dynamicClient: fakeClientWithResource,
		}

		// Action
		ctx := context.Background()
		err = crdGateway.Delete(ctx, crd)
		assert.Error(t, err)
	})
}

func TestConditionalUpsert(t *testing.T) {
	t.Run("test upsert non existing CRD", func(t *testing.T) {
		// Prepare

		apiExtClientStub := apiExtFake.NewSimpleClientset()
		crdGateway := gateway{
			logger:        zap.NewExample(),
			apiExtClient:  apiExtClientStub,
			dynamicClient: fakeClientNoResource,
		}

		// Action
		ctx := context.Background()
		crd, err := readCRDFromFile(testCRDManifestDir + "/project.pb.yaml")
		assert.NotNil(t, crd)
		assert.NoError(t, err)
		err = crdGateway.ConditionalUpsert(ctx, crd, false)
		assert.NoError(t, err)
	})

	t.Run("test upsert existing CRD with no change", func(t *testing.T) {
		// Prepare
		crd, err := readCRDFromFile(testCRDManifestDir + "/project.pb.yaml")
		assert.NotNil(t, crd)
		assert.NoError(t, err)
		apiExtClientStub := apiExtFake.NewClientset(crd)
		crdGateway := gateway{
			logger:        zap.NewExample(),
			apiExtClient:  apiExtClientStub,
			dynamicClient: fakeClientWithResource,
		}

		// Action
		ctx := context.Background()
		err = crdGateway.ConditionalUpsert(ctx, crd, false)
		assert.NoError(t, err)
	})

	t.Run("test upsert existing CRD with compatible change", func(t *testing.T) {
		// Prepare
		crd, err := readCRDFromFile(testCRDManifestDir + "/project.pb.yaml")
		assert.NotNil(t, crd)
		assert.NoError(t, err)
		apiExtClientStub := apiExtFake.NewSimpleClientset(crd)
		crdGateway := gateway{
			logger:        zap.NewExample(),
			apiExtClient:  apiExtClientStub,
			dynamicClient: fakeClientWithResource,
		}

		// Action
		ctx := context.Background()
		newCRD := crd.DeepCopy()
		// add a new property
		newCRD.Spec.Versions[0].Schema.OpenAPIV3Schema.Properties["newTestProperty"] = apiextv1.JSONSchemaProps{Type: "integer"}
		err = crdGateway.ConditionalUpsert(ctx, newCRD, false)
		assert.NoError(t, err)
	})

	t.Run("test upsert existing CRD with non compatible change but no instance", func(t *testing.T) {
		// Prepare
		crd, err := readCRDFromFile(testCRDManifestDir + "/project.pb.yaml")
		assert.NotNil(t, crd)
		assert.NoError(t, err)
		apiExtClientStub := apiExtFake.NewSimpleClientset(crd)
		crdGateway := gateway{
			logger:        zap.NewExample(),
			apiExtClient:  apiExtClientStub,
			dynamicClient: fakeClientNoResource,
		}

		// Action
		ctx := context.Background()
		newCRD := crd.DeepCopy()
		// change property type
		testProperty := newCRD.Spec.Versions[0].Schema.OpenAPIV3Schema.Properties["spec"].Properties["meta"].Properties["testProperty"]
		testProperty.Type = "object"
		newCRD.Spec.Versions[0].Schema.OpenAPIV3Schema.Properties["spec"].Properties["meta"].Properties["testProperty"] = testProperty
		err = crdGateway.ConditionalUpsert(ctx, newCRD, false)
		assert.NoError(t, err)
	})

	t.Run("test upsert existing CRD with incompatible change enabled ", func(t *testing.T) {
		// Prepare
		crd, err := readCRDFromFile(testCRDManifestDir + "/project.pb.yaml")
		assert.NotNil(t, crd)
		assert.NoError(t, err)
		apiExtClientStub := apiExtFake.NewSimpleClientset(crd)
		crdGateway := gateway{
			logger:        zap.NewExample(),
			apiExtClient:  apiExtClientStub,
			dynamicClient: fakeClientWithResource,
		}

		// Action
		ctx := context.Background()
		newCRD := crd.DeepCopy()
		// change property type
		testProperty := newCRD.Spec.Versions[0].Schema.OpenAPIV3Schema.Properties["spec"].Properties["meta"].Properties["testProperty"]
		testProperty.Type = "object"
		newCRD.Spec.Versions[0].Schema.OpenAPIV3Schema.Properties["spec"].Properties["meta"].Properties["testProperty"] = testProperty
		err = crdGateway.ConditionalUpsert(ctx, newCRD, true)
		assert.NoError(t, err)
	})

	t.Run("test upsert existing CRD with incompatible change", func(t *testing.T) {
		// Prepare
		crd, err := readCRDFromFile(testCRDManifestDir + "/project.pb.yaml")
		assert.NotNil(t, crd)
		assert.NoError(t, err)
		apiExtClientStub := apiExtFake.NewClientset(crd)
		crdGateway := gateway{
			logger:        zap.NewExample(),
			apiExtClient:  apiExtClientStub,
			dynamicClient: fakeClientWithResource,
		}

		// Action
		ctx := context.Background()
		newCRD := crd.DeepCopy()
		// change property type
		testProperty := newCRD.Spec.Versions[0].Schema.OpenAPIV3Schema.Properties["spec"].Properties["meta"].Properties["testProperty"]
		testProperty.Type = "object"
		newCRD.Spec.Versions[0].Schema.OpenAPIV3Schema.Properties["spec"].Properties["meta"].Properties["testProperty"] = testProperty
		err = crdGateway.ConditionalUpsert(ctx, newCRD, false)
		assert.Error(t, err)
		assert.True(t, strings.Contains(err.Error(), "Abort updating CRD"))
	})

	t.Run("test failed to create CRD", func(t *testing.T) {
		crd, err := readCRDFromFile(testCRDManifestDir + "/project.pb.yaml")
		assert.NotNil(t, crd)
		assert.NoError(t, err)
		apiExtClientStub := apiExtFake.NewClientset()
		apiExtClientStub.PrependReactor("create", "customresourcedefinitions",
			func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
				return true, nil, errors.New("test error")
			})
		crdGateway := gateway{
			logger:        zap.NewExample(),
			apiExtClient:  apiExtClientStub,
			dynamicClient: fakeClientNoResource,
		}
		ctx := context.Background()
		err = crdGateway.ConditionalUpsert(ctx, crd, false)
		assert.Error(t, err, "failed to create CRD project.test: test error")
	})

	t.Run("test failed to get existing CRD", func(t *testing.T) {
		crd, err := readCRDFromFile(testCRDManifestDir + "/project.pb.yaml")
		assert.NotNil(t, crd)
		assert.NoError(t, err)
		apiExtClientStub := apiExtFake.NewClientset()
		apiExtClientStub.PrependReactor("get", "customresourcedefinitions",
			func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
				return true, nil, errors.New("test error")
			})
		crdGateway := gateway{
			logger:        zap.NewExample(),
			apiExtClient:  apiExtClientStub,
			dynamicClient: fakeClientNoResource,
		}
		ctx := context.Background()
		err = crdGateway.ConditionalUpsert(ctx, crd, false)
		assert.Error(t, err, "failed to get CRD project.test: test error")
	})
}

func TestList(t *testing.T) {
	t.Run("test listing existing CRDs", func(t *testing.T) {
		crd, err := readCRDFromFile(testCRDManifestDir + "/project.pb.yaml")
		assert.NotNil(t, crd)
		assert.NoError(t, err)
		apiExtClientStub := apiExtFake.NewClientset(crd)
		crdGateway := gateway{logger: zap.NewExample(), apiExtClient: apiExtClientStub}

		ctx := context.Background()
		crds, err := crdGateway.List(ctx)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(crds.Items))
		assert.Equal(t, crd.Name, crds.Items[0].Name)
	})

	t.Run("test listing with error", func(t *testing.T) {
		apiExtClientStub := apiExtFake.NewClientset()
		apiExtClientStub.PrependReactor("list", "customresourcedefinitions",
			func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
				return true, nil, errors.New("test error")
			})
		crdGateway := gateway{logger: zap.NewExample(), apiExtClient: apiExtClientStub}

		ctx := context.Background()
		_, err := crdGateway.List(ctx)
		assert.Error(t, err, "failed to list existing CRDs: test error")
	})
}

func TestHasInstances(t *testing.T) {
	crd, err := readCRDFromFile(testCRDManifestDir + "/project.pb.yaml")
	assert.NotNil(t, crd)
	assert.NoError(t, err)

	t.Run("test CRD with no instance", func(t *testing.T) {
		crdGateway := gateway{logger: zap.NewExample(), apiExtClient: nil, dynamicClient: fakeClientNoResource}

		has, e := crdGateway.hasInstances(context.Background(), crd)
		assert.False(t, has)
		assert.NoError(t, e)
	})

	t.Run("test CRD with instance", func(t *testing.T) {
		crdGateway := gateway{logger: zap.NewExample(), apiExtClient: nil, dynamicClient: fakeClientWithResource}

		has, e := crdGateway.hasInstances(context.Background(), crd)
		assert.True(t, has)
		assert.NoError(t, e)
	})

	t.Run("test list error", func(t *testing.T) {
		dynamicClientWithListError := fake.NewSimpleDynamicClient(scheme.Scheme)
		dynamicClientWithListError.PrependReactor("list", "projects",
			func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
				return true, nil, errors.New("test error")
			})
		crdGateway := gateway{
			logger:        zap.NewExample(),
			apiExtClient:  nil,
			dynamicClient: dynamicClientWithListError,
		}

		_, e := crdGateway.hasInstances(context.Background(), crd)
		assert.Error(t, e, "failed to list existing instances of CRD projects.test: test error")
	})
}

func TestNewCRDGateway(t *testing.T) {
	os.Setenv("KUBECONFIG", "test_manifest/k8s.config")
	k8sConfig, err := ctrl.GetConfig()
	assert.NoError(t, err)
	crdGateway := NewCRDGateway(GatewayParams{
		Logger:    zap.Must(zap.NewDevelopment()),
		Scheme:    scheme.Scheme,
		K8sConfig: k8sConfig,
	})
	assert.NotNil(t, crdGateway)
}
