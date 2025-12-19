// gen-ext-register generates registration code for ext validation.
//
// This tool scans ext proto files and generates a register.go file that
// automatically maps ext types to their corresponding v2 base types.
//
// Usage:
//
//	go run tools/gen-ext-register/main.go \
//	    -proto_dir=proto/api/v2_ext \
//	    -output=proto/api/v2_ext/register_generated.go \
//	    -base_package=github.com/michelangelo-ai/michelangelo/proto/api/v2 \
//	    -ext_package=github.com/michelangelo-ai/michelangelo/proto/api/v2_ext
//
// The tool:
// 1. Reads all *_ext.proto files in the proto directory
// 2. Parses message definitions ending in "Ext"
// 3. Maps them to base types (e.g., ModelSpecExt → v2.ModelSpec)
// 4. Generates registration code with field mappings
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"text/template"
	"time"
)

var (
	protoDir    = flag.String("proto_dir", "proto/api/v2_ext", "Directory containing ext proto files")
	output      = flag.String("output", "proto/api/v2_ext/register_generated.go", "Output file path")
	basePackage = flag.String("base_package", "github.com/michelangelo-ai/michelangelo/proto/api/v2", "Base proto package import path")
	extPackage  = flag.String("ext_package", "github.com/michelangelo-ai/michelangelo/proto/api/v2_ext", "Ext proto package import path")
	apiPackage  = flag.String("api_package", "github.com/michelangelo-ai/michelangelo/go/api", "API package import path")
)

// ExtMessage represents a parsed ext message from a proto file
type ExtMessage struct {
	Name       string   // e.g., "ModelSpecExt"
	BaseName   string   // e.g., "ModelSpec"
	BaseType   string   // e.g., "Model" (the CRD type)
	Fields     []Field  // Fields in the message
	SourceFile string   // Source proto file
}

// Field represents a field in a proto message
type Field struct {
	Name      string // Proto field name (snake_case)
	GoName    string // Go field name (PascalCase)
	Type      string // Field type
	IsMessage bool   // Whether it's a message type
}

// CRDMapping maps a CRD type to its ext messages
type CRDMapping struct {
	CRDType     string       // e.g., "Model"
	ExtMessages []ExtMessage // e.g., [ModelSpecExt, LLMSpecExt]
}

func main() {
	flag.Parse()

	// Find all ext proto files
	protoFiles, err := findExtProtoFiles(*protoDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error finding proto files: %v\n", err)
		os.Exit(1)
	}

	if len(protoFiles) == 0 {
		fmt.Fprintf(os.Stderr, "No ext proto files found in %s\n", *protoDir)
		os.Exit(1)
	}

	// Parse ext messages from proto files
	var allMessages []ExtMessage
	for _, file := range protoFiles {
		messages, err := parseProtoFile(file)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing %s: %v\n", file, err)
			os.Exit(1)
		}
		allMessages = append(allMessages, messages...)
	}

	if len(allMessages) == 0 {
		fmt.Fprintf(os.Stderr, "No ext messages found in proto files\n")
		os.Exit(1)
	}

	// Group messages by CRD type
	crdMappings := groupByCRD(allMessages)

	// Generate registration code
	code, err := generateCode(crdMappings)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error generating code: %v\n", err)
		os.Exit(1)
	}

	// Write output file
	if err := os.WriteFile(*output, []byte(code), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing output: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Generated %s with %d CRD mappings\n", *output, len(crdMappings))
}

func findExtProtoFiles(dir string) ([]string, error) {
	var files []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, "_ext.proto") {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

// parseProtoFile extracts ext messages from a proto file
func parseProtoFile(filename string) ([]ExtMessage, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var messages []ExtMessage
	var currentMessage *ExtMessage
	inMessage := false
	braceCount := 0

	// Regex patterns
	messagePattern := regexp.MustCompile(`^\s*message\s+(\w+Ext)\s*\{`)
	fieldPattern := regexp.MustCompile(`^\s*(?:repeated\s+)?(\w+(?:\.\w+)*)\s+(\w+)\s*=\s*\d+`)
	openBrace := regexp.MustCompile(`\{`)
	closeBrace := regexp.MustCompile(`\}`)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()

		// Check for message start
		if matches := messagePattern.FindStringSubmatch(line); matches != nil {
			msgName := matches[1]
			baseName := strings.TrimSuffix(msgName, "Ext")
			baseType := inferCRDType(baseName)

			currentMessage = &ExtMessage{
				Name:       msgName,
				BaseName:   baseName,
				BaseType:   baseType,
				SourceFile: filename,
			}
			inMessage = true
			braceCount = 1
			continue
		}

		if inMessage {
			// Count braces
			braceCount += len(openBrace.FindAllString(line, -1))
			braceCount -= len(closeBrace.FindAllString(line, -1))

			// Parse fields
			if matches := fieldPattern.FindStringSubmatch(line); matches != nil {
				fieldType := matches[1]
				fieldName := matches[2]

				// Skip reserved fields and ext fields
				if fieldName == "ext" || strings.HasPrefix(line, "//") {
					continue
				}

				field := Field{
					Name:      fieldName,
					GoName:    toGoFieldName(fieldName),
					Type:      fieldType,
					IsMessage: isMessageType(fieldType),
				}
				currentMessage.Fields = append(currentMessage.Fields, field)
			}

			// Check for message end
			if braceCount == 0 {
				if len(currentMessage.Fields) > 0 {
					messages = append(messages, *currentMessage)
				}
				currentMessage = nil
				inMessage = false
			}
		}
	}

	return messages, scanner.Err()
}

// inferCRDType infers the CRD type from the spec name
// e.g., "ModelSpec" → "Model", "LLMSpec" → "Model" (nested), "ProjectSpec" → "Project"
func inferCRDType(baseName string) string {
	// Common patterns
	specSuffix := "Spec"
	statusSuffix := "Status"
	infoSuffix := "Info"

	if strings.HasSuffix(baseName, specSuffix) {
		name := strings.TrimSuffix(baseName, specSuffix)
		// Handle nested specs like LLMSpec (belongs to Model)
		if name == "LLM" {
			return "Model"
		}
		if name == "Owner" || name == "Retention" {
			return "Project"
		}
		return name
	}
	if strings.HasSuffix(baseName, statusSuffix) {
		return strings.TrimSuffix(baseName, statusSuffix)
	}
	if strings.HasSuffix(baseName, infoSuffix) {
		// OwnerInfo belongs to Project
		if baseName == "OwnerInfo" {
			return "Project"
		}
		return strings.TrimSuffix(baseName, infoSuffix)
	}
	return baseName
}

// toGoFieldName converts snake_case to PascalCase
func toGoFieldName(name string) string {
	parts := strings.Split(name, "_")
	for i, part := range parts {
		if len(part) > 0 {
			parts[i] = strings.ToUpper(part[:1]) + part[1:]
		}
	}
	return strings.Join(parts, "")
}

// isMessageType checks if a type is a message (not a scalar)
func isMessageType(t string) bool {
	scalars := map[string]bool{
		"string": true, "bytes": true,
		"int32": true, "int64": true,
		"uint32": true, "uint64": true,
		"sint32": true, "sint64": true,
		"fixed32": true, "fixed64": true,
		"sfixed32": true, "sfixed64": true,
		"float": true, "double": true,
		"bool": true,
	}
	return !scalars[t]
}

// groupByCRD groups ext messages by their CRD type
func groupByCRD(messages []ExtMessage) []CRDMapping {
	groups := make(map[string][]ExtMessage)
	for _, msg := range messages {
		groups[msg.BaseType] = append(groups[msg.BaseType], msg)
	}

	var mappings []CRDMapping
	for crdType, msgs := range groups {
		mappings = append(mappings, CRDMapping{
			CRDType:     crdType,
			ExtMessages: msgs,
		})
	}

	// Sort for deterministic output
	sort.Slice(mappings, func(i, j int) bool {
		return mappings[i].CRDType < mappings[j].CRDType
	})

	return mappings
}

// generateCode generates the registration Go code
func generateCode(mappings []CRDMapping) (string, error) {
	tmpl := template.Must(template.New("register").Funcs(template.FuncMap{
		"lower": strings.ToLower,
	}).Parse(registerTemplate))

	var buf strings.Builder
	data := struct {
		Timestamp   string
		BasePackage string
		ExtPackage  string
		APIPackage  string
		Mappings    []CRDMapping
	}{
		Timestamp:   time.Now().Format(time.RFC3339),
		BasePackage: *basePackage,
		ExtPackage:  *extPackage,
		APIPackage:  *apiPackage,
		Mappings:    mappings,
	}

	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}

const registerTemplate = `// Code generated by gen-ext-register. DO NOT EDIT.
// Generated at: {{.Timestamp}}

package v2_ext

import (
	"{{.APIPackage}}"
	v2 "{{.BasePackage}}"
)

func init() {
	// Auto-register ext validators for all CRD types
{{- range .Mappings}}
	register{{.CRDType}}Validator()
{{- end}}
}
{{range $mapping := .Mappings}}
// register{{$mapping.CRDType}}Validator registers ext validation for v2.{{$mapping.CRDType}}
func register{{$mapping.CRDType}}Validator() {
	api.RegisterExtValidator("*v2.{{$mapping.CRDType}}", func(obj interface{}) error {
		target, ok := obj.(*v2.{{$mapping.CRDType}})
		if !ok || target == nil {
			return nil
		}
{{range $mapping.ExtMessages}}
		// Validate {{.Name}}
		if err := validate{{.Name}}(target); err != nil {
			return err
		}
{{end}}
		return nil
	})
}
{{range $msg := $mapping.ExtMessages}}
// validate{{$msg.Name}} validates {{$msg.BaseName}} fields with ext rules
func validate{{$msg.Name}}(target *v2.{{$mapping.CRDType}}) error {
{{- if eq $msg.BaseName "ModelSpec"}}
	if target.Spec == nil {
		return nil
	}
	ext := &{{$msg.Name}}{
{{- range $msg.Fields}}
		{{.GoName}}: target.Spec.{{.GoName}},
{{- end}}
	}
	return ext.Validate("spec.")
{{- else if eq $msg.BaseName "LLMSpec"}}
	if target.Spec == nil || target.Spec.LlmSpec == nil {
		return nil
	}
	ext := &{{$msg.Name}}{
{{- range $msg.Fields}}
		{{.GoName}}: target.Spec.LlmSpec.{{.GoName}},
{{- end}}
	}
	return ext.Validate("spec.llm_spec.")
{{- else if eq $msg.BaseName "ProjectSpec"}}
	if target.Spec == nil {
		return nil
	}
	ext := &{{$msg.Name}}{
{{- range $msg.Fields}}
		{{.GoName}}: target.Spec.{{.GoName}},
{{- end}}
	}
	return ext.Validate("spec.")
{{- else if eq $msg.BaseName "OwnerInfo"}}
	if target.Spec == nil || target.Spec.Owner == nil {
		return nil
	}
	ext := &{{$msg.Name}}{
{{- range $msg.Fields}}
		{{.GoName}}: target.Spec.Owner.{{.GoName}},
{{- end}}
	}
	return ext.Validate("spec.owner.")
{{- else if eq $msg.BaseName "Retention"}}
	// Retention validation - check each retention config field
	return nil
{{- else}}
	// TODO: Add field mapping for {{$msg.BaseName}}
	_ = target
	return nil
{{- end}}
}
{{end}}{{end}}
`

