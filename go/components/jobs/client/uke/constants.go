package uke

// exported constants
const (
	// RayHeadNodeEnv is the env variable key in the init container
	// for ray cluster's head pod's name.
	RayHeadNodeEnv = "RAY_HEAD_NODE"
	// RayLocalNamespace is the single namespace used for all Ray jobs.
	RayLocalNamespace = "ma-ray"
	// SparkLocalNamespace is the single namespace used for all Spark jobs. Spark operator currently only
	// supports one namespace https://t3.uberinternal.com/browse/SPARKUS-647.
	// To make this work, we change the namespace of all Spark jobs to this namespace. Additional
	// context at https://t3.uberinternal.com/browse/SPARKUS-667
	SparkLocalNamespace = "spark-operator"
	// Pod name prefix for Ray Head node
	RayHeadNodePrefix = "head-"
	// Pod name prefix for Ray Worker nodes
	RayWorkerNodePrefix = "worker-"
	// PtraceEnabledAnnotation is the annotation key to enable ptrace capability.
	// Used for profiling Ray jobs with PySpy.
	PtraceEnabledAnnotation = "michelangelo/profiling-ptrace-enabled"
)
