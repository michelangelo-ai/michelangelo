package kuberay

import (
	"testing"

	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// TestScheme_RoundTripsKubeRayTypes encodes and decodes each KubeRay type with
// the codecs the Ray rest client uses. List and watch responses are decoded
// without a target object, so the type has to come from the scheme: a type
// missing from register.go fails here rather than in a compute cluster's
// informer.
func TestScheme_RoundTripsKubeRayTypes(t *testing.T) {
	codecs := Codecs.WithoutConversion()
	info, ok := runtime.SerializerInfoForMediaType(codecs.SupportedMediaTypes(), runtime.ContentTypeJSON)
	require.True(t, ok)
	encoder := codecs.EncoderForVersion(info.Serializer, SchemeGroupVersion)
	decoder := codecs.DecoderToVersion(info.Serializer, SchemeGroupVersion)

	tests := []struct {
		name string
		obj  runtime.Object
	}{
		{
			name: "RayCluster",
			obj:  &rayv1.RayCluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster", Namespace: "ns"}},
		},
		{
			name: "RayClusterList",
			obj:  &rayv1.RayClusterList{Items: []rayv1.RayCluster{{ObjectMeta: metav1.ObjectMeta{Name: "cluster", Namespace: "ns"}}}},
		},
		{
			name: "RayJob",
			obj: &rayv1.RayJob{
				ObjectMeta: metav1.ObjectMeta{Name: "job", Namespace: "ns"},
				Status: rayv1.RayJobStatus{
					JobStatus:           rayv1.JobStatusRunning,
					JobDeploymentStatus: rayv1.JobDeploymentStatusRunning,
					Message:             "job is running",
				},
			},
		},
		{
			name: "RayJobList",
			obj:  &rayv1.RayJobList{Items: []rayv1.RayJob{{ObjectMeta: metav1.ObjectMeta{Name: "job", Namespace: "ns"}}}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := runtime.Encode(encoder, tt.obj)
			require.NoError(t, err)

			got, err := runtime.Decode(decoder, data)
			require.NoError(t, err)
			require.IsType(t, tt.obj, got)
			// Semantic: a zero resource.Quantity decodes as "0", which DeepEqual
			// would report as a difference.
			assert.True(t, apiequality.Semantic.DeepEqual(tt.obj, got), "decoded %#v", got)
		})
	}
}
