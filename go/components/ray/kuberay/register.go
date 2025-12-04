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
	// Scheme is the runtime scheme for KubeRay types.
	//
	// This scheme contains type registration information needed for encoding,
	// decoding, and converting KubeRay resources. It is initialized during
	// package initialization and includes RayCluster and related types.
	Scheme = runtime.NewScheme()

	// Codecs is the codec factory for serializing and deserializing KubeRay types.
	//
	// This factory provides encoders and decoders for converting KubeRay resources
	// between their Go representation and wire formats (JSON, YAML, etc.).
	Codecs = serializer.NewCodecFactory(Scheme)

	// ParameterCodec handles query parameter encoding for the ray.io/v1 API.
	//
	// This codec converts between Go types and URL query parameters when making
	// API requests to Kubernetes for KubeRay resources.
	ParameterCodec = runtime.NewParameterCodec(Scheme)
)

// addKnownTypes registers KubeRay types with the provided runtime scheme.
//
// This function adds the following types to the scheme:
//   - RayCluster: Represents a Ray cluster resource
//   - RayClusterList: Represents a list of Ray clusters
//
// The function also adds GroupVersion metadata to ensure proper API discovery
// and versioning support.
func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(SchemeGroupVersion,
		&rayv1.RayCluster{},
		&rayv1.RayClusterList{},
	)
	metav1.AddToGroupVersion(scheme, SchemeGroupVersion)
	return nil
}

// init performs package-level initialization of the KubeRay scheme.
//
// This function registers the addKnownTypes function with the SchemeBuilder
// and then applies all registered functions to the package-level Scheme.
// This ensures KubeRay types are available for use immediately when the
// package is imported.
//
// The function will panic if scheme registration fails, as this indicates
// a fundamental configuration problem that prevents KubeRay integration.
func init() {
	SchemeBuilder.Register(addKnownTypes)
	utilruntime.Must(AddToScheme(Scheme))
}
