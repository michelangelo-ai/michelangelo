package crd

import (
	"fmt"
	"reflect"

	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

// A valueDiff is compatible only if both from and To are nil.
type valueDiff struct {
	from interface{}
	to   interface{}
}

func (vd *valueDiff) compatible() bool {
	return vd == nil || *vd == valueDiff{}
}

// schemaDiff checks compatibility of two k8s CRD schemas
// k8s use OpenAPI schema for CRD definition [https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definitions/#specifying-a-structural-schema]
// When checking compatibility, we ignore following OPENAPI props:
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
type schemaDiff struct {
	typeDiff       *valueDiff
	propertiesDiff *schemaObjectDiff
	itemsDiff      *schemaDiff
	oneOfDiff      *schemaArrayDiff
	anyOfDiff      *schemaArrayDiff
	allOfDiff      *schemaArrayDiff
}

// newSchemaDiff compare two JSONSchemaProps and return the diff
func newSchemaDiff(old *apiextv1.JSONSchemaProps, new *apiextv1.JSONSchemaProps) *schemaDiff {
	diff := schemaDiff{}

	if old == nil {
		old = &apiextv1.JSONSchemaProps{}
	}

	if new == nil {
		new = &apiextv1.JSONSchemaProps{}
	}

	if old.Type != new.Type {
		diff.typeDiff = &valueDiff{from: old.Type, to: new.Type}
	}

	if old.Properties != nil || new.Properties != nil {
		propertiesDiff := newSchemaObjectDiff(old.Properties, new.Properties)
		diff.propertiesDiff = propertiesDiff
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
		itemsDiff := newSchemaDiff(oldItemSchema, newItemSchema)
		diff.itemsDiff = itemsDiff
	}

	if old.OneOf != nil || new.OneOf != nil {
		oneOfDiff := newSchemaArrayDiff(old.OneOf, new.OneOf)
		diff.oneOfDiff = oneOfDiff
	}

	if old.AnyOf != nil || new.AnyOf != nil {
		anyOfDiff := newSchemaArrayDiff(old.AnyOf, new.AnyOf)
		diff.anyOfDiff = anyOfDiff
	}

	if old.AllOf != nil || new.AllOf != nil {
		allOfDiff := newSchemaArrayDiff(old.AllOf, new.AllOf)
		diff.allOfDiff = allOfDiff
	}

	// return nil if no diff exists
	emptyDiff := schemaDiff{}
	if diff == emptyDiff {
		return nil
	}

	return &diff
}

// Following changes are not compatible for a schema
// 1) type change
// 2) remove properties
// 3) any 1) and 2) changes in nested schema hierarchy (recursive)
func (d *schemaDiff) compatible() bool {
	if d == nil {
		return true
	}

	if !d.typeDiff.compatible() {
		return false
	}

	if !d.propertiesDiff.compatible() {
		return false
	}

	if !d.itemsDiff.compatible() {
		return false
	}

	if !d.anyOfDiff.compatible() {
		return false
	}

	if !d.allOfDiff.compatible() {
		return false
	}

	if !d.oneOfDiff.compatible() {
		return false
	}

	return true
}

func (d *schemaDiff) incompatibleReasons(path string) []string {
	if d == nil {
		return nil
	}
	var reasons []string
	if d.typeDiff != nil && !d.typeDiff.compatible() {
		reasons = append(reasons, fmt.Sprintf("type changed at %q: %v → %v", path, d.typeDiff.from, d.typeDiff.to))
	}
	if d.propertiesDiff != nil {
		reasons = append(reasons, d.propertiesDiff.incompatibleReasons(path)...)
	}
	if d.itemsDiff != nil && !d.itemsDiff.compatible() {
		reasons = append(reasons, d.itemsDiff.incompatibleReasons(path+"[*]")...)
	}
	if d.anyOfDiff != nil && !d.anyOfDiff.compatible() {
		reasons = append(reasons, d.anyOfDiff.incompatibleReasons(path+".anyOf")...)
	}
	if d.allOfDiff != nil && !d.allOfDiff.compatible() {
		reasons = append(reasons, d.allOfDiff.incompatibleReasons(path+".allOf")...)
	}
	if d.oneOfDiff != nil && !d.oneOfDiff.compatible() {
		reasons = append(reasons, d.oneOfDiff.incompatibleReasons(path+".oneOf")...)
	}
	return reasons
}

// schemaArrayDiff compares two arrays of JSON property schemas
type schemaArrayDiff struct {
	deleted  []*apiextv1.JSONSchemaProps
	added    []*apiextv1.JSONSchemaProps
	modified []*schemaDiff
}

// newSchemaArrayDiff compares two arrays of JSON property schemas
func newSchemaArrayDiff(old []apiextv1.JSONSchemaProps, new []apiextv1.JSONSchemaProps) *schemaArrayDiff {

	var deletedProps []*apiextv1.JSONSchemaProps
	var addedProps []*apiextv1.JSONSchemaProps
	var modifiedProps []*schemaDiff
	n := len(old)
	for i := 0; i < n; i++ {
		if i >= len(new) {
			deletedProps = append(deletedProps, &old[i])
			continue
		}

		diff := newSchemaDiff(&old[i], &new[i])
		if diff != nil {
			modifiedProps = append(modifiedProps, diff)
		}
	}

	for i := n; i < len(new); i++ {
		addedProps = append(addedProps, &new[i])
	}

	if len(deletedProps) > 0 || len(addedProps) > 0 || len(modifiedProps) > 0 {
		return &schemaArrayDiff{deleted: deletedProps, added: addedProps, modified: modifiedProps}
	}

	return nil
}

func (d *schemaArrayDiff) compatible() bool {
	if d == nil {
		return true
	}

	// not allow deletion
	if len(d.deleted) > 0 {
		return false
	}

	// not compatible if there is incompatible modifications
	for _, v := range d.modified {
		if !v.compatible() {
			return false
		}
	}

	return true
}

func (d *schemaArrayDiff) incompatibleReasons(prefix string) []string {
	if d == nil {
		return nil
	}
	var reasons []string
	for i := range d.deleted {
		reasons = append(reasons, fmt.Sprintf("deleted element at %s[%d]", prefix, i))
	}
	for _, mod := range d.modified {
		reasons = append(reasons, mod.incompatibleReasons(prefix)...)
	}
	return reasons
}

// schemaObjectDiff compares the property maps of two JSON object schemas
type schemaObjectDiff struct {
	deleted  map[string]*apiextv1.JSONSchemaProps
	added    map[string]*apiextv1.JSONSchemaProps
	modified map[string]*schemaDiff
}

func newSchemaObjectDiff(old map[string]apiextv1.JSONSchemaProps, new map[string]apiextv1.JSONSchemaProps) *schemaObjectDiff {

	deletedProps := make(map[string]*apiextv1.JSONSchemaProps)
	addedProps := make(map[string]*apiextv1.JSONSchemaProps)
	modifiedProps := make(map[string]*schemaDiff)
	for key, oldValue := range old {
		newValue, present := new[key]
		if !present {
			deletedProps[key] = &oldValue
			continue
		}

		diff := newSchemaDiff(&oldValue, &newValue)

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
		return &schemaObjectDiff{
			deleted:  deletedProps,
			added:    addedProps,
			modified: modifiedProps,
		}
	}

	return nil
}

func (d *schemaObjectDiff) compatible() bool {
	if d == nil {
		return true
	}

	// not allow deletion
	if len(d.deleted) > 0 {
		return false
	}

	// not compatible if there is incompatible modifications
	for _, v := range d.modified {
		if !v.compatible() {
			return false
		}
	}

	return true
}

func (d *schemaObjectDiff) incompatibleReasons(prefix string) []string {
	if d == nil {
		return nil
	}
	var reasons []string
	for name := range d.deleted {
		reasons = append(reasons, fmt.Sprintf("deleted field %s.%s", prefix, name))
	}
	for name, mod := range d.modified {
		reasons = append(reasons, mod.incompatibleReasons(fmt.Sprintf("%s.%s", prefix, name))...)
	}
	return reasons
}

// VersionIncompatibility describes which fields made a specific CRD version's schema incompatible.
type VersionIncompatibility struct {
	Version string
	Reasons []string
}

// CompareResult is the result of CompareCRDSchemas function,
// indicating whether there is a change and whether the change is compatible
type CompareResult struct {
	HasChange              bool
	Compatible             bool
	IncompatibilityDetails []VersionIncompatibility // populated when Compatible=false
}

// CompareCRDSchemas compares two CRD schemas
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

	details, err := incompatibleVersions(oldCRD, newCRD)
	if err != nil {
		return nil, err
	}

	return &CompareResult{
		HasChange:              true,
		Compatible:             len(details) == 0,
		IncompatibilityDetails: details,
	}, nil
}

func hasChange(oldCRD *apiextv1.CustomResourceDefinition, newCRD *apiextv1.CustomResourceDefinition) bool {
	versionsChanged := !reflect.DeepEqual(oldCRD.Spec.Versions, newCRD.Spec.Versions)
	conversionChanged := !reflect.DeepEqual(oldCRD.Spec.Conversion, newCRD.Spec.Conversion)
	return versionsChanged || conversionChanged
}

func isSpecChangeCompatible(oldCRD *apiextv1.CustomResourceDefinition, newCRD *apiextv1.CustomResourceDefinition) (bool, error) {
	details, err := incompatibleVersions(oldCRD, newCRD)
	if err != nil {
		return false, err
	}
	return len(details) == 0, nil
}

func incompatibleVersions(oldCRD *apiextv1.CustomResourceDefinition, newCRD *apiextv1.CustomResourceDefinition) ([]VersionIncompatibility, error) {
	oldVersions := map[string]*apiextv1.CustomResourceDefinitionVersion{}
	for _, v := range oldCRD.Spec.Versions {
		oldVersions[v.Name] = &v
	}
	newVersions := map[string]*apiextv1.CustomResourceDefinitionVersion{}
	for _, v := range newCRD.Spec.Versions {
		newVersions[v.Name] = &v
	}

	if len(newVersions) == 0 {
		return []VersionIncompatibility{{Version: "*", Reasons: []string{"new CRD has no versions defined"}}}, nil
	}

	var result []VersionIncompatibility
	for k, oldVersion := range oldVersions {
		newVersion, present := newVersions[k]
		if !present {
			return nil, fmt.Errorf("CRD %s has version %s that is not in the new CRD", oldCRD.Name, k)
		}

		// compare the two schemas with the same version name
		diff := newSchemaDiff(oldVersion.Schema.OpenAPIV3Schema, newVersion.Schema.OpenAPIV3Schema)

		if diff != nil && !diff.compatible() {
			result = append(result, VersionIncompatibility{
				Version: k,
				Reasons: diff.incompatibleReasons(""),
			})
		}
	}

	// Allow adding new CRD versions, so no need to check newVersions again

	return result, nil
}
