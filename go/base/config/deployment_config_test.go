package config

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/config"
)

func TestGetDeploymentConfig(t *testing.T) {
	yamlStr := `
deployment:
  rollout:
    skipCanary: true
    modelLoadTimeout: 20m
    soakPeriod: 5m
  rollback:
    modelLoadTimeout: 10m
  metricGate:
    prometheusURL: http://prometheus.monitoring:9090
    timeout: 3s
    failClosed: true
    queries:
      - name: errors
        expr: 'rate(nv_inference_request_failure{model="{{.Model}}"}[5m])'
        threshold: 0.1
        comparison: gt
`
	provider, err := config.NewYAML(config.Source(strings.NewReader(yamlStr)))
	require.NoError(t, err)

	cfg, err := GetDeploymentConfig(provider)

	require.NoError(t, err)
	assert.Equal(t, DeploymentConfig{
		Rollout:  RolloutConfig{SkipCanary: true, ModelLoadTimeout: 20 * time.Minute, SoakPeriod: 5 * time.Minute},
		Rollback: RollbackConfig{ModelLoadTimeout: 10 * time.Minute},
		MetricGate: MetricGateConfig{
			PrometheusURL: "http://prometheus.monitoring:9090",
			Timeout:       3 * time.Second,
			FailClosed:    true,
			Queries: []MetricGateQuery{{
				Name:       "errors",
				Expr:       `rate(nv_inference_request_failure{model="{{.Model}}"}[5m])`,
				Threshold:  0.1,
				Comparison: "gt",
			}},
		},
	}, cfg)
}

func TestGetDeploymentConfigDefaults(t *testing.T) {
	provider, err := config.NewYAML(config.Source(strings.NewReader("k8s:\n  qps: 1\n")))
	require.NoError(t, err)

	cfg, err := GetDeploymentConfig(provider)

	require.NoError(t, err, "a missing deployment section must not be an error")
	assert.Equal(t, DeploymentConfig{}, cfg)
}

func TestGetDeploymentConfigInvalidDuration(t *testing.T) {
	provider, err := config.NewYAML(config.Source(strings.NewReader("deployment:\n  rollout:\n    modelLoadTimeout: soon\n")))
	require.NoError(t, err)

	_, err = GetDeploymentConfig(provider)

	assert.Error(t, err)
}

func TestGetInferenceServerConfigReadinessProbe(t *testing.T) {
	provider, err := config.NewYAML(config.Source(strings.NewReader("inferenceServer:\n  triton:\n    readinessProbe: server\n")))
	require.NoError(t, err)

	cfg, err := GetInferenceServerConfig(provider)

	require.NoError(t, err)
	assert.Equal(t, "server", cfg.Triton.ReadinessProbe)
}
