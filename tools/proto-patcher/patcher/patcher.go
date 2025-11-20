package patcher

import (
	"fmt"
	"strings"

	"github.com/michelangelo-ai/michelangelo/tools/proto-patcher/parser"
)

// Config defines the patching configuration
type Config struct {
	Patches             []PatchRule
	ValidationOverrides []ValidationOverride
	FieldPrefix         string
	TagStart            int
}

// PatchRule defines how to patch a message
type PatchRule struct {
	TargetProto      string
	TargetMessage    string
	ExtensionProto   string
	ExtensionMessage string
	PatchMode        string // "merge" (merge fields) or "copy" (copy whole message)
}

// ValidationOverride defines validation rule overrides
type ValidationOverride struct {
	TargetProto   string
	TargetMessage string
	Field         string
	NewValidation map[string]string
}

// Patcher applies patches to proto files
type Patcher struct {
	config *Config
}

// NewPatcher creates a new patcher
func NewPatcher(config *Config) *Patcher {
	return &Patcher{config: config}
}

// Patch applies patches to base protos using extension protos
func (p *Patcher) Patch(baseProtos, extProtos map[string]*parser.ProtoFile) (map[string]*parser.ProtoFile, error) {
	result := make(map[string]*parser.ProtoFile)

	// Process each patch rule
	for _, rule := range p.config.Patches {
		// Find base proto file
		baseFile, ok := baseProtos[rule.TargetProto]
		if !ok {
			// Try with just filename
			for _, file := range baseProtos {
				if strings.HasSuffix(file.FilePath, rule.TargetProto) {
					baseFile = file
					ok = true
					break
				}
			}
		}
		if !ok {
			return nil, fmt.Errorf("base proto not found: %s", rule.TargetProto)
		}

		// Find extension proto file
		extFile, ok := extProtos[rule.ExtensionProto]
		if !ok {
			// Try with just filename
			for _, file := range extProtos {
				if strings.HasSuffix(file.FilePath, rule.ExtensionProto) {
					extFile = file
					ok = true
					break
				}
			}
		}
		if !ok {
			return nil, fmt.Errorf("extension proto not found: %s", rule.ExtensionProto)
		}

		// Get or create patched file
		// If we already have a patched version from a previous rule, use that
		// Otherwise, clone the base file
		outputName := strings.TrimSuffix(rule.TargetProto, ".proto") + "_patched.proto"
		patchedFile, exists := result[outputName]
		if !exists {
			patchedFile = p.cloneProtoFile(baseFile)
		}

		// Find extension message
		extMsg := p.findMessage(extFile.Messages, rule.ExtensionMessage)
		if extMsg == nil {
			return nil, fmt.Errorf("extension message not found: %s", rule.ExtensionMessage)
		}

		// Determine patch mode (default to "copy")
		patchMode := rule.PatchMode
		if patchMode == "" {
			patchMode = "copy"
		}

		if patchMode == "copy" {
			// Copy the entire extension message as-is
			if err := p.copyMessage(patchedFile, extMsg); err != nil {
				return nil, fmt.Errorf("failed to copy message: %w", err)
			}
		} else if patchMode == "merge" {
			// Merge fields into existing target message
			targetMsg := p.findMessage(patchedFile.Messages, rule.TargetMessage)
			if targetMsg == nil {
				return nil, fmt.Errorf("target message not found: %s", rule.TargetMessage)
			}
			if err := p.mergeFields(targetMsg, extMsg); err != nil {
				return nil, fmt.Errorf("failed to merge fields: %w", err)
			}
		} else {
			return nil, fmt.Errorf("unknown patch mode: %s (use 'copy' or 'merge')", patchMode)
		}

		// Store patched file (may already be in result from previous patch to same file)
		result[outputName] = patchedFile
	}

	// Apply validation overrides
	if err := p.applyValidationOverrides(result); err != nil {
		return nil, fmt.Errorf("failed to apply validation overrides: %w", err)
	}

	return result, nil
}

// copyMessage copies an entire message into the proto file
func (p *Patcher) copyMessage(protoFile *parser.ProtoFile, msg *parser.Message) error {
	// Clone the entire message (including all nested messages and fields)
	clonedMsg := p.cloneMessage(msg)

	// Check if a message with this name already exists
	for _, existingMsg := range protoFile.Messages {
		if existingMsg.Name == msg.Name {
			return fmt.Errorf("message %s already exists in proto file", msg.Name)
		}
	}

	// Add the cloned message to the proto file
	protoFile.Messages = append(protoFile.Messages, clonedMsg)

	return nil
}

// mergeFields merges extension fields into target message
func (p *Patcher) mergeFields(target, extension *parser.Message) error {
	tagOffset := 0

	for _, extField := range extension.Fields {
		// Clone the field
		newField := p.cloneField(extField)

		// Add prefix to field name
		newField.Name = p.config.FieldPrefix + extField.Name

		// Assign new tag number
		newField.Tag = p.config.TagStart + tagOffset
		tagOffset++

		// Check for tag collisions
		if p.hasTagCollision(target.Fields, newField.Tag) {
			return fmt.Errorf("tag number collision: %d already exists", newField.Tag)
		}

		// Check for name collisions
		if p.hasNameCollision(target.Fields, newField.Name) {
			return fmt.Errorf("field name collision: %s already exists", newField.Name)
		}

		// Add field to target
		target.Fields = append(target.Fields, newField)
	}

	return nil
}

// applyValidationOverrides applies validation rule overrides
func (p *Patcher) applyValidationOverrides(patchedProtos map[string]*parser.ProtoFile) error {
	for _, override := range p.config.ValidationOverrides {
		// Find the proto file
		var targetFile *parser.ProtoFile
		for name, file := range patchedProtos {
			if strings.Contains(name, override.TargetProto) {
				targetFile = file
				break
			}
		}
		if targetFile == nil {
			continue
		}

		// Find the message
		msg := p.findMessage(targetFile.Messages, override.TargetMessage)
		if msg == nil {
			continue
		}

		// Find the field
		for _, field := range msg.Fields {
			if field.Name == override.Field {
				// Replace validation options
				// This is simplified - full implementation would parse and merge properly
				field.Options = make([]*parser.FieldOption, 0)
				for key, value := range override.NewValidation {
					field.Options = append(field.Options, &parser.FieldOption{
						Name:  key,
						Value: value,
					})
				}
				break
			}
		}
	}

	return nil
}

// Helper functions

func (p *Patcher) cloneProtoFile(src *parser.ProtoFile) *parser.ProtoFile {
	dst := &parser.ProtoFile{
		Syntax:   src.Syntax,
		Package:  src.Package,
		Imports:  make([]string, len(src.Imports)),
		Options:  make(map[string]string),
		Messages: make([]*parser.Message, 0),
		FilePath: src.FilePath,
	}

	copy(dst.Imports, src.Imports)
	for k, v := range src.Options {
		dst.Options[k] = v
	}

	for _, msg := range src.Messages {
		dst.Messages = append(dst.Messages, p.cloneMessage(msg))
	}

	return dst
}

func (p *Patcher) cloneMessage(src *parser.Message) *parser.Message {
	dst := &parser.Message{
		Name:     src.Name,
		Fields:   make([]*parser.Field, 0),
		Nested:   make([]*parser.Message, 0),
		Comments: src.Comments,
	}

	for _, field := range src.Fields {
		dst.Fields = append(dst.Fields, p.cloneField(field))
	}

	for _, nested := range src.Nested {
		dst.Nested = append(dst.Nested, p.cloneMessage(nested))
	}

	return dst
}

func (p *Patcher) cloneField(src *parser.Field) *parser.Field {
	dst := &parser.Field{
		Name:     src.Name,
		Type:     src.Type,
		Tag:      src.Tag,
		Repeated: src.Repeated,
		Optional: src.Optional,
		Options:  make([]*parser.FieldOption, 0),
		Comments: src.Comments,
	}

	for _, opt := range src.Options {
		dst.Options = append(dst.Options, &parser.FieldOption{
			Name:  opt.Name,
			Value: opt.Value,
		})
	}

	return dst
}

func (p *Patcher) findMessage(messages []*parser.Message, name string) *parser.Message {
	for _, msg := range messages {
		if msg.Name == name {
			return msg
		}
		// Search nested messages
		if nested := p.findMessage(msg.Nested, name); nested != nil {
			return nested
		}
	}
	return nil
}

func (p *Patcher) hasTagCollision(fields []*parser.Field, tag int) bool {
	for _, field := range fields {
		if field.Tag == tag {
			return true
		}
	}
	return false
}

func (p *Patcher) hasNameCollision(fields []*parser.Field, name string) bool {
	for _, field := range fields {
		if field.Name == name {
			return true
		}
	}
	return false
}
