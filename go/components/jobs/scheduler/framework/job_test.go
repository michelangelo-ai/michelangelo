package framework

import (
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// sparkJobWith returns a BatchSparkJob whose driver and executors ask for the
// given resources.
func sparkJobWith(driver, executor *v2pb.ResourceSpec, instances int32) BatchSparkJob {
	return BatchSparkJob{
		SparkJob: &v2pb.SparkJob{
			Spec: v2pb.SparkJobSpec{
				Driver: &v2pb.DriverSpec{
					Pod: &v2pb.PodSpec{Resource: driver},
				},
				Executor: &v2pb.ExecutorSpec{
					Instances: instances,
					Pod:       &v2pb.PodSpec{Resource: executor},
				},
			},
		},
	}
}

// TestBatchSparkJobGetResourceRequirement covers the nil embedded pointer guard,
// mirroring BatchRayCluster.GetResourceRequirement, the driver + executors
// aggregation of CPU, memory, GPU and ephemeral storage, and the error path.
//
// Memory and disk sizes are Kubernetes quantities because that is what
// ConvertToResourceList parses. The rows pin the arithmetic (driver plus
// instances times executor), not which memory syntax a SparkJob may use, and
// they set ephemeral storage explicitly wherever they assert it, so they do not
// depend on the default ConvertToResourceList applies when a pod sets none.
func TestBatchSparkJobGetResourceRequirement(t *testing.T) {
	tests := []struct {
		name      string
		job       BatchSparkJob
		wantEmpty bool
		wantErr   bool
		// want is the quantity expected in the aggregated requirement, by
		// resource. A resource a row leaves out is not checked.
		want map[v1.ResourceName]string
	}{
		{
			name:      "nil embedded SparkJob pointer returns an empty map and no error",
			job:       BatchSparkJob{},
			wantEmpty: true,
		},
		{
			name: "driver plus scaled executors are aggregated under a single key",
			job: sparkJobWith(
				&v2pb.ResourceSpec{Cpu: 2, Memory: "4G"},
				&v2pb.ResourceSpec{Cpu: 1, Memory: "2G"},
				3,
			),
			want: map[v1.ResourceName]string{v1.ResourceCPU: "5"},
		},
		{
			name: "memory, GPU and ephemeral storage are each driver plus instances times executor",
			job: sparkJobWith(
				&v2pb.ResourceSpec{Cpu: 2, Gpu: 1, Memory: "4Gi", DiskSize: "10Gi"},
				&v2pb.ResourceSpec{Cpu: 1, Gpu: 2, Memory: "2Gi", DiskSize: "20Gi"},
				3,
			),
			want: map[v1.ResourceName]string{
				v1.ResourceCPU:              "5",
				v1.ResourceMemory:           "10Gi",
				constants.ResourceNvidiaGPU: "7",
				v1.ResourceEphemeralStorage: "70Gi",
			},
		},
		{
			name: "an invalid driver quantity is an error",
			job: sparkJobWith(
				&v2pb.ResourceSpec{Cpu: 2, Memory: "four gigabytes"},
				&v2pb.ResourceSpec{Cpu: 1, Memory: "2Gi"},
				3,
			),
			wantErr: true,
		},
		{
			name: "an invalid executor quantity is an error",
			job: sparkJobWith(
				&v2pb.ResourceSpec{Cpu: 2, Memory: "4Gi"},
				&v2pb.ResourceSpec{Cpu: 1, DiskSize: "lots"},
				3,
			),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.job.GetResourceRequirement()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("GetResourceRequirement() = %v, want an error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("GetResourceRequirement() returned error: %v", err)
			}
			if got == nil {
				t.Fatalf("GetResourceRequirement() returned a nil map, want non-nil")
			}

			if tt.wantEmpty {
				if len(got) != 0 {
					t.Fatalf("GetResourceRequirement() = %v, want an empty map", got)
				}
				return
			}

			// Spark jobs are homogeneous, so everything lands under a single key.
			if len(got) != 1 {
				t.Fatalf("GetResourceRequirement() returned %d keys (%v), want exactly 1", len(got), got)
			}
			total, ok := got[constants.DefaultCPU]
			if !ok {
				t.Fatalf("GetResourceRequirement() = %v, want the %q key", got, constants.DefaultCPU)
			}
			for name, want := range tt.want {
				gotQuantity := total[name]
				wantQuantity := resource.MustParse(want)
				if gotQuantity.Cmp(wantQuantity) != 0 {
					t.Fatalf("aggregated %s = %s, want %s", name, gotQuantity.String(), want)
				}
			}
		})
	}
}
