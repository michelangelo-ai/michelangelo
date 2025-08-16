package main

import (
	"encoding/json"
	"strings"
	"testing"

	testpb "github.com/michelangelo-ai/michelangelo/proto/test/kubeproto"
	"github.com/michelangelo-ai/michelangelo/go/logging"

	"github.com/dave/dst"
	"github.com/dave/dst/decorator"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/types/pluginpb"
)

type goFileInfo struct {
	imports []string
	structs map[string]*dst.StructType
}

func getImports(file *dst.File) []string {
	var imports []string
	for _, i := range file.Imports {
		imports = append(imports, i.Path.Value)
	}
	return imports
}

func parse(gofile string) goFileInfo {
	dst, _ := decorator.Parse(gofile)
	file := goFileInfo{}
	file.imports = getImports(dst)
	return file
}

func TestGen(t *testing.T) {
	data := testpb.GetProtocReqData()
	resp := generate(data)

	var projectFile *pluginpb.CodeGeneratorResponse_File
	for _, f := range resp.GetFile() {
		if strings.HasSuffix(*f.Name, "project_ut.pb.kubeyarpc.go") {
			projectFile = f
		}
	}

	assert.True(t, projectFile != nil)
	c := projectFile.GetContent()
	p := parse(c)

	assert.Contains(t, p.imports, `"go.uber.org/fx"`)
	assert.Contains(t, p.imports, `"k8s.io/apimachinery/pkg/apis/meta/v1"`)

	assert.Contains(t, c, `func NewProjectServiceHandler(params FxProjectServiceHandlerParams) ProjectServiceYARPCServer {`)
	assert.Contains(t, c, `type ProjectAPIHook interface`)
	assert.Contains(t, c, `var ProjectSvcModule =`)
}

func TestSensitiveFieldEndToEnd(t *testing.T) {
	// Generate kubeyarpc code including sensitive field auto-registration
	data := testpb.GetProtocReqData()
	resp := generate(data)

	var projectFile *pluginpb.CodeGeneratorResponse_File
	for _, f := range resp.GetFile() {
		if strings.HasSuffix(*f.Name, "project_ut.pb.kubeyarpc.go") {
			projectFile = f
		}
	}

	assert.True(t, projectFile != nil)
	content := projectFile.GetContent()

	// Verify that sensitiveField is auto-registered in the generated service handler
	assert.Contains(t, content, `logging.RegisterSensitiveField("sensitiveField")`)

	// Clear any existing sensitive field registrations for clean test
	logging.ClearSensitiveFields()

	// Manually register the sensitive field (simulating what the generated code would do)
	logging.RegisterSensitiveField("sensitiveField")

	// Test end-to-end logging redaction using actual protobuf struct
	project := &testpb.Project{
		Spec: testpb.ProjectSpec{
			SensitiveField: "secret-data-should-be-redacted",
		},
	}

	// Test MarshalToStringForLogging redacts the sensitive field
	loggedJSON := logging.MarshalToStringForLogging(project)

	// Parse the logged JSON to verify redaction
	var logged map[string]interface{}
	err := json.Unmarshal([]byte(loggedJSON), &logged)
	assert.NoError(t, err)

	// Verify sensitiveField is redacted in the spec
	spec, specExists := logged["spec"].(map[string]interface{})
	assert.True(t, specExists, "spec field should exist in logged JSON")
	assert.Equal(t, "[REDACTED]", spec["sensitiveField"])

	// Test normal JSON marshaling (should not redact)
	normalJSON, err := json.Marshal(project)
	assert.NoError(t, err)
	var normal map[string]interface{}
	err = json.Unmarshal(normalJSON, &normal)
	assert.NoError(t, err)

	// Verify normal marshaling preserves the actual value
	normalSpec := normal["spec"].(map[string]interface{})
	assert.Equal(t, "secret-data-should-be-redacted", normalSpec["sensitiveField"])

	// Clean up
	logging.ClearSensitiveFields()
}
