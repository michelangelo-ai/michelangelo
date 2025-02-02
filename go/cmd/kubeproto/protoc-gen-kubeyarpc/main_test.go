package main

import (
	"strings"
	"testing"

	testpb "github.com/michelangelo-ai/michelangelo/proto/test/kubeproto"

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

	assert.Contains(t, c, `func NewProjectServiceHandler(handler api.Handler, metricsScope tally.Scope, auth authapi.Auth, auditLog logging.AuditLog) ProjectServiceYARPCServer`)
	assert.Contains(t, c, `var ProjectSvcModule =`)
}
