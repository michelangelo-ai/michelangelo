package ray

import (
	corev1 "k8s.io/api/core/v1"
)

// HeadGroupSpec represents the specification for the head group of a Ray cluster.
type HeadGroupSpec struct {
	Resources    corev1.ResourceList `json:"resources,omitempty"`
	Annotations  map[string]string   `json:"annotations,omitempty"`
	Env          []EnvVar            `json:"env,omitempty"`
	Architecture string              `json:"architecture,omitempty"`
}

// WorkerGroupSpec represents the specification for the worker group of a Ray cluster.
type WorkerGroupSpec struct {
	Replicas     int                 `json:"replicas,omitempty"`
	MinReplicas  int                 `json:"minReplicas,omitempty"`
	MaxReplicas  int                 `json:"maxReplicas,omitempty"`
	Resources    corev1.ResourceList `json:"resources,omitempty"`
	GroupName    string              `json:"groupName,omitempty"`
	Annotations  map[string]string   `json:"annotations,omitempty"`
	Env          []EnvVar            `json:"env,omitempty"`
	Architecture string              `json:"architecture,omitempty"`
}

// RayClusterSpec represents the specification for a Ray cluster.
type RayClusterSpec struct {
	RayVersion       string            `json:"rayVersion,omitempty"`
	HeadGroupSpec    HeadGroupSpec     `json:"headGroupSpec,omitempty"`
	WorkerGroupSpecs []WorkerGroupSpec `json:"workerGroupSpecs,omitempty"`
	Image            string            `json:"image,omitempty"`
}

// EnvVar represents an environment variable.
type EnvVar struct {
	Name  string `json:"name" protobuf:"bytes,1,opt,name=name"`
	Value string `json:"value,omitempty" protobuf:"bytes,2,opt,name=value"`
}
