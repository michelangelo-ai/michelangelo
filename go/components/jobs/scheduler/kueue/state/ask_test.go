package state

import (
	"errors"
	"testing"

	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	maconfig "github.com/michelangelo-ai/michelangelo/go/base/config"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/client/k8sengine"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/types"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/types/typesmocks"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
)

// resources returns the resource list of name/quantity pairs.
func resources(pairs ...string) corev1.ResourceList {
	list := corev1.ResourceList{}
	for i := 0; i < len(pairs); i += 2 {
		list[corev1.ResourceName(pairs[i])] = resource.MustParse(pairs[i+1])
	}
	return list
}

func container(requests, limits corev1.ResourceList) corev1.Container {
	return corev1.Container{Resources: corev1.ResourceRequirements{Requests: requests, Limits: limits}}
}

func sidecar(requests corev1.ResourceList) corev1.Container {
	c := container(requests, nil)
	c.RestartPolicy = ptr.To(corev1.ContainerRestartPolicyAlways)
	return c
}

// requireAsk checks that got holds exactly the quantities of want.
func requireAsk(t *testing.T, want, got corev1.ResourceList) {
	t.Helper()
	wantStrings := map[corev1.ResourceName]string{}
	for name, q := range want {
		wantStrings[name] = q.String()
	}
	requireResources(t, wantStrings, got)
}

func TestPodRequests(t *testing.T) {
	tests := []struct {
		name string
		spec corev1.PodSpec
		want corev1.ResourceList
	}{
		{
			name: "no containers",
			spec: corev1.PodSpec{},
			want: resources(),
		},
		{
			name: "regular containers sum",
			spec: corev1.PodSpec{Containers: []corev1.Container{
				container(resources("cpu", "1", "memory", "1Gi"), nil),
				container(resources("cpu", "500m", "memory", "512Mi"), nil),
			}},
			want: resources("cpu", "1500m", "memory", "1536Mi"),
		},
		{
			name: "missing requests default to limits",
			spec: corev1.PodSpec{Containers: []corev1.Container{
				container(resources("cpu", "1"), resources("cpu", "2", "memory", "4Gi", "nvidia.com/gpu", "1")),
			}},
			want: resources("cpu", "1", "memory", "4Gi", "nvidia.com/gpu", "1"),
		},
		{
			name: "an init container larger than the regular ones sets the peak",
			spec: corev1.PodSpec{
				InitContainers: []corev1.Container{container(resources("cpu", "4", "memory", "512Mi"), nil)},
				Containers:     []corev1.Container{container(resources("cpu", "1", "memory", "1Gi"), nil)},
			},
			want: resources("cpu", "4", "memory", "1Gi"),
		},
		{
			name: "init containers run one at a time",
			spec: corev1.PodSpec{
				InitContainers: []corev1.Container{
					container(resources("cpu", "3"), nil),
					container(resources("cpu", "2"), nil),
				},
				Containers: []corev1.Container{container(resources("cpu", "1"), nil)},
			},
			want: resources("cpu", "3"),
		},
		{
			name: "an init container smaller than the regular ones",
			spec: corev1.PodSpec{
				InitContainers: []corev1.Container{container(resources("cpu", "1"), nil)},
				Containers:     []corev1.Container{container(resources("cpu", "2"), nil)},
			},
			want: resources("cpu", "2"),
		},
		{
			name: "a sidecar runs alongside the regular containers",
			spec: corev1.PodSpec{
				InitContainers: []corev1.Container{sidecar(resources("cpu", "500m", "memory", "256Mi"))},
				Containers:     []corev1.Container{container(resources("cpu", "1", "memory", "1Gi"), nil)},
			},
			want: resources("cpu", "1500m", "memory", "1280Mi"),
		},
		{
			name: "an init container after a sidecar runs alongside it",
			spec: corev1.PodSpec{
				InitContainers: []corev1.Container{
					sidecar(resources("cpu", "1")),
					container(resources("cpu", "3"), nil),
				},
				Containers: []corev1.Container{container(resources("cpu", "1"), nil)},
			},
			want: resources("cpu", "4"),
		},
		{
			name: "an init container before a sidecar does not",
			spec: corev1.PodSpec{
				InitContainers: []corev1.Container{
					container(resources("cpu", "3"), nil),
					sidecar(resources("cpu", "1")),
				},
				Containers: []corev1.Container{container(resources("cpu", "1"), nil)},
			},
			want: resources("cpu", "3"),
		},
		{
			name: "a sidecar's missing requests default to its limits",
			spec: corev1.PodSpec{
				InitContainers: []corev1.Container{func() corev1.Container {
					c := sidecar(nil)
					c.Resources.Limits = resources("memory", "1Gi")
					return c
				}()},
				Containers: []corev1.Container{container(resources("memory", "1Gi"), nil)},
			},
			want: resources("memory", "2Gi"),
		},
		{
			name: "overhead is added",
			spec: corev1.PodSpec{
				Containers: []corev1.Container{container(resources("cpu", "1", "memory", "1Gi"), nil)},
				Overhead:   resources("cpu", "250m", "memory", "120Mi"),
			},
			want: resources("cpu", "1250m", "memory", "1144Mi"),
		},
		{
			name: "overhead is added to an init peak",
			spec: corev1.PodSpec{
				InitContainers: []corev1.Container{container(resources("cpu", "4"), nil)},
				Containers:     []corev1.Container{container(resources("cpu", "1"), nil)},
				Overhead:       resources("cpu", "250m"),
			},
			want: resources("cpu", "4250m"),
		},
		{
			name: "ephemeral storage is left out",
			spec: corev1.PodSpec{
				InitContainers: []corev1.Container{container(resources("ephemeral-storage", "20Gi"), nil)},
				Containers: []corev1.Container{
					container(resources("cpu", "1", "ephemeral-storage", "10Gi"), resources("ephemeral-storage", "10Gi")),
				},
				Overhead: resources("ephemeral-storage", "1Gi"),
			},
			want: resources("cpu", "1"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := tt.spec.DeepCopy()
			requireAsk(t, tt.want, PodRequests(spec))
			assert.Equal(t, tt.spec, *spec, "the pod spec was modified")
		})
	}
}

func podTemplate(containers ...corev1.Container) corev1.PodTemplateSpec {
	return corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: containers}}
}

func TestRayClusterAsk(t *testing.T) {
	head := podTemplate(container(resources("cpu", "2", "memory", "8Gi"), nil))
	tests := []struct {
		name    string
		workers []rayv1.WorkerGroupSpec
		want    corev1.ResourceList
	}{
		{
			name: "head only",
			want: resources("cpu", "2", "memory", "8Gi"),
		},
		{
			name: "worker groups times their replicas",
			workers: []rayv1.WorkerGroupSpec{
				{
					Replicas: ptr.To[int32](3),
					Template: podTemplate(container(resources("cpu", "4", "memory", "16Gi"), resources("nvidia.com/gpu", "1"))),
				},
				{
					Replicas: ptr.To[int32](2),
					Template: podTemplate(container(resources("cpu", "1", "memory", "2Gi"), nil)),
				},
			},
			want: resources("cpu", "16", "memory", "60Gi", "nvidia.com/gpu", "3"),
		},
		{
			name: "multi-host groups times their hosts",
			workers: []rayv1.WorkerGroupSpec{{
				Replicas:   ptr.To[int32](2),
				NumOfHosts: 4,
				Template:   podTemplate(container(resources("cpu", "1"), nil)),
			}},
			want: resources("cpu", "10", "memory", "8Gi"),
		},
		{
			name: "unset replicas are none",
			workers: []rayv1.WorkerGroupSpec{{
				Template: podTemplate(container(resources("cpu", "4"), nil)),
			}},
			want: resources("cpu", "2", "memory", "8Gi"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rc := &rayv1.RayCluster{Spec: rayv1.RayClusterSpec{
				HeadGroupSpec:    rayv1.HeadGroupSpec{Template: head},
				WorkerGroupSpecs: tt.workers,
			}}
			requireAsk(t, tt.want, RayClusterAsk(rc))
		})
	}
}

// _kueueCluster is a Kueue-managed cluster.
var _kueueCluster = &v2pb.Cluster{
	ObjectMeta: metav1.ObjectMeta{Name: "cluster-1"},
	Spec:       v2pb.ClusterSpec{SchedulerType: v2pb.SCHEDULER_TYPE_KUEUE},
}

// newMapper returns the k8sengine mapper of the Kueue backend, with log
// persistence, and so the collector sidecar, on or off.
func newMapper(logPersistence bool) types.Mapper {
	return k8sengine.NewMapper(
		k8sengine.LogPersistenceConfig{Enabled: logPersistence, CollectorImage: "kuberay/collector:v1.7.1"},
		maconfig.SchedulerConfig{Backend: kueueBackend},
	).Mapper
}

// newJob returns a RayCluster of team-a with a 2-CPU head and the given
// number of 4-CPU, 1-GPU workers.
func newJob(name string, workers int32) *v2pb.RayCluster {
	head := podTemplate(container(resources("cpu", "2", "memory", "8Gi"), nil))
	worker := podTemplate(container(resources("cpu", "4", "memory", "16Gi"), resources("nvidia.com/gpu", "1")))
	return &v2pb.RayCluster{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "team-a"},
		Spec: v2pb.RayClusterSpec{
			Head:    &v2pb.RayHeadSpec{Pod: &head},
			Workers: []*v2pb.RayWorkerSpec{{Pod: &worker, MinInstances: workers, MaxInstances: workers}},
		},
	}
}

func TestAsk(t *testing.T) {
	tests := []struct {
		name           string
		logPersistence bool
		want           corev1.ResourceList
	}{
		{
			name: "the head and the workers",
			want: resources("cpu", "10", "memory", "40Gi", "nvidia.com/gpu", "2"),
		},
		{
			// the collector sidecar asks for 100m CPU and 128Mi of memory in
			// each pod
			name:           "with the collector",
			logPersistence: true,
			want:           resources("cpu", "10300m", "memory", "41344Mi", "nvidia.com/gpu", "2"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr := newTracker(nil, newMapper(tt.logPersistence), logr.Discard())
			job := newJob("job-1", 2)
			original := job.DeepCopy()
			for i := 0; i < 2; i++ {
				ask, err := tr.ask(job, _kueueCluster)
				require.NoError(t, err)
				requireAsk(t, tt.want, ask)
			}
			assert.Equal(t, original, job, "the job was modified")
		})
	}
}

func TestAskErrors(t *testing.T) {
	tests := []struct {
		name    string
		mapped  runtime.Object
		err     error
		wantErr string
	}{
		{
			name:    "the job cannot be mapped",
			err:     errors.New("no project identity"),
			wantErr: "no project identity",
		},
		{
			name:    "the job maps to something else",
			mapped:  &v2pb.RayCluster{},
			wantErr: "RayCluster team-a/job-1 mapped to *v2.RayCluster, not a KubeRay RayCluster",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapper := typesmocks.NewMockMapper(gomock.NewController(t))
			mapper.EXPECT().MapGlobalJobClusterToLocal(gomock.Any(), _kueueCluster).Return(tt.mapped, tt.err)
			_, err := newTracker(nil, mapper, logr.Discard()).ask(newJob("job-1", 2), _kueueCluster)
			require.ErrorContains(t, err, tt.wantErr)
		})
	}

	t.Run("a job of no project on a Kueue-managed cluster", func(t *testing.T) {
		job := newJob("job-1", 2)
		job.Namespace = ""
		_, err := newTracker(nil, newMapper(false), logr.Discard()).ask(job, _kueueCluster)
		require.ErrorContains(t, err, "job has no project identity")
	})
}
