package main

import (
	"bytes"
	"io"
	"log"
	"os"
	"strings"

	"github.com/michelangelo-ai/michelangelo/go/api/utils"
	"github.com/michelangelo-ai/michelangelo/go/kubeproto/pboptions"
	"github.com/michelangelo-ai/michelangelo/go/kubeproto/templates"
	"github.com/michelangelo-ai/michelangelo/go/kubeproto/util"

	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
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
					if firstSubfield == true {
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
	return buf.Bytes()
}

func generateSQL(reqData []byte) *pluginpb.CodeGeneratorResponse {
	req := &pluginpb.CodeGeneratorRequest{}
	err := proto.Unmarshal(reqData, req)
	if err != nil {
		logger.Panicf("Failed to unmarshal input from protoc %v.", err)
	}
	util.ReplaceImportPath(req)

	// Initialize protobuf generator
	gen, err := protogen.Options{}.New(req)
	if err != nil {
		logger.Panicf("Failed to initialize golang proto generator %v.", err)
	}

	// Load protobuf extensions from all the imported protobuf files
	extTypes := pboptions.LoadPBExtensions(gen.Files)

	for _, f := range gen.Files {
		// Skip the proto file that don't need to generate go code,
		// such as imported proto files.
		if !f.Generate {
			continue
		}

		filename := f.GeneratedFilenamePrefix + ".pb.sql"
		g := gen.NewGeneratedFile(filename, f.GoImportPath)
		var buf []byte
		for _, msg := range f.Messages {
			pbOptions := msg.Desc.Options().(*descriptorpb.MessageOptions)
			options, e := pboptions.ReadOptions(extTypes, pbOptions)
			if e != nil {
				logger.Panicf("Failed to parse the options of message %v: %v", msg.GoIdent.GoName, e)
			}

			if options.Bool("has_resource") {
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
	reqData, _ := io.ReadAll(os.Stdin)
	resp := generateSQL(reqData)
	out, _ := proto.Marshal(resp)
	os.Stdout.Write(out)
}
