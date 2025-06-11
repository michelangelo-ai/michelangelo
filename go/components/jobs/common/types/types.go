package types

import (
	"errors"

	"code.uber.internal/rt/flipr-client-go.git/flipr"
	"go.uber.org/fx"
	"golang.org/x/exp/maps"
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
	// PelotonCluster is the cluster type for Peloton clusters
	PelotonCluster ClusterType = "Peloton"
	// KubernetesCluster is the cluster type for Kubernetes clusters
	KubernetesCluster ClusterType = "Kubernetes"
)

// Module provides FliprConstraintsBuilder
var Module = fx.Provide(NewFliprConstraintsBuilder)

// FliprConstraintsBuilder interface is written mainly to test the constraint values supplied to flipr.
// Using this interface allows us to intercept and test the flipr constraints in gomock tests.
// This is otherwise not possible because the contraints argument is a function type. Therefore
// we cannot validate the constraint values supplied to flipr.
type FliprConstraintsBuilder interface {
	GetFliprConstraints(map[string]interface{}) flipr.Constraints
}

type fliprConstraintsBuilderImpl struct{}

var _ FliprConstraintsBuilder = fliprConstraintsBuilderImpl{}

// GetFliprConstraints returns a flipr.Constraints object with the given constraints
func (f fliprConstraintsBuilderImpl) GetFliprConstraints(constraints map[string]interface{}) flipr.Constraints {
	return flipr.Constraints{
		func(m map[string]interface{}) {
			maps.Copy(m, constraints)
		},
	}
}

// NewFliprConstraintsBuilder returns a new instance of FliprConstraintsBuilder
func NewFliprConstraintsBuilder() FliprConstraintsBuilder {
	return fliprConstraintsBuilderImpl{}
}

// MTLSHandler interface defines the interface for MTLS handling operations
type MTLSHandler interface {
	EnableMTLS(projectName string) (bool, error)
	EnableMTLSRuntimeClass(projectName string) (bool, error)
}
