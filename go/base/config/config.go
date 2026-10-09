package config

import (
	"bytes"
	"flag"
	"text/template"
	"time"

	"github.com/michelangelo-ai/michelangelo/go/base/env"
	"go.uber.org/config"

	"os"
	"strings"

	"github.com/michelangelo-ai/michelangelo/go/storage"
	"go.uber.org/fx"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	_configKeySeparator       = ":"
	_defaultConfigDir         = "config"
	_k8sConfigKey             = "k8s"
	_metadataStorageConfigKey = "metadataStorage"
	_workflowClientConfigKey  = "workflowClient"
	_mysqlConfigKey           = "mysql"
	_ingesterConfigKey        = "ingester"
	_inferenceServerConfigKey = "inferenceServer"
	_deploymentConfigKey      = "deployment"
	_schedulerConfigKey       = "jobs.scheduler"
)

// K8sConfig is the configuration for k8s REST client.
type K8sConfig struct {
	QPS   float32 `yaml:"qps"`
	Burst int     `yaml:"burst"`
}

type WorkflowClientConfig struct {
	Service            string `yaml:"service"`
	Host               string `yaml:"host"`
	Transport          string `yaml:"transport"`
	Domain             string `yaml:"domain"`
	TaskList           string `yaml:"taskList"`
	Provider           string `yaml:"provider"`
	UseTLS             bool   `yaml:"useTLS"`
	ExecutionUrlFormat string `yaml:"executionUrlFormat"`
}

// BuildWorkflowUrl constructs a monitoring URL for a workflow execution from
// this config's ExecutionUrlFormat template.
//
// executionID identifies the workflow (e.g. a pipeline run's name, or
// TriggerRun's generated "<namespace>.<name>" workflow ID), and runID is the
// specific execution's run ID. Cadence/Temporal Web route a workflow's
// detail/summary page by (workflow ID, run ID) together, not by workflow ID
// alone, so a format string that only references {{.ExecutionID}} resolves
// to a listing rather than a specific execution. Pass "" for runID only if
// it isn't known yet -- callers should defer calling this until it is,
// rather than publish a URL that won't resolve to the intended execution.
//
// Returns "" if ExecutionUrlFormat or Domain is unset.
func (c WorkflowClientConfig) BuildWorkflowUrl(executionID string, runID string) string {
	if c.ExecutionUrlFormat == "" || c.Domain == "" {
		return ""
	}
	tmpl, err := template.New("url").Parse(c.ExecutionUrlFormat)
	if err != nil {
		return ""
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, map[string]string{
		"Domain":      c.Domain,
		"ExecutionID": executionID,
		"RunID":       runID,
	}); err != nil {
		return ""
	}
	return buf.String()
}

// KueueConfig configures the Kueue scheduler backend.
type KueueConfig struct {
	// LocalQueueTemplate resolves a project's LocalQueue name on the
	// target cluster; "{project}" is substituted with the project name.
	LocalQueueTemplate string `yaml:"localQueueTemplate"`
	// LocalQueueOverrides maps a project name to an explicit LocalQueue
	// name, winning over LocalQueueTemplate.
	LocalQueueOverrides map[string]string `yaml:"localQueueOverrides"`
	// APIVersion is the kueue.x-k8s.io API version used when talking to
	// Kueue on compute clusters. Defaults to "v1beta2" (Kueue v0.15+); set
	// "v1beta1" for older Kueue installations.
	APIVersion string `yaml:"apiVersion"`
}

// SchedulerConfig selects and configures the job scheduler backend.
type SchedulerConfig struct {
	// Backend is the scheduler backend: "default" (or empty) for the
	// built-in immediate-admission queue, "kueue" for Kueue-managed
	// admission.
	Backend string `yaml:"backend"`
	// Kueue holds Kueue backend settings; ignored unless Backend is "kueue".
	Kueue KueueConfig `yaml:"kueue"`
}

// Params defines the dependencies of the config fx module.
type Params struct {
	fx.In

	Environment env.Context
}

// Result defines the objects that the config fx module provides.
type Result struct {
	fx.Out

	Provider config.Provider
}

// Module load config.Provider based on the environment context.
var Module = fx.Module("config",
	fx.Provide(New),
	// SchedulerConfig is consumed by both the k8s engine mapper and the
	// scheduler module, so it is provided once here.
	fx.Provide(GetSchedulerConfig),
)

// New exports functionality similar to Module, but allows the caller to wrap
// or modify Result. Most users should use Module instead.
func New(p Params) (Result, error) {
	// use os.LookupEnv to look up environment variables
	lookupFun := os.LookupEnv
	cfg, err := newYAML(p.Environment, lookupFun)
	if err != nil {
		return Result{}, err
	}

	return Result{
		Provider: cfg,
	}, nil
}

// getConfigDirs extract config dirs from env if ConfigPath was set as environment variable,
// otherwise use default config dir
func getConfigDirs(env env.Context) []string {
	// Allow overriding the directory where config is loaded from
	if env.ConfigPath != "" {
		return strings.Split(env.ConfigPath, _configKeySeparator)
	}
	return []string{_defaultConfigDir}
}

// GetK8sConfig parses the configuration file and returns the k8s REST client configuration
func GetK8sConfig(provider config.Provider) (*rest.Config, error) {
	flag.Parse()
	conf, err := ctrl.GetConfig()
	if err != nil {
		return nil, err
	}
	k8sConfig := K8sConfig{}
	err = provider.Get(_k8sConfigKey).Populate(&k8sConfig)
	if err != nil {
		return nil, err
	}
	conf.QPS = k8sConfig.QPS
	conf.Burst = k8sConfig.Burst
	return conf, nil
}

// GetMetadataStorageConfig parses the configuration file and returns the metadata storage configuration
func GetMetadataStorageConfig(provider config.Provider) (storage.MetadataStorageConfig, error) {
	storageConfig := storage.MetadataStorageConfig{}
	err := provider.Get(_metadataStorageConfigKey).Populate(&storageConfig)
	return storageConfig, err
}

// GetWorkflowClientConfig parses the configuration file and returns the workflow client configuration
func GetWorkflowClientConfig(provider config.Provider) (WorkflowClientConfig, error) {
	workflowClientConfig := WorkflowClientConfig{}
	err := provider.Get(_workflowClientConfigKey).Populate(&workflowClientConfig)
	return workflowClientConfig, err
}

// GetSchedulerConfig parses the configuration file and returns the job scheduler configuration
func GetSchedulerConfig(provider config.Provider) (SchedulerConfig, error) {
	schedulerConfig := SchedulerConfig{}
	err := provider.Get(_schedulerConfigKey).Populate(&schedulerConfig)
	return schedulerConfig, err
}

// GetMySQLConfig parses the configuration file and returns the MySQL configuration.
func GetMySQLConfig(provider config.Provider) (MySQLConfig, error) {
	mysqlConfig := MySQLConfig{}
	err := provider.Get(_mysqlConfigKey).Populate(&mysqlConfig)
	return mysqlConfig, err
}

// GetIngesterConfig parses the configuration file and returns the ingester configuration.
func GetIngesterConfig(provider config.Provider) (IngesterConfig, error) {
	ingesterConfig := IngesterConfig{}
	err := provider.Get(_ingesterConfigKey).Populate(&ingesterConfig)
	return ingesterConfig, err
}

// InferenceServerConfig is the controller-side configuration for the inference
// server controller.
type InferenceServerConfig struct {
	Gateway GatewayConfig `yaml:"gateway"`
	Triton  TritonConfig  `yaml:"triton"`
}

// TritonConfig holds operator-level defaults for the Triton backend. DefaultImage
// is the container image for InferenceServers that do not set
// spec.initSpec.servingSpec.image. Empty means the backend's built-in image.
type TritonConfig struct {
	DefaultImage string `yaml:"defaultImage"`
	// ReadinessProbe selects how Triton replicas report readiness: "model-aware" (the
	// default, a replica is ready only when every serving model in its model config is
	// loaded), "server" (Triton's /v2/health/ready only) or "none".
	ReadinessProbe string `yaml:"readinessProbe"`
	// Probes overrides the timing of the Triton pod probes. Fields left at zero keep the
	// built-in values, so an empty block changes nothing.
	Probes TritonProbesConfig `yaml:"probes"`
	// Drain tunes how a Triton pod shuts down when it is replaced. Zero fields keep the
	// built-in values.
	Drain TritonDrainConfig `yaml:"drain"`
}

// TritonDrainConfig tunes the shutdown of a replaced Triton pod so in-flight requests finish.
type TritonDrainConfig struct {
	// PreStopSeconds delays SIGTERM so the pod leaves the Service endpoints first.
	PreStopSeconds int32 `yaml:"preStopSeconds"`
	// ExitTimeoutSeconds bounds how long Triton waits for in-flight requests after SIGTERM.
	ExitTimeoutSeconds int32 `yaml:"exitTimeoutSeconds"`
}

// TritonProbesConfig holds the timing overrides for each Triton pod probe.
type TritonProbesConfig struct {
	// Startup waits for Triton's HTTP server to come up before liveness applies.
	Startup ProbeTimingConfig `yaml:"startup"`
	// Liveness restarts a container whose HTTP server has stopped answering.
	Liveness ProbeTimingConfig `yaml:"liveness"`
	// Readiness gates a replica's membership in the inference Service.
	Readiness ProbeTimingConfig `yaml:"readiness"`
}

// ProbeTimingConfig overrides one probe's timing. Zero keeps the probe's built-in value.
type ProbeTimingConfig struct {
	PeriodSeconds    int32 `yaml:"periodSeconds"`
	TimeoutSeconds   int32 `yaml:"timeoutSeconds"`
	FailureThreshold int32 `yaml:"failureThreshold"`
}

// GatewayConfig describes the k8s Gateway resource and its Istio-generated
// backing Service. Both are provisioned out-of-band (e.g., by `ma sandbox
// create`). Name is the Gateway CRD resource name, referenced in HTTPRoute
// parentRefs. ServiceName is the Istio-generated Service used by EndpointSource
// to discover the gateway's node IP and port.
type GatewayConfig struct {
	Name             string `yaml:"name"`
	ServiceName      string `yaml:"serviceName"`
	ServiceNamespace string `yaml:"serviceNamespace"`
	PortName         string `yaml:"portName"`
}

// GetInferenceServerConfig parses the configuration file and returns the
// inference server controller configuration.
func GetInferenceServerConfig(provider config.Provider) (InferenceServerConfig, error) {
	inferenceServerConfig := InferenceServerConfig{}
	err := provider.Get(_inferenceServerConfigKey).Populate(&inferenceServerConfig)
	return inferenceServerConfig, err
}

// DeploymentConfig is the controller-side configuration for the deployment controller's
// rollout safety: canary, load timeouts, soaks and the metric gate.
type DeploymentConfig struct {
	Rollout    RolloutConfig    `yaml:"rollout"`
	Rollback   RollbackConfig   `yaml:"rollback"`
	MetricGate MetricGateConfig `yaml:"metricGate"`
}

// RolloutConfig tunes how a new model is rolled out across a cluster's replicas.
type RolloutConfig struct {
	// SkipCanary disables loading and validating the model on one replica per cluster
	// before the rest of the cluster loads it.
	SkipCanary bool `yaml:"skipCanary"`
	// ModelLoadTimeout bounds how long a cluster may take to load the model on every
	// replica. Zero uses the plugin default.
	ModelLoadTimeout time.Duration `yaml:"modelLoadTimeout"`
	// SoakPeriod is how long a cluster serves the new model, with the health and metric
	// gates active, before the rollout moves to the next cluster. A Zonal strategy's
	// rollout period overrides it. Zero skips soaking.
	SoakPeriod time.Duration `yaml:"soakPeriod"`
}

// RollbackConfig tunes how a rollback restores the previous model.
type RollbackConfig struct {
	// ModelLoadTimeout bounds how long a rollback waits for the previous model to be
	// loaded again on every replica before restoring traffic to it. Zero uses the
	// plugin default.
	ModelLoadTimeout time.Duration `yaml:"modelLoadTimeout"`
}

// MetricGateConfig configures the Prometheus-backed rollout gate. The gate is evaluated on
// every reconcile while a rollout or soak is in progress; a breach rolls the candidate back.
type MetricGateConfig struct {
	// PrometheusURL is the base URL of the Prometheus HTTP API. Empty disables the gate.
	PrometheusURL string `yaml:"prometheusURL"`
	// Timeout bounds each query. Zero uses the gate default.
	Timeout time.Duration `yaml:"timeout"`
	// FailClosed treats an unreachable Prometheus as a breach. Off by default, so an
	// observability outage stalls nothing and is logged instead.
	FailClosed bool `yaml:"failClosed"`
	// Queries are evaluated in order. Empty uses the gate's built-in Triton failure-ratio
	// query.
	Queries []MetricGateQuery `yaml:"queries"`
}

// MetricGateQuery is one PromQL expression with the threshold that marks a breach.
type MetricGateQuery struct {
	// Name identifies the query in logs and rollback reasons.
	Name string `yaml:"name"`
	// Expr is a Go template rendering a PromQL instant query. It may reference
	// {{.Model}}, {{.Deployment}}, {{.Namespace}} and {{.InferenceServer}}.
	Expr string `yaml:"expr"`
	// Threshold is compared against every sample the query returns.
	Threshold float64 `yaml:"threshold"`
	// Comparison is "gt" (default: a sample above the threshold breaches) or "lt".
	Comparison string `yaml:"comparison"`
}

// GetDeploymentConfig parses the configuration file and returns the deployment controller
// configuration. An absent section yields the zero value, which the plugin fills with its
// defaults.
func GetDeploymentConfig(provider config.Provider) (DeploymentConfig, error) {
	deploymentConfig := DeploymentConfig{}
	err := provider.Get(_deploymentConfigKey).Populate(&deploymentConfig)
	return deploymentConfig, err
}
