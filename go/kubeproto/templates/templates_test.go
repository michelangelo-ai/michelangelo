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

// TestCrdSvcHandlerAuthEnforcement locks the auth contract of the generated
// service handlers: every verb handler must authenticate the caller, then
// authorize the matching auth Action against the request namespace, and the
// audit-log defer must be registered before the auth checks so that denied
// requests are still audit-logged. Get and List historically rendered with no
// auth block at all, so this guards the template against regressing again.
func TestCrdSvcHandlerAuthEnforcement(t *testing.T) {
	var buf strings.Builder
	crdSvcHandlerInfo := struct {
		KindName      string
		LowerKindName string
	}{"TestName", "LowerTestName"}
	assert.NoError(t, CrdSvcHandler.Execute(&buf, crdSvcHandlerInfo))
	code := buf.String()

	handlers := []struct {
		method string
		action string
	}{
		{"CreateTestName", "authapi.Create"},
		{"GetTestName", "authapi.Get"},
		{"UpdateTestName", "authapi.Update"},
		{"DeleteTestName", "authapi.Delete"},
		{"DeleteTestNameCollection", "authapi.DeleteCollection"},
		{"ListTestName", "authapi.List"},
	}

	for _, h := range handlers {
		body := handlerBody(t, code, h.method)

		authnIdx := strings.Index(body, "authapi.Authenticate(ctx, c.authenticator)")
		assert.GreaterOrEqual(t, authnIdx, 0, "%s must authenticate the caller", h.method)

		authzCall := `authapi.Authorize(ctx, c.authorizer, userInfo, projectName, ` + h.action + `, "TestName")`
		assert.Contains(t, body, authzCall, "%s must authorize %s", h.method, h.action)

		auditIdx := strings.Index(body, "defer c.auditLogEmitter.Emit(")
		assert.GreaterOrEqual(t, auditIdx, 0, "%s must emit an audit-log event", h.method)
		assert.Less(t, auditIdx, authnIdx,
			"%s must register the audit-log defer before the auth checks so denied requests are audit-logged", h.method)
	}
}

// handlerBody returns the rendered source of one generated handler method,
// from its func declaration up to the next top-level func declaration.
func handlerBody(t *testing.T, code, method string) string {
	t.Helper()
	marker := "func (c LowerTestNameServiceHandler) " + method + "("
	start := strings.Index(code, marker)
	if start < 0 {
		t.Fatalf("generated code has no handler %s", method)
	}
	rest := code[start:]
	if end := strings.Index(rest[1:], "\nfunc "); end >= 0 {
		rest = rest[:end+1]
	}
	return rest
}
