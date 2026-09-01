package backends

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

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
	for _, vm := range podSpec.Containers[0].VolumeMounts {
		assert.NotEqual(t, "python-deps", vm.Name, "no python-deps volume mount when unset")
	}
	for _, env := range podSpec.Containers[0].Env {
		assert.NotEqual(t, "PYTHONPATH", env.Name, "no PYTHONPATH env when unset")
	}
}

func TestCreateTritonDeployment_WithPythonDependencies(t *testing.T) {
	scheme := newTestScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	backend := &tritonBackend{}
	logger := zap.NewNop()

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

	deployment := &appsv1.Deployment{}
	require.NoError(t, k8sClient.Get(context.Background(), client.ObjectKey{
		Name:      generateK8sDeploymentName(inferenceServer.Name),
		Namespace: inferenceServer.Namespace,
	}, deployment))

	podSpec := deployment.Spec.Template.Spec
	require.Len(t, podSpec.InitContainers, 1)
	initContainer := podSpec.InitContainers[0]
	assert.Equal(t, []string{"pip3", "install", "--no-cache-dir", "--target=/deps"}, initContainer.Command)
	assert.Equal(t, deps, initContainer.Args)

	require.Len(t, podSpec.Containers, 1)
	tritonContainer := podSpec.Containers[0]

	var foundMount bool
	for _, vm := range tritonContainer.VolumeMounts {
		if vm.Name == "python-deps" && vm.MountPath == "/deps" {
			foundMount = true
		}
	}
	assert.True(t, foundMount, "triton container should mount the python-deps volume")

	var foundEnv bool
	for _, env := range tritonContainer.Env {
		if env.Name == "PYTHONPATH" && env.Value == "/deps" {
			foundEnv = true
		}
	}
	assert.True(t, foundEnv, "triton container should have PYTHONPATH=/deps")

	var foundVolume bool
	for _, v := range podSpec.Volumes {
		if v.Name == "python-deps" && v.VolumeSource.EmptyDir != nil {
			foundVolume = true
		}
	}
	assert.True(t, foundVolume, "pod should have an emptyDir python-deps volume")
}
