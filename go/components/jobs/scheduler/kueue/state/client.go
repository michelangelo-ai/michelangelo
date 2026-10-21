package state

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured/unstructuredscheme"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/client-go/rest"
)

// kueueGroup is Kueue's API group.
const kueueGroup = "kueue.x-k8s.io"

// watchEventScheme holds only the metav1 types, to decode watch event frames
// with.
var watchEventScheme = runtime.NewScheme()

func init() {
	metav1.AddToGroupVersion(watchEventScheme, schema.GroupVersion{Version: "v1"})
}

// newKueueClient returns a client for the kueue.x-k8s.io API at apiVersion,
// built from cfg, that reads Kueue objects as unstructured.
func newKueueClient(cfg *rest.Config, apiVersion string) (rest.Interface, error) {
	config := rest.CopyConfig(cfg)
	config.GroupVersion = &schema.GroupVersion{Group: kueueGroup, Version: apiVersion}
	config.APIPath = "/apis"
	config.ContentType = runtime.ContentTypeJSON
	config.NegotiatedSerializer = newUnstructuredSerializer()
	return rest.RESTClientFor(config)
}

// unstructuredSerializer reads objects, lists and watch events as
// unstructured, as client-go's dynamic client does. The unstructured scheme on
// its own types nothing but Unstructured, so it decodes no watch event frame
// and no metav1 type, and it converts what it decodes.
type unstructuredSerializer struct {
	runtime.NegotiatedSerializer
	mediaTypes []runtime.SerializerInfo
}

func newUnstructuredSerializer() unstructuredSerializer {
	base := unstructuredscheme.NewUnstructuredNegotiatedSerializer()
	mediaTypes := base.SupportedMediaTypes()
	for i := range mediaTypes {
		if mediaTypes[i].StreamSerializer == nil {
			continue
		}
		// watch event frames decode with the metav1 types; the objects in them
		// decode with the object serializer
		stream := *mediaTypes[i].StreamSerializer
		stream.Serializer = json.NewSerializerWithOptions(
			json.DefaultMetaFactory, watchEventScheme, watchEventScheme, json.SerializerOptions{})
		mediaTypes[i].StreamSerializer = &stream
	}
	return unstructuredSerializer{NegotiatedSerializer: base, mediaTypes: mediaTypes}
}

func (s unstructuredSerializer) SupportedMediaTypes() []runtime.SerializerInfo {
	return s.mediaTypes
}

// DecoderToVersion decodes objects as they are, without conversion.
func (unstructuredSerializer) DecoderToVersion(decoder runtime.Decoder, _ runtime.GroupVersioner) runtime.Decoder {
	return decoder
}
