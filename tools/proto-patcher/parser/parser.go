package parser

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/desc/protoparse"
	"google.golang.org/protobuf/types/descriptorpb"
)

// ProtoFile represents a parsed proto file
type ProtoFile struct {
	Syntax   string
	Package  string
	Imports  []string
	Options  map[string]string
	Messages []*Message
	FilePath string
}

// Message represents a proto message
type Message struct {
	Name     string
	Fields   []*Field
	Nested   []*Message
	Comments string
}

// Field represents a proto field
type Field struct {
	Name     string
	Type     string
	Tag      int
	Repeated bool
	Optional bool
	Options  []*FieldOption
	Comments string
}

// FieldOption represents a field option (like validation annotations)
type FieldOption struct {
	Name  string
	Value string
}

// Parser parses proto files
type Parser struct {
	importPaths []string
}

// NewParser creates a new proto parser
func NewParser(importPaths []string) *Parser {
	return &Parser{
		importPaths: importPaths,
	}
}

// ParseFile parses a single proto file
func (p *Parser) ParseFile(filename string) (*ProtoFile, error) {
	// Create parser with import paths
	parser := &protoparse.Parser{
		ImportPaths:           p.importPaths,
		IncludeSourceCodeInfo: true,
	}

	// If filename is an absolute path and we have import paths,
	// extract the relative path
	parseFilename := filename
	if len(p.importPaths) > 0 && filepath.IsAbs(filename) {
		for _, importPath := range p.importPaths {
			if filepath.IsAbs(importPath) {
				rel, err := filepath.Rel(importPath, filename)
				if err == nil && !filepath.IsAbs(rel) {
					parseFilename = rel
					break
				}
			}
		}
	}

	// Parse the file
	fds, err := parser.ParseFiles(parseFilename)
	if err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", filename, err)
	}

	if len(fds) == 0 {
		return nil, fmt.Errorf("no descriptors returned for %s", filename)
	}

	fd := fds[0]

	// Convert to our internal representation
	protoFile := &ProtoFile{
		Syntax:   getSyntax(fd),
		Package:  fd.GetPackage(),
		Imports:  getImports(fd),
		Options:  getFileOptions(fd),
		Messages: make([]*Message, 0),
		FilePath: filename,
	}

	// Parse messages
	for _, msgDesc := range fd.GetMessageTypes() {
		msg := p.parseMessage(msgDesc)
		protoFile.Messages = append(protoFile.Messages, msg)
	}

	return protoFile, nil
}

// ParseFiles parses multiple proto files
func (p *Parser) ParseFiles(filenames []string) (map[string]*ProtoFile, error) {
	result := make(map[string]*ProtoFile)

	for _, filename := range filenames {
		pf, err := p.ParseFile(filename)
		if err != nil {
			return nil, err
		}
		key := filepath.Base(filename)
		result[key] = pf
	}

	return result, nil
}

// parseMessage converts a message descriptor to our Message type
func (p *Parser) parseMessage(msgDesc *desc.MessageDescriptor) *Message {
	msg := &Message{
		Name:     msgDesc.GetName(),
		Fields:   make([]*Field, 0),
		Nested:   make([]*Message, 0),
		Comments: "", // Comments can be added later if needed
	}

	// Parse fields
	for _, fieldDesc := range msgDesc.GetFields() {
		field := p.parseField(fieldDesc)
		msg.Fields = append(msg.Fields, field)
	}

	// Parse nested messages
	for _, nestedDesc := range msgDesc.GetNestedMessageTypes() {
		nested := p.parseMessage(nestedDesc)
		msg.Nested = append(msg.Nested, nested)
	}

	return msg
}

// parseField converts a field descriptor to our Field type
func (p *Parser) parseField(fieldDesc *desc.FieldDescriptor) *Field {
	field := &Field{
		Name:     fieldDesc.GetName(),
		Type:     getFieldType(fieldDesc),
		Tag:      int(fieldDesc.GetNumber()),
		Repeated: fieldDesc.IsRepeated(),
		Optional: fieldDesc.GetOneOf() == nil && !fieldDesc.IsRequired() && !fieldDesc.IsRepeated(),
		Options:  p.parseFieldOptions(fieldDesc),
		Comments: "", // Comments can be added later if needed
	}

	return field
}

// parseFieldOptions extracts field options
func (p *Parser) parseFieldOptions(fieldDesc *desc.FieldDescriptor) []*FieldOption {
	options := make([]*FieldOption, 0)

	// Get options from the field descriptor
	opts := fieldDesc.GetFieldOptions()
	if opts == nil {
		return options
	}

	// Extract known extensions (like michelangelo.api.validation)
	// This is a simplified version - full implementation would parse all options
	if opts.String() != "" {
		options = append(options, &FieldOption{
			Name:  "raw_options",
			Value: opts.String(),
		})
	}

	return options
}

// Helper functions

func getSyntax(fd *desc.FileDescriptor) string {
	if fd.IsProto3() {
		return "proto3"
	}
	return "proto2"
}

func getImports(fd *desc.FileDescriptor) []string {
	imports := make([]string, 0)
	for _, dep := range fd.GetDependencies() {
		imports = append(imports, dep.GetName())
	}
	return imports
}

func getFileOptions(fd *desc.FileDescriptor) map[string]string {
	options := make(map[string]string)

	opts := fd.GetFileOptions()
	if opts == nil {
		return options
	}

	// Extract go_package option
	if opts.GetGoPackage() != "" {
		options["go_package"] = opts.GetGoPackage()
	}

	return options
}

func getFieldType(fieldDesc *desc.FieldDescriptor) string {
	if fieldDesc.IsMap() {
		keyType := getScalarTypeName(fieldDesc.GetMapKeyType().GetType())
		valType := getScalarTypeName(fieldDesc.GetMapValueType().GetType())
		return fmt.Sprintf("map<%s, %s>", keyType, valType)
	}

	if fieldDesc.GetMessageType() != nil {
		return fieldDesc.GetMessageType().GetName()
	}

	if fieldDesc.GetEnumType() != nil {
		return fieldDesc.GetEnumType().GetName()
	}

	return getScalarTypeName(fieldDesc.GetType())
}

func getScalarTypeName(t descriptorpb.FieldDescriptorProto_Type) string {
	// Map the protobuf type enum to the proto type name
	typeName := t.String()
	// Remove "TYPE_" prefix if present
	if len(typeName) > 5 && typeName[:5] == "TYPE_" {
		return strings.ToLower(typeName[5:])
	}
	return strings.ToLower(typeName)
}

func getComments(fd *desc.FileDescriptor) string {
	// Comments extraction is optional and can be enhanced later
	// For now, we'll skip this to avoid dependency issues
	return ""
}
