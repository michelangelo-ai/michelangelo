package crd

import (
	"bytes"

	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/util/json"
)

// Diff composite pattern for diff calculation and compatibility validation
type Diff interface {

	// Compatible define compatibility rules. Separate concerns of diff calculation and compatibility validation
	Compatible() bool
}

// ValueDiff implements Diff interface. A ValueDiff is compatible only if both From and To are nil.
type ValueDiff struct {
	From interface{}
	To   interface{}
}

// Compatible implements Diff.Compatible
func (vd *ValueDiff) Compatible() bool {
	return vd == nil || *vd == ValueDiff{}
}

// SchemaDiff checks compatability of two k8s CRD schemas
// k8s use OpenAPI schema for CRD definition [https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definitions/#specifying-a-structural-schema]
// When checking compatability, we ignore following OPENAPI props:
// 1) OPENAPI props only used for documentation
//   - ExternalDocs
//   - Example
//   - Description
//   - Title
//
// 2) OPENAPI props not used in k8s
//   - Discriminator
//   - XML
//   - Deprecated
//   - ReadOnly
//   - WriteOnly
//   - AdditionalPropertiesAllowed
//   - AdditionalProperties
//
// Validation changes (e.g. maximum, minimum, format, etc.) in the schema are not checked for compatibility.
type SchemaDiff struct {
	TypeDiff       *ValueDiff
	PropertiesDiff *SchemaObjectDiff
	ItemsDiff      *SchemaDiff
	OneOfDiff      *SchemaArrayDiff
	AnyOfDiff      *SchemaArrayDiff
	AllOfDiff      *SchemaArrayDiff
}

// NewSchemaDiff compare two JSONSchemaProps and return the diff
func NewSchemaDiff(old *apiextv1.JSONSchemaProps, new *apiextv1.JSONSchemaProps) (*SchemaDiff, error) {
	diff := SchemaDiff{}

	if old == nil {
		old = &apiextv1.JSONSchemaProps{}
	}

	if new == nil {
		new = &apiextv1.JSONSchemaProps{}
	}

	if old.Type != new.Type {
		diff.TypeDiff = &ValueDiff{From: old.Type, To: new.Type}
	}

	if old.Properties != nil || new.Properties != nil {
		propertiesDiff, err := NewSchemaObjectDiff(old.Properties, new.Properties)
		if err != nil {
			return nil, err
		}
		diff.PropertiesDiff = propertiesDiff
	}

	if old.Items != nil || new.Items != nil {
		var oldItemSchema *apiextv1.JSONSchemaProps
		if old.Items != nil {
			oldItemSchema = old.Items.Schema
		}
		var newItemSchema *apiextv1.JSONSchemaProps
		if new.Items != nil {
			newItemSchema = new.Items.Schema
		}
		itemsDiff, err := NewSchemaDiff(oldItemSchema, newItemSchema)
		if err != nil {
			return nil, err
		}
		diff.ItemsDiff = itemsDiff
	}

	if old.OneOf != nil || new.OneOf != nil {
		oneOfDiff, err := NewSchemaArrayDiff(old.OneOf, new.OneOf)
		if err != nil {
			return nil, err
		}
		diff.OneOfDiff = oneOfDiff
	}

	if old.AnyOf != nil || new.AnyOf != nil {
		anyOfDiff, err := NewSchemaArrayDiff(old.AnyOf, new.AnyOf)
		if err != nil {
			return nil, err
		}
		diff.AnyOfDiff = anyOfDiff
	}

	if old.AllOf != nil || new.AllOf != nil {
		allOfDiff, err := NewSchemaArrayDiff(old.AllOf, new.AllOf)
		if err != nil {
			return nil, err
		}
		diff.AllOfDiff = allOfDiff
	}

	if old.AllOf != nil || new.AllOf != nil {
		allOfDiff, err := NewSchemaArrayDiff(old.AllOf, new.AllOf)
		if err != nil {
			return nil, err
		}
		diff.AllOfDiff = allOfDiff
	}

	// return nil if no diff exists
	emptyDiff := SchemaDiff{}
	if diff == emptyDiff {
		return nil, nil
	}

	return &diff, nil
}

// Compatible implement Diff.Compatible
// Following changes are not compatible
// 1) type change
// 2) remove properties
// 3) any 1) and 2) changes in nested schema hierarchy (recursive)
func (d *SchemaDiff) Compatible() bool {
	if d == nil {
		return true
	}

	if !d.TypeDiff.Compatible() {
		return false
	}

	if !d.PropertiesDiff.Compatible() {
		return false
	}

	if !d.ItemsDiff.Compatible() {
		return false
	}

	if !d.AnyOfDiff.Compatible() {
		return false
	}

	if !d.AllOfDiff.Compatible() {
		return false
	}

	if !d.OneOfDiff.Compatible() {
		return false
	}

	return true
}

// SchemaArrayDiff implements Diff interface, compares two arrays of JSON property schemas
type SchemaArrayDiff struct {
	Deleted  []*apiextv1.JSONSchemaProps
	Added    []*apiextv1.JSONSchemaProps
	Modified []*SchemaDiff
}

// NewSchemaArrayDiff compares two arrays of JSON property schemas
func NewSchemaArrayDiff(old []apiextv1.JSONSchemaProps, new []apiextv1.JSONSchemaProps) (*SchemaArrayDiff, error) {

	var deletedProps []*apiextv1.JSONSchemaProps
	var addedProps []*apiextv1.JSONSchemaProps
	var modifiedProps []*SchemaDiff
	n := len(old)
	for i := 0; i < n; i++ {
		if i >= len(new) {
			deletedProps = append(deletedProps, &old[i])
			continue
		}

		diff, err := NewSchemaDiff(&old[i], &new[i])
		if err != nil {
			return nil, err
		}
		if diff != nil {
			modifiedProps = append(modifiedProps, diff)
		}
	}

	for i := n; i < len(new); i++ {
		addedProps = append(addedProps, &new[i])
	}

	if len(deletedProps) > 0 || len(addedProps) > 0 || len(modifiedProps) > 0 {
		return &SchemaArrayDiff{Deleted: deletedProps, Added: addedProps, Modified: modifiedProps}, nil
	}

	return nil, nil
}

// Compatible implements Diff.Compatible, allows both deletion and addition
func (d *SchemaArrayDiff) Compatible() bool {
	if d == nil {
		return true
	}

	// not allow deletion
	if len(d.Deleted) > 0 {
		return false
	}

	// not compatible if there is incompatible modifications
	for _, v := range d.Modified {
		if !v.Compatible() {
			return false
		}
	}

	return true
}

// SchemaObjectDiff implements Diff interface, compares the property maps of two JSON object schemas
type SchemaObjectDiff struct {
	Deleted  map[string]*apiextv1.JSONSchemaProps
	Added    map[string]*apiextv1.JSONSchemaProps
	Modified map[string]*SchemaDiff
}

func NewSchemaObjectDiff(old map[string]apiextv1.JSONSchemaProps, new map[string]apiextv1.JSONSchemaProps) (*SchemaObjectDiff, error) {

	deletedProps := make(map[string]*apiextv1.JSONSchemaProps)
	addedProps := make(map[string]*apiextv1.JSONSchemaProps)
	modifiedProps := make(map[string]*SchemaDiff)
	for key, oldValue := range old {
		newValue, present := new[key]
		if !present {
			deletedProps[key] = &oldValue
			continue
		}

		diff, err := NewSchemaDiff(&oldValue, &newValue)
		if err != nil {
			return nil, err
		}

		if diff != nil {
			modifiedProps[key] = diff
		}
	}

	for key, newValue := range new {
		_, present := old[key]
		if !present {
			addedProps[key] = &newValue
			continue
		}
	}

	if len(deletedProps) != 0 || len(addedProps) != 0 || len(modifiedProps) != 0 {
		return &SchemaObjectDiff{
			Deleted:  deletedProps,
			Added:    addedProps,
			Modified: modifiedProps,
		}, nil
	}

	return nil, nil
}

// Compatible implements Diff.Compatible, allows both deletion and addition
func (d *SchemaObjectDiff) Compatible() bool {
	if d == nil {
		return true
	}

	// not allow deletion
	if len(d.Deleted) > 0 {
		return false
	}

	// not compatible if there is incompatible modifications
	for _, v := range d.Modified {
		if !v.Compatible() {
			return false
		}
	}

	return true
}

// CompareResult is the result of CompareCRDSchemas function,
// indicating whether there is a change and whether the change is compatible
type CompareResult struct {
	HasChange  bool
	Compatible bool
}

// CompareCRDSchemas compare two CRD schemas
// This function compares if the schema of the two CRDs has changed.
// If the CRD schema has changed, it will check if the schemas of the same version are backward compatible.
// For example, if both oldCRD and newCRD have a version named "v1", it will check the compatibility of
// the two "v1" schemas in oldCRD and newCRD.
func CompareCRDSchemas(oldCRD *apiextv1.CustomResourceDefinition, newCRD *apiextv1.CustomResourceDefinition) (*CompareResult, error) {
	schemaHasChange := hasChange(oldCRD, newCRD)
	if !schemaHasChange {
		return &CompareResult{
			HasChange:  false,
			Compatible: true,
		}, nil
	}

	changeCompatible, err := isSpecChangeCompatible(oldCRD, newCRD)
	if err != nil {
		return nil, err
	}

	return &CompareResult{
		HasChange:  true,
		Compatible: changeCompatible,
	}, nil
}

func hasChange(oldCRD *apiextv1.CustomResourceDefinition, newCRD *apiextv1.CustomResourceDefinition) bool {
	var oldSpec bytes.Buffer
	json.NewEncoder(&oldSpec).Encode(oldCRD.Spec.Versions)
	var newSpec bytes.Buffer
	json.NewEncoder(&newSpec).Encode(newCRD.Spec.Versions)
	if oldSpec.String() == newSpec.String() {
		return false
	}

	return true
}

func isSpecChangeCompatible(oldCRD *apiextv1.CustomResourceDefinition, newCRD *apiextv1.CustomResourceDefinition) (bool, error) {
	oldVersions := map[string]*apiextv1.CustomResourceDefinitionVersion{}
	for _, v := range oldCRD.Spec.Versions {
		oldVersions[v.Name] = &v
	}
	newVersions := map[string]*apiextv1.CustomResourceDefinitionVersion{}
	for _, v := range newCRD.Spec.Versions {
		newVersions[v.Name] = &v
	}

	if len(newVersions) == 0 {
		// new CRD has no versions defined
		return false, nil
	}

	for k, oldVersion := range oldVersions {
		newVersion, present := newVersions[k]
		if !present {
			// Allow deleting old CRD version
			continue
		}

		// compare the two schemas with the same version name
		schemaDiff, err := NewSchemaDiff(oldVersion.Schema.OpenAPIV3Schema, newVersion.Schema.OpenAPIV3Schema)
		if err != nil {
			return false, err
		}

		if schemaDiff != nil && !schemaDiff.Compatible() {
			return false, nil
		}
	}

	// Allow adding new CRD versions, so no need to check newVersions again

	return true, nil
}
