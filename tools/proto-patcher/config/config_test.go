package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	configContent := `{
  "field_prefix": "EXT_",
  "tag_start": 999,
  "patches": [
    {
      "target_proto": "project.proto",
      "target_message": "ProjectSpec",
      "extension_proto": "project_ext.proto",
      "extension_message": "ProjectSpecExtension",
      "patch_mode": "merge"
    }
  ],
  "validation_overrides": [
    {
      "target_proto": "project.proto",
      "target_message": "ProjectSpec",
      "field": "name",
      "new_validation": {
        "required": "true",
        "min_length": "3"
      }
    }
  ]
}`

	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	cfg, err := LoadConfig(configPath)
	require.NoError(t, err)

	assert.Equal(t, "EXT_", cfg.FieldPrefix)
	assert.Equal(t, 999, cfg.TagStart)
	assert.Len(t, cfg.Patches, 1)
	assert.Len(t, cfg.ValidationOverrides, 1)

	patch := cfg.Patches[0]
	assert.Equal(t, "project.proto", patch.TargetProto)
	assert.Equal(t, "ProjectSpec", patch.TargetMessage)
	assert.Equal(t, "project_ext.proto", patch.ExtensionProto)
	assert.Equal(t, "ProjectSpecExtension", patch.ExtensionMessage)

	override := cfg.ValidationOverrides[0]
	assert.Equal(t, "project.proto", override.TargetProto)
	assert.Equal(t, "name", override.Field)
	assert.Equal(t, "true", override.NewValidation["required"])
}

func TestSaveConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	// Create a config
	cfg, err := GenerateConfig([]string{"project_ext.proto"}, "EXT_", 999)
	require.NoError(t, err)

	// Save it
	err = SaveConfig(cfg, configPath)
	require.NoError(t, err)

	// Verify file was created
	_, err = os.Stat(configPath)
	require.NoError(t, err)

	// Load it back
	loadedCfg, err := LoadConfig(configPath)
	require.NoError(t, err)

	assert.Equal(t, cfg.FieldPrefix, loadedCfg.FieldPrefix)
	assert.Equal(t, cfg.TagStart, loadedCfg.TagStart)
}

func TestGenerateConfig(t *testing.T) {
	cfg, err := GenerateConfig([]string{"project_ext.proto"}, "UBER_", 999)
	require.NoError(t, err)

	assert.Equal(t, "UBER_", cfg.FieldPrefix)
	assert.Equal(t, 999, cfg.TagStart)
	assert.Len(t, cfg.Patches, 1)

	patch := cfg.Patches[0]
	assert.Equal(t, "project.proto", patch.TargetProto)
	assert.Equal(t, "project_ext.proto", patch.ExtensionProto)
}

func TestExtractBaseName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"project_ext.proto", "project.proto"},
		{"model_ext.proto", "model.proto"},
		{"other.proto", "other.proto"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := extractBaseName(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
