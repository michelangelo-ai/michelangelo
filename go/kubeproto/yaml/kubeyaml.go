package yaml

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gobuffalo/flect"
	"github.com/michelangelo-ai/michelangelo/go/kubeproto/pboptions"
	"github.com/michelangelo-ai/michelangelo/go/kubeproto/util"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"
	apiext "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

var _reGetComment = regexp.MustCompile("\r?\n|\r")

var logger = log.New(os.Stderr, "", 0)

// GroupInfo contains the information of a CRD group from groupversion_info.proto file.
type GroupInfo struct {
	Name    string
	Version string
}

type crdInfo struct {
	Kind         string // CamelCased singular type
	PluralName   string // Lower case. If it is not specified defaults to proto msg name + 's'
	SingularName string // Lower case. If it is not specified defaults to the singular form of plural name.
	Scope        apiext.ResourceScope
	MainMsg      *protogen.Message
}

func getComment(protoComments *protogen.CommentSet) string {
	var output string

	for _, comment := range protoComments.LeadingDetached {
		output += strings.TrimSpace(string(comment))
		output += " "
	}

	output += strings.TrimSpace(string(protoComments.Leading))

	output = _reGetComment.ReplaceAllString(output, "")
	return output
}

func setTypeFormat(field *protogen.Field, props *apiext.JSONSchemaProps) {
	switch field.Desc.Kind() {
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		props.Type = "integer"
		props.Format = "int32"
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		logger.Panicf("Failed to generate field %v: unsigned type is not supported. Please use int32 instead.",
			field.GoIdent.GoName)
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		props.Type = "string"
		props.Pattern = "^[-]?\\d{1,19}$"
		props.Format = "int64"
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		logger.Panicf("Failed to generate field %v: unsigned type is not supported. Please use int64 instead.",
			field.GoIdent.GoName)
	case protoreflect.StringKind:
		props.Type = "string"
	case protoreflect.BoolKind:
		props.Type = "boolean"
	case protoreflect.FloatKind:
		props.Type = "number"
		props.Format = "float"
	case protoreflect.DoubleKind:
		props.Type = "number"
		props.Format = "double"
	case protoreflect.BytesKind:
		props.Type = "string"
		props.Format = "byte"
	case protoreflect.EnumKind:
		props.Type = "string"

		var prefixLen int
		if field.Enum.Desc.Parent().FullName() == field.Parent.Desc.FullName() {
			// Nested enum
			prefixLen = len(field.Parent.GoIdent.GoName) + 1
		} else {
			prefixLen = len(field.Enum.GoIdent.GoName) + 1
		}

		for _, enumVal := range field.Enum.Values {
			bytes, err := json.Marshal(enumVal.GoIdent.GoName[prefixLen:])
			if err != nil {
				logger.Panicf("Failed to marshal enum field %v", enumVal.GoIdent.GoName)
			}
			props.Enum = append(props.Enum, apiext.JSON{Raw: bytes})
		}
	default:
		logger.Panicf("Fained to generate field %v, unsupported kind %v",
			field.GoIdent.GoName, field.Desc.Kind())
	}
}

func getAnyFieldProperties(comment string) *apiext.JSONSchemaProps {
	schema := &apiext.JSONSchemaProps{
		Type:        "object",
		Description: comment,
	}

	schema.Properties = make(map[string]apiext.JSONSchemaProps)
	schema.Properties["@type"] = apiext.JSONSchemaProps{
		Description: "A URL/resource name that uniquely identifies the type of " +
			"the serialized protocol buffer message.",
		Type: "string",
	}
	schema.Properties["value"] = apiext.JSONSchemaProps{
		Description: "Serialized data of the above type.",
		Format:      "byte",
		Type:        "string",
	}
	return schema
}

func getPreserveUnknownFieldProperties(comment string) *apiext.JSONSchemaProps {
	isTrue := true
	schema := &apiext.JSONSchemaProps{
		Type:                   "object",
		Description:            comment,
		XPreserveUnknownFields: &isTrue,
	}
	return schema
}

func getDurationFieldProperties(comment string) *apiext.JSONSchemaProps {
	schema := &apiext.JSONSchemaProps{
		Type:        "string",
		Description: comment,
	}
	return schema
}

// We should stop recursing if the field's message type is parsed within this recursion path, to prevent from going into
// infinite recursion.
// For a simple example:
//
//	message A {
//	  int id = 1;
//	  string name = 2;
//	  A nested = 3;
//	  repeated A nested_arrays = 4;
//	}
//
// cycle: A -> A
// We should not recurse when seeing field `nested`, otherwise we will fall into infinite recursion. Same logic applies
// to `nested_arrays` field.
//
// When involving multiple messages, things can be more complicated and less obvious:
//
//	message A {
//	  B b = 1;
//	}
//
//	message B {
//	  A a = 1;
//	}
//
// cycle: A -> B -> A
//
// or
//
//	message A {
//	  B b = 1;
//	}
//
//	message B {
//	  C c = 1;
//	}
//
//	message C {
//	  A a = 1;
//	}
//
// cycle: A -> B -> C -> A
//
// In short, we should stop the recursion when we encounter an already processed message type, i.e.,
// a cycle is detected along the recursion.
// We need to properly maintain (put and remove) the processed message types along with each recursion path, instead of
// simply having a global map.
// For example:
//
//	message A {
//		 B b1 = 1;
//		 B b2 = 2;
//	}
//
//	message B {
//		 int id = 1;
//		 string name = 2;
//	}
//
// Above messages form no cycle so we should recurse all paths. If we just use a global map, we won't recurse on b2
// field.
func parseMessageField(field *protogen.Field, fieldComment string,
	parsedMessageTypes *map[protogen.GoIdent]bool) *apiext.JSONSchemaProps {
	// Set schema to be XPreserveUnknownFields if the field's message type has been parsed within the current recursion
	if _, found := (*parsedMessageTypes)[field.Message.GoIdent]; found {
		return getPreserveUnknownFieldProperties(getComment(&field.Comments))
	}

	// mark this message type as parsed
	(*parsedMessageTypes)[field.Message.GoIdent] = true
	// after the return, remove this message type as parsed
	defer delete(*parsedMessageTypes, field.Message.GoIdent)
	return parseMessageFields(field.Message, fieldComment, false, parsedMessageTypes)
}

func parseMessageFields(msg *protogen.Message, comment string, isRootSchema bool,
	parsedMessageTypes *map[protogen.GoIdent]bool) *apiext.JSONSchemaProps {
	schema := new(apiext.JSONSchemaProps)
	schema.Type = "object"
	schema.Description = comment
	schema.Properties = make(map[string]apiext.JSONSchemaProps)

	for _, field := range msg.Fields {
		var props *apiext.JSONSchemaProps
		fieldName := strings.ToLower(field.GoName[0:1]) + field.GoName[1:]
		fieldComment := getComment(&field.Comments)
		if field.Message == nil {
			// primitive type
			props = &apiext.JSONSchemaProps{
				Description: fieldComment,
			}
			setTypeFormat(field, props)
		} else {
			// message type
			if field.Message.Desc.FullName() == "google.protobuf.Any" {
				props = getAnyFieldProperties(fieldComment)
			} else if field.Message.Desc.FullName() == "google.protobuf.Timestamp" {
				props = &apiext.JSONSchemaProps{
					Type:        "string",
					Format:      "date-time",
					Description: fieldComment,
				}
			} else if field.Message.Desc.FullName() == "google.protobuf.Struct" {
				props = getPreserveUnknownFieldProperties(getComment(&field.Comments))
			} else if field.Message.Desc.FullName() == "google.protobuf.Duration" {
				props = getDurationFieldProperties(getComment(&field.Comments))
			} else {
				// Many k8s types have incompatible protobuf and yaml schemas. We have to hard code the yaml schemas
				// for these types, rather than directly translate from protobuf schemas.
				jsonSchema, ok := jsonSchemas[string(field.Message.Desc.FullName())]
				if ok {
					props = jsonSchema
					if fieldComment != "" {
						props.Description = comment
					}
				} else {
					if isRootSchema && (field.Message.Desc.FullName() == "k8s.io.apimachinery.pkg.apis.meta.v1.TypeMeta" ||
						field.Message.Desc.FullName() == "k8s.io.apimachinery.pkg.apis.meta.v1.ObjectMeta" ||
						field.Message.Desc.FullName() == "k8s.io.apimachinery.pkg.apis.meta.v1.ListMeta") {
						continue // skip special fields in CRD and CRDList messages
					}
					props = parseMessageField(field, fieldComment, parsedMessageTypes)
				}
			}
		}

		if field.Desc.IsList() {
			// array type
			schema.Properties[fieldName] = apiext.JSONSchemaProps{
				Description: props.Description,
				Type:        "array",
				Items: &apiext.JSONSchemaPropsOrArray{
					Schema: props,
				},
			}
			props.Description = ""
		} else if field.Desc.IsMap() {
			// map type
			if props.Properties["key"].Type != "string" {
				logger.Panicf("Failed to generate CRD for field %s. Map type only supports string key",
					fieldName)
			}

			mapProps := apiext.JSONSchemaProps{
				Description: getComment(&field.Comments),
				Type:        "object",
			}
			if props.Properties["value"].Type == "object" {
				mapValueProps := props.Properties["value"]
				mapProps.AdditionalProperties = &apiext.JSONSchemaPropsOrBool{
					Schema: &mapValueProps,
				}
			} else {
				mapProps.AdditionalProperties = &apiext.JSONSchemaPropsOrBool{
					Schema: &apiext.JSONSchemaProps{
						Type: props.Properties["value"].Type,
					},
				}
			}
			schema.Properties[fieldName] = mapProps
		} else {
			schema.Properties[fieldName] = *props
		}
	}

	return schema
}

func parseRootSchema(msg *protogen.Message) *apiext.JSONSchemaProps {
	parsedMessageTypes := make(map[protogen.GoIdent]bool)
	schema := parseMessageFields(msg, getComment(&msg.Comments), true, &parsedMessageTypes)

	// Removes k8s TypeMeta and ObjectMeta that is already included in apiext.CustomResourceDefinition
	if _, found := schema.Properties["typeMeta"]; found {
		delete(schema.Properties, "typeMeta")
	}
	if _, found := schema.Properties["metadata"]; found {
		delete(schema.Properties, "metadata")
	}

	_, foundSpec := schema.Properties["spec"]
	_, foundStatus := schema.Properties["status"]

	if foundSpec == false || foundStatus == false {
		logger.Panicf("Failed to parse %v.proto. Make sure both %vSpec and %vStatus are defined.",
			msg.GoIdent.GoName, msg.GoIdent.GoName, msg.GoIdent.GoName)
	}

	return schema
}

func getCrdInfo(file *protogen.File, extTypes *protoregistry.Types) *crdInfo {
	cInfo := crdInfo{
		Scope: apiext.NamespaceScoped,
	}

	// Get the top level message
	for _, msg := range file.Messages {
		pbOptions := msg.Desc.Options().(*descriptorpb.MessageOptions)
		options, err := pboptions.ReadOptions(extTypes, pbOptions)
		if err != nil {
			logger.Panicf("Failed to parse the options of message %v: %v", msg.GoIdent.GoName, err)
		}

		if options.Bool("has_resource") {
			cInfo.MainMsg = msg
			if options.String("resource.name") != "" {
				cInfo.PluralName = options.String("resource.name")
				if options.String("resource.singular") != "" {
					cInfo.SingularName = options.String("resource.singular")
				} else {
					cInfo.SingularName = flect.Singularize(options.String("resource.name"))
				}
			} else if options.String("resource.singular") != "" {
				cInfo.SingularName = options.String("resource.singular")
				cInfo.PluralName = cInfo.SingularName + "s"
			} else {
				cInfo.SingularName = msg.GoIdent.GoName
				cInfo.PluralName = cInfo.SingularName + "s"
			}
			cInfo.Kind = strings.ToUpper(msg.GoIdent.GoName[:1]) + msg.GoIdent.GoName[1:]
			cInfo.SingularName = strings.ToLower(cInfo.SingularName)
			cInfo.PluralName = strings.ToLower(cInfo.PluralName)

			if options.String("resource.scope") != "" {
				if strings.ToLower(options.String("resource.scope")) == "cluster" {
					cInfo.Scope = apiext.ClusterScoped
				} else if strings.ToLower(options.String("resource.scope")) != "namespaced" {
					logger.Panicln("Invalid CRD scope: " + options.String("resource.scope") +
						". It should be either Namespaced or Cluster")
				}
			}
		}
	}

	return &cInfo
}

func generateFileHeader(gen *protogen.Plugin, file *protogen.File) *protogen.GeneratedFile {
	filename := file.GeneratedFilenamePrefix + ".pb.yaml"
	g := gen.NewGeneratedFile(filename, file.GoImportPath)
	g.P("# Generated by protoc-gen-kubeyaml. DO NOT EDIT.")
	g.P("---")
	return g
}

// GenerateYamlFile generates CRD Yaml file
func GenerateYamlFile(gen *protogen.Plugin, file *protogen.File,
	extTypes *protoregistry.Types, gInfo GroupInfo) *protogen.GeneratedFile {
	// Output to CRD yaml file.
	g := generateFileHeader(gen, file)

	buf := GenerateCRDYaml(file, extTypes, gInfo)
	_, err := g.Write(buf)
	if err != nil {
		logger.Panicf("Failed to write to generated file: %v", err)
	}

	return g
}

// GenerateCRDYaml generates CRD Yaml schema
func GenerateCRDYaml(file *protogen.File, extTypes *protoregistry.Types, gInfo GroupInfo) []byte {
	cInfo := getCrdInfo(file, extTypes)
	if cInfo.MainMsg == nil {
		// No michelangelo api resource is found.
		return []byte{}
	}
	schema := parseRootSchema(cInfo.MainMsg)

	// Define CRD metadata.
	myCrd := apiext.CustomResourceDefinition{
		TypeMeta: metav1.TypeMeta{
			APIVersion: apiext.SchemeGroupVersion.String(),
			Kind:       "CustomResourceDefinition",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: cInfo.PluralName + "." + gInfo.Name,
		},
		Spec: apiext.CustomResourceDefinitionSpec{
			Group: gInfo.Name,
			Names: apiext.CustomResourceDefinitionNames{
				Kind:     cInfo.Kind,
				ListKind: cInfo.Kind + "List",
				Plural:   cInfo.PluralName,
				Singular: cInfo.SingularName,
			},
			Scope: cInfo.Scope,
		},
		Status: apiext.CustomResourceDefinitionStatus{
			Conditions:     []apiext.CustomResourceDefinitionCondition{},
			StoredVersions: []string{},
		},
	}

	// Define CRD schema.
	ver := apiext.CustomResourceDefinitionVersion{
		Name:    gInfo.Version,
		Served:  true, // Being served via REST APIs
		Storage: true, // Persisting custom resources to storage. One and only one version must be marked as true.
		Schema: &apiext.CustomResourceValidation{
			OpenAPIV3Schema: schema,
		},
		Subresources: &apiext.CustomResourceSubresources{
			Status: &apiext.CustomResourceSubresourceStatus{},
		},
	}
	myCrd.Spec.Versions = append(myCrd.Spec.Versions, ver)

	buf, err := yaml.Marshal(myCrd)

	if err != nil {
		logger.Panicf("Failed to marshal yaml: %v", err)
	}
	return buf
}

func isGroupFile(filename string) bool {
	groupFileID := "groupversion_info"
	if len(filename) < len(groupFileID) {
		return false
	}
	return filename[:len(groupFileID)] == groupFileID
}

// LoadGroupInfo loads group version info from groupversion_info.proto file
func LoadGroupInfo(gen *protogen.Plugin, extTypes *protoregistry.Types, mustHave bool) *GroupInfo {
	var groupInfoFile *protogen.File
	var gInfo GroupInfo

	for _, f := range gen.Files {
		if isGroupFile(filepath.Base(f.GeneratedFilenamePrefix)) {
			groupInfoFile = f
		}
	}

	if groupInfoFile == nil {
		if mustHave {
			logger.Panicln("Failed to derive API group version info. " +
				"Make sure to define groupversion_info.proto for the API group.")
		} else {
			return nil
		}
	}

	options, err := pboptions.ReadOptions(extTypes, groupInfoFile.Proto.Options)
	if err != nil {
		logger.Panicf("Failed to read protobuf options: %v", err)
	}

	if options.Bool("has_group_info") {
		gInfo.Name = options.String("group_info.name")
		gInfo.Version = options.String("group_info.version")
	}

	if gInfo.Name == "" || gInfo.Version == "" {
		logger.Panicln("Failed to derive API group version info. " +
			"Make sure both name and version are defined in groupversion_info.proto")
	}

	return &gInfo
}

// GenerateYaml generates CRD yaml files for the protoc request.
func GenerateYaml(reqData []byte) *pluginpb.CodeGeneratorResponse {
	gen, extTypes, err := util.GetPluginAndExtensions(reqData, true)
	if err != nil {
		logger.Panic(err)
	}
	gInfo := LoadGroupInfo(gen, extTypes, true)

	for _, f := range gen.Files {
		// Skip the proto file that don't need to generate yaml files,
		// such as imported proto files.
		if !f.Generate {
			continue
		}

		GenerateYamlFile(gen, f, extTypes, *gInfo)
	}

	return gen.Response()
}
