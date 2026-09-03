package util

import (
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/michelangelo-ai/michelangelo/go/kubeproto/pboptions"

	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/reflect/protoreflect"
)

var logger = log.New(os.Stderr, "", 0)

// IndexFlag is used to describe properties of an IndexedField. Check the available flags below.
type IndexFlag int

var pathPattern = regexp.MustCompile("^\\w+(\\.\\w+)*$")

const (
	// IndexFlagPrimitive is set if the Indexed field is a primitive type field, i.e. (string, int32, etc.).
	IndexFlagPrimitive IndexFlag = 1 << iota
	// IndexFlagCompositeKey is used to determine whether to generate composite or non-composite
	// index for a message field.
	IndexFlagCompositeKey
	// IndexFlagEnum is set if the indexed field is an enum field.
	IndexFlagEnum
)

// IndexedField represents an indexed field in CRD that is annotated by the michelangelo.api.resource option.
// An indexed field can be either a primitive type field (IndexFlagPrimitive is set) or a message type field.
// For a message type field, SubFields represents the indexed subfields in the message type field.
// If the IndexFlagCompositeKey is set, the compiler will build composite key on all the subfields.
type IndexedField struct {
	Key       string
	Flag      IndexFlag
	GoPaths   []string
	ProtoPath string
	Type      string
	SubFields []IndexedSubField
}

// IndexedSubField represents an indexed subfield in the indexed message field.
type IndexedSubField struct {
	Key       string
	GoPath    string
	ProtoPath string
	Type      string
}

// HasFlag checks if the field has the given flag.
func (f *IndexedField) HasFlag(flag IndexFlag) bool {
	return f.Flag&flag != 0
}

func buildSubKey(key, subkey string) string {
	return key + "_" + subkey
}

func buildSubKeyProtoPath(path, subkey string) string {
	return path + "." + subkey
}

// ParseIndexedFields parses and validates the indexing fields in a CRD and returns a list of indexed fields
// for further code generation.
func ParseIndexedFields(crdRootMsg *protogen.Message, crdOptions *pboptions.Options) []IndexedField {
	// This map is used to log all the keys that presented in the CRD
	indexedKeys := make(map[string]bool)

	if !crdOptions.Bool("has_index[0]") {
		return nil
	}

	indexedFields := make([]IndexedField, int(crdOptions.Int64("len(index)")))
	for i := 0; i < len(indexedFields); i++ {
		key := crdOptions.String("index[" + strconv.Itoa(i) + "].key")
		path := crdOptions.String("index[" + strconv.Itoa(i) + "].path")
		typeOverride := crdOptions.String("index[" + strconv.Itoa(i) + "].type_override")

		if key == "" || path == "" {
			logger.Panicf("Invalid index annotation. Either key or path is not specified. key: %v, path :%v",
				key, path)
		}
		if _, found := indexedKeys[key]; found {
			logger.Panicf("Invalid index annotation. Duplicated key. key: %v, path: %v", key, path)
		}
		indexedKeys[key] = true

		newIndexedField := buildIndexedField(key, path, typeOverride, crdRootMsg)

		for _, subField := range newIndexedField.SubFields {
			if _, found := indexedKeys[subField.Key]; found {
				logger.Panicf("Invalid index annotation. Duplicated key. key: %v, path: %v, subKey: %v",
					key, path, subField.Key)
			}
			indexedKeys[subField.Key] = true
		}
		indexedFields[i] = newIndexedField
	}

	return indexedFields
}

// RevisionedIndex describes a revisioned base CRD's index set and the wrapper
// kinds it is mirrored into. A base CRD that lists resource.revisioned_in has
// its full michelangelo.api.index set materialized into a
// "<base>_<kind>_unmarshalled" sidecar table for each listed kind, so
// List<Wrapper> can filter on the wrapped base resource's fields server-side.
// Every listed kind mirrors the same base index set, so Fields is shared across
// all Kinds rather than duplicated per kind.
type RevisionedIndex struct {
	// Kinds are the wrapper CRD kinds that mirror this index set, e.g.
	// "revision", "draft". Each resolves to a wrapper CRD by convention
	// (e.g. "revision" -> Revision, keyed on revision_uid).
	Kinds []string
	// Fields is the base CRD's full index set, resolved against the base
	// message — which is exactly what each wrapper stores at spec.content.
	Fields []IndexedField
}

// ParseRevisionedIndex returns the base CRD's michelangelo.api.index set,
// with a synthesized "name" field (metadata.name) always prepended, together
// with the wrapper kinds listed in resource.revisioned_in that mirror it into
// "<base>_<kind>_unmarshalled" sidecar tables. Each index path resolves against
// the base CRD message itself, which is what each wrapper stores at
// spec.content, so every kind mirrors the same base index set. The base
// object's own SQL table always carries a "name" column regardless of
// michelangelo.api.index (see protoc-gen-sql's CRDMySQLMainTableColumn), so
// sidecar tables mirror that same guarantee rather than requiring every
// revisioned CRD to redeclare "metadata.name" explicitly.
//
// Returns nil when the CRD is not revisioned (resource.revisioned_in is empty).
// Panics if revisioned_in is set but names an empty kind, is set on a CRD that
// declares no michelangelo.api.index fields to mirror, or if the CRD declares
// an index field with the reserved key "name" (which would collide with the
// synthesized metadata.name column).
func ParseRevisionedIndex(crdRootMsg *protogen.Message, crdOptions *pboptions.Options) *RevisionedIndex {
	kindCount := int(crdOptions.Int64("resource.len(revisioned_in)"))
	if kindCount == 0 {
		return nil
	}

	kinds := make([]string, 0, kindCount)
	for i := 0; i < kindCount; i++ {
		kind := crdOptions.String("resource.revisioned_in[" + strconv.Itoa(i) + "]")
		if kind == "" {
			logger.Panicf("Invalid revisioned_in annotation on %v. kind is not specified at index %d",
				crdRootMsg.GoIdent.GoName, i)
		}
		kinds = append(kinds, kind)
	}

	fields := ParseIndexedFields(crdRootMsg, crdOptions)
	if len(fields) == 0 {
		logger.Panicf("Invalid revisioned_in annotation on %v. CRD declares revisioned_in %v "+
			"but has no michelangelo.api.index fields to mirror", crdRootMsg.GoIdent.GoName, kinds)
	}
	for _, field := range fields {
		if field.Key == "name" {
			logger.Panicf("Invalid index annotation on %v. key \"name\" is reserved for the "+
				"synthesized metadata.name column mirrored onto revisioned sidecar tables",
				crdRootMsg.GoIdent.GoName)
		}
	}

	nameField := buildIndexedField("name", "metadata.name", "", crdRootMsg)
	fields = append([]IndexedField{nameField}, fields...)

	return &RevisionedIndex{Kinds: kinds, Fields: fields}
}

// Sidecar names the storage artifacts of one (base table, wrapper kind) pair:
// the "<base>_<kind>_unmarshalled" table and its "<kind>_uid" primary-key column.
// protoc-gen-sql emits the CREATE TABLE under these names and
// protoc-gen-kubeproto bakes them into the generated RevisionedIndexSpecs, so
// both generators MUST derive them from SidecarFor — the runtime queries
// whatever the spec says, and a divergence between the two would only surface
// as a missing table in production.
type Sidecar struct {
	Table     string
	UIDColumn string
}

// SidecarFor derives the sidecar identity for a (base table, wrapper kind) pair,
// e.g. ("pipeline", "revision") -> {pipeline_revision_unmarshalled, revision_uid}.
func SidecarFor(baseTableName, kind string) Sidecar {
	return Sidecar{
		Table:     baseTableName + "_" + kind + "_unmarshalled",
		UIDColumn: kind + "_uid",
	}
}

// buildIndexedField resolves a single (key, path) against rootMsg and returns the
// fully-typed IndexedField (primitive type, enum flag, or composite message
// subfields).
func buildIndexedField(key, path, typeOverride string, rootMsg *protogen.Message) IndexedField {
	parsedGoPaths, field, leafMsg := validateIndex(key, path, rootMsg)

	newIndexedField := IndexedField{Key: key, GoPaths: parsedGoPaths, ProtoPath: path}

	if leafMsg == nil {
		// primitive type field
		// If type_override is specified, use the specified type.
		if typeOverride != "" {
			newIndexedField.Type = typeOverride
		} else {
			switch field.Desc.Kind() {
			case protoreflect.StringKind:
				newIndexedField.Type = "VARCHAR(255)"
			case protoreflect.Int32Kind, protoreflect.Fixed32Kind, protoreflect.Sint32Kind:
				newIndexedField.Type = "INT"
			case protoreflect.Int64Kind, protoreflect.Fixed64Kind, protoreflect.Sint64Kind:
				newIndexedField.Type = "BIGINT"
			case protoreflect.EnumKind:
				newIndexedField.Type = "VARCHAR(255)"
				newIndexedField.Flag |= IndexFlagEnum
			case protoreflect.BoolKind:
				newIndexedField.Type = "BOOLEAN"
			default:
				logger.Panicf("Invalid index annotation. Unsupported primitive type: %v, key: %v, path: %v",
					field.Desc.Kind(), key, path)
			}
		}

		newIndexedField.Flag |= IndexFlagPrimitive
	} else {
		switch leafMsg.Desc.FullName() {
		case "michelangelo.api.ResourceIdentifier":
			newIndexedField.Flag |= IndexFlagCompositeKey
			newIndexedField.SubFields = append(newIndexedField.SubFields,
				IndexedSubField{Key: buildSubKey(key, "namespace"), GoPath: "Namespace",
					ProtoPath: buildSubKeyProtoPath(path, "namespace"), Type: "VARCHAR(255)"})
			newIndexedField.SubFields = append(newIndexedField.SubFields,
				IndexedSubField{Key: buildSubKey(key, "name"), GoPath: "Name",
					ProtoPath: buildSubKeyProtoPath(path, "name"), Type: "VARCHAR(255)"})
		case "michelangelo.api.v2beta1.UserInfo", "michelangelo.api.v2.UserInfo":
			newIndexedField.SubFields = append(newIndexedField.SubFields,
				IndexedSubField{Key: buildSubKey(key, "name"), GoPath: "Name",
					ProtoPath: buildSubKeyProtoPath(path, "name"), Type: "VARCHAR(255)"})
			newIndexedField.SubFields = append(newIndexedField.SubFields,
				IndexedSubField{Key: buildSubKey(key, "proxy_user"), GoPath: "ProxyUser",
					ProtoPath: buildSubKeyProtoPath(path, "proxy_user"), Type: "VARCHAR(255)"})
		case "google.protobuf.Timestamp":
			newIndexedField.Type = "DATETIME"
			newIndexedField.Flag |= IndexFlagPrimitive
		case "k8s.io.apimachinery.pkg.apis.meta.v1.Time":
			newIndexedField.Type = "DATETIME"
			newIndexedField.Flag |= IndexFlagPrimitive
		default:
			logger.Panicf("Invalid index annotation. Unsupported message type: %v. key: %v, path: %v",
				leafMsg.Desc.FullName(), key, path)
		}
	}

	return newIndexedField
}

func validateIndex(key, fullPath string, curMsg *protogen.Message) ([]string, *protogen.Field, *protogen.Message) {
	var parsedGoPaths []string
	var lastField *protogen.Field

	if !pathPattern.MatchString(fullPath) {
		logger.Panicf("Invalid path in index annotation. key: %v, path: %v", key, fullPath)
	}

	path := fullPath
	for {
		index := strings.Index(path, ".")
		var fieldName string
		if index == -1 {
			fieldName = path
		} else {
			fieldName = path[:index]
			path = path[index+1:]
		}

		// check if it is a valid field name in the message
		field := validateField(curMsg, fieldName)
		if field == nil {
			logger.Panicf("Invalid index annotation. Specified field does not exist. path: %v", fullPath)
		}

		parsedGoPaths = append(parsedGoPaths, getGoPath(curMsg, field))

		curMsg = field.Message

		if index == -1 {
			lastField = field
			break
		}
	}

	return parsedGoPaths, lastField, lastField.Message
}

// validateField validates that a field with the given name exists on the given message
// Returns the field if such field exists, otherwise returns nil
func validateField(curMsg *protogen.Message, fieldName string) *protogen.Field {
	if curMsg == nil {
		return nil
	}
	for _, field := range curMsg.Fields {
		if fieldName == string(field.Desc.Name()) {
			return field
		}
	}

	return nil
}

// getGoPath gets the path in order to access the field on the given message.
// For example,
//
//	message foo {
//	  string str_field = 1;
//	  int64 int_field = 2;
//	  oneof {
//	    string one_of_str = 3;
//	    int64 one_of_int = 4;
//	  }
//	}
//
// For fields not inside oneof, we can directly access the field through:
// - str_field -> StrField
// - int_field -> IntField
// For fields inside oneof, we need to access them through GetXXX() method:
// - one_of_str -> GetOneOfStr()
// - one_of_int -> GetOneOfInt()
func getGoPath(message *protogen.Message, field *protogen.Field) string {
	goPath := field.GoName
	if isPartOfOneOf(message, field) {
		goPath = fmt.Sprintf("Get%s()", goPath)
	}

	return goPath
}

func isPartOfOneOf(message *protogen.Message, field *protogen.Field) bool {
	fieldName := field.Desc.Name()
	for _, oneof := range message.Oneofs {
		for _, f := range oneof.Fields {
			if fieldName == f.Desc.Name() {
				return true
			}
		}
	}

	return false
}
