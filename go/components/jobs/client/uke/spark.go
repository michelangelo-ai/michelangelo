package uke

import (
	"strconv"

	computeconstants "code.uber.internal/infra/compute/compute-common/constants"
	kubespark "code.uber.internal/infra/compute/k8s-crds/apis/sparkoperator.k8s.io/v1beta2"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/constants"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/secrets"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/pointer"
	v2beta1pb "michelangelo/api/v2beta1"
)

// TODO confirm with spark team what these configs do and if they work in all cases
var _defaultSparkConf = map[string]string{
	"spark.task.cpus":                          "1",
	"spark.driver.maxResultSize":               "10g",
	"spark.rdd.useDatasetWrapperRDD":           "true",
	"spark.rdd.postRDDActionHook.enabled":      "true",
	"container.log.enableTerraBlobIntegration": "true",
}

var _overrideSparkConf = map[string]string{
	// These are required on the driver pod so that the correct topic is used to pipe the logs to umonitor.
	// Without these the default spark-on-k8s topic is used.
	"spark.uber.k8s.logging.service": "ml-code",
}

const (
	_sparkMetaKind       = "SparkApplication"
	_sparkMetaAPIVersion = "sparkoperator.k8s.io/v1beta2"
	_sparkEnv            = "SPARK_24"
)

func (m Mapper) mapSpark(job *v2beta1pb.SparkJob, _ *v2beta1pb.Cluster) (runtime.Object, error) {
	sparkApp := &kubespark.SparkApplication{
		TypeMeta: metav1.TypeMeta{
			Kind:       _sparkMetaKind,
			APIVersion: _sparkMetaAPIVersion,
		},
		ObjectMeta: job.ObjectMeta,
	}

	// Add a label to preserve the original namespace of the job
	if sparkApp.Labels == nil {
		sparkApp.Labels = make(map[string]string)
	}
	sparkApp.Labels[constants.ProjectNameLabelKey] = job.Namespace

	sparkApp.Namespace = SparkLocalNamespace

	spec := job.Spec
	deps := spec.GetDeps()

	sparkConf := m.getSparkConf(spec)

	sparkApp.Spec = kubespark.SparkApplicationSpec{
		Arguments:      spec.MainArgs,
		BatchScheduler: pointer.String(computeconstants.K8sResourceManagerSchedulerName),
		Deps: kubespark.Dependencies{
			Jars:    deps.Jars,
			PyFiles: deps.PyFiles,
			Files:   deps.Files,
		},
		Driver: kubespark.DriverSpec{
			SparkPodSpec: kubespark.SparkPodSpec{
				ServiceAccount: pointer.String("sparkoperator"),
				Cores:          pointer.Int32(spec.Driver.Pod.Resource.Cpu),
				Memory:         pointer.String(spec.Driver.Pod.Resource.Memory),
				Env:            m.getEnv(spec.Driver.Pod.Env),
				HostNetwork:    pointer.Bool(true),
				Secrets: []kubespark.SecretInfo{
					{
						Name: secrets.GetKubeSecretName(job.Name),
						Path: "/mnt/tokens",
						Type: kubespark.HadoopDelegationTokenSecret,
					},
				},
				Annotations:   getComputeAnnotations(job),
				SchedulerName: pointer.String(computeconstants.K8sResourceManagerSchedulerName),
			},
		},
		Executor: kubespark.ExecutorSpec{
			SparkPodSpec: kubespark.SparkPodSpec{
				Cores:       pointer.Int32(spec.Executor.Pod.Resource.Cpu),
				Memory:      pointer.String(spec.Executor.Pod.Resource.Memory),
				Env:         m.getEnv(spec.Executor.Pod.Env),
				HostNetwork: pointer.Bool(true),
				Secrets: []kubespark.SecretInfo{
					{
						Name: secrets.GetKubeSecretName(job.Name),
						Path: "/mnt/tokens",
						Type: kubespark.HadoopDelegationTokenSecret,
					},
				},
				Annotations:   getComputeAnnotations(job),
				SchedulerName: pointer.String(computeconstants.K8sResourceManagerSchedulerName),
			},
			Instances: pointer.Int32(spec.Executor.Instances),
		},
		Image:               pointer.String(spec.Driver.Pod.Image),
		MainApplicationFile: pointer.String(spec.MainApplicationFile),
		MainClass:           pointer.String(spec.MainClass),
		ProxyUser:           pointer.String(spec.User.ProxyUser),
		SparkConf:           sparkConf,
		SparkEnv:            pointer.String(_sparkEnv),
		Type:                kubespark.ScalaApplicationType,
	}

	m.preprocessSparkRequest(sparkApp)
	return sparkApp, nil
}

func getComputeAnnotations(job *v2beta1pb.SparkJob) map[string]string {
	annotationMap := map[string]string{
		computeconstants.ResourcePoolAnnotationKey: job.Status.Assignment.ResourcePool,
	}
	//Jobs will be marked as preemptible by default.
	//We will allow the customers to optionally mark their workloads as non-preemptible
	annotationMap[computeconstants.PreemptibleAnnotationKey] = "true"
	if job.Spec.Scheduling != nil {
		annotationMap[computeconstants.PreemptibleAnnotationKey] = strconv.FormatBool(job.Spec.GetScheduling().GetPreemptible())
	}
	return annotationMap
}

func (m Mapper) getSparkConf(spec v2beta1pb.SparkJobSpec) map[string]string {
	sparkConf := spec.SparkConf
	if sparkConf == nil {
		sparkConf = make(map[string]string)
	}

	// add defaults
	for k, v := range _defaultSparkConf {
		if _, ok := sparkConf[k]; !ok {
			sparkConf[k] = v
		}
	}

	// add overrides
	for k, v := range _overrideSparkConf {
		sparkConf[k] = v
	}
	return sparkConf
}

func (m Mapper) getEnv(envs []*v2beta1pb.Environment) []corev1.EnvVar {
	var sparkEnvs []corev1.EnvVar
	for _, e := range envs {
		sparkEnvs = append(sparkEnvs, corev1.EnvVar{
			Name:  e.Name,
			Value: e.Value,
		})
	}
	return sparkEnvs
}

func (m Mapper) preprocessSparkRequest(job *kubespark.SparkApplication) {
	// shed the resource version before sending the request
	job.ResourceVersion = ""

	// remove any finalizers
	job.ObjectMeta.Finalizers = nil
}
