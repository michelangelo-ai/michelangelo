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

//go:embed test/test_base_expected_output.sql
var testBaseSQL string

//go:embed test/test_wrapper_expected_output.sql
var testWrapperSQL string

//go:embed test/test_draft_expected_output.sql
var testDraftSQL string

//go:embed test/test_msg3_expected_output.sql
var testMsg3SQL string

func TestSqlGen(t *testing.T) {
	tests := map[string]string{
		"testobject.pb.sql": testObjectSQL,
		"indexing.pb.sql":   testIndexingSQL,
		// TestBase opts into "test_wrapper" via revisioned_in: its output must
		// include the test_base_test_wrapper_unmarshalled sidecar mirroring the
		// full base index set.
		"test_base.pb.sql": testBaseSQL,
		// The wrappers themselves are ordinary CRDs: no sidecar of their own.
		"test_wrapper.pb.sql": testWrapperSQL,
		// TestBase does NOT list "test_draft", so no test_base_test_draft
		// sidecar exists anywhere in the output.
		"test_draft.pb.sql": testDraftSQL,
		"test_msg3.pb.sql":  testMsg3SQL,
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
