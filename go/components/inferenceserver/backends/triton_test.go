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

func TestTritonImage_DefaultsToStockImage(t *testing.T) {
	inferenceServer := &v2pb.InferenceServer{
		Spec: v2pb.InferenceServerSpec{
			InitSpec: &v2pb.InitSpec{
				ServingSpec: &v2pb.ServingSpec{},
			},
		},
	}
	assert.Equal(t, "nvcr.io/nvidia/tritonserver:23.04-py3", tritonImage(inferenceServer))
}

func TestTritonImage_UsesContainerBuildTemplateOverride(t *testing.T) {
	inferenceServer := &v2pb.InferenceServer{
		Spec: v2pb.InferenceServerSpec{
			InitSpec: &v2pb.InitSpec{
				ServingSpec: &v2pb.ServingSpec{
					ContainerBuildTemplate: "ghcr.io/michelangelo-ai/bert-cola-triton-serving:latest",
				},
			},
		},
	}
	assert.Equal(t, "ghcr.io/michelangelo-ai/bert-cola-triton-serving:latest", tritonImage(inferenceServer))
}

func TestCreateTritonDeployment_UsesOverrideImage(t *testing.T) {
	scheme := newTestScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	backend := &tritonBackend{}
	logger := zap.NewNop()

	inferenceServer := &v2pb.InferenceServer{
		ObjectMeta: metav1.ObjectMeta{Name: "my-is", Namespace: "default"},
		Spec: v2pb.InferenceServerSpec{
			InitSpec: &v2pb.InitSpec{
				ResourceSpec: &v2pb.ResourceSpec{},
				ServingSpec: &v2pb.ServingSpec{
					ContainerBuildTemplate: "ghcr.io/michelangelo-ai/bert-cola-triton-serving:latest",
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

	require.Len(t, deployment.Spec.Template.Spec.Containers, 1)
	assert.Equal(t, "ghcr.io/michelangelo-ai/bert-cola-triton-serving:latest",
		deployment.Spec.Template.Spec.Containers[0].Image)
}
