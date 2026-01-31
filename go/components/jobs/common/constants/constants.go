package constants

import (
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	corev1 "k8s.io/api/core/v1"
)

// These are valid condition types of a Ray Job
const (
	RayClusterReadyCondition string = "RayClusterReady"
)

// These are valid condition types of a Spark Job
const (
	SparkAppRunningCondition string = "SparkAppRunning"
	SparkAppFailedCondition  string = "SparkAppFailed"
)

// These are valid condition types of all Jobs
const (
	EnqueuedCondition             string = "Enqueued"
	KillingCondition              string = "Killing"
	KilledCondition               string = "Killed"
	LaunchedCondition             string = "Launched"
	PendingCondition              string = "Pending"
	ScheduledCondition            string = "Scheduled"
	MetricsConfigCreatedCondition string = "MetricsConfigCreated"
	SecretCreatedCondition        string = "SecretCreated"
	SucceededCondition            string = "Succeeded"
)

// condition reasons
const (
	AddedToSchedulerQueue             string = "AddedToSchedulerQueue"
	NoResourcePoolsFoundInCache       string = "NoResourcePoolsFoundInCache"
	ResourcePoolMatchedBasedOnLoad    string = "ResourcePoolMatchedBasedOnLoad"
	NoResourcePoolMatchedRequirements string = "NoResourcePoolMatchedRequirements"
	AssignedFallbackResourcePool      string = "AssignedFallbackResourcePool"

	ClusterNotReady    string = "ClusterNotReady"
	ClusterKilled      string = "ClusterKilled"
	SparkAppNotRunning string = "SparkAppNotRunning"
	SparkAppKilled     string = "SparkAppKilled"
)

// condition messages - the prefix
// is the type of the condition
const (
	KilledMessageJobNotLaunched string = "job could be killed early because it was not yet launched"
	KilledMessagedJobFinished   string = "Skip killing job since it is finished"
	KilledMessageJobObsolete    string = "job could be killed because it has been running too long"
	KilledMessageJobKilledByUI  string = "job has been killed by compute UI"
)

// Condition metadata key names. These should be unique with a given condition.
const (
	NumSchedulerAttempts string = "numSchedulerAttempts"
)

// These are valid condition types of a cluster
const (
	// ClusterReady means the cluster is ready to accept workloads.
	ClusterReady string = "Ready"
	// ClusterOffline means the cluster is temporarily down or not reachable
	ClusterOffline string = "Offline"
	// ClusterConfigMalformed means the cluster's configuration may be malformed.
	ClusterConfigMalformed string = "ConfigMalformed"
)

// MA2.0 related constants
const (
	ClustersNamespace string = "ma-system"
)

// Michelangelo label keys
const (
	JobNameLabelKey       string = "ma/job-name"
	ProjectNameLabelKey   string = "ma/project-name"
	JobControlPlaneEnvKey string = "ma/control-plane-env"
	UserLabelKey          string = "ma/user"
	OwnerServiceLabelKey  string = "ma/owner-service"
)

// Ray label keys
const (
	RayClusterNameLabelKey string = "ray.io/cluster"
	RayNodeTypeLabelKey    string = "ray.io/node-type"
	RayNodeLabelKey        string = "ray.io/is-ray-node"
)

// Secret label keys and values
const (
	SecretAppNameKey   string = "app"
	SecretAppNameValue string = "michelangelo-controllermgr"
)

// Label values
const (
	GenericSpireIdentityLabelValue string = "michelangelo.ray.workload"
	SecureServiceMeshMTLSValue     string = "lts"
	MAOwnerServiceLabelValue       string = "michelangelo-ray"
	MAOwnerSparkLabelValue         string = "michelangelo-spark"
)

// Annotations
const (
	GenericSpiffeAnnotationValue string = "michelangelo/ray/workload"
)

// Ray node type labels
const (
	RayHeadNodeLabel    = "HEAD_NODE"
	RayDataNodeLabel    = "DATA_NODE"
	RayTrainerNodeLabel = "TRAINER_NODE"
)

// Generic constants
const (
	HeadContainerName          string = "ray-head"
	KubeRayResource            string = "rayclusters"
	KubeRayJobResource         string = "rayjobs"
	KubeSparkResource          string = "sparkapplications"
	IsRayNodeValue             string = "yes"
	RayHeadNodeType            string = "head"
	RayWorkerNodeType          string = "worker"
	VolumePrefix               string = "volume-"
	WorkerContainerName        string = "ray-worker"
	SparkDriverContainerName   string = "spark-kubernetes-driver"
	SparkExecutorContainerName string = "spark-kubernetes-executor"
)

// Runtime classes
const (
	GPURuntimeClassName     string = "nvidia"
	MTLSGPURuntimeClassName string = "nvidia-runc-with-hooks"
	MTLSRuntimeClassName    string = "runc-with-hooks"
)

// Pod related constants
const (
	ResourceNvidiaGPU corev1.ResourceName = "nvidia.com/gpu"
)

// Scheduler default resource SKU constants (OSS)
const (
	// DefaultCPU is the default resource SKU key for CPU-only jobs
	DefaultCPU string = "defaultCPU"
)

// secret related constants
const (
	SecretNamePrefix string = "ma-job-secret-"
)

// port annotations
const (
	DynamicPortAnnotationKeyPrefix string = "com.scheduler.port."
	DynamicPortAnnotationValue     string = "dynamic"
)

// ports
const (
	RayPort                  string = "RAY_PORT"
	RayClientPort            string = "RAY_CLIENT_PORT"
	NodeManagerPort          string = "NODE_MANAGER_PORT"
	ObjectManagerPort        string = "OBJECT_MANAGER_PORT"
	DashboardPort            string = "DASHBOARD_PORT"
	DashboardAgentGrpcPort   string = "DASHBOARD_AGENT_GRPC_PORT"
	DashboardAgentListenPort string = "DASHBOARD_AGENT_LISTEN_PORT"
	MetricsExportPort        string = "METRICS_EXPORT_PORT"
	JupyterNotebookPort      string = "JUPYTER_NOTEBOOK_PORT"
)

// RayPorts refers to all the ports required by Ray cluster. Refer
// https://docs.ray.io/en/latest/ray-core/configure.html#ports-configurations
var RayPorts = []string{
	RayPort,
	RayClientPort,
	NodeManagerPort,
	ObjectManagerPort,
	DashboardPort,
	DashboardAgentGrpcPort,
	DashboardAgentListenPort,
	MetricsExportPort,
	JupyterNotebookPort,
}

// PortsMap is a map of ports. Note that we do not use the boolean values
// in the map. This is used for find operations
// only. Keep in sync with RayPorts.
var PortsMap = map[string]bool{
	RayPort:                  true,
	RayClientPort:            true,
	NodeManagerPort:          true,
	ObjectManagerPort:        true,
	DashboardPort:            true,
	DashboardAgentGrpcPort:   true,
	DashboardAgentListenPort: true,
	MetricsExportPort:        true,
	JupyterNotebookPort:      true,
}

// ray runtime related constants
const (
	PodIP string = "MY_POD_IP"
)

// Constants related to metric name and tags
const (
	ControllerTag = "controller"

	// Client calls latency
	CreateMetricsConfigLatency string = "create_metrics_config_latency"
	CreateSecretLatency        string = "create_secret_latency"
	CreateJobLatency           string = "create_job_latency"
	DeleteJobLatency           string = "delete_job_latency"
	GetResourcePoolsLatency    string = "get_resource_pools_latency"
	GetSkuConfigMapLatency     string = "get_sku_config_map_latency"
	WatcherLatency             string = "watcher_latency"

	FailureReasonErrorCreatingJob                      string = "error_create"
	FailureReasonErrorEnqueue                          string = "error_enqueue"
	FailureReasonErrorFetchingJobName                  string = "error_fetching_job_name"
	FailureReasonErrorFetchingFederatedClientJobStatus string = "error_fetching_federated_client_job_status"
	FailureReasonErrorParsingApplicationID             string = "error_parsing_application_id"
	FailureReasonErrorGetAssignedCluster               string = "error_get_assigned_cluster"
	FailureReasonErrorGetCondition                     string = "error_get_condition"
	FailureReasonErrorReconcileJob                     string = "error_reconcile_job"
	FailureReasonErrorReconcileMetricsConfig           string = "error_reconcile_metrics_config"
	FailureReasonErrorReconcileSecret                  string = "error_reconcile_secret"
	FailureReasonErrorUpdateCondition                  string = "error_update_condition"
	FailureReasonErrorProcessJobTermination            string = "error_process_job_termination"
	FailureReasonErrorUpdateJobStatus                  string = "error_update_status"
	FailureReasonErrorFetchingProjectName              string = "error_fetching_project"
	FailureReasonErrorFetchingResourcePools            string = "error_fetching_resource_pools"
	FailureReasonErrorKillOldJob                       string = "error_kill_old_job"
	FailureReasonMaxSchedulingAttemptsReached          string = "error_max_scheduling_attempts"

	FailureReasonKey               string = "failure_reason"
	JobFailedCountMetricName       string = "failed_count"
	JobInitiatedCountMetricName    string = "reconcile_count"
	JobReconcileDurationMetricName string = "success_reconcile_duration"
	JobSuccessCountMetricName      string = "success_count"
	JobLaunchMetricName            string = "job_launch"

	RayClusterReadyLatency     string = "cluster_ready_latency"
	RayHeadReadyLatency        string = "head_ready_latency"
	RayClusterTerminateLatency string = "cluster_terminate_latency"
	SparkLaunchLatency         string = "spark_launch_latency"
	SparkAppRunningLatency     string = "app_running_latency"
	SparkAppTerminateLatency   string = "app_terminate_latency"
)

// Constants for logging
const (
	Component = "component"
	Job       = "job"
)

// Constants for job env
const (
	Production  string = "production"
	Development string = "development"
	Testing     string = "testing"
)

// SparkJobStatus is the spark job status
type SparkJobStatus string

const (
	// JobStatusPending indicates that the job is in a pending state.
	JobStatusPending SparkJobStatus = "Pending"
	// JobStatusRunning indicates that the job is in a running state.
	JobStatusRunning SparkJobStatus = "Running"
	// JobStatusSucceeded indicates that the job has succeeded.
	JobStatusSucceeded SparkJobStatus = "Succeeded"
	// JobStatusFailed indicates that the job has failed.
	JobStatusFailed SparkJobStatus = "Failed"
)

// Affinity labels
const (
	ClusterAffinityLabelKey string = "ma/affinity-cluster"
)

// Assignment reasons
const (
	AssignmentReasonClusterMatchedByAffinity string = "cluster_matched_by_affinity"
	AssignmentReasonClusterDefaultSelected   string = "cluster_default_selected"
	AssignmentReasonNoClustersFound          string = "no_clusters_found"
)

// RayCluster String to CRD State Mapping
var RayClusterStrStateToCRDStateMapping = map[string]v2pb.RayClusterState{
	"":                      v2pb.RAY_CLUSTER_STATE_UNKNOWN,
	string(rayv1.Unhealthy): v2pb.RAY_CLUSTER_STATE_UNHEALTHY,
	string(rayv1.Failed):    v2pb.RAY_CLUSTER_STATE_FAILED,
	string(rayv1.Ready):     v2pb.RAY_CLUSTER_STATE_READY,
}

// RayJobStatus captures the lifecycle states reported for Ray jobs by the Ray operator.
type RayJobStatus string

const (
	// RayJobStatusInvalid indicates that the status is not set or invalid.
	RayJobStatusInvalid RayJobStatus = "INVALID"
	// RayJobStatusPending indicates that the job has been submitted but not yet started.
	RayJobStatusPending RayJobStatus = RayJobStatus(rayv1.JobStatusPending)
	// RayJobStatusInitializing indicates that the job is initializing.
	RayJobStatusInitializing RayJobStatus = "INITIALIZING"
	// RayJobStatusRunning indicates that the job is currently executing.
	RayJobStatusRunning RayJobStatus = RayJobStatus(rayv1.JobStatusRunning)
	// RayJobStatusSucceeded indicates that the job has completed successfully.
	RayJobStatusSucceeded RayJobStatus = RayJobStatus(rayv1.JobStatusSucceeded)
	// RayJobStatusFailed indicates that the job has failed.
	RayJobStatusFailed RayJobStatus = RayJobStatus(rayv1.JobStatusFailed)
	// RayJobStatusStopped indicates that the job was intentionally stopped.
	RayJobStatusStopped RayJobStatus = RayJobStatus(rayv1.JobStatusStopped)
	// RayJobStatusUnknown indicates that the status could not be determined.
	RayJobStatusUnknown RayJobStatus = "UNKNOWN"
)

var RayJobStatusSet = map[RayJobStatus]struct{}{
	RayJobStatusInvalid:      {},
	RayJobStatusPending:      {},
	RayJobStatusInitializing: {},
	RayJobStatusRunning:      {},
	RayJobStatusSucceeded:    {},
	RayJobStatusFailed:       {},
	RayJobStatusStopped:      {},
	RayJobStatusUnknown:      {},
}
