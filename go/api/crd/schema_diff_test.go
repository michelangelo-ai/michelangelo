package crd

import (
	"bytes"
	"os"
	"testing"

	assertion "github.com/stretchr/testify/assert"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
)

var testCRDManifestDir = "test_manifest"

var testMatrix = []struct {
	Name       string
	OldCRDFile string
	NewCRDFile string
	HasChange  bool
	Compatible bool
}{
	{Name: "test no change", OldCRDFile: "/project.pb.yaml", NewCRDFile: "/project.pb.yaml", HasChange: false, Compatible: true},
	{Name: "test add props", OldCRDFile: "/project.pb.yaml", NewCRDFile: "/project_add_props.pb.yaml", HasChange: true, Compatible: true},
	{Name: "test delete props", OldCRDFile: "/project.pb.yaml", NewCRDFile: "/project_delete_props.pb.yaml", HasChange: true, Compatible: false},
	{Name: "test change props type", OldCRDFile: "/project.pb.yaml", NewCRDFile: "/project_change_type.pb.yaml", HasChange: true, Compatible: false},
	{Name: "test add nested props", OldCRDFile: "/project.pb.yaml", NewCRDFile: "/project_add_nested_props.pb.yaml", HasChange: true, Compatible: true},
	{Name: "test delete nested props", OldCRDFile: "/project.pb.yaml", NewCRDFile: "/project_delete_nested_props.pb.yaml", HasChange: true, Compatible: false},
	{Name: "test change nested props type", OldCRDFile: "/project.pb.yaml", NewCRDFile: "/project_change_nested_type.pb.yaml", HasChange: true, Compatible: false},
	{Name: "test add oneOf, anyOf", OldCRDFile: "/project.pb.yaml", NewCRDFile: "/project_add_array_schema.pb.yaml", HasChange: true, Compatible: true},
	{Name: "test delete oneOf, anyOf", OldCRDFile: "/project.pb.yaml", NewCRDFile: "/project_delete_array_schema.pb.yaml", HasChange: true, Compatible: false},
}

func TestSchemaDiff(t *testing.T) {
	for _, test := range testMatrix {
		oldCrd, err := readCRDFromFile(testCRDManifestDir + test.OldCRDFile)
		assertion.NoError(t, err, test.Name)
		newCrd, err := readCRDFromFile(testCRDManifestDir + test.NewCRDFile)
		assertion.NoError(t, err, test.Name)

		diff, err := compareCRDSchemas(oldCrd, newCrd)
		assertion.NoError(t, err, test.Name)
		assertion.NotNil(t, diff, test.Name)
		assertion.Equal(t, test.HasChange, diff.hasChange, test.Name)
		assertion.Equal(t, test.Compatible, diff.compatible, test.Name)
	}
}

func TestIsSpecChangeCompatible(t *testing.T) {
	// Helper function to create a basic CRD version
	createVersion := func(name string, storage bool, propertyType string) apiextv1.CustomResourceDefinitionVersion {
		return apiextv1.CustomResourceDefinitionVersion{
			Name:    name,
			Served:  true,
			Storage: storage,
			Schema: &apiextv1.CustomResourceValidation{
				OpenAPIV3Schema: &apiextv1.JSONSchemaProps{
					Type: "object",
					Properties: map[string]apiextv1.JSONSchemaProps{
						"spec": {
							Type: "object",
							Properties: map[string]apiextv1.JSONSchemaProps{
								"testProperty": {
									Type: propertyType,
								},
							},
						},
					},
				},
			},
		}
	}

	// Helper function to create a CRD with given versions
	createCRD := func(name string, versions ...apiextv1.CustomResourceDefinitionVersion) *apiextv1.CustomResourceDefinition {
		return &apiextv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
			Spec: apiextv1.CustomResourceDefinitionSpec{
				Versions: versions,
			},
		}
	}

	t.Run("no changes - same versions", func(t *testing.T) {
		oldCRD := createCRD("test.example.com", createVersion("v1", true, "string"))
		newCRD := createCRD("test.example.com", createVersion("v1", true, "string"))

		compatible, err := isSpecChangeCompatible(oldCRD, newCRD)
		assertion.NoError(t, err)
		assertion.True(t, compatible)
	})

	t.Run("compatible - adding new version", func(t *testing.T) {
		oldCRD := createCRD("test.example.com", createVersion("v1", true, "string"))
		newCRD := createCRD("test.example.com",
			createVersion("v1", false, "string"),
			createVersion("v2", true, "string"))

		compatible, err := isSpecChangeCompatible(oldCRD, newCRD)
		assertion.NoError(t, err)
		assertion.True(t, compatible)
	})

	t.Run("compatible - adding property to existing version", func(t *testing.T) {
		oldCRD := createCRD("test.example.com", createVersion("v1", true, "string"))

		// Create new CRD with additional property
		newVersion := createVersion("v1", true, "string")
		newVersion.Schema.OpenAPIV3Schema.Properties["spec"].Properties["newProperty"] = apiextv1.JSONSchemaProps{
			Type: "integer",
		}
		newCRD := createCRD("test.example.com", newVersion)

		compatible, err := isSpecChangeCompatible(oldCRD, newCRD)
		assertion.NoError(t, err)
		assertion.True(t, compatible)
	})

	t.Run("incompatible - removing existing version", func(t *testing.T) {
		oldCRD := createCRD("test.example.com",
			createVersion("v1", false, "string"),
			createVersion("v2", true, "string"))
		newCRD := createCRD("test.example.com", createVersion("v2", true, "string"))

		compatible, err := isSpecChangeCompatible(oldCRD, newCRD)
		assertion.Error(t, err)
		assertion.False(t, compatible)
		assertion.Contains(t, err.Error(), "has version v1 that is not in the new CRD")
	})

	t.Run("incompatible - changing property type", func(t *testing.T) {
		oldCRD := createCRD("test.example.com", createVersion("v1", true, "string"))
		newCRD := createCRD("test.example.com", createVersion("v1", true, "integer"))

		compatible, err := isSpecChangeCompatible(oldCRD, newCRD)
		assertion.NoError(t, err)
		assertion.False(t, compatible)
	})

	t.Run("incompatible - removing property from existing version", func(t *testing.T) {
		oldCRD := createCRD("test.example.com", createVersion("v1", true, "string"))

		// Create new CRD without testProperty
		newVersion := createVersion("v1", true, "string")
		delete(newVersion.Schema.OpenAPIV3Schema.Properties["spec"].Properties, "testProperty")
		newCRD := createCRD("test.example.com", newVersion)

		compatible, err := isSpecChangeCompatible(oldCRD, newCRD)
		assertion.NoError(t, err)
		assertion.False(t, compatible)
	})

	t.Run("incompatible - new CRD with no versions", func(t *testing.T) {
		oldCRD := createCRD("test.example.com", createVersion("v1", true, "string"))
		newCRD := createCRD("test.example.com") // No versions

		compatible, err := isSpecChangeCompatible(oldCRD, newCRD)
		assertion.NoError(t, err)
		assertion.False(t, compatible)
	})

	t.Run("compatible - old CRD with no versions", func(t *testing.T) {
		oldCRD := createCRD("test.example.com") // No versions
		newCRD := createCRD("test.example.com", createVersion("v1", true, "string"))

		compatible, err := isSpecChangeCompatible(oldCRD, newCRD)
		assertion.NoError(t, err)
		assertion.True(t, compatible)
	})

	t.Run("compatible - multiple versions with compatible changes", func(t *testing.T) {
		oldCRD := createCRD("test.example.com",
			createVersion("v1", false, "string"),
			createVersion("v2", true, "string"))

		// Add new property to both versions and add v3
		newV1 := createVersion("v1", false, "string")
		newV1.Schema.OpenAPIV3Schema.Properties["spec"].Properties["newProp"] = apiextv1.JSONSchemaProps{Type: "integer"}
		newV2 := createVersion("v2", false, "string")
		newV2.Schema.OpenAPIV3Schema.Properties["spec"].Properties["newProp"] = apiextv1.JSONSchemaProps{Type: "integer"}
		newV3 := createVersion("v3", true, "string")

		newCRD := createCRD("test.example.com", newV1, newV2, newV3)

		compatible, err := isSpecChangeCompatible(oldCRD, newCRD)
		assertion.NoError(t, err)
		assertion.True(t, compatible)
	})
}

func readCRDFromFile(path string) (*apiextv1.CustomResourceDefinition, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	crd := apiextv1.CustomResourceDefinition{}
	err = yaml.NewYAMLToJSONDecoder(bytes.NewReader(data)).Decode(&crd)
	if err != nil {
		return nil, err
	}

	return &crd, nil
}
