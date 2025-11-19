package generator

import (
	"strings"
	"testing"

	"github.com/michelangelo-ai/michelangelo/tools/proto-patcher/parser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateSimpleMessage(t *testing.T) {
	pf := &parser.ProtoFile{
		Syntax:  "proto3",
		Package: "test",
		Messages: []*parser.Message{
			{
				Name: "TestMessage",
				Fields: []*parser.Field{
					{Name: "name", Type: "string", Tag: 1},
					{Name: "age", Type: "int32", Tag: 2},
				},
			},
		},
	}

	gen := NewGenerator()
	content, err := gen.Generate(pf)
	require.NoError(t, err)

	assert.Contains(t, content, `syntax = "proto3";`)
	assert.Contains(t, content, "package test;")
	assert.Contains(t, content, "message TestMessage {")
	assert.Contains(t, content, "string name = 1;")
	assert.Contains(t, content, "int32 age = 2;")
}

func TestGenerateRepeatedField(t *testing.T) {
	pf := &parser.ProtoFile{
		Syntax:  "proto3",
		Package: "test",
		Messages: []*parser.Message{
			{
				Name: "TestMessage",
				Fields: []*parser.Field{
					{Name: "tags", Type: "string", Tag: 1, Repeated: true},
				},
			},
		},
	}

	gen := NewGenerator()
	content, err := gen.Generate(pf)
	require.NoError(t, err)

	assert.Contains(t, content, "repeated string tags = 1;")
}

func TestGenerateNestedMessage(t *testing.T) {
	pf := &parser.ProtoFile{
		Syntax:  "proto3",
		Package: "test",
		Messages: []*parser.Message{
			{
				Name: "Outer",
				Nested: []*parser.Message{
					{
						Name: "Inner",
						Fields: []*parser.Field{
							{Name: "value", Type: "int32", Tag: 1},
						},
					},
				},
				Fields: []*parser.Field{
					{Name: "inner", Type: "Inner", Tag: 1},
				},
			},
		},
	}

	gen := NewGenerator()
	content, err := gen.Generate(pf)
	require.NoError(t, err)

	assert.Contains(t, content, "message Outer {")
	assert.Contains(t, content, "message Inner {")
	assert.Contains(t, content, "int32 value = 1;")
	assert.Contains(t, content, "Inner inner = 1;")
}

func TestGenerateWithImports(t *testing.T) {
	pf := &parser.ProtoFile{
		Syntax:  "proto3",
		Package: "test",
		Imports: []string{
			"google/protobuf/any.proto",
			"michelangelo/api/options.proto",
		},
		Messages: []*parser.Message{
			{
				Name: "TestMessage",
				Fields: []*parser.Field{
					{Name: "data", Type: "google.protobuf.Any", Tag: 1},
				},
			},
		},
	}

	gen := NewGenerator()
	content, err := gen.Generate(pf)
	require.NoError(t, err)

	assert.Contains(t, content, `import "google/protobuf/any.proto";`)
	assert.Contains(t, content, `import "michelangelo/api/options.proto";`)
}

func TestGenerateWithOptions(t *testing.T) {
	pf := &parser.ProtoFile{
		Syntax:  "proto3",
		Package: "test",
		Options: map[string]string{
			"go_package": "github.com/example/test",
		},
		Messages: []*parser.Message{
			{
				Name:   "TestMessage",
				Fields: []*parser.Field{},
			},
		},
	}

	gen := NewGenerator()
	content, err := gen.Generate(pf)
	require.NoError(t, err)

	assert.Contains(t, content, `option go_package = "github.com/example/test";`)
}

func TestGenerateWithComments(t *testing.T) {
	pf := &parser.ProtoFile{
		Syntax:  "proto3",
		Package: "test",
		Messages: []*parser.Message{
			{
				Name:     "TestMessage",
				Comments: "This is a test message",
				Fields: []*parser.Field{
					{
						Name:     "name",
						Type:     "string",
						Tag:      1,
						Comments: "User's name",
					},
				},
			},
		},
	}

	gen := NewGenerator()
	content, err := gen.Generate(pf)
	require.NoError(t, err)

	assert.Contains(t, content, "// This is a test message")
	assert.Contains(t, content, "// User's name")
}

func TestFieldSorting(t *testing.T) {
	pf := &parser.ProtoFile{
		Syntax:  "proto3",
		Package: "test",
		Messages: []*parser.Message{
			{
				Name: "TestMessage",
				Fields: []*parser.Field{
					{Name: "field3", Type: "string", Tag: 3},
					{Name: "field1", Type: "string", Tag: 1},
					{Name: "field2", Type: "string", Tag: 2},
				},
			},
		},
	}

	gen := NewGenerator()
	content, err := gen.Generate(pf)
	require.NoError(t, err)

	// Find positions
	pos1 := strings.Index(content, "field1")
	pos2 := strings.Index(content, "field2")
	pos3 := strings.Index(content, "field3")

	// Should be in order
	assert.Less(t, pos1, pos2)
	assert.Less(t, pos2, pos3)
}



