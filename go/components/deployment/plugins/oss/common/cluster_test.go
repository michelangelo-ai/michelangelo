package common

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

const (
	testNamespace = "default"
	testISName    = "test-is"
)

// newScheme returns a runtime scheme with v2pb registered, for the fake controller-runtime client.
func newScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, v2pb.AddToScheme(scheme))
	return scheme
}

// newDeployment builds a Deployment with an InferenceServer reference. Pass empty isName
// to omit the reference entirely.
func newDeployment(isName string) *v2pb.Deployment {
	dep := &v2pb.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: testNamespace},
	}
	if isName != "" {
		dep.Spec = v2pb.DeploymentSpec{
			Target: &v2pb.DeploymentSpec_InferenceServer{
				InferenceServer: &apipb.ResourceIdentifier{Name: isName},
			},
		}
	}
	return dep
}

// newClusterTarget builds a ClusterTarget with full Kubernetes connection info. Pass empty
// host to skip the connection block (cluster-id-only target).
func newClusterTarget(id, host, port, tokenTag, caDataTag string) *v2pb.ClusterTarget {
	target := &v2pb.ClusterTarget{ClusterId: id}
	if host != "" {
		target.Connection = &v2pb.ClusterTarget_Kubernetes{
			Kubernetes: &v2pb.ConnectionSpec{
				Host:      host,
				Port:      port,
				TokenTag:  tokenTag,
				CaDataTag: caDataTag,
			},
		}
	}
	return target
}

func TestFetchInferenceServer(t *testing.T) {
	tests := []struct {
		name        string
		deployment  *v2pb.Deployment
		seedIS      *v2pb.InferenceServer
		expectedErr string
	}{
		{
			name:       "happy path",
			deployment: newDeployment(testISName),
			seedIS: &v2pb.InferenceServer{
				ObjectMeta: metav1.ObjectMeta{Name: testISName, Namespace: testNamespace},
			},
		},
		{
			name:        "deployment has no inference server reference",
			deployment:  newDeployment(""),
			expectedErr: "has no inference server reference",
		},
		{
			name:        "inference server not found",
			deployment:  newDeployment(testISName),
			expectedErr: "get inference server",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := newScheme(t)
			builder := fake.NewClientBuilder().WithScheme(scheme)
			if tt.seedIS != nil {
				builder = builder.WithObjects(tt.seedIS)
			}
			c := builder.Build()

			got, err := FetchInferenceServer(context.Background(), c, tt.deployment)
			if tt.expectedErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErr)
				assert.Nil(t, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, testISName, got.Name)
		})
	}
}

func TestReadTargetClustersAnnotation(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		expected    []*v2pb.ClusterTarget
		expectedErr string
	}{
		{
			name:        "nil annotations",
			annotations: nil,
			expected:    nil,
		},
		{
			name:        "annotation absent",
			annotations: map[string]string{"some-other": "value"},
			expected:    nil,
		},
		{
			name: "valid JSON with full kubernetes connection info",
			annotations: map[string]string{
				TargetClustersAnnotation: `[{"clusterId":"c1","host":"https://k8s","port":"443","tokenTag":"tk","caDataTag":"ca"}]`,
			},
			expected: []*v2pb.ClusterTarget{
				newClusterTarget("c1", "https://k8s", "443", "tk", "ca"),
			},
		},
		{
			name: "valid JSON with cluster-id-only entries",
			annotations: map[string]string{
				TargetClustersAnnotation: `[{"clusterId":"c1"},{"clusterId":"c2"}]`,
			},
			expected: []*v2pb.ClusterTarget{
				{ClusterId: "c1", Connection: &v2pb.ClusterTarget_Kubernetes{Kubernetes: &v2pb.ConnectionSpec{}}},
				{ClusterId: "c2", Connection: &v2pb.ClusterTarget_Kubernetes{Kubernetes: &v2pb.ConnectionSpec{}}},
			},
		},
		{
			name: "invalid JSON",
			annotations: map[string]string{
				TargetClustersAnnotation: "not json",
			},
			expectedErr: "unmarshal",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deployment := &v2pb.Deployment{ObjectMeta: metav1.ObjectMeta{Annotations: tt.annotations}}

			got, err := ReadTargetClustersAnnotation(deployment)
			if tt.expectedErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestWriteTargetClustersAnnotation(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string // pre-existing annotations on the deployment
		targets     []*v2pb.ClusterTarget
		expectedRaw string
	}{
		{
			name:        "empty targets",
			targets:     nil,
			expectedRaw: "[]",
		},
		{
			name: "multiple targets with full kubernetes spec",
			targets: []*v2pb.ClusterTarget{
				newClusterTarget("c1", "https://k8s-1", "443", "tk1", "ca1"),
				newClusterTarget("c2", "https://k8s-2", "6443", "tk2", "ca2"),
			},
			expectedRaw: `[{"clusterId":"c1","host":"https://k8s-1","port":"443","tokenTag":"tk1","caDataTag":"ca1"},{"clusterId":"c2","host":"https://k8s-2","port":"6443","tokenTag":"tk2","caDataTag":"ca2"}]`,
		},
		{
			name: "targets with no kubernetes connection",
			targets: []*v2pb.ClusterTarget{
				{ClusterId: "c1"},
			},
			expectedRaw: `[{"clusterId":"c1"}]`,
		},
		{
			name:        "annotations map nil gets created",
			annotations: nil,
			targets:     []*v2pb.ClusterTarget{{ClusterId: "c1"}},
			expectedRaw: `[{"clusterId":"c1"}]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deployment := &v2pb.Deployment{ObjectMeta: metav1.ObjectMeta{Annotations: tt.annotations}}

			require.NoError(t, WriteTargetClustersAnnotation(deployment, tt.targets))
			require.NotNil(t, deployment.Annotations)
			assert.Equal(t, tt.expectedRaw, deployment.Annotations[TargetClustersAnnotation])
		})
	}
}

func TestClusterTargetsEqual(t *testing.T) {
	tests := []struct {
		name     string
		left     []*v2pb.ClusterTarget
		right    []*v2pb.ClusterTarget
		expected bool
	}{
		{
			name:     "both empty",
			expected: true,
		},
		{
			name:     "different lengths",
			left:     []*v2pb.ClusterTarget{{ClusterId: "c1"}},
			right:    []*v2pb.ClusterTarget{{ClusterId: "c1"}, {ClusterId: "c2"}},
			expected: false,
		},
		{
			name:     "same set different order",
			left:     []*v2pb.ClusterTarget{{ClusterId: "c1"}, {ClusterId: "c2"}},
			right:    []*v2pb.ClusterTarget{{ClusterId: "c2"}, {ClusterId: "c1"}},
			expected: true,
		},
		{
			name:     "same length different IDs",
			left:     []*v2pb.ClusterTarget{{ClusterId: "c1"}, {ClusterId: "c2"}},
			right:    []*v2pb.ClusterTarget{{ClusterId: "c1"}, {ClusterId: "c3"}},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, ClusterTargetsEqual(tt.left, tt.right))
		})
	}
}

// TestAnnotationRoundTrip verifies Write followed by Read preserves cluster IDs and connection info.
func TestAnnotationRoundTrip(t *testing.T) {
	originals := []*v2pb.ClusterTarget{
		newClusterTarget("c1", "https://k8s-1", "443", "tk1", "ca1"),
		newClusterTarget("c2", "https://k8s-2", "6443", "tk2", "ca2"),
	}
	deployment := &v2pb.Deployment{ObjectMeta: metav1.ObjectMeta{}}

	require.NoError(t, WriteTargetClustersAnnotation(deployment, originals))
	got, err := ReadTargetClustersAnnotation(deployment)
	require.NoError(t, err)

	require.Len(t, got, len(originals))
	for i, want := range originals {
		assert.Equal(t, want.GetClusterId(), got[i].GetClusterId())
		assert.Equal(t, want.GetKubernetes().GetHost(), got[i].GetKubernetes().GetHost())
		assert.Equal(t, want.GetKubernetes().GetPort(), got[i].GetKubernetes().GetPort())
		assert.Equal(t, want.GetKubernetes().GetTokenTag(), got[i].GetKubernetes().GetTokenTag())
		assert.Equal(t, want.GetKubernetes().GetCaDataTag(), got[i].GetKubernetes().GetCaDataTag())
	}
}

func TestBackendTypeOf(t *testing.T) {
	annotated := func(value string) *v2pb.Deployment {
		return &v2pb.Deployment{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{BackendTypeAnnotation: value}}}
	}
	tests := []struct {
		name       string
		deployment *v2pb.Deployment
		want       v2pb.BackendType
	}{
		{name: "no annotations defaults to triton", deployment: &v2pb.Deployment{}, want: v2pb.BACKEND_TYPE_TRITON},
		{name: "snapshotted backend wins", deployment: annotated("BACKEND_TYPE_DYNAMO"), want: v2pb.BACKEND_TYPE_DYNAMO},
		{name: "unknown value defaults to triton", deployment: annotated("nonsense"), want: v2pb.BACKEND_TYPE_TRITON},
		{name: "invalid value defaults to triton", deployment: annotated("BACKEND_TYPE_INVALID"), want: v2pb.BACKEND_TYPE_TRITON},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, BackendTypeOf(tt.deployment))
		})
	}

	d := &v2pb.Deployment{}
	WriteBackendTypeAnnotation(d, v2pb.BACKEND_TYPE_LLM_D)
	assert.Equal(t, v2pb.BACKEND_TYPE_LLM_D, BackendTypeOf(d))
}
