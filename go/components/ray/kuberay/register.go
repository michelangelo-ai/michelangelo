// Package kuberay provides kuberay types
// +kubebuilder:object:generate=true
// +groupName=ray.io
package kuberay

import (
	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
)

var (
	// Scheme is the pluggable scheme
	Scheme = runtime.NewScheme()

	// Codecs is the codec factory for this scheme
	Codecs = serializer.NewCodecFactory(Scheme)

	// ParameterCodec knows about query parameters used with the meta v1 API spec.
	ParameterCodec = runtime.NewParameterCodec(Scheme)
)

func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(SchemeGroupVersion,
		&rayv1.RayCluster{},
		&rayv1.RayClusterList{},
	)
	metav1.AddToGroupVersion(scheme, SchemeGroupVersion)
	return nil
}

func init() {
	SchemeBuilder.Register(addKnownTypes)
	utilruntime.Must(AddToScheme(Scheme))
}
