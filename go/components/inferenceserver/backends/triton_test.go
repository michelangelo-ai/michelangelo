package backends

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/depsimage"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

func newTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	return scheme
}

func TestCreateTritonDeployment_NoPythonDependencies(t *testing.T) {
	scheme := newTestScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	backend := &tritonBackend{}
	logger := zap.NewNop()

	inferenceServer := &v2pb.InferenceServer{
		ObjectMeta: metav1.ObjectMeta{Name: "my-is", Namespace: "default"},
		Spec: v2pb.InferenceServerSpec{
			InitSpec: &v2pb.InitSpec{
				ResourceSpec: &v2pb.ResourceSpec{},
				ServingSpec:  &v2pb.ServingSpec{},
			},
		},
	}

	err := backend.createTritonDeployment(context.Background(), logger, k8sClient, inferenceServer)
	require.NoError(t, err)

	deployment := &appsv1.Deployment{}
	require.NoError(t, k8sClient.Get(context.Background(), client.ObjectKey{
		Name:      generateK8sDeploymentName(inferenceServer.Name),
		Namespace: inferenceServer.Namespace,
	}, deployment))

	podSpec := deployment.Spec.Template.Spec
	assert.Empty(t, podSpec.InitContainers, "no python_dependencies set -> no init container")
	require.Len(t, podSpec.Containers, 1)
	assert.Contains(t, podSpec.Containers[0].Image, "nvcr.io/nvidia/tritonserver", "no python_dependencies set -> stock image")
}

func TestCreateTritonDeployment_PythonDependencies_ImageAlreadyExists(t *testing.T) {
	scheme := newTestScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	logger := zap.NewNop()

	var triggerBuildCalled bool
	backend := &tritonBackend{
		httpClient: http.DefaultClient,
		depsImageExists: func(ctx context.Context, httpClient *http.Client, tag string) (bool, error) {
			return true, nil
		},
		depsImageTriggerBuild: func(ctx context.Context, httpClient *http.Client, deps []string, tag string) error {
			triggerBuildCalled = true
			return nil
		},
	}

	deps := []string{"torch==2.4.1", "transformers==4.44.2"}
	inferenceServer := &v2pb.InferenceServer{
		ObjectMeta: metav1.ObjectMeta{Name: "my-is", Namespace: "default"},
		Spec: v2pb.InferenceServerSpec{
			InitSpec: &v2pb.InitSpec{
				ResourceSpec: &v2pb.ResourceSpec{},
				ServingSpec: &v2pb.ServingSpec{
					PythonDependencies: deps,
				},
			},
		},
	}

	err := backend.createTritonDeployment(context.Background(), logger, k8sClient, inferenceServer)
	require.NoError(t, err)
	assert.False(t, triggerBuildCalled, "shouldn't trigger a build when the image already exists")

	deployment := &appsv1.Deployment{}
	require.NoError(t, k8sClient.Get(context.Background(), client.ObjectKey{
		Name:      generateK8sDeploymentName(inferenceServer.Name),
		Namespace: inferenceServer.Namespace,
	}, deployment))

	podSpec := deployment.Spec.Template.Spec
	assert.Empty(t, podSpec.InitContainers, "no init container -- deps are baked into the image")
	require.Len(t, podSpec.Containers, 1)
	assert.Equal(t, depsimage.Image(deps), podSpec.Containers[0].Image)
}

func TestCreateTritonDeployment_PythonDependencies_ImageMissing_TriggersBuildAndDefers(t *testing.T) {
	scheme := newTestScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	logger := zap.NewNop()

	var triggerBuildCalled bool
	backend := &tritonBackend{
		httpClient: http.DefaultClient,
		depsImageExists: func(ctx context.Context, httpClient *http.Client, tag string) (bool, error) {
			return false, nil
		},
		depsImageTriggerBuild: func(ctx context.Context, httpClient *http.Client, deps []string, tag string) error {
			triggerBuildCalled = true
			return nil
		},
	}

	inferenceServer := &v2pb.InferenceServer{
		ObjectMeta: metav1.ObjectMeta{Name: "my-is", Namespace: "default"},
		Spec: v2pb.InferenceServerSpec{
			InitSpec: &v2pb.InitSpec{
				ResourceSpec: &v2pb.ResourceSpec{},
				ServingSpec: &v2pb.ServingSpec{
					PythonDependencies: []string{"torch==2.4.1"},
				},
			},
		},
	}

	err := backend.createTritonDeployment(context.Background(), logger, k8sClient, inferenceServer)
	require.NoError(t, err)
	assert.True(t, triggerBuildCalled, "should trigger a build when the image doesn't exist yet")

	deployment := &appsv1.Deployment{}
	getErr := k8sClient.Get(context.Background(), client.ObjectKey{
		Name:      generateK8sDeploymentName(inferenceServer.Name),
		Namespace: inferenceServer.Namespace,
	}, deployment)
	assert.True(t, apierrors.IsNotFound(getErr), "Deployment shouldn't be created until the dependency image exists")
}

func TestCreateTritonDeployment_PythonDependencies_ExistsCheckError(t *testing.T) {
	scheme := newTestScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	logger := zap.NewNop()

	backend := &tritonBackend{
		httpClient: http.DefaultClient,
		depsImageExists: func(ctx context.Context, httpClient *http.Client, tag string) (bool, error) {
			return false, errors.New("registry unreachable")
		},
		depsImageTriggerBuild: func(ctx context.Context, httpClient *http.Client, deps []string, tag string) error {
			t.Fatal("shouldn't attempt to trigger a build when the existence check itself failed")
			return nil
		},
	}

	inferenceServer := &v2pb.InferenceServer{
		ObjectMeta: metav1.ObjectMeta{Name: "my-is", Namespace: "default"},
		Spec: v2pb.InferenceServerSpec{
			InitSpec: &v2pb.InitSpec{
				ResourceSpec: &v2pb.ResourceSpec{},
				ServingSpec: &v2pb.ServingSpec{
					PythonDependencies: []string{"torch==2.4.1"},
				},
			},
		},
	}

	err := backend.createTritonDeployment(context.Background(), logger, k8sClient, inferenceServer)
	require.Error(t, err)
}
