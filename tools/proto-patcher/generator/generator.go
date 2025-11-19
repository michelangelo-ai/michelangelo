package generator

import (
	"fmt"
	"sort"
	"strings"

	"github.com/michelangelo-ai/michelangelo/tools/proto-patcher/parser"
)

// Generator generates proto file content from parsed proto structures
type Generator struct {
	indent string
}

// NewGenerator creates a new proto file generator
func NewGenerator() *Generator {
	return &Generator{
		indent: "  ",
	}
}

// Generate generates proto file content
func (g *Generator) Generate(pf *parser.ProtoFile) (string, error) {
	var sb strings.Builder

	// Write syntax
	sb.WriteString(fmt.Sprintf("syntax = \"%s\";\n\n", pf.Syntax))

	// Write package
	if pf.Package != "" {
		sb.WriteString(fmt.Sprintf("package %s;\n\n", pf.Package))
	}

	// Write imports (sorted for consistency)
	if len(pf.Imports) > 0 {
		sortedImports := make([]string, len(pf.Imports))
		copy(sortedImports, pf.Imports)
		sort.Strings(sortedImports)

		for _, imp := range sortedImports {
			sb.WriteString(fmt.Sprintf("import \"%s\";\n", imp))
		}
		sb.WriteString("\n")
	}

	// Write file options
	if len(pf.Options) > 0 {
		for key, value := range pf.Options {
			sb.WriteString(fmt.Sprintf("option %s = \"%s\";\n", key, value))
		}
		sb.WriteString("\n")
	}

	// Write messages
	for i, msg := range pf.Messages {
		if i > 0 {
			sb.WriteString("\n")
		}
		if err := g.writeMessage(&sb, msg, 0); err != nil {
			return "", err
		}
	}

	return sb.String(), nil
}

// writeMessage writes a message to the string builder
func (g *Generator) writeMessage(sb *strings.Builder, msg *parser.Message, level int) error {
	indent := strings.Repeat(g.indent, level)

	// Write comments
	if msg.Comments != "" {
		for _, line := range strings.Split(strings.TrimSpace(msg.Comments), "\n") {
			sb.WriteString(fmt.Sprintf("%s// %s\n", indent, line))
		}
	}

	// Write message header
	sb.WriteString(fmt.Sprintf("%smessage %s {\n", indent, msg.Name))

	// Write nested messages first
	for _, nested := range msg.Nested {
		if err := g.writeMessage(sb, nested, level+1); err != nil {
			return err
		}
		sb.WriteString("\n")
	}

	// Sort fields by tag number for consistency
	sortedFields := make([]*parser.Field, len(msg.Fields))
	copy(sortedFields, msg.Fields)
	sort.Slice(sortedFields, func(i, j int) bool {
		return sortedFields[i].Tag < sortedFields[j].Tag
	})

	// Write fields
	for _, field := range sortedFields {
		if err := g.writeField(sb, field, level+1); err != nil {
			return err
		}
	}

	// Write message closing
	sb.WriteString(fmt.Sprintf("%s}\n", indent))

	return nil
}

// writeField writes a field to the string builder
func (g *Generator) writeField(sb *strings.Builder, field *parser.Field, level int) error {
	indent := strings.Repeat(g.indent, level)

	// Write field comments
	if field.Comments != "" {
		for _, line := range strings.Split(strings.TrimSpace(field.Comments), "\n") {
			sb.WriteString(fmt.Sprintf("%s// %s\n", indent, line))
		}
	}

	// Build field line
	var fieldLine strings.Builder

	// Add repeated/optional modifier
	if field.Repeated {
		fieldLine.WriteString("repeated ")
	}

	// Add type
	fieldLine.WriteString(field.Type)
	fieldLine.WriteString(" ")

	// Add name
	fieldLine.WriteString(field.Name)
	fieldLine.WriteString(" = ")

	// Add tag
	fieldLine.WriteString(fmt.Sprintf("%d", field.Tag))

	// Add options
	if len(field.Options) > 0 {
		fieldLine.WriteString(" [")
		optionsStrs := make([]string, 0, len(field.Options))
		for _, opt := range field.Options {
			// Handle different option formats
			if opt.Name == "raw_options" {
				// Raw options from proto parsing - parse and format them
				if opt.Value != "" {
					optionsStrs = append(optionsStrs, g.formatRawOptions(opt.Value))
				}
			} else {
				// Standard option
				optionsStrs = append(optionsStrs, fmt.Sprintf("%s = %s", opt.Name, opt.Value))
			}
		}
		fieldLine.WriteString(strings.Join(optionsStrs, ", "))
		fieldLine.WriteString("]")
	}

	fieldLine.WriteString(";")

	sb.WriteString(fmt.Sprintf("%s%s\n", indent, fieldLine.String()))

	return nil
}

// formatRawOptions formats raw options string
func (g *Generator) formatRawOptions(rawOpts string) string {
	// This is simplified - full implementation would properly parse and format
	// For now, just return as-is if it looks like valid options
	cleaned := strings.TrimSpace(rawOpts)
	
	// Remove outer brackets if present
	cleaned = strings.TrimPrefix(cleaned, "[")
	cleaned = strings.TrimSuffix(cleaned, "]")
	
	return cleaned
}

// GenerateAll generates proto files for all parsed protos
func (g *Generator) GenerateAll(protos map[string]*parser.ProtoFile) (map[string]string, error) {
	result := make(map[string]string)

	for filename, pf := range protos {
		content, err := g.Generate(pf)
		if err != nil {
			return nil, fmt.Errorf("failed to generate %s: %w", filename, err)
		}
		result[filename] = content
	}

	return result, nil
}



