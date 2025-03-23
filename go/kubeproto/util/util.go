package util

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"

	"github.com/michelangelo-ai/michelangelo/go/kubeproto/pboptions"

	"github.com/dave/dst"
	gogoproto "github.com/gogo/protobuf/proto"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"
)

var reGogoTypeRemapPath = regexp.MustCompile(`github\.com\/gogo\/protobuf\/types`)

// ReplaceImportPath replaces the import path of GOGO_WELL_KNOW_TYPE_REMAPS modifiers in
// the CodeGeneratorRequest's parameter so that we can pass the golang's protobuf
// compiler's consistency check.
func ReplaceImportPath(req *pluginpb.CodeGeneratorRequest) {
	// golang's protobuf compiler enforces a consistency check that requires all the protobufs
	// reside in the same import path to have the same package name.
	//   https://github.com/protocolbuffers/protobuf-go/blob/master/compiler/protogen/protogen.go#L281
	// If package name is not specified, it is derived from the base name of the file path.
	//
	// In the build rule, GOGO_WELL_KNOW_TYPE_REMAPS introduces multiple modifiers, i,e.,
	//   Mgoogle/protobuf/any.proto=github.com/gogo/protobuf/types and
	//   Mgoogle/protobuf/timestamp.proto=github.com/gogo/protobuf/types
	// These modifiers violate the consistency check as the compiler thinks package 'any'
	// and 'timestamp' are from the same import path.
	//
	// Thus, we change the remap modifiers to have the same package name to work around
	// the consistency check.
	newParameter := reGogoTypeRemapPath.ReplaceAllString(
		req.GetParameter(), "github.com/gogo/protobuf/types;types")
	req.Parameter = &newParameter
}

// GetPluginAndExtensions generates protogen plugin and extensions given the data.
//
// When overrideGoPackageOpt is set, it sets the go_package option to the protobuf package. This is useful when the
// caller is a protoc plugin that generates code for languages (e.g. yaml, sql) other than Go. We are using
// "google.golang.org/protobuf/compiler/protogen" to parse protobuf code. Since this library is a protobuf to Go
// compiler, it will fail if the go_package option is not set properly. However, when generating code for other
// languages, we shouldn't require the go_package option. So, we set the go_package option to the protobuf package,
// which is always a "valid" value for go_package, to make the library happy.
func GetPluginAndExtensions(data []byte, overrideGoPackageOpt bool) (*protogen.Plugin, *protoregistry.Types, error) {
	req := &pluginpb.CodeGeneratorRequest{}
	err := proto.Unmarshal(data, req)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal input from protoc %v", err)
	}
	ReplaceImportPath(req)

	if overrideGoPackageOpt {
		for _, protoFile := range req.GetProtoFile() {
			protoPackage := protoFile.Package
			packageStr := strings.ReplaceAll(*protoPackage, ".", "/")
			protoFile.Options.GoPackage = &packageStr
		}
	}

	// initialize protobuf generator
	gen, err := protogen.Options{}.New(req)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialize golang proto generator %v", err)
	}

	// Load protobuf extensions from all the imported protobuf files
	extTypes := pboptions.LoadPBExtensions(gen.Files)
	return gen, extTypes, nil
}

// GetFieldOptions gets the field options for the given field
func GetFieldOptions(field *protogen.Field, extTypes *protoregistry.Types) (*pboptions.Options, error) {
	pbOptions := field.Desc.Options().(*descriptorpb.FieldOptions)
	return pboptions.ReadOptions(extTypes, pbOptions)
}

// GenericResolver implements jsonpb.AnyResolver to support marshal/unmarshal generic
// protobuf types packed in Any.  We implement this resovler as the defaultResolveAny
// in jsonpb https://github.com/gogo/protobuf/blob/master/jsonpb/jsonpb.go#L92
// reports unknown message type error if the protobuf type does not register in the
// global registry (https://github.com/gogo/protobuf/blob/master/proto/properties.go#L545).
type GenericResolver struct {
}

// Resolve wrap and unwrap the value field in Any to and from a byte slice.
func (*GenericResolver) Resolve(_ string) (gogoproto.Message, error) {
	return &bytesMessage{}, nil
}

type bytesMessage struct {
	V []byte
}

func (*bytesMessage) ProtoMessage()             {}
func (*bytesMessage) XXX_WellKnownType() string { return "BytesValue" }
func (m *bytesMessage) Reset()                  { *m = bytesMessage{} }
func (m *bytesMessage) String() string {
	return string(m.V)
}

func (m *bytesMessage) Marshal() ([]byte, error) {
	return m.V, nil
}

func (m *bytesMessage) Unmarshal(b []byte) error {
	m.V = append([]byte{}, b...)
	return nil
}

// getLastPathComponent returns the last component of a package path
// e.g., "k8s.io/apimachinery/pkg/apis/meta/v1" -> "v1"
func getLastPathComponent(path string) string {
	components := strings.Split(path, "/")
	return components[len(components)-1]
}

// SetPackageAlias modifies the alias of a package import and updates all its references
func SetPackageAlias(file *dst.File, pkgPath string, newAlias string) {
	// Iterate through imports
	for _, imp := range file.Imports {
		// Remove quotes from package path for comparison
		quotedPath := imp.Path.Value
		path := quotedPath[1 : len(quotedPath)-1]

		if path == pkgPath {
			var oldAlias string
			// Get the old alias - either explicit or derived from path
			if imp.Name != nil {
				oldAlias = imp.Name.Name
			} else {
				oldAlias = getLastPathComponent(path)
			}

			// Do not update blank imports or if the alias is already the same
			if oldAlias == "_" || oldAlias == newAlias {
				continue
			}

			// Set the new alias
			if imp.Name != nil {
				imp.Name.Name = newAlias
			} else {
				imp.Name = &dst.Ident{Name: newAlias}
			}

			// Update all references
			dst.Inspect(file, func(n dst.Node) bool {
				// Look for selector expressions
				if sel, ok := n.(*dst.SelectorExpr); ok {
					// Check if X is an identifier with our old alias name
					if ident, ok := sel.X.(*dst.Ident); ok {
						if ident.Name == oldAlias {
							ident.Name = newAlias
						}
					}
				}
				return true
			})
			break
		}
	}
}

// InlineFieldMapping holds old and new paths for inline fields.
type InlineFieldMapping struct {
	Path             string
	FieldToBeTrimmed string
}

// RemoveInlineFields identifies old and new JSON paths for inline fields.
//
// Parameters:
// - typ: The reflect.Type of the struct to process.
// - currentPath: The current JSON path being processed.
// - visited: A map to track visited types and avoid infinite recursion.
// - paths: A slice to store the identified InlineFieldMapping structs.
func RemoveInlineFields(typ reflect.Type, currentPath string, visited map[reflect.Type]bool, paths *[]InlineFieldMapping) {
	// Dereference pointer types
	for typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}

	// Handle slices and arrays by processing their element types
	if typ.Kind() == reflect.Slice || typ.Kind() == reflect.Array {
		RemoveInlineFields(typ.Elem(), currentPath+".#", visited, paths)
		return
	}

	// Skip non-struct types
	if typ.Kind() != reflect.Struct {
		return
	}
	// Check if the type has already been visited to avoid infinite recursion
	if visited[typ] {
		return
	}
	visited[typ] = true
	// Iterate over the fields of the struct
	for i := 0; i < typ.NumField(); i++ {
		fieldType := typ.Field(i)
		jsonTag := fieldType.Tag.Get("json")
		protobufTag := fieldType.Tag.Get("protobuf")
		fieldName := extractFieldName(jsonTag, protobufTag, fieldType.Name)

		// Skip unexported fields or explicitly ignored ones
		if fieldName == "-" || fieldType.PkgPath != "" {
			continue
		}

		oldPath := strings.TrimPrefix(currentPath+"."+fieldName, ".")
		if jsonTag == ",inline" {
			*paths = append(*paths, InlineFieldMapping{
				Path:             currentPath,
				FieldToBeTrimmed: fieldName,
			})
		}
		// Recursively process the field type
		RemoveInlineFields(fieldType.Type, oldPath, visited, paths)
	}

	delete(visited, typ)
}

// extractFieldName extracts the field name from the JSON tag, protobuf tag, or the Go field name.
func extractFieldName(jsonTag, protobufTag, defaultName string) string {
	// If the JSON tag explicitly says "-", ignore the field
	if jsonTag == "-" {
		return ""
	}

	// Attempt to extract the name from the JSON tag
	if jsonTag != "" {
		parts := strings.Split(jsonTag, ",")
		name := parts[0]
		if name != "" {
			return name
		}
	}

	// If no name in JSON, try extracting from the protobuf tag
	if protobufTag != "" {
		parts := strings.Split(protobufTag, ",")
		for _, part := range parts {
			if strings.HasPrefix(part, "name=") {
				return strings.TrimPrefix(part, "name=")
			}
		}
	}

	// Fallback to the Go field name
	return defaultName
}

// MatchedResult holds the path, value, and new path for a matched field.
type MatchedResult struct {
	Path    string
	NewPath string
	Value   gjson.Result
}

// ApplyInlineFields processes JSON data to apply inline field mappings.
// It takes a JSON byte slice and a slice of InlineFieldMapping, and returns the modified JSON byte slice.
//
// Parameters:
// - jsonData: A byte slice containing the JSON data to be processed.
// - fields: A slice of InlineFieldMapping structs that define the old and new paths for inline fields.
//
// Returns:
// - A byte slice containing the modified JSON data.
// - An error if any issues occur during the processing.
func ApplyInlineFields(jsonData []byte, fields []InlineFieldMapping) ([]byte, error) {
	sort.Slice(fields, func(i, j int) bool {
		return len(fields[i].Path) > len(fields[j].Path) // longest first
	})

	jsonStr := string(jsonData)
	var resolvePath func(jsonStr, path, resolvedOld string, results *[]MatchedResult)
	resolvePath = func(jsonStr, path, currentResolved string, results *[]MatchedResult) {
		tokens := strings.SplitN(path, ".", 2)
		current := tokens[0]
		rest := ""
		if len(tokens) > 1 {
			rest = tokens[1]
		}

		// If current is a wildcard
		if current == "#" {
			array := gjson.Get(jsonStr, currentResolved)
			if !array.IsArray() {
				return
			}
			array.ForEach(func(index, item gjson.Result) bool {
				nextPath := currentResolved
				if nextPath != "" {
					nextPath += "."
				}
				nextPath += fmt.Sprintf("%d", index.Int())
				resolvePath(jsonStr, rest, nextPath, results)
				return true
			})
		} else {
			nextPath := currentResolved
			if nextPath != "" {
				nextPath += "."
			}
			nextPath += current

			if rest != "" {
				resolvePath(jsonStr, rest, nextPath, results)
			} else {
				val := gjson.Get(jsonStr, nextPath)
				lastDot := strings.LastIndex(nextPath, ".")
				if val.Exists() && lastDot > 0 {
					*results = append(*results, MatchedResult{
						Path:  nextPath[:lastDot],
						Value: val,
					})
				}
			}
		}
	}

	for _, field := range fields {
		var matchedResults []MatchedResult
		resolvePath(jsonStr, fmt.Sprintf("%s.%s", field.Path, field.FieldToBeTrimmed), "", &matchedResults)

		for _, match := range matchedResults {
			var err error
			if strings.HasSuffix(field.Path, ".#") {
				// Get the full object at the matched path
				original := gjson.Get(jsonStr, match.Path)
				var obj map[string]interface{}
				if err = json.Unmarshal([]byte(original.Raw), &obj); err != nil {
					return nil, err
				}

				// Apply match.Value to the existing object
				var patch map[string]interface{}
				if err = json.Unmarshal([]byte(match.Value.Raw), &patch); err != nil {
					return nil, err
				}
				for k, v := range patch {
					obj[k] = v // overwrite or add
				}

				// Marshal the updated object
				updatedBytes, jErr := json.Marshal(obj)
				if jErr != nil {
					return nil, jErr
				}

				// Replace the full element at match.Path
				jsonStr, err = sjson.SetRaw(jsonStr, match.Path, string(updatedBytes))
				if err != nil {
					return nil, err
				}
			} else {
				// Normal non-array update
				jsonStr, err = sjson.SetRaw(jsonStr, match.Path, match.Value.Raw)
				if err != nil {
					return nil, err
				}
			}
		}
	}

	return []byte(jsonStr), nil
}

func joinPath(prefix, token string) string {
	if prefix == "" {
		return token
	}
	return prefix + "." + token
}
