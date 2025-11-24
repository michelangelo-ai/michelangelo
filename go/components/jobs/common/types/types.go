package types

import (
	"errors"
)

// ErrJobAlreadyExists indicates that the job is already present in the scheduler queue
var ErrJobAlreadyExists = errors.New("job already exists")

// SchedulableJob is a job that can be scheduled
type SchedulableJob interface {
	// GetName returns the name of the job.
	GetName() string
	// GetNamespace returns the namespace of the job.
	GetNamespace() string
	// GetGeneration returns the job generation
	GetGeneration() int64
	// GetJobType return the type of the job
	GetJobType() JobType
}

// JobType is the type of the job
type JobType int

// ToString returns a string representation of known job types
func (j JobType) ToString() string {
	switch j {
	case SparkJob:
		return "SparkJob"
	case RayJob:
		return "RayJob"
	default:
		return ""
	}
}

const (
	// SparkJob is the job type for Spark jobs
	SparkJob JobType = iota + 1
	// RayJob is the job type for Ray jobs
	RayJob
	// RayCluster is the job type for Ray clusters
	RayCluster
)

// SchedulableJobParams is the param to NewScheduledJob
type SchedulableJobParams struct {
	Name       string
	Namespace  string
	Generation int64
	JobType    JobType
}

// NewSchedulableJob return a new scheduled job
func NewSchedulableJob(p SchedulableJobParams) SchedulableJob {
	return schedulableQueueJob{
		name:       p.Name,
		namespace:  p.Namespace,
		generation: p.Generation,
		jobType:    p.JobType,
	}
}

type schedulableQueueJob struct {
	name       string
	namespace  string
	generation int64
	jobType    JobType
}

func (s schedulableQueueJob) GetName() string {
	return s.name
}

func (s schedulableQueueJob) GetNamespace() string {
	return s.namespace
}

func (s schedulableQueueJob) GetGeneration() int64 {
	return s.generation
}

func (s schedulableQueueJob) GetJobType() JobType {
	return s.jobType
}

// ClusterType is the type of the cluster
type ClusterType string

const (
	// KubernetesCluster is the cluster type for Kubernetes clusters
	KubernetesCluster ClusterType = "Kubernetes"
)

// MTLSHandler interface defines the interface for MTLS handling operations
type MTLSHandler interface {
	EnableMTLS(projectName string) (bool, error)
	EnableMTLSRuntimeClass(projectName string) (bool, error)
}
