package k8sengine

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sptr "k8s.io/utils/ptr"

	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

const testCredentialsSecret = "minio-credentials"

// credentialsSource is the envFrom entry attached for testCredentialsSecret.
var credentialsSource = corev1.EnvFromSource{
	SecretRef: &corev1.SecretEnvSource{
		LocalObjectReference: corev1.LocalObjectReference{Name: testCredentialsSecret},
		Optional:             k8sptr.To(true),
	},
}

// configMapSource is the envFrom entry Uniflow authors on every Ray container.
var configMapSource = corev1.EnvFromSource{
	ConfigMapRef: &corev1.ConfigMapEnvSource{
		LocalObjectReference: corev1.LocalObjectReference{Name: "michelangelo-config"},
	},
}

// userCredentialsSource lists the credentials Secret the way a caller would,
// unprefixed and required.
var userCredentialsSource = corev1.EnvFromSource{
	SecretRef: &corev1.SecretEnvSource{
		LocalObjectReference: corev1.LocalObjectReference{Name: testCredentialsSecret},
	},
}

// prefixedCredentialsSource loads the same Secret under other variable names.
var prefixedCredentialsSource = corev1.EnvFromSource{
	Prefix: "LOGS_",
	SecretRef: &corev1.SecretEnvSource{
		LocalObjectReference: corev1.LocalObjectReference{Name: testCredentialsSecret},
	},
}

// credentialsTestCluster returns a v2 RayCluster whose head and single worker
// group each run their own copy of containers, next to an init container that
// lists the ConfigMap.
func credentialsTestCluster(containers []corev1.Container) *v2pb.RayCluster {
	pod := func() *corev1.PodTemplateSpec {
		template := &corev1.PodTemplateSpec{Spec: corev1.PodSpec{
			InitContainers: []corev1.Container{{Name: "init", EnvFrom: []corev1.EnvFromSource{configMapSource}}},
			Containers:     containers,
		}}
		return template.DeepCopy()
	}
	return &v2pb.RayCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
		Spec: v2pb.RayClusterSpec{
			Head:    &v2pb.RayHeadSpec{ServiceType: string(corev1.ServiceTypeClusterIP), Pod: pod()},
			Workers: []*v2pb.RayWorkerSpec{{MinInstances: 1, MaxInstances: 2, Pod: pod()}},
		},
	}
}

func TestWithCredentialsSecret(t *testing.T) {
	tests := []struct {
		name     string
		envFrom  []corev1.EnvFromSource
		secret   string
		expected []corev1.EnvFromSource
	}{
		{
			name:     "no sources -> secret only",
			secret:   testCredentialsSecret,
			expected: []corev1.EnvFromSource{credentialsSource},
		},
		{
			name:     "existing sources -> secret ahead of them",
			envFrom:  []corev1.EnvFromSource{configMapSource},
			secret:   testCredentialsSecret,
			expected: []corev1.EnvFromSource{credentialsSource, configMapSource},
		},
		{
			name:     "empty secret -> unchanged",
			envFrom:  []corev1.EnvFromSource{configMapSource},
			expected: []corev1.EnvFromSource{configMapSource},
		},
		{
			name:     "empty secret and no sources -> still none",
			expected: nil,
		},
		{
			name:     "secret already listed unprefixed -> unchanged",
			envFrom:  []corev1.EnvFromSource{configMapSource, userCredentialsSource},
			secret:   testCredentialsSecret,
			expected: []corev1.EnvFromSource{configMapSource, userCredentialsSource},
		},
		{
			name:     "secret listed only with a prefix -> unprefixed one prepended",
			envFrom:  []corev1.EnvFromSource{prefixedCredentialsSource},
			secret:   testCredentialsSecret,
			expected: []corev1.EnvFromSource{credentialsSource, prefixedCredentialsSource},
		},
		{
			name: "config map of the same name -> secret prepended",
			envFrom: []corev1.EnvFromSource{{ConfigMapRef: &corev1.ConfigMapEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: testCredentialsSecret},
			}}},
			secret: testCredentialsSecret,
			expected: []corev1.EnvFromSource{credentialsSource, {ConfigMapRef: &corev1.ConfigMapEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: testCredentialsSecret},
			}}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, WithCredentialsSecret(tt.envFrom, tt.secret))
		})
	}
}

// Callers pass envFrom slices that alias their source objects, so the result
// must be built in a new backing array, even when the input has spare room.
func TestWithCredentialsSecret_DoesNotWriteToInput(t *testing.T) {
	envFrom := make([]corev1.EnvFromSource, 1, 2)
	envFrom[0] = configMapSource

	got := WithCredentialsSecret(envFrom, testCredentialsSecret)

	assert.Equal(t, []corev1.EnvFromSource{credentialsSource, configMapSource}, got)
	assert.Equal(t, []corev1.EnvFromSource{configMapSource, {}}, envFrom[:2])
}

func TestMapper_MapGlobalJobClusterToLocal_CredentialsSecret(t *testing.T) {
	collectorEnabled := LogPersistenceConfig{
		Enabled:           true,
		StorageEndpoint:   "minio:9091",
		Bucket:            "ray-history",
		CredentialsSecret: testCredentialsSecret,
		CollectorImage:    "quay.io/kuberay/collector:v1.7.1",
	}

	tests := []struct {
		name           string
		logPersistence LogPersistenceConfig
		containers     []corev1.Container
		// expected is each Ray container's envFrom, in container order, for the
		// head and the worker group alike.
		expected [][]corev1.EnvFromSource
	}{
		{
			name:           "log persistence disabled -> secret ahead of the config map",
			logPersistence: LogPersistenceConfig{CredentialsSecret: testCredentialsSecret},
			containers:     []corev1.Container{{Name: "ray", EnvFrom: []corev1.EnvFromSource{configMapSource}}},
			expected:       [][]corev1.EnvFromSource{{credentialsSource, configMapSource}},
		},
		{
			name:           "log persistence enabled -> secret ahead of the config map, collector left alone",
			logPersistence: collectorEnabled,
			containers:     []corev1.Container{{Name: "ray", EnvFrom: []corev1.EnvFromSource{configMapSource}}},
			expected:       [][]corev1.EnvFromSource{{credentialsSource, configMapSource}},
		},
		{
			name:       "empty secret -> envFrom unchanged",
			containers: []corev1.Container{{Name: "ray", EnvFrom: []corev1.EnvFromSource{configMapSource}}},
			expected:   [][]corev1.EnvFromSource{{configMapSource}},
		},
		{
			name:           "secret already listed unprefixed -> not duplicated",
			logPersistence: LogPersistenceConfig{CredentialsSecret: testCredentialsSecret},
			containers: []corev1.Container{
				{Name: "ray", EnvFrom: []corev1.EnvFromSource{configMapSource, userCredentialsSource}},
			},
			expected: [][]corev1.EnvFromSource{{configMapSource, userCredentialsSource}},
		},
		{
			name:           "secret listed only with a prefix -> unprefixed one prepended",
			logPersistence: LogPersistenceConfig{CredentialsSecret: testCredentialsSecret},
			containers: []corev1.Container{
				{Name: "ray", EnvFrom: []corev1.EnvFromSource{prefixedCredentialsSource, configMapSource}},
			},
			expected: [][]corev1.EnvFromSource{{credentialsSource, prefixedCredentialsSource, configMapSource}},
		},
		{
			name:           "multiple containers -> each one gets the secret",
			logPersistence: collectorEnabled,
			containers: []corev1.Container{
				{Name: "ray", EnvFrom: []corev1.EnvFromSource{configMapSource}},
				{Name: "proxy"},
			},
			expected: [][]corev1.EnvFromSource{{credentialsSource, configMapSource}, {credentialsSource}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := Mapper{LogPersistence: tt.logPersistence}
			obj, err := m.MapGlobalJobClusterToLocal(credentialsTestCluster(tt.containers), nil)
			require.NoError(t, err)
			require.IsType(t, &rayv1.RayCluster{}, obj)
			rc := obj.(*rayv1.RayCluster)
			require.Len(t, rc.Spec.WorkerGroupSpecs, 1)

			for _, group := range []struct {
				role     string
				template corev1.PodTemplateSpec
			}{
				{role: "head", template: rc.Spec.HeadGroupSpec.Template},
				{role: "worker", template: rc.Spec.WorkerGroupSpecs[0].Template},
			} {
				containers := group.template.Spec.Containers
				if tt.logPersistence.Enabled {
					// The collector is appended after the Ray containers and
					// reads the Secret only through its required secretKeyRefs.
					require.Len(t, containers, len(tt.expected)+1, group.role)
					collector := containers[len(containers)-1]
					assert.Equal(t, "collector", collector.Name, group.role)
					assert.Empty(t, collector.EnvFrom, group.role)
					_, secretRefs, _ := splitEnv(collector.Env)
					assert.Equal(t, corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: testCredentialsSecret},
						Key:                  "AWS_ACCESS_KEY_ID",
					}, secretRefs["AWS_ACCESS_KEY_ID"], group.role)
					containers = containers[:len(containers)-1]
				}

				require.Len(t, containers, len(tt.expected), group.role)
				for i, c := range containers {
					assert.Equal(t, tt.expected[i], c.EnvFrom, "%s container %q", group.role, c.Name)
				}

				require.Len(t, group.template.Spec.InitContainers, 1, group.role)
				assert.Equal(t, []corev1.EnvFromSource{configMapSource}, group.template.Spec.InitContainers[0].EnvFrom,
					"%s init container", group.role)
			}
		})
	}
}

// The mapped templates' Containers alias the v2 object's slices, and the
// RayCluster controller keeps using that object after the mapping. Neither the
// Secret nor the collector injected after it may write through.
func TestMapper_MapGlobalJobClusterToLocal_CredentialsSecretDoesNotMutateSource(t *testing.T) {
	source := credentialsTestCluster([]corev1.Container{
		{Name: "ray", EnvFrom: []corev1.EnvFromSource{configMapSource}},
	})
	before := source.DeepCopy()

	m := Mapper{LogPersistence: LogPersistenceConfig{
		Enabled:           true,
		CredentialsSecret: testCredentialsSecret,
		CollectorImage:    "quay.io/kuberay/collector:v1.7.1",
	}}
	_, err := m.MapGlobalJobClusterToLocal(source, nil)
	require.NoError(t, err)

	assert.Equal(t, before, source)
}
