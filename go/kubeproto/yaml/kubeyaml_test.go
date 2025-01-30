package yaml

import (
	_ "embed"
	"path/filepath"
	"testing"

	"github.com/michelangelo-ai/michelangelo/go/kubeproto/pboptions"
	"github.com/michelangelo-ai/michelangelo/go/kubeproto/util"
	testpb "github.com/michelangelo-ai/michelangelo/proto/test/kubeproto"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/pluginpb"
	apiext "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

func readInput(t *testing.T) (*protogen.Plugin, *protoregistry.Types) {
	reqData := testpb.GetProtocReqData()

	req := &pluginpb.CodeGeneratorRequest{}
	err := proto.Unmarshal(reqData, req)
	assert.True(t, err == nil)
	util.ReplaceImportPath(req)

	gen, err := protogen.Options{}.New(req)
	assert.True(t, err == nil)

	extTypes := pboptions.LoadPBExtensions(gen.Files)

	return gen, extTypes
}

func TestGroupVersionInfo(t *testing.T) {
	gen, extTypes := readInput(t)
	gInfo := LoadGroupInfo(gen, extTypes, true)
	assert.Equal(t, "v2", gInfo.Version)
	assert.Equal(t, "michelangelo.api", gInfo.Name)
}

func TestCrdInfo(t *testing.T) {
	tests := map[string]crdInfo{
		"project_ut": {
			// Set both the singular and plural name.
			SingularName: "project-singular",
			PluralName:   "projects-plural",
			Kind:         "Project",
			// Default to namespace scope if scope is unset.
			Scope: apiext.NamespaceScoped,
		},
		"testobject": {
			// Both singular and plural name are not set.  Default to message name.
			SingularName: "testobject",
			PluralName:   "testobjects",
			Kind:         "TestObject",
			// Set the namespace scope.
			Scope: apiext.NamespaceScoped,
		},
		"crd_info_ut": {
			// Only set the plural name.
			SingularName: "testcrd",
			PluralName:   "testcrds",
			Kind:         "TestCRD",
			// Set the cluster scope.
			Scope: apiext.ClusterScoped,
		},
	}

	tested := 0
	gen, extTypes := readInput(t)
	for _, f := range gen.Files {
		if !f.Generate {
			continue
		}

		crdInfo := getCrdInfo(f, extTypes)
		filename := filepath.Base(f.GeneratedFilenamePrefix)
		if test, ok := tests[filename]; ok {
			assert.Equal(t, test.SingularName, crdInfo.SingularName)
			assert.Equal(t, test.PluralName, crdInfo.PluralName)
			assert.Equal(t, test.Kind, crdInfo.Kind)
			assert.Equal(t, test.Scope, crdInfo.Scope)
			tested++
		}
	}

	assert.Equal(t, len(tests), tested)
}

//go:embed test/project.yaml
var projectYAML string

//go:embed test/testobject.yaml
var testObjectYAML string

func TestYamlGen(t *testing.T) {
	tests := map[string]string{
		"project_ut.pb.yaml": projectYAML,
		"testobject.pb.yaml": testObjectYAML,
	}

	data := testpb.GetProtocReqData()
	resp := GenerateYaml(data)
	tested := 0
	for _, f := range resp.GetFile() {
		filename := filepath.Base(f.GetName())
		if expectedYaml, ok := tests[filename]; ok {
			assert.Equal(t, expectedYaml, f.GetContent())
			tested++
		}
	}

	assert.Equal(t, len(tests), tested)
}

func TestK8sTypes(t *testing.T) {
	assert.Contains(t, jsonSchemas, "k8s.io.apimachinery.pkg.apis.meta.v1.LabelSelector")
}
