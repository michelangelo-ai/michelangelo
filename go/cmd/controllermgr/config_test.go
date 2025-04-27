package main

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/config"
)

func TestGetK8sConfig(t *testing.T) {
	file, _ := os.CreateTemp(t.TempDir(), "k8s-conf")
	os.Setenv("KUBECONFIG", file.Name())

	yamlStr := `
controllermgr:
  k8s:
    qps: 300
    burst: 600
`
	provider, err := config.NewYAML(config.Source(strings.NewReader(yamlStr)))
	assert.NoError(t, err)

	file.WriteString("invalid conf")
	file.Sync()
	conf, err := getK8sRestConfig(provider)
	assert.Error(t, err) // error when the content of the configuration file is invalid

	file.Seek(0, 0)
	file.WriteString(yamlStr)
	file.Close()

	conf, err = getK8sRestConfig(provider)
	assert.NoError(t, err)
	assert.Equal(t, float32(300), conf.QPS)
	assert.Equal(t, 600, conf.Burst)

	// invalid k8s configuration
	yamlStr2 := `
controllermgr:
  k8s:
    qps: "invalid"
`
	provider2, err := config.NewYAML(config.Source(strings.NewReader(yamlStr2)))
	assert.NoError(t, err)
	_, err = getK8sRestConfig(provider2)
	assert.Error(t, err)
}

func TestGetMetadataStorageConfig(t *testing.T) {
	file, _ := os.CreateTemp(t.TempDir(), "metadata-storage-conf")
	os.Setenv("KUBECONFIG", file.Name())

	yamlStr := `
controllermgr:
  metadataStorage:
    enableMetadataStorage: true
`
	provider, err := config.NewYAML(config.Source(strings.NewReader(yamlStr)))
	assert.NoError(t, err)
	conf, err := getMetadataStorageConfig(provider)
	assert.NoError(t, err)
	assert.True(t, conf.EnableMetadataStorage)

	yamlStrInvalid := `
controllermgr:
  metadataStorage:
    enableMetadataStorage: invalid
`
	providerInvalid, err := config.NewYAML(config.Source(strings.NewReader(yamlStrInvalid)))
	assert.NoError(t, err)
	_, err = getMetadataStorageConfig(providerInvalid)
	assert.Error(t, err) // error when the content of the configuration file is invalid
}
