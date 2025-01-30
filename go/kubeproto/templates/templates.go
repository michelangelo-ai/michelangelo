package templates

import (
	_ "embed"
	"text/template"
)

//go:embed crd.tmpl
var crdTmpl string

// CRD template of CRD type
var CRD = template.Must(template.New("crd").Parse(crdTmpl))

//go:embed crd_list.tmpl
var crdListTmpl string

// CRDList template of CRD list type
var CRDList = template.Must(template.New("crdList").Parse(crdListTmpl))

// RegisterCRDObject is a template that registers CRD Object to CrdObjects map
var RegisterCRDObject = template.Must(template.New("registerCrdObject").Parse(`func init() {
	CrdObjects["{{.Kind}}"] = &{{.Kind}}{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "{{.Group}}/{{.Version}}",
			Kind: "{{.Kind}}",
		},
	}
}
`))

// CRDImmutability is a template of the IsImmutableKind() function.
var CRDImmutability = template.Must(template.New("crdIsImmutableKind").Parse(`func (m *{{.Name}}) IsImmutableKind() bool {
{{if .Immutable}}
	return true
{{- else}}
	return false
{{- end}}
}
`))

// CRDHasBlobFields is a template of the HasBlobFields() function
var CRDHasBlobFields = template.Must(template.New("crdHasBlobFields").Parse(`func (m *{{.Name}}) HasBlobFields() bool {
{{if .HasBlobFields}}	return true
{{- else}}	return false
{{- end}}
}
`))

// CRDClearBlobFieldsHeader is a template of the ClearBlobFields() function signature
var CRDClearBlobFieldsHeader = template.Must(template.New("crdClearBlobFields").Parse(`func (m *{{.Name}}) ClearBlobFields() {
`))

// CRDFillBlobFields is a template of the FillBlobFields() function
var CRDFillBlobFields = template.Must(template.New("crFillBlobFields").Parse(`func (m *{{.Name}}) FillBlobFields(object k8sruntime.Object) {
	other := object.(*{{.Name}})
	m.Spec = other.Spec
	m.Status = other.Status
}
`))

//go:embed group_version.tmpl
var groupVersionTmpl string

// GroupVersion template of group version info file
var GroupVersion = template.Must(
	template.New("groupversion").Parse(groupVersionTmpl))

// CRDImports add these imports in the generated go code, if the file contains CRD types
var CRDImports = `
	"bytes"
	"encoding/json"
	"github.com/michelangelo-ai/michelangelo/go/kubeproto/util"
	"github.com/gogo/protobuf/jsonpb"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
`

// GroupK8sClient template for group k8s client
//
//go:embed group_k8s_client.tmpl
var GroupK8sClient string

// CrdSvcHandlerImports imports of CRD YARPC handlers
var CrdSvcHandlerImports = `
import (
	"context"

	"go.uber.org/fx"

	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	"github.com/michelangelo-ai/michelangelo/go/api"
	authapi "github.com/michelangelo-ai/michelangelo/go/auth"
	"github.com/michelangelo-ai/michelangelo/go/logging"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"github.com/uber-go/tally"
)
`

//go:embed crd_svc.tmpl
var crdSvcTmpl string

// CrdSvcHandler template of CRD YARPC handler
var CrdSvcHandler = template.Must(template.New("crdsvc").Parse(crdSvcTmpl))
