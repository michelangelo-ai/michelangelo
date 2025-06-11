package constants

import (
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

// Labels
const (
	GenericSpireIdentityLabelKey   string = "com.uber.spiffe"
	GenericSpireIdentityLabelValue string = "michelangelo.ray.workload"
	JobNameLabelKey                string = "ma/job-name"
	ProjectNameLabelKey            string = "ma/project-name"
	JobControlPlaneEnvKey          string = "ma/control-plane-env"
	RayClusterNameLabelKey         string = "ray.io/cluster"
	RayNodeTypeLabelKey            string = "ray.io/node-type"
	RayNodeLabelKey                string = "ray.io/is-ray-node"
	SecretAppNameKey               string = "app"
	SecretAppNameValue             string = "michelangelo-controllermgr"
	SecureServiceMeshKey           string = "com.uber.secure_service_mesh"
	SecureServiceMeshMTLSValue     string = "lts"
	UOwnLabelKey                   string = "ma/project-uown"
	UserLabelKey                   string = "ma/user"
	OwnerServiceLabelKey           string = "ma/owner-service"
	MAOwnerServiceLabelValue       string = "michelangelo-ray" // This value is set to the name of this uOwn https://uown.uberinternal.com/assets/8f68c1e1-13ca-4117-b5da-c0d6894a340e
	MAOwnerSparkLabelValue         string = "michelangelo-spark"
)

// Annotations
const (
	SpiffeAnnotationKey          string = "com.uber.spiffe"
	GenericSpiffeAnnotationValue string = "michelangelo/ray/workload"
	UserUIDAnnotationKey         string = "com.uber.identity.uid"
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

// secret related constants
const (
	SecretHadoopNamePrefix string = "ma-hadoop-"
	SecretHadoopMountPath  string = "/etc/ma_hadoop"
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

// init container related constants
const (
	InitContainerImage string = "127.0.0.1:5055/uber-usi/michelangelo-ray-init:bkt1-produ-1676577210-e0176" // TODO: move to flipr
)

// idle detection sidecar container related constants
const (
	IdleDetectionImage string = "127.0.0.1:5055/uber-usi/michelangelo-ray-idle-detection:bkt1-produ-1748621658-3cf3f"
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
	GetUOwnAssetLatency        string = "get_u_own_asset_latency"
	WatcherLatency             string = "watcher_latency"

	FailureReasonErrorCreatingJob                      string = "error_create"
	FailureReasonErrorEnqueue                          string = "error_enqueue"
	FailureReasonErrorFetchingJobName                  string = "error_fetching_job_name"
	FailureReasonErrorFetchingFederatedClientJobStatus string = "error_fetching_federated_client_job_status"
	FailureReasonErrorFetchingDrogonJobStatus          string = "error_fetching_drogon_job_status"
	FailureReasonErrorFetchingHDFSDelegationToken      string = "error_fetching_hdfs_delegation_token"
	FailureReasonErrorFetchingDrogonClusterName        string = "error_fetching_drogon_cluster_name"
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

// Constants for resource pool labels
const (
	ResourcePoolEnvProd              string = "resourcepool.michelangelo/support-env-prod"
	ResourcePoolEnvDev               string = "resourcepool.michelangelo/support-env-dev"
	ResourcePoolEnvTest              string = "resourcepool.michelangelo/support-env-test"
	ResourcePoolSpecialResourceAlias string = "compute.uber.com/resourcepool-special-resource-alias"
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
	//JobStatusPending indicates that the job is in a pending state.
	JobStatusPending SparkJobStatus = "Pending"
	//JobStatusRunning indicates that the job is in a running state.
	JobStatusRunning SparkJobStatus = "Running"
	//JobStatusSucceeded indicates that the job has succeeded.
	JobStatusSucceeded SparkJobStatus = "Succeeded"
	//JobStatusFailed indicates that the job has failed.
	JobStatusFailed SparkJobStatus = "Failed"
)
