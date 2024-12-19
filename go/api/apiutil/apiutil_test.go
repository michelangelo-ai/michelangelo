package apiutil_test

import (
	"testing"

	"github.com/michelangelo-ai/michelangelo/go/api/apiutil"

	"github.com/stretchr/testify/assert"
)

func TestNameConversion(t *testing.T) {
	assert.Equal(t, "model", apiutil.ToSnakeCase("Model"))
	assert.Equal(t, "test_indexing", apiutil.ToSnakeCase("TestIndexing"))
	assert.Equal(t, "pipeline_run", apiutil.ToSnakeCase("pipelineRun"))
}
