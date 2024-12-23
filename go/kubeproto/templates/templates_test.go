package templates

import (
	_ "embed"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

//go:embed test/group_version_test.txt
var expectedGroupVersionCode string

func TestTemplates(t *testing.T) {
	var buf strings.Builder

	typeInfo := struct {
		Name             string
		FileName         string
		DetachedComments string
		Comments         string
	}{"CRDName", "FileName",
		"// detached comment 1\n// detached comment 2",
		"// comments"}
	CRD.Execute(&buf, typeInfo)

	crdCode := buf.String()
	assert.Contains(t, crdCode, "type CRDName struct")
	assert.Contains(t, crdCode, "// detached comment 1\n// detached comment 2")
	assert.Contains(t, crdCode, "SchemeBuilder.Register(&CRDName{})")

	buf.Reset()
	listTypeInfo := struct {
		Name             string
		FileName         string
		DetachedComments string
		Comments         string
	}{"CRDName", "FileName",
		"// detached comment 1\n// detached comment 2",
		"// comments"}
	CRDList.Execute(&buf, listTypeInfo)

	crdListCode := buf.String()
	assert.Contains(t, crdListCode, "type CRDNameList struct")
	assert.Contains(t, crdListCode, "// detached comment 1\n// detached comment 2")
	assert.Contains(t, crdListCode, "SchemeBuilder.Register(&CRDNameList{})")

	buf.Reset()
	GroupVersion.Execute(&buf, struct {
		Group     string
		Version   string
		GoPackage string
	}{"TestGroup", "TestVersion", "TestPackage"})
	assert.Equal(t, expectedGroupVersionCode, buf.String())

	assert.Equal(t, `
	"bytes"
	"github.com/michelangelo-ai/michelangelo/go/kubeproto/util"
	"github.com/gogo/protobuf/jsonpb"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
`, CRDImports)
}
