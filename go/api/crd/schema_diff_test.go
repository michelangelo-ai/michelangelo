package crd

import (
	"bytes"
	"os"
	"testing"

	assertion "github.com/stretchr/testify/assert"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
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
