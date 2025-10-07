package types

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSchedulableRayJob(t *testing.T) {
	job := NewSchedulableJob(SchedulableJobParams{
		Name:       "test-job",
		Namespace:  "test-ns",
		Generation: 1,
		JobType:    RayJob,
	})

	require.Equal(t, "test-job", job.GetName())
	require.Equal(t, "test-ns", job.GetNamespace())
	require.Equal(t, int64(1), job.GetGeneration())
	require.Equal(t, RayJob, job.GetJobType())
	require.Equal(t, "RayJob", job.GetJobType().ToString())
}

func TestSchedulableSparkJob(t *testing.T) {
	job := NewSchedulableJob(SchedulableJobParams{
		Name:       "test-job",
		Namespace:  "test-ns",
		Generation: 1,
		JobType:    SparkJob,
	})

	require.Equal(t, "test-job", job.GetName())
	require.Equal(t, "test-ns", job.GetNamespace())
	require.Equal(t, int64(1), job.GetGeneration())
	require.Equal(t, SparkJob, job.GetJobType())
	require.Equal(t, "SparkJob", job.GetJobType().ToString())
}
