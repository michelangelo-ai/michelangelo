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
	"encoding/json"
	"github.com/gogo/protobuf/jsonpb"
	"github.com/michelangelo-ai/michelangelo/go/kubeproto/util"
	"github.com/michelangelo-ai/michelangelo/go/kubeproto/metrics"
	"github.com/michelangelo-ai/michelangelo/go/storage"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
`, CRDImports)

	buf.Reset()
	crdSvcHandlerInfo := struct {
		KindName      string
		LowerKindName string
	}{"TestName", "LowerTestName"}

	CrdSvcHandler.Execute(&buf, crdSvcHandlerInfo)
	crdSvcHandlerCode := buf.String()
	assert.Contains(t, crdSvcHandlerCode, "TestName")
	assert.Contains(t, crdSvcHandlerCode, "LowerTestName")
	assert.Contains(t, crdSvcHandlerCode, "CreateTestName")
	assert.Contains(t, crdSvcHandlerCode, "GetTestName")
	assert.Contains(t, crdSvcHandlerCode, "UpdateTestName")
	assert.Contains(t, crdSvcHandlerCode, "DeleteTestName")
	assert.Contains(t, crdSvcHandlerCode, "DeleteTestNameCollection")
	assert.Contains(t, crdSvcHandlerCode, "ListTestName")
	assert.NotContains(t, crdSvcHandlerCode, "LogYARPCAudit")
	assert.NotContains(t, crdSvcHandlerCode, "UnifiedLogInfo")
	assert.NotContains(t, crdSvcHandlerCode, "UnifiedLogError")
}
