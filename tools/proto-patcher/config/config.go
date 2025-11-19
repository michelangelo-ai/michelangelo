package config

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/michelangelo-ai/michelangelo/tools/proto-patcher/patcher"
)

// JSONConfig represents the JSON configuration file structure
type JSONConfig struct {
	FieldPrefix string `json:"field_prefix"`
	TagStart    int    `json:"tag_start"`
	Patches     []struct {
		TargetProto      string `json:"target_proto"`
		TargetMessage    string `json:"target_message"`
		ExtensionProto   string `json:"extension_proto"`
		ExtensionMessage string `json:"extension_message"`
		PatchMode        string `json:"patch_mode"`
	} `json:"patches"`
	ValidationOverrides []struct {
		TargetProto   string            `json:"target_proto"`
		TargetMessage string            `json:"target_message"`
		Field         string            `json:"field"`
		NewValidation map[string]string `json:"new_validation"`
	} `json:"validation_overrides"`
}

// LoadConfig loads configuration from a JSON file
func LoadConfig(path string) (*patcher.Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var jsonCfg JSONConfig
	if err := json.Unmarshal(data, &jsonCfg); err != nil {
		return nil, fmt.Errorf("failed to parse config JSON: %w", err)
	}

	// Convert to patcher.Config
	cfg := &patcher.Config{
		FieldPrefix:         jsonCfg.FieldPrefix,
		TagStart:            jsonCfg.TagStart,
		Patches:             make([]patcher.PatchRule, 0),
		ValidationOverrides: make([]patcher.ValidationOverride, 0),
	}

	// Convert patches
	for _, p := range jsonCfg.Patches {
		cfg.Patches = append(cfg.Patches, patcher.PatchRule{
			TargetProto:      p.TargetProto,
			TargetMessage:    p.TargetMessage,
			ExtensionProto:   p.ExtensionProto,
			ExtensionMessage: p.ExtensionMessage,
			PatchMode:        p.PatchMode,
		})
	}

	// Convert validation overrides
	for _, vo := range jsonCfg.ValidationOverrides {
		cfg.ValidationOverrides = append(cfg.ValidationOverrides, patcher.ValidationOverride{
			TargetProto:   vo.TargetProto,
			TargetMessage: vo.TargetMessage,
			Field:         vo.Field,
			NewValidation: vo.NewValidation,
		})
	}

	return cfg, nil
}

// GenerateConfig generates a config based on naming conventions
func GenerateConfig(extensionFiles []string, fieldPrefix string, tagStart int) (*patcher.Config, error) {
	// Auto-detect patch rules based on naming conventions
	// Extension files should be named like: project_ext.proto -> patches project.proto
	cfg := &patcher.Config{
		FieldPrefix:         fieldPrefix,
		TagStart:            tagStart,
		Patches:             make([]patcher.PatchRule, 0),
		ValidationOverrides: make([]patcher.ValidationOverride, 0),
	}

	// Simple implementation - can be enhanced with proto parsing
	for _, extFile := range extensionFiles {
		// Extract base proto name from extension name
		// e.g., "project_ext.proto" -> "project.proto"
		baseName := extractBaseName(extFile)

		cfg.Patches = append(cfg.Patches, patcher.PatchRule{
			TargetProto:      baseName,
			TargetMessage:    "Project", // Convention-based
			ExtensionProto:   extFile,
			ExtensionMessage: "ProjectExtension",
			PatchMode:        "merge",
		})
	}

	return cfg, nil
}

// extractBaseName extracts the base proto name from extension name
func extractBaseName(extName string) string {
	// Simple implementation - assumes _ext.proto suffix
	if len(extName) > 10 && extName[len(extName)-10:] == "_ext.proto" {
		return extName[:len(extName)-10] + ".proto"
	}
	return extName
}

// SaveConfig saves configuration to a JSON file
func SaveConfig(cfg *patcher.Config, path string) error {
	jsonCfg := JSONConfig{
		FieldPrefix: cfg.FieldPrefix,
		TagStart:    cfg.TagStart,
	}

	// Convert patches
	for _, p := range cfg.Patches {
		jsonCfg.Patches = append(jsonCfg.Patches, struct {
			TargetProto      string `json:"target_proto"`
			TargetMessage    string `json:"target_message"`
			ExtensionProto   string `json:"extension_proto"`
			ExtensionMessage string `json:"extension_message"`
			PatchMode        string `json:"patch_mode"`
		}{
			TargetProto:      p.TargetProto,
			TargetMessage:    p.TargetMessage,
			ExtensionProto:   p.ExtensionProto,
			ExtensionMessage: p.ExtensionMessage,
			PatchMode:        p.PatchMode,
		})
	}

	// Convert validation overrides
	for _, vo := range cfg.ValidationOverrides {
		jsonCfg.ValidationOverrides = append(jsonCfg.ValidationOverrides, struct {
			TargetProto   string            `json:"target_proto"`
			TargetMessage string            `json:"target_message"`
			Field         string            `json:"field"`
			NewValidation map[string]string `json:"new_validation"`
		}{
			TargetProto:   vo.TargetProto,
			TargetMessage: vo.TargetMessage,
			Field:         vo.Field,
			NewValidation: vo.NewValidation,
		})
	}

	data, err := json.MarshalIndent(&jsonCfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}
