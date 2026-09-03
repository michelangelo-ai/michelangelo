package main

import (
	"bytes"
	"log"
	"os"
	"strings"

	"github.com/michelangelo-ai/michelangelo/go/api/utils"
	"github.com/michelangelo-ai/michelangelo/go/kubeproto/pboptions"
	"github.com/michelangelo-ai/michelangelo/go/kubeproto/templates"
	"github.com/michelangelo-ai/michelangelo/go/kubeproto/util"

	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"
)

var logger = log.New(os.Stderr, "", 0)

func getIndexName(tableName, key string) string {
	return tableName + "_" + key
}

func generateSQLSchema(crdRootMsg *protogen.Message, crdOptions *pboptions.Options) []byte {
	var buf bytes.Buffer
	indexedFields := util.ParseIndexedFields(crdRootMsg, crdOptions)
	crdName := strings.ToUpper(crdRootMsg.GoIdent.GoName[:1]) + crdRootMsg.GoIdent.GoName[1:]
	crdTableName := utils.ToSnakeCase(crdName)

	// Generate main table
	typeInfo := struct {
		TableName string
	}{crdTableName}
	templates.CRDMySQLMainTableColumn.Execute(&buf, typeInfo)

	// Generate CRD specified indexed columns
	for _, field := range indexedFields {
		if field.Flag&util.IndexFlagPrimitive != 0 {
			buf.Write([]byte("    `" + field.Key + "`    " + field.Type + ",\n"))
		} else {
			for _, subField := range field.SubFields {
				buf.Write([]byte("    `" + subField.Key + "`    " + subField.Type + ",\n"))
			}

		}
	}

	templates.CRDMySQLMainTableIndex.Execute(&buf, typeInfo)

	// Generate CRD specified indexes
	for _, field := range indexedFields {
		buf.Write([]byte(",\n"))
		if field.Flag&util.IndexFlagPrimitive != 0 {
			buf.Write([]byte("    KEY    `" + getIndexName(crdTableName, field.Key) + "` (`" + field.Key + "`)"))
		} else {
			if field.Flag&util.IndexFlagCompositeKey != 0 {
				buf.Write([]byte("    KEY    `" + getIndexName(crdTableName, field.Key) + "` ("))
				firstSubfield := true
				for _, subField := range field.SubFields {
					if firstSubfield {
						firstSubfield = false
					} else {
						buf.Write([]byte(", "))
					}
					buf.Write([]byte("`" + subField.Key + "`"))
				}
				buf.Write([]byte(")"))
			} else {
				firstSubField := true
				for _, subField := range field.SubFields {
					if firstSubField {
						firstSubField = false
					} else {
						buf.Write([]byte(",\n"))
					}
					buf.Write([]byte("    KEY    `" + getIndexName(crdTableName, subField.Key) + "` (`" + subField.Key + "`)"))
				}
			}
		}
	}
	buf.Write([]byte("\n);"))

	templates.CRDMySQLLabelAnnotationTable.Execute(&buf, typeInfo)

	// If this CRD is a revisioned base type (resource.revisioned_in is non-empty),
	// emit one sidecar "<base>_<wrapper>_unmarshalled" table per wrapper kind it
	// opts into. The wrapper kind resolves to a wrapper CRD by convention
	// (e.g. "revision" -> keyed on revision_uid; the wrapped resource lives at
	// spec.content).
	if revisioned := util.ParseRevisionedIndex(crdRootMsg, crdOptions); revisioned != nil {
		for _, kind := range revisioned.Kinds {
			emitUnmarshalledTable(&buf, crdTableName, revisioned.Fields, kind)
		}
	}
	return buf.Bytes()
}

// emitUnmarshalledTable writes one revisioned-index sidecar table for a (base, wrapper)
// pair, with a column per mirrored base index field:
//
//	CREATE TABLE `<base>_<wrapper>_unmarshalled` (
//	    `<wrapper>_uid`  VARCHAR(255) NOT NULL,
//	    `<key>`      <type>, ...
//	    PRIMARY KEY (`<wrapper>_uid`),
//	    KEY `..._<key>` (`<key>`), ...
//	);
func emitUnmarshalledTable(buf *bytes.Buffer, baseTableName string, fields []util.IndexedField, wrapperKind string) {
	sidecar := util.SidecarFor(baseTableName, wrapperKind)

	templates.CRDMySQLUnmarshalledTable.Execute(buf, struct {
		TableName string
		UIDColumn string
	}{sidecar.Table, sidecar.UIDColumn})

	for _, field := range fields {
		if field.Flag&util.IndexFlagPrimitive != 0 {
			buf.Write([]byte("    `" + field.Key + "`    " + field.Type + ",\n"))
		} else {
			for _, subField := range field.SubFields {
				buf.Write([]byte("    `" + subField.Key + "`    " + subField.Type + ",\n"))
			}
		}
	}

	buf.Write([]byte("    PRIMARY KEY (`" + sidecar.UIDColumn + "`)"))
	for _, field := range fields {
		if field.Flag&util.IndexFlagPrimitive != 0 {
			buf.Write([]byte(",\n    KEY    `" + getIndexName(sidecar.Table, field.Key) + "` (`" + field.Key + "`)"))
		} else if field.Flag&util.IndexFlagCompositeKey != 0 {
			// Composite message field (e.g. ResourceIdentifier): emit one
			// composite KEY over all subfields, matching the base table so a
			// mirrored composite index has the same semantics in the sidecar.
			buf.Write([]byte(",\n    KEY    `" + getIndexName(sidecar.Table, field.Key) + "` ("))
			firstSubField := true
			for _, subField := range field.SubFields {
				if firstSubField {
					firstSubField = false
				} else {
					buf.Write([]byte(", "))
				}
				buf.Write([]byte("`" + subField.Key + "`"))
			}
			buf.Write([]byte(")"))
		} else {
			for _, subField := range field.SubFields {
				buf.Write([]byte(",\n    KEY    `" + getIndexName(sidecar.Table, subField.Key) + "` (`" + subField.Key + "`)"))
			}
		}
	}
	buf.Write([]byte("\n);\n"))
}

func generateSQL(reqData []byte) *pluginpb.CodeGeneratorResponse {
	gen, extTypes, err := util.GetPluginAndExtensions(reqData, true)
	if err != nil {
		logger.Panic(err)
	}

	for _, f := range gen.Files {
		// Skip the proto file that don't need to generate go code,
		// such as imported proto files.
		if !f.Generate {
			continue
		}

		filename := f.GeneratedFilenamePrefix + ".pb.sql"
		g := gen.NewGeneratedFile(filename, f.GoImportPath)
		var buf []byte
		// Convention: at most one CRD (resource-annotated message) per proto file.
		// Without this check a second CRD's schema would silently overwrite the
		// first's below.
		crdName := ""
		for _, msg := range f.Messages {
			pbOptions := msg.Desc.Options().(*descriptorpb.MessageOptions)
			options, e := pboptions.ReadOptions(extTypes, pbOptions)
			if e != nil {
				logger.Panicf("Failed to parse the options of message %v: %v", msg.GoIdent.GoName, e)
			}

			if options.Bool("has_resource") {
				if crdName != "" {
					logger.Panicf("Multiple CRDs in %v: %v and %v. Each proto file may declare at most one "+
						"message with the michelangelo.api.resource option; move one to its own file",
						f.Desc.Path(), crdName, msg.GoIdent.GoName)
				}
				crdName = msg.GoIdent.GoName
				buf = generateSQLSchema(msg, options)
			}
		}

		_, err = g.Write(buf)
		if err != nil {
			logger.Panicf("failed to write to generated file: %v", err)
		}
	}

	return gen.Response()
}

func main() {
	reqData := util.ReadRequest()
	resp := generateSQL(reqData)
	util.WriteResponse(resp)
}
