package main

import (
	_ "embed"
	"path/filepath"
	"testing"

	testpb "github.com/michelangelo-ai/michelangelo/proto-go/test/kubeproto"

	"github.com/stretchr/testify/assert"
)

//go:embed test/object_expected_output.sql
var testObjectSQL string

//go:embed test/index_expected_output.sql
var testIndexingSQL string

func TestSqlGen(t *testing.T) {
	tests := map[string]string{
		"testobject.pb.sql": testObjectSQL,
		"indexing.pb.sql":   testIndexingSQL,
	}

	data := testpb.GetProtocReqData()
	resp := generateSQL(data)

	tested := 0
	for _, f := range resp.GetFile() {
		filename := filepath.Base(f.GetName())
		if test, ok := tests[filename]; ok {
			assert.Equal(t, test, f.GetContent())
			tested++
		}
	}

	assert.Equal(t, len(tests), tested)
}
