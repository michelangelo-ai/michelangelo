package patcher

import (
	"testing"

	"github.com/michelangelo-ai/michelangelo/tools/proto-patcher/parser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMergeFields(t *testing.T) {
	config := &Config{
		FieldPrefix: "EXT_",
		TagStart:    999,
	}

	patcher := NewPatcher(config)

	target := &parser.Message{
		Name: "TestMessage",
		Fields: []*parser.Field{
			{Name: "name", Type: "string", Tag: 1},
			{Name: "age", Type: "int32", Tag: 2},
		},
	}

	extension := &parser.Message{
		Name: "Extension",
		Fields: []*parser.Field{
			{Name: "owner_id", Type: "string", Tag: 1},
			{Name: "cost_center", Type: "string", Tag: 2},
		},
	}

	err := patcher.mergeFields(target, extension)
	require.NoError(t, err)

	// Should have 4 fields now
	assert.Len(t, target.Fields, 4)

	// Check extension fields
	assert.Equal(t, "EXT_owner_id", target.Fields[2].Name)
	assert.Equal(t, 999, target.Fields[2].Tag)

	assert.Equal(t, "EXT_cost_center", target.Fields[3].Name)
	assert.Equal(t, 1000, target.Fields[3].Tag)
}

func TestTagCollision(t *testing.T) {
	config := &Config{
		FieldPrefix: "EXT_",
		TagStart:    2, // Will collide with existing tag
	}

	patcher := NewPatcher(config)

	target := &parser.Message{
		Name: "TestMessage",
		Fields: []*parser.Field{
			{Name: "name", Type: "string", Tag: 1},
			{Name: "age", Type: "int32", Tag: 2},
		},
	}

	extension := &parser.Message{
		Name: "Extension",
		Fields: []*parser.Field{
			{Name: "owner_id", Type: "string", Tag: 1},
		},
	}

	err := patcher.mergeFields(target, extension)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "tag number collision")
}

func TestNameCollision(t *testing.T) {
	config := &Config{
		FieldPrefix: "", // No prefix means collision
		TagStart:    999,
	}

	patcher := NewPatcher(config)

	target := &parser.Message{
		Name: "TestMessage",
		Fields: []*parser.Field{
			{Name: "owner_id", Type: "string", Tag: 1},
		},
	}

	extension := &parser.Message{
		Name: "Extension",
		Fields: []*parser.Field{
			{Name: "owner_id", Type: "string", Tag: 1},
		},
	}

	err := patcher.mergeFields(target, extension)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "field name collision")
}

func TestPatchMultipleFiles(t *testing.T) {
	config := &Config{
		Patches: []PatchRule{
			{
				TargetProto:      "base.proto",
				TargetMessage:    "BaseMessage",
				ExtensionProto:   "ext.proto",
				ExtensionMessage: "ExtMessage",
				PatchMode:        "merge",
			},
		},
		FieldPrefix: "EXT_",
		TagStart:    999,
	}

	baseProtos := map[string]*parser.ProtoFile{
		"base.proto": {
			Syntax:  "proto3",
			Package: "test",
			Messages: []*parser.Message{
				{
					Name: "BaseMessage",
					Fields: []*parser.Field{
						{Name: "field1", Type: "string", Tag: 1},
					},
				},
			},
			FilePath: "base.proto",
		},
	}

	extProtos := map[string]*parser.ProtoFile{
		"ext.proto": {
			Syntax:  "proto3",
			Package: "test",
			Messages: []*parser.Message{
				{
					Name: "ExtMessage",
					Fields: []*parser.Field{
						{Name: "ext_field", Type: "string", Tag: 1},
					},
				},
			},
			FilePath: "ext.proto",
		},
	}

	patcher := NewPatcher(config)
	result, err := patcher.Patch(baseProtos, extProtos)
	require.NoError(t, err)

	assert.Len(t, result, 1)

	patchedFile := result["base_patched.proto"]
	require.NotNil(t, patchedFile)

	msg := patchedFile.Messages[0]
	assert.Len(t, msg.Fields, 2)
	assert.Equal(t, "EXT_ext_field", msg.Fields[1].Name)
	assert.Equal(t, 999, msg.Fields[1].Tag)
}

func TestCloneMessage(t *testing.T) {
	patcher := NewPatcher(&Config{})

	original := &parser.Message{
		Name: "Original",
		Fields: []*parser.Field{
			{Name: "field1", Type: "string", Tag: 1},
		},
		Comments: "Original comment",
	}

	cloned := patcher.cloneMessage(original)

	// Should be equal but different objects
	assert.Equal(t, original.Name, cloned.Name)
	assert.Len(t, cloned.Fields, 1)
	assert.Equal(t, "field1", cloned.Fields[0].Name)

	// Modify clone shouldn't affect original
	cloned.Name = "Cloned"
	assert.Equal(t, "Original", original.Name)
	assert.Equal(t, "Cloned", cloned.Name)
}

func TestCopyMessageMode(t *testing.T) {
	config := &Config{
		Patches: []PatchRule{
			{
				TargetProto:      "project.proto",
				TargetMessage:    "Project",  // Not used in copy mode
				ExtensionProto:   "project_ext.proto",
				ExtensionMessage: "ProjectExtension",
				PatchMode:        "copy",
			},
		},
	}

	baseProtos := map[string]*parser.ProtoFile{
		"project.proto": {
			Syntax:  "proto3",
			Package: "michelangelo.api.v2",
			Messages: []*parser.Message{
				{
					Name: "Project",
					Fields: []*parser.Field{
						{Name: "name", Type: "string", Tag: 1},
					},
				},
			},
			FilePath: "project.proto",
		},
	}

	extProtos := map[string]*parser.ProtoFile{
		"project_ext.proto": {
			Syntax:  "proto3",
			Package: "uber.michelangelo.extensions",
			Messages: []*parser.Message{
				{
					Name: "ProjectExtension",
					Fields: []*parser.Field{
						{Name: "owner_id", Type: "string", Tag: 1},
						{Name: "cost_center", Type: "string", Tag: 2},
					},
					Nested: []*parser.Message{
						{
							Name: "NestedConfig",
							Fields: []*parser.Field{
								{Name: "setting", Type: "string", Tag: 1},
							},
						},
					},
				},
			},
			FilePath: "project_ext.proto",
		},
	}

	patcher := NewPatcher(config)
	result, err := patcher.Patch(baseProtos, extProtos)
	require.NoError(t, err)

	patchedFile := result["project_patched.proto"]
	require.NotNil(t, patchedFile)

	// Should have 2 messages: original Project + copied ProjectExtension
	assert.Len(t, patchedFile.Messages, 2)

	// Original Project message should be unchanged
	projectMsg := patchedFile.Messages[0]
	assert.Equal(t, "Project", projectMsg.Name)
	assert.Len(t, projectMsg.Fields, 1)
	assert.Equal(t, "name", projectMsg.Fields[0].Name)

	// ProjectExtension should be copied as-is with all fields and nested messages
	extMsg := patchedFile.Messages[1]
	assert.Equal(t, "ProjectExtension", extMsg.Name)
	assert.Len(t, extMsg.Fields, 2)
	assert.Equal(t, "owner_id", extMsg.Fields[0].Name)
	assert.Equal(t, 1, extMsg.Fields[0].Tag) // Original tag preserved
	assert.Equal(t, "cost_center", extMsg.Fields[1].Name)
	assert.Equal(t, 2, extMsg.Fields[1].Tag)

	// Nested messages should also be copied
	assert.Len(t, extMsg.Nested, 1)
	assert.Equal(t, "NestedConfig", extMsg.Nested[0].Name)
}

func TestCopyMessageWithCollision(t *testing.T) {
	config := &Config{
		Patches: []PatchRule{
			{
				TargetProto:      "project.proto",
				ExtensionProto:   "project_ext.proto",
				ExtensionMessage: "Project", // Same name as existing message
				PatchMode:        "copy",
			},
		},
	}

	baseProtos := map[string]*parser.ProtoFile{
		"project.proto": {
			Syntax: "proto3",
			Messages: []*parser.Message{
				{Name: "Project"},
			},
			FilePath: "project.proto",
		},
	}

	extProtos := map[string]*parser.ProtoFile{
		"project_ext.proto": {
			Syntax: "proto3",
			Messages: []*parser.Message{
				{Name: "Project"}, // Collision!
			},
			FilePath: "project_ext.proto",
		},
	}

	patcher := NewPatcher(config)
	_, err := patcher.Patch(baseProtos, extProtos)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}
