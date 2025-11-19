package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

var (
	baseSources = flag.String("base_sources", "", "File containing list of base proto files")
	extProtos   = flag.String("ext_protos", "", "Space-separated list of extension proto files")
	fieldPrefix = flag.String("field_prefix", "EXT_", "Prefix for extension fields")
	tagStart    = flag.Int("tag_start", 999, "Starting tag number")
	output      = flag.String("output", "", "Output JSON file")
)

type Config struct {
	Patches             []Patch              `json:"patches"`
	ValidationOverrides []ValidationOverride `json:"validation_overrides,omitempty"`
	FieldPrefix         string               `json:"field_prefix"`
	TagStart            int                  `json:"tag_start"`
}

type Patch struct {
	TargetProto      string `json:"target_proto"`
	TargetMessage    string `json:"target_message"`
	ExtensionProto   string `json:"extension_proto"`
	ExtensionMessage string `json:"extension_message"`
	PatchMode        string `json:"patch_mode"`
}

type ValidationOverride struct {
	TargetProto   string            `json:"target_proto"`
	TargetMessage string            `json:"target_message"`
	Field         string            `json:"field"`
	NewValidation map[string]string `json:"new_validation"`
}

func main() {
	flag.Parse()

	if *extProtos == "" {
		log.Fatal("--ext_protos is required")
	}
	if *output == "" {
		log.Fatal("--output is required")
	}

	// Parse extension proto files
	extFiles := strings.Fields(*extProtos)

	config := Config{
		Patches:     []Patch{},
		FieldPrefix: *fieldPrefix,
		TagStart:    *tagStart,
	}

	// Auto-generate patches based on naming conventions
	for _, extFile := range extFiles {
		basename := filepath.Base(extFile)
		if !strings.HasSuffix(basename, "_ext.proto") {
			continue
		}

		// Extract base name: project_ext.proto -> project
		baseName := strings.TrimSuffix(basename, "_ext.proto")

		// Convention: ProjectSpecExtension extends ProjectSpec
		targetProto := fmt.Sprintf("michelangelo/api/v2/%s.proto", baseName)

		// For now, assume Spec and Status extensions
		// TODO: Parse proto files to auto-detect message names
		patches := []Patch{
			{
				TargetProto:      targetProto,
				TargetMessage:    capitalize(baseName) + "Spec",
				ExtensionProto:   extFile,
				ExtensionMessage: capitalize(baseName) + "SpecExtension",
				PatchMode:        "merge_fields",
			},
			{
				TargetProto:      targetProto,
				TargetMessage:    capitalize(baseName) + "Status",
				ExtensionProto:   extFile,
				ExtensionMessage: capitalize(baseName) + "StatusExtension",
				PatchMode:        "merge_fields",
			},
		}

		config.Patches = append(config.Patches, patches...)
	}

	// Write JSON
	data, err := json.MarshalIndent(&config, "", "  ")
	if err != nil {
		log.Fatalf("Failed to marshal config: %v", err)
	}

	if err := os.WriteFile(*output, data, 0644); err != nil {
		log.Fatalf("Failed to write output: %v", err)
	}

	fmt.Printf("Generated config: %s\n", *output)
	fmt.Printf("Patches: %d\n", len(config.Patches))
}

func capitalize(s string) string {
	if len(s) == 0 {
		return s
	}
	// Convert snake_case to PascalCase
	parts := strings.Split(s, "_")
	for i, part := range parts {
		if len(part) > 0 {
			parts[i] = strings.ToUpper(part[:1]) + part[1:]
		}
	}
	return strings.Join(parts, "")
}

