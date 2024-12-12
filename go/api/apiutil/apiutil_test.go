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

package apiutil_test

import (
	"errors"
	"testing"

	"github.com/michelangelo-ai/michelangelo/go/api/apiutil"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

/*
TODO: uncomment these test cases after model and pipeline CRDs are moved to OSS repo
func TestTypeMetaConversion(t *testing.T) {
	v2beta1pb.AddToScheme(scheme.Scheme)

	model := &v2beta1pb.Model{}
	modelList := &v2beta1pb.ModelList{}

	objTypeMeta, err := apiutil.GetObjectTypeMetafromObject(model, scheme.Scheme)
	assert.Nil(t, err)
	assert.Equal(t, "Model", objTypeMeta.Kind)

	derivedObjTypeMeta, err := apiutil.GetObjectTypeMetaFromList(modelList, scheme.Scheme)
	assert.Nil(t, err)
	assert.Equal(t, objTypeMeta, derivedObjTypeMeta)

	_, err = apiutil.GetObjectTypeMetaFromList(model, scheme.Scheme)
	assert.NotNil(t, err)
}

func TestAnnotations(t *testing.T) {
	assert.Equal(t, false, apiutil.IsDeleting(nil))

	pipeline := &v2beta1pb.Pipeline{}
	assert.Equal(t, false, apiutil.IsImmutable(pipeline))

	annotations := make(map[string]string)
	pipeline.SetAnnotations(annotations)

	annotations[api.DeletingAnnotation] = "true"
	annotations[api.ImmutableAnnotation] = "true"
	assert.Equal(t, true, apiutil.IsDeleting(pipeline))
	assert.Equal(t, true, apiutil.IsImmutable(pipeline))

	annotations[api.DeletingAnnotation] = "false"
	annotations[api.ImmutableAnnotation] = "false"
	assert.Equal(t, false, apiutil.IsDeleting(pipeline))
	assert.Equal(t, false, apiutil.IsImmutable(pipeline))

	apiutil.MarkImmutable(pipeline)
	assert.Equal(t, true, apiutil.IsImmutable(pipeline))

	// create annotations and mark immutable
	pipelineRun := &v2beta1pb.PipelineRun{}
	apiutil.MarkImmutable(pipelineRun)
	assert.Equal(t, true, apiutil.IsImmutable(pipeline))

}*/

func TestNameConversion(t *testing.T) {
	assert.Equal(t, "model", apiutil.ToSnakeCase("Model"))
	assert.Equal(t, "test_indexing", apiutil.ToSnakeCase("TestIndexing"))
	assert.Equal(t, "pipeline_run", apiutil.ToSnakeCase("pipelineRun"))
}

func TestStringInSlice(t *testing.T) {
	assert.Equal(t, false, apiutil.StringInSlice("element-1", nil))
	assert.Equal(t, false, apiutil.StringInSlice("element-1", []string{}))
	assert.Equal(t, true, apiutil.StringInSlice("element-1", []string{"element-1", "element-2"}))
	assert.Equal(t, false, apiutil.StringInSlice("element-3", []string{"element-1", "element-2"}))
}

func TestSplitSliceIntoChunks(t *testing.T) {
	arr := []string{"1", "2", "3", "4"}

	assert.Equal(t, [][]string{{"1", "2", "3", "4"}}, apiutil.SplitSliceIntoChunks(arr, 10))
	assert.Equal(t, [][]string{{"1", "2", "3", "4"}}, apiutil.SplitSliceIntoChunks(arr, 4))
	assert.Equal(t, [][]string{{"1", "2", "3"}, {"4"}}, apiutil.SplitSliceIntoChunks(arr, 3))
	assert.Equal(t, [][]string{{"1"}, {"2"}, {"3"}, {"4"}}, apiutil.SplitSliceIntoChunks(arr, 1))

	assert.Equal(t, [][]string{}, apiutil.SplitSliceIntoChunks([]string{}, 10))
}

func TestIsNotFoundError(t *testing.T) {
	notFoundErr := status.Errorf(codes.NotFound, "%s: %v", "not found error", errors.New("not found"))
	assert.True(t, apiutil.IsNotFoundError(notFoundErr))

	internalErr := status.Errorf(codes.Internal, "%s: %v", "not found error", errors.New("not found"))
	assert.False(t, apiutil.IsNotFoundError(internalErr))
}

func TestIgnoreNotFoundError(t *testing.T) {
	notFoundErr := status.Errorf(codes.NotFound, "%s: %v", "not found error", errors.New("not found"))
	assert.NoError(t, apiutil.IgnoreNotFoundError(notFoundErr))

	internalErr := status.Errorf(codes.Internal, "%s: %v", "not found error", errors.New("not found"))
	assert.Error(t, apiutil.IgnoreNotFoundError(internalErr))
}
