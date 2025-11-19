package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindProtoFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create some proto files
	proto1 := filepath.Join(tmpDir, "test1.proto")
	proto2 := filepath.Join(tmpDir, "subdir", "test2.proto")
	nonProto := filepath.Join(tmpDir, "test.txt")

	err := os.WriteFile(proto1, []byte("syntax = \"proto3\";"), 0644)
	require.NoError(t, err)

	err = os.MkdirAll(filepath.Dir(proto2), 0755)
	require.NoError(t, err)
	err = os.WriteFile(proto2, []byte("syntax = \"proto3\";"), 0644)
	require.NoError(t, err)

	err = os.WriteFile(nonProto, []byte("text file"), 0644)
	require.NoError(t, err)

	// Find proto files
	files, err := findProtoFiles(tmpDir)
	require.NoError(t, err)

	// Should find both proto files but not the txt file
	assert.Len(t, files, 2)
	assert.Contains(t, files, proto1)
	assert.Contains(t, files, proto2)
	assert.NotContains(t, files, nonProto)
}

func TestFindProtoFilesEmptyDir(t *testing.T) {
	tmpDir := t.TempDir()

	files, err := findProtoFiles(tmpDir)
	require.NoError(t, err)

	assert.Len(t, files, 0)
}

func TestFindProtoFilesNonexistentDir(t *testing.T) {
	_, err := findProtoFiles("/nonexistent/path")
	assert.Error(t, err)
}



