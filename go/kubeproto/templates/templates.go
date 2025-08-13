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

// ValidateFuncTmp is the shared template for the Validate function
// TypeName will be provided by the calling compiler
var ValidateFuncTmp = template.Must(template.New("validateFunc").Parse(`
func (this *{{.TypeName}}) Validate(prefix string) error {
{{.ValidateLogic}}
	return nil
}`))

// ValidateFieldTmp is the template for field validation
var ValidateFieldTmp = template.Must(template.New("validateField").Parse(`
		if {{.Condition}} {
			return status.Error(codes.InvalidArgument, prefix + n + " " + {{.Msg}})
		}`))

// ValidateMsg is the template for message validation
var ValidateMsg = `
		var i interface{}
		if reflect.ValueOf(v).Kind() == reflect.Ptr {
			i = reflect.ValueOf(v).Interface()
			if reflect.ValueOf(v).IsNil() {
				i = nil
			}
		} else {
			i = reflect.ValueOf(&v).Interface()
		}
		validate, hasValidate := i.(interface{ Validate(string) error })
		if hasValidate {
			if err := validate.Validate(prefix + n + "."); err != nil {
				return err
			}
		}`

// ValidateOneofFmt is the format string for oneof validation
var ValidateOneofFmt = `
	if this.Get%s() == nil {
		return status.Error(codes.InvalidArgument, "one field in oneof " + prefix + "%s(%s) must be set")
	}
`

// FileHeader is the common header for generated validation files
var FileHeader = `// Code generated by %s. DO NOT EDIT.

package %s

import (
	"fmt"
	"net"
	"net/mail"
	"net/url"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Reference imports to suppress errors if they are not otherwise used.
var _ = fmt.Errorf
var _ = reflect.ValueOf
var _ = regexp.MatchString
var _ = uuid.Parse
var _ = mail.ParseAddress
var _ = strings.Contains
var _ = net.ParseIP
var _ = mail.ParseAddress
var _ = url.ParseRequestURI
var _ = codes.InvalidArgument
var _ = status.Error
var _ = strconv.Itoa
`
