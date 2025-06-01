package config

import (
	"strings"

	envfx "github.com/michelangelo-ai/michelangelo/go/base/env"
	"github.com/stretchr/testify/assert"
	"go.uber.org/config"

	"os"
	"testing"
)

func TestNew(t *testing.T) {
	dir := t.TempDir()
	defer overwriteFile(t, dir, "base.yaml", "foo: ${ENV_FOO:123}")()
	defer overwriteFile(t, dir, "secrets.yaml", "password: ${PASSWORD}")()

	newEnv := func() envfx.Context {
		e := envfx.New().Environment
		e.ConfigPath = dir
		return e
	}

	t.Run("base only without env", func(t *testing.T) {
		cfg, err := New(Params{
			Environment: newEnv(),
		})
		assert.NoError(t, err)
		assert.Equal(t, "123", cfg.Provider.Get("foo").String())
		assert.Equal(t, "${PASSWORD}", cfg.Provider.Get("password").String())
	})

	t.Run("base with env override", func(t *testing.T) {
		defer setEnv("ENV_FOO", "666")()
		defer setEnv("PASSWORD", "fake_password")()
		cfg, err := New(Params{
			Environment: newEnv(),
		})
		assert.NoError(t, err)
		assert.Equal(t, "666", cfg.Provider.Get("foo").String())
		// secrets file is not expanded
		assert.Equal(t, "${PASSWORD}", cfg.Provider.Get("password").String())
	})

	t.Run("base with production override", func(t *testing.T) {
		defer overwriteFile(t, dir, "production.yaml", "foo: bar")()
		defer setEnv("RUNTIME_ENVIRONMENT", "production")()
		cfg, err := New(Params{Environment: newEnv()})
		assert.NoError(t, err)
		assert.Equal(t, "bar", cfg.Provider.Get("foo").String())
		assert.Equal(t, "${PASSWORD}", cfg.Provider.Get("password").String())
	})
}

func TestGetConfigDirs(t *testing.T) {
	t.Run("env not set", func(t *testing.T) {
		env := envfx.New().Environment
		dirs := getConfigDirs(env)
		assert.Equal(t, []string{"config"}, dirs)
	})

	t.Run("env with CONFIG_DIR", func(t *testing.T) {
		defer setEnv("CONFIG_DIR", "config/one:config/two")()
		env := envfx.New().Environment
		dirs := getConfigDirs(env)
		assert.Equal(t, []string{"config/one", "config/two"}, dirs)
	})
}

// setEnv sets the environment variable with provided key-value pair and returns a function to revert the change.
// often used in a deferred function call
func setEnv(key, value string) func() {
	res := func() { os.Unsetenv(key) }
	if oldVal, present := os.LookupEnv(key); present {
		res = func() { os.Setenv(key, oldVal) }
	}

	os.Setenv(key, value)
	return res
}

var k8sConf = `
apiVersion: v1
clusters:
- cluster:
    certificate-authority-data: LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURMRENDQWhRQ0NRRFhhaHE1VHlvWVpUQU5CZ2txaGtpRzl3MEJBUXNGQURCWU1Rc3dDUVlEVlFRR0V3SlYKVXpFTk1Bc0dBMVVFQ0F3RVRtOXVaVEVOTUFzR0ExVUVCd3dFVG05dVpURU5NQXNHQTFVRUNnd0VUbTl1WlRFYwpNQm9HQTFVRUF3d1RiV2xqYUdWc1lXNW5aV3h2TFdzNGN5MWpZVEFlRncweU1UQXpNalF3TVRReE1qaGFGdzB6Ck1UQXpNakl3TVRReE1qaGFNRmd4Q3pBSkJnTlZCQVlUQWxWVE1RMHdDd1lEVlFRSURBUk9iMjVsTVEwd0N3WUQKVlFRSERBUk9iMjVsTVEwd0N3WURWUVFLREFST2IyNWxNUnd3R2dZRFZRUUREQk50YVdOb1pXeGhibWRsYkc4dAphemh6TFdOaE1JSUJJakFOQmdrcWhraUc5dzBCQVFFRkFBT0NBUThBTUlJQkNnS0NBUUVBeVlYQjU2NEhqZG8xClRZVERnTTVuNVJFSEU1am1hanNlR0QzNlNnUzdCazN5V0R3V0Z4T3VRaWNXRnFocnhpR2NScE16VndvZTNKQjAKRERjUTRLQzhLS3FSbC9oRWVybGE5UUpiU3gybEs4Ny9Fck5vaVh1OWNDeHdCcmlmbFN5WGQrWkhQa0pCdnd6Swp6cExxQXhqb00wMXM1K082SFRnZkoxWDBmeFordnRibC9EOGN0SUliV2t6K29NTUg0dmZNekVTVld1dDM2eTdYCkdCQjJOTndrSEtqMnR5eHNycjA5a1J5anRvcFFNN2ZGTnJHSDJtM3lrTmhJNzUxUWVsWWVreDRJdGxzTFFPR3IKUkxFTEkwL09CTTdWMFZPTC9OdlJBZndMTlRIUXVuUlpiVnB2RTZ3WHY1YlVtRDZLY2ErQ294LzQ1MlhnTFZOKwp5SWN5Mm9xQmVRSURBUUFCTUEwR0NTcUdTSWIzRFFFQkN3VUFBNElCQVFCSFBaNmlpVzNwdm1qL2t0RUUvYlRpCmVYL3U4SERRNktZdnRjYlByNEovVVlGQ01JZHdmVUtRclJrNnRkRUFReHhHcXhHOUlqL3B3RHpGYytUcjBXRHYKeTB3akx1WGw0K0tncEhUbFdWY1U1Q2QxanJlejFEUXpsZEhQaUpmRkhHQTVoNmVwdVRkbU9KREFmQnhRU2YzTAo4aUQwdU1Ma2NmUlYrVHk1S1BIcWdpYVBwclNScnRiUUhOZDFIcE5oOHpTaEJrK1N0amMyNUMwbEs4d0t4SUY2CnZBSW1kWDM1QmIrc1dzYjVEeFZrRTZtK05aT3dwU1V6Tk1NRnBQbXBlMEtMWlZlNDJDN0NzZ05YcWtUVVB4bWQKMms2REtySXA2aHg4YkNyVGVvakJBSlNnMGp3WCtYSnlnVVdXWFJra3F0dlNlYjhYOVJLeG40QXpvNHM5Sko4agotLS0tLUVORCBDRVJUSUZJQ0FURS0tLS0tCg==
    server: https://10.1.130.1:1234
  name: michelangelo
contexts:
- context:
    cluster: michelangelo
    user: system:michelangelo-apiserver
  name: system:michelangelo-apiserver@michelangelo
current-context: system:michelangelo-apiserver@michelangelo
kind: Config
preferences: {}
users:
- name: system:michelangelo-apiserver
  user:
    client-certificate-data: LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURNVENDQWhrQ0FRRXdEUVlKS29aSWh2Y05BUUVMQlFBd1dERUxNQWtHQTFVRUJoTUNWVk14RFRBTEJnTlYKQkFnTUJFNXZibVV4RFRBTEJnTlZCQWNNQkU1dmJtVXhEVEFMQmdOVkJBb01CRTV2Ym1VeEhEQWFCZ05WQkFNTQpFMjFwWTJobGJHRnVaMlZzYnkxck9ITXRZMkV3SGhjTk1qRXdNekkwTURFME1USTRXaGNOTXpFd016SXlNREUwCk1USTRXakJsTVFzd0NRWURWUVFHRXdKVlV6RU5NQXNHQTFVRUNBd0VUbTl1WlRFTk1Bc0dBMVVFQnd3RVRtOXUKWlRFWE1CVUdBMVVFQ2d3T2MzbHpkR1Z0T20xaGMzUmxjbk14SHpBZEJnTlZCQU1NRm0xcFkyaGxiR0Z1WjJWcwpieTFyT0hNdFlXUnRhVzR3Z2dFaU1BMEdDU3FHU0liM0RRRUJBUVVBQTRJQkR3QXdnZ0VLQW9JQkFRREY5bnAzCmt2S3FSQ3crZFV6K29sRmxBSEhzckM3eXhROWphRGhvSnhGaHovVkhPeGVNcXloQ1RXVGZXb0czMEkvb0poanQKT3NzK05tKzVnalA2eWZNRUNPNXhvTEVPNDg3cmtjL0ZDVzB2cHFkU0tGZlJSWVE4RUJRZUNvQ0dTbit4aEdsUgo0NEtUREdQRU5RRmtZNjh3czlHRTJqSmxEOGZCcURhRTJ2MlhlejRQNnlPQUdEYXNsZXBRWW44cVFhcG5nQmd4CmpZdGNWRTBjUG4xWXRnQkpSemdTZCszSVZQelV1UWZCSkVjcG1zWE9rUEVXYXNzVTR5em5qYnBrVE90RTYrKzkKUVd3Yld0QlFhU0doWGVkb2NIRG1yS25oVU4yWHpGTnZhNVFNcElkSWluY1M5anJUdGZ4SW5jTndDVnh2WjhNYgpabXgwaGtDRFBOc1M4L21GQWdNQkFBRXdEUVlKS29aSWh2Y05BUUVMQlFBRGdnRUJBRTl5cEo4ellLS3pKV0hUCkkwZytBYi9LaFdCOUJjcnUwZWxxU0pUdUdFL0NhcTJUNnJiU3h5T1hGTnZweGNBSXBvR04vcGhGeEM4VlVxMVMKSTlXZW56alIwSTFLdVY0ZlFsRDlqS3BNWWtQQXRCZHhQeEI2eEQxVlpIQ3diRlNpbjBxbmhaT0lvRUtmZGM4Wgp1SlBPMytqckh4WGFPMUpUeVVka1Vqb09talpmL1hsRitBZ3pzUm9FbGtObCs5TWxoRjUxK1pWQTloNXpScGZNCnlEVnl5dm5kOUd2N2tvWDBKdGZMV3Fyck5kZGhUSDRuakt1MU1sU3dMZVVnSk4wb3hnU1FEOWxqK3BRUUZXRkMKSlk5bTk3VjZreWVrelBkOHVxdS9tUDdwQy95Z2hoSFNaSDZjN3hyTE42eGR2UlJNRDl6K3d0RVBRd0IzbVpBTApBc08rK1k4PQotLS0tLUVORCBDRVJUSUZJQ0FURS0tLS0tCg==
    client-key-data: LS0tLS1CRUdJTiBSU0EgUFJJVkFURSBLRVktLS0tLQpNSUlFcFFJQkFBS0NBUUVBeGZaNmQ1THlxa1FzUG5WTS9xSlJaUUJ4N0t3dThzVVBZMmc0YUNjUlljLzFSenNYCmpLc29RazFrMzFxQnQ5Q1A2Q1lZN1RyTFBqWnZ1WUl6K3NuekJBanVjYUN4RHVQTzY1SFB4UWx0TDZhblVpaFgKMFVXRVBCQVVIZ3FBaGtwL3NZUnBVZU9Da3d4anhEVUJaR092TUxQUmhOb3laUS9Id2FnMmhOcjlsM3MrRCtzagpnQmcyckpYcVVHSi9La0dxWjRBWU1ZMkxYRlJOSEQ1OVdMWUFTVWM0RW5mdHlGVDgxTGtId1NSSEtackZ6cER4CkZtckxGT01zNTQyNlpFenJST3Z2dlVGc0cxclFVR2tob1YzbmFIQnc1cXlwNFZEZGw4eFRiMnVVREtTSFNJcDMKRXZZNjA3WDhTSjNEY0FsY2IyZkRHMlpzZElaQWd6emJFdlA1aFFJREFRQUJBb0lCQVFDMWdCbWl2aWtjOE9vNwp6YUp0Si9kR2lsVHBjS2lPenh3ZndUTk9VdHQ5K1lQaDdneldYU3JKTjFwUDZoUmFxcTcyWXMraDJiRDBka2FUCktBeHVXSEFsOStaZWFiMU1jSk9McWlGN2Z2d0xuMWx6V3lmQTZHS29Ddm5XM1R3ZlkvaVZrbGhaeXlVeG16OXgKYWtPL2ZpSG1ua2R2N2ZvY2JJNXlRTDdUcWxDdGE2Vnl1amdDVTZOZ1RlUVQvUExiMjFpeVRZdHhTcWp6bjBFeApOZHA2UXB4R3k2bnRiUExUZGFBSVl1UTdLeFFsbzZjRlVsSElZWDROVm1LaUpTRTBNcVNaV3pCbS8wWDJrd3JsCmR3R0xHRFhIYWorR08vR283Wm1SWGJUTnpteXZDek9CSUJzOHlRRnBWQUdyYVBsYjZvczNLek5BZHFiYnZySWcKVnZRaW9XWFZBb0dCQU9kbjFxUnJuVFIrQVdDQ25CYmZXcHluNkluUXNyYnErZHhHYWRORXJuZ1V2ZExtYWx2YQorN2twR1hxMUhoSUFkTi9MRU1FQktOS2VYdDVGQnVLRnJqZGVMQWQ5WWJaNC9DYzFLZHY0L0srWnhONm44NElsCk5tOVRLdHovWHZUS0NSRmdHaEJoeEtPUVRYVDZ0Ri9WVWRQYjM5OEgwbmtzZmtGM0RobHRSbU12QW9HQkFOc0EKdDNsWTZmcDk5TTFyRktmN3hIR0NYaVdzV3IvLzhDMHJEOFYrZHU3YkQvc2d6VFNyL0pJdjJReEN1a1J4dmsxSQpwYU1YaWtjWmVvbTV3RzB3bUw1QkhaSWl2ZlZGQW5oWTVJQ1o1ajkveEExalhJbHlCRGtUZHFnamJHWnJRLzZpCldmYS9qeDNGRU9VQTVYWStsWktVeGVybk9PNmZlc3V6Tk16UHVoR0xBb0dCQUxEd0RnaFVuTFNwY0dZYUdEM0kKOU9ENTVtMlNYVVJPTVZVRHBpRTd6K2ZUZkQzSm55T3pNbXluQjJ0ekY1WU9NVTk1VnNzdEZzak0vWjhZeXFYawpMNHo0ZmRRUVErbWhZclNjQ3ZDKzFuOXlwVHpXMFBQL2ZqcnJMY2dqbjdpdXp2WXhORnk0VlFIMzhiSHpqSDRHCmYzWHVGcVRUdDFTZDk4QVl4M2didlFsVEFvR0FKcVNkdXovQktYNElNQ2J3NGlNK3FuakNmQXRKaUE5MUpjTXYKYVQzRFpxb295N3NoK21WT2o4ejVrM3hDdWNrSU4wTFdWMHpVRFcrbGU1L1hJRzB1eG9OZTRHWlk5bXBTNFVGdQpNSEwzZWNUbHB5Y2RNUE41WTBqWDZ4czFDVzFyOWdaWHNYNWpsbkVyWmYwZWdCclM4YVptdGVoTzEydzBrclR3CllDTlhSYmtDZ1lFQXdJSWRxWUtTOFdPdHZOMis1eGIxRkluTHozRTYvUENxMmI3SWVaNDY2T3NjVGZZOTJlYk4KRElHdlVKdFI1T29DeHJBSlBKcnBLVElUTFN5RlR6OExNekc2MzNSM0pId2dPYjlmWUswZE1FNDVselhVUHovNgplOFlZOGJ5Y1V1UllkcDJjNGovWDJkVTBEUnptQ1R2SWNRUi91MG5PUHdEN3gzaU0rSjhHeE44PQotLS0tLUVORCBSU0EgUFJJVkFURSBLRVktLS0tLQo=
`

func TestGetK8sConfig(t *testing.T) {
	file, _ := os.CreateTemp(t.TempDir(), "k8s-conf")
	os.Setenv("KUBECONFIG", file.Name())

	yamlStr := `
k8s:
  qps: 300
  burst: 600
`
	provider, err := config.NewYAML(config.Source(strings.NewReader(yamlStr)))
	assert.NoError(t, err)

	file.WriteString("invalid conf")
	file.Sync()
	conf, err := GetK8sConfig(provider)
	assert.Error(t, err) // error when the content of the configuration file is invalid

	file.Seek(0, 0)
	file.WriteString(k8sConf)
	file.Close()

	conf, err = GetK8sConfig(provider)
	assert.NoError(t, err)
	assert.Equal(t, float32(300), conf.QPS)
	assert.Equal(t, 600, conf.Burst)
	assert.Equal(t, "https://10.1.130.1:1234", conf.Host)

	// invalid k8s configuration
	yamlStr2 := `
k8s:
  qps: "invalid"
`
	provider2, err := config.NewYAML(config.Source(strings.NewReader(yamlStr2)))
	assert.NoError(t, err)
	_, err = GetK8sConfig(provider2)
	assert.Error(t, err)
}

func TestGetMetadataStorageConfig(t *testing.T) {
	file, _ := os.CreateTemp(t.TempDir(), "metadata-storage-conf")
	os.Setenv("KUBECONFIG", file.Name())

	yamlStr := `
metadataStorage:
  enableMetadataStorage: true
`
	provider, err := config.NewYAML(config.Source(strings.NewReader(yamlStr)))
	assert.NoError(t, err)
	conf, err := GetMetadataStorageConfig(provider)
	assert.NoError(t, err)
	assert.True(t, conf.EnableMetadataStorage)

	yamlStrInvalid := `
metadataStorage:
  enableMetadataStorage: invalid
`
	providerInvalid, err := config.NewYAML(config.Source(strings.NewReader(yamlStrInvalid)))
	assert.NoError(t, err)
	_, err = GetMetadataStorageConfig(providerInvalid)
	assert.Error(t, err) // error when the content of the configuration file is invalid
}

func TestGetWorkflowClientConfig(t *testing.T) {
	file, _ := os.CreateTemp(t.TempDir(), "workflow-client-conf")
	os.Setenv("KUBECONFIG", file.Name())

	yamlStr := `
workflowClient:
  host: https://10.1.130.1:1234
  domain: default
  transport: grpc
  service: cadence-frontend
`
	provider, err := config.NewYAML(config.Source(strings.NewReader(yamlStr)))
	assert.NoError(t, err)
	conf, err := GetWorkflowClientConfig(provider)
	assert.NoError(t, err)
	assert.Equal(t, "https://10.1.130.1:1234", conf.Host)
	assert.Equal(t, "default", conf.Domain)
	assert.Equal(t, "grpc", conf.Transport)
	assert.Equal(t, "cadence-frontend", conf.Service)
}
