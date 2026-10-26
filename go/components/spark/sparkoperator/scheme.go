// Package sparkoperator provides a REST client for sparkoperator.k8s.io/v1beta2 types.
package sparkoperator

import (
	sparkv1beta2 "github.com/michelangelo-ai/michelangelo/go/thirdparty/k8s-crds/apis/sparkoperator.k8s.io/v1beta2"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
)

// SchemeGroupVersion is the group/version this client speaks. It is a copy, taken at init, of
// the vendored CRD package's own value, never a second literal: a literal of its own would keep
// compiling, addressing the wrong API path, if the CRD were re-vendored at another version.
var SchemeGroupVersion = sparkv1beta2.SchemeGroupVersion

var (
	// Scheme is the pluggable scheme
	Scheme = runtime.NewScheme()

	// Codecs is the codec factory for this scheme
	Codecs = serializer.NewCodecFactory(Scheme)

	// ParameterCodec knows about query parameters used with the meta v1 API spec.
	ParameterCodec = runtime.NewParameterCodec(Scheme)
)

func init() {
	utilruntime.Must(sparkv1beta2.AddToScheme(Scheme))
}
