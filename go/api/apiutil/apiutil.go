// Copyright (c) 2022 Uber Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package apiutil

import (
	"regexp"
	"strings"

	"github.com/michelangelo-ai/michelangelo/go/api"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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

// MarkImmutable is used to mark the ImmutableAnnotation.
func MarkImmutable(object client.Object) {
	annotations := object.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
		object.SetAnnotations(annotations)
	}
	annotations[api.ImmutableAnnotation] = "true"
}

var matchFirstCap = regexp.MustCompile("(.)([A-Z][a-z]+)")
var matchAllCap = regexp.MustCompile("([a-z0-9])([A-Z])")

// ToSnakeCase converts a string from CamelCase to snake_case
func ToSnakeCase(camelStr string) string {
	snake := matchFirstCap.ReplaceAllString(camelStr, "${1}_${2}")
	snake = matchAllCap.ReplaceAllString(snake, "${1}_${2}")
	return strings.ToLower(snake)
}

// StringInSlice returns whether string is contained in the slice
func StringInSlice(a string, strings []string) bool {
	for _, b := range strings {
		if b == a {
			return true
		}
	}
	return false
}

// SplitSliceIntoChunks splits slices into multiple chunks having size <= provided size
func SplitSliceIntoChunks(slice []string, chunkSize int) [][]string {
	if chunkSize == 0 {
		return [][]string{slice}
	}
	chunks := [][]string{}
	for i := 0; i < len(slice); i += chunkSize {
		end := i + chunkSize
		if end > len(slice) {
			end = len(slice)
		}
		chunks = append(chunks, slice[i:end])
	}
	return chunks
}

// IgnoreNotFoundError returns nil if err is a NotFound error. Otherwise it returns the err.
func IgnoreNotFoundError(err error) error {
	if IsNotFoundError(err) {
		return nil
	}

	return err
}

// IsNotFoundError checks if the error is not found error
func IsNotFoundError(err error) bool {
	if e, ok := status.FromError(err); ok {
		return e.Code() == codes.NotFound
	}
	return false
}

// ToReversePredicate gets the reverse predicate for a given predicate
// It appends `~` in front of the predicate.
func ToReversePredicate(predicate string) string {
	return "~" + predicate
}
