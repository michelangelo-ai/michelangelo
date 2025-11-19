package parser

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSimpleProto(t *testing.T) {
	// Create a temporary test proto file
	tmpDir := t.TempDir()
	testProto := filepath.Join(tmpDir, "test.proto")

	content := `syntax = "proto3";

package test;

option go_package = "test";

// Test message
message TestMessage {
  // Field 1
  string name = 1;
  int32 age = 2;
  repeated string tags = 3;
}
`
	err := os.WriteFile(testProto, []byte(content), 0644)
	require.NoError(t, err)

	// Parse the file
	parser := NewParser([]string{tmpDir})
	pf, err := parser.ParseFile(testProto)
	require.NoError(t, err)

	// Verify parsed content
	assert.Equal(t, "proto3", pf.Syntax)
	assert.Equal(t, "test", pf.Package)
	assert.Len(t, pf.Messages, 1)

	msg := pf.Messages[0]
	assert.Equal(t, "TestMessage", msg.Name)
	assert.Len(t, msg.Fields, 3)

	// Check fields
	assert.Equal(t, "name", msg.Fields[0].Name)
	assert.Equal(t, "string", msg.Fields[0].Type)
	assert.Equal(t, 1, msg.Fields[0].Tag)
	assert.False(t, msg.Fields[0].Repeated)

	assert.Equal(t, "age", msg.Fields[1].Name)
	assert.Equal(t, "int32", msg.Fields[1].Type)
	assert.Equal(t, 2, msg.Fields[1].Tag)

	assert.Equal(t, "tags", msg.Fields[2].Name)
	assert.True(t, msg.Fields[2].Repeated)
	assert.Equal(t, 3, msg.Fields[2].Tag)
}

func TestParseNestedMessage(t *testing.T) {
	tmpDir := t.TempDir()
	testProto := filepath.Join(tmpDir, "nested.proto")

	content := `syntax = "proto3";

package test;

message Outer {
  string name = 1;
  
  message Inner {
    int32 value = 1;
  }
  
  Inner inner = 2;
}
`
	err := os.WriteFile(testProto, []byte(content), 0644)
	require.NoError(t, err)

	parser := NewParser([]string{tmpDir})
	pf, err := parser.ParseFile(testProto)
	require.NoError(t, err)

	assert.Len(t, pf.Messages, 1)
	outer := pf.Messages[0]
	assert.Equal(t, "Outer", outer.Name)
	assert.Len(t, outer.Nested, 1)

	inner := outer.Nested[0]
	assert.Equal(t, "Inner", inner.Name)
	assert.Len(t, inner.Fields, 1)
	assert.Equal(t, "value", inner.Fields[0].Name)
}

func TestParseMultipleFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create first proto
	proto1 := filepath.Join(tmpDir, "test1.proto")
	err := os.WriteFile(proto1, []byte(`syntax = "proto3";
package test;
message Message1 { string field1 = 1; }
`), 0644)
	require.NoError(t, err)

	// Create second proto
	proto2 := filepath.Join(tmpDir, "test2.proto")
	err = os.WriteFile(proto2, []byte(`syntax = "proto3";
package test;
message Message2 { int32 field2 = 1; }
`), 0644)
	require.NoError(t, err)

	// Parse both
	parser := NewParser([]string{tmpDir})
	files, err := parser.ParseFiles([]string{proto1, proto2})
	require.NoError(t, err)

	assert.Len(t, files, 2)
	assert.Contains(t, files, "test1.proto")
	assert.Contains(t, files, "test2.proto")
}



