package yaml

import (
	"embed"
	"io/fs"
	"strings"

	"github.com/gogo/protobuf/protoc-gen-gogo/generator"
	apiext "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	k8syaml "sigs.k8s.io/yaml"
)

var (
	// json schemas for the special types that cannot be directly mapped from protobuf schema to json / yaml schemas
	jsonSchemas = make(map[string]*apiext.JSONSchemaProps)

	//go:embed k8s/meta_v1/*.yaml
	k8sMetaV1Schemas embed.FS
)

func init() {
	loadSchemas("k8s.io.apimachinery.pkg.apis.meta.v1", k8sMetaV1Schemas)
}

func loadSchemas(packageName string, embedFS embed.FS) {
	fs.WalkDir(embedFS, ".", func(path string, file fs.DirEntry, err error) error {
		if file.IsDir() {
			return nil
		}
		yamlSchema, _ := embedFS.ReadFile(path)
		schema := apiext.JSONSchemaProps{}
		k8syaml.Unmarshal(yamlSchema, &schema)
		typeName := generator.CamelCase(strings.TrimSuffix(file.Name(), ".yaml"))
		jsonSchemas[packageName+"."+typeName] = &schema
		return nil
	})
}
