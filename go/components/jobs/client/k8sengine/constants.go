package k8sengine

// exported constants
const (
	// RayLocalNamespace is the single namespace used for all Ray jobs.
	RayLocalNamespace = "default"

	// RayAPIVersion is the API version of the RayJob CRD
	RayAPIVersion = "ray.io/v1"

	// RayClusterKind is the kind of the RayCluster CRD
	RayClusterKind = "RayCluster"

	// RayJobKind is the kind of the RayJob CRD
	RayJobKind = "RayJob"

	// RayWorkerNodePrefix is the pod name prefix for Ray Worker nodes
	RayWorkerNodePrefix = "worker-"

	// SparkLocalNamespace is the single namespace used for all Spark jobs
	// projected into a member cluster. It is deliberately the same value as
	// RayLocalNamespace; see the compile-time assertion in
	// go/components/jobs/scheduler/kueue/kueue.go.
	SparkLocalNamespace = "default"

	// SparkAPIVersion is the API version of the SparkApplication CRD
	SparkAPIVersion = "sparkoperator.k8s.io/v1beta2"

	// SparkApplicationKind is the kind of the SparkApplication CRD
	SparkApplicationKind = "SparkApplication"

	// SparkApplicationTTLSeconds is how long a finished SparkApplication stays
	// in the member cluster before the spark-operator garbage-collects it;
	// nothing else reaps an application that ends on its own. The watcher and
	// the SparkJob controller's periodic re-check recover a terminal update
	// they missed only while the application still exists: once the TTL has
	// reaped it, the job is recorded as deleted (Succeeded=FALSE, reason
	// SparkApplicationDeleted) rather than with its real outcome. So the TTL
	// has to outlast a controllermgr outage or a member-cluster NotReady
	// window. A day also keeps driver logs inspectable.
	SparkApplicationTTLSeconds int64 = 86400
)
