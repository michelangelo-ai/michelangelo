package utils

import (
	"context"
	"regexp"
	"strings"

	"github.com/michelangelo-ai/michelangelo/go/api"
	"go.uber.org/yarpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

// GetObjectTypeMetaFromList derives object TypeMeta from list object
// Returns nil if successful, otherwise a gRPC status error is returned.
func GetObjectTypeMetaFromList(list runtime.Object, scheme *runtime.Scheme) (*metav1.TypeMeta, error) {
	listGVK, err := apiutil.GVKForObject(list, scheme)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, err.Error())
	}

	kindLen := len(listGVK.Kind)
	if kindLen < 4 || listGVK.Kind[kindLen-4:] != "List" {
		return nil, status.Errorf(codes.InvalidArgument, "failed to derive object Kind from %v", listGVK)
	}

	objGVK := listGVK.GroupVersion().WithKind(listGVK.Kind[:kindLen-4])
	objTypeMeta := &metav1.TypeMeta{}
	objTypeMeta.SetGroupVersionKind(objGVK)

	return objTypeMeta, nil
}

// GetObjectTypeMetafromObject derives object TypeMeta from object
// Returns nil if successful, otherwise a gRPC status error is returned.
func GetObjectTypeMetafromObject(obj runtime.Object, scheme *runtime.Scheme) (*metav1.TypeMeta, error) {
	typeMeta := &metav1.TypeMeta{}
	gvk, err := apiutil.GVKForObject(obj, scheme)

	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, err.Error())
	}

	typeMeta.SetGroupVersionKind(gvk)
	return typeMeta, nil
}

// IsDeleting checks if the DeletingAnnotation is set
func IsDeleting(object client.Object) bool {
	return checkAnnotation(object, api.DeletingAnnotation)
}

// IsImmutable checks if the ImmutableAnnotation is set
func IsImmutable(object client.Object) bool {
	return checkAnnotation(object, api.ImmutableAnnotation)
}

// MarkImmutable is used to mark the ImmutableAnnotation.
func MarkImmutable(object client.Object) {
	annotations := object.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
		object.SetAnnotations(annotations)
	}
	annotations[api.ImmutableAnnotation] = "true"
}

func checkAnnotation(object client.Object, key string) bool {
	if object == nil {
		return false
	}

	annotations := object.GetAnnotations()
	if annotations == nil {
		return false
	}
	if val, ok := annotations[key]; ok && val == "true" {
		return true
	}
	return false
}

var matchFirstCap = regexp.MustCompile("(.)([A-Z][a-z]+)")
var matchAllCap = regexp.MustCompile("([a-z0-9])([A-Z])")

// ToSnakeCase converts a string from CamelCase to snake_case
func ToSnakeCase(camelStr string) string {
	snake := matchFirstCap.ReplaceAllString(camelStr, "${1}_${2}")
	snake = matchAllCap.ReplaceAllString(snake, "${1}_${2}")
	return strings.ToLower(snake)
}

// IsNotFoundError checks if the error is not found error
// It handles grpc not found error and k8s client not found error
func IsNotFoundError(err error) bool {
	if e, ok := status.FromError(err); ok {
		return e.Code() == codes.NotFound
	}
	// Handle Kubernetes REST client errors
	if errors.IsNotFound(err) {
		return true
	}
	return false
}

// GetHeaders gets yarpc headers from context
func GetHeaders(ctx context.Context) map[string]string {
	headers := map[string]string{}
	call := yarpc.CallFromContext(ctx)
	headerNames := call.HeaderNames()
	for _, headerName := range headerNames {
		headerValue := call.Header(headerName)
		headers[headerName] = headerValue
	}
	return headers
}
