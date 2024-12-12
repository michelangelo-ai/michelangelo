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

func TestNameConversion(t *testing.T) {
	assert.Equal(t, "model", apiutil.ToSnakeCase("Model"))
	assert.Equal(t, "test_indexing", apiutil.ToSnakeCase("TestIndexing"))
	assert.Equal(t, "pipeline_run", apiutil.ToSnakeCase("pipelineRun"))
}
