package templates

import (
	_ "embed"
	"strings"
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

//go:embed validation.tmpl
var validationTmpl string

// Validation template for validation functions with extension hooks
var Validation = template.Must(template.New("validation").Parse(validationTmpl))

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

//go:embed unmarshal_enum.tmpl
var unmarshalEnum string

// CRDUnmarshalEnum is a template for the HasEnumFields() function.
var CRDUnmarshalEnum = template.Must(template.New("unmarshalJSON").Parse(unmarshalEnum))

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

// CRDGetIndexedFieldsHeader is a template for the GetIndexedKeyValuePairs() function signature.
var CRDGetIndexedFieldsHeader = template.Must(template.New("crdGetIndexedFieldsHeader").Parse(`
func (m *{{.Name}}) GetIndexedKeyValuePairs() ([]storage.IndexedField){
	var indexedFields []storage.IndexedField
`))

// CRDIndexesPathToKeyMapHeader is a template for generating CRDIndexesPathToKeyMap for a CRD.
var CRDIndexesPathToKeyMapHeader = template.Must(template.New("crdIndexesPathToKeyMapHeader").Parse(`
func init() {
	gvk := schema.GroupVersionKind{
		Group: "{{.Group}}",
		Version: "{{.Version}}",
		Kind: "{{.Kind}}",
	}
	IndexesPathToKeyMap[gvk] = make(map[string]string)

	// default index
	IndexesPathToKeyMap[gvk]["metadata.namespace"] = "namespace"
	IndexesPathToKeyMap[gvk]["metadata.name"] = "name"
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
	"github.com/gogo/protobuf/jsonpb"
	"github.com/michelangelo-ai/michelangelo/go/kubeproto/util"
	"github.com/michelangelo-ai/michelangelo/go/kubeproto/metrics"
	"github.com/michelangelo-ai/michelangelo/go/storage"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
`

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

//go:embed mysql_main_table_columns.tmpl
var mysqlMainTableColumns string

// CRDMySQLMainTableColumn consists of the base columns of a CRD's main table.
var CRDMySQLMainTableColumn = template.Must(template.New("MySQLMainTableColumn").Parse(mysqlMainTableColumns))

//go:embed mysql_main_table_indices.tmpl
var mysqlMainTableIndices string

// CRDMySQLMainTableIndex consists of the base indexes of a CRD's main table.
var CRDMySQLMainTableIndex = template.Must(template.New("MySQLMainTableIndex").Parse(strings.TrimSuffix(mysqlMainTableIndices, "\n")))

//go:embed mysql_label_annotation_tables.tmpl
var mysqlLabelAnnotationTables string

// CRDMySQLLabelAnnotationTable is a template of a CRD's label and annotation table schema.
var CRDMySQLLabelAnnotationTable = template.Must(template.New("MySQLLabelAnnotationTable").Parse(mysqlLabelAnnotationTables))
