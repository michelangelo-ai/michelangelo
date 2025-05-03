package client

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/michelangelo-ai/michelangelo/go/components/spark/job"
	sparkv1beta2 "github.com/michelangelo-ai/michelangelo/go/thirdparty/k8s-crds/apis/sparkoperator.k8s.io/v1beta2"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
)

type SparkClient struct {
	K8sClient      rest.Interface
	ParameterCodec runtime.ParameterCodec
}

var _ job.Client = &SparkClient{}

// CreateJob creates a new Spark job
func (r SparkClient) CreateJob(ctx context.Context, log logr.Logger, job *v2pb.SparkJob) error {
	spec := job.Spec
	serviceAcount := "spark-operator-spark"

	sparkApplication := &sparkv1beta2.SparkApplication{
		ObjectMeta: metav1.ObjectMeta{
			Name:      job.Name,
			Namespace: job.Namespace,
		},
		Spec: sparkv1beta2.SparkApplicationSpec{
			Type:                sparkv1beta2.PythonApplicationType,
			SparkVersion:        spec.SparkVersion,
			Mode:                sparkv1beta2.ClusterMode,
			Image:               &spec.Driver.Pod.Image,
			ImagePullPolicy:     &spec.Driver.Pod.ImagePullingPolicy,
			MainClass:           &(spec.MainClass),
			MainApplicationFile: &(spec.MainApplicationFile),
			Arguments:           spec.MainArgs,
			SparkConf:           spec.SparkConf,
			Driver: sparkv1beta2.DriverSpec{
				SparkPodSpec: r.toSparkPodSpec(spec.Driver.Pod, &serviceAcount),
			},
			Executor: sparkv1beta2.ExecutorSpec{
				SparkPodSpec: r.toSparkPodSpec(spec.Executor.Pod, nil),
				Instances:    &(spec.Executor.Instances),
			},
		},
	}

	if spec.Deps != nil {
		sparkApplication.Spec.Deps = sparkv1beta2.Dependencies{
			Jars:    spec.Deps.Jars,
			Files:   spec.Deps.Files,
			PyFiles: spec.Deps.PyFiles,
		}
	}

	opts := metav1.CreateOptions{}
	result := &sparkv1beta2.SparkApplication{}
	err := r.K8sClient.Post().
		Namespace(job.Namespace).
		Resource("sparkapplications").
		VersionedParams(&opts, r.ParameterCodec).
		Body(sparkApplication).
		Do(ctx).
		Into(result)

	if err != nil {
		log.Error(err, "Failed to create SparkApplication")
		return err
	}

	job.Status.ApplicationId = string(result.UID)
	job.Status.JobUrl = result.Status.DriverInfo.WebUIIngressAddress
	log.Info("Created SparkApplication", "id", job.Status.ApplicationId, "jobUrl", job.Status.JobUrl)
	return nil
}

// GetJobStatus retrieves the status of the Spark job
func (r SparkClient) GetJobStatus(ctx context.Context, logger logr.Logger, job *v2pb.SparkJob) (*string, string, error) {
	result := &sparkv1beta2.SparkApplication{}
	options := metav1.GetOptions{}
	err := r.K8sClient.Get().
		Namespace(job.Namespace).
		Resource("sparkapplications").
		Name(job.Name).
		VersionedParams(&options, r.ParameterCodec).
		Do(ctx).
		Into(result)
	if err != nil {
		return nil, "", err
	}

	appID := result.Status.AppState.State
	url := result.Status.DriverInfo.WebUIIngressAddress

	job.Status.ApplicationId = string(result.UID)
	job.Status.JobUrl = url

	appIDStr := string(appID)
	return &appIDStr, url, nil
}

// toSparkPodSpec converts a PodSpec from the v2pb package to a SparkPodSpec
func (r SparkClient) toSparkPodSpec(pod *v2pb.PodSpec, serviceAccount *string) sparkv1beta2.SparkPodSpec {
	if pod == nil {
		return sparkv1beta2.SparkPodSpec{}
	}

	// Convert environment variables
	envVars := make([]corev1.EnvVar, 0, len(pod.Env))
	for _, e := range pod.Env {
		envVars = append(envVars, corev1.EnvVar{
			Name:  e.Name,
			Value: e.Value,
		})
	}

	// Convert envFrom fields
	envFrom := make([]corev1.EnvFromSource, 0, len(pod.EnvFrom))
	for _, ef := range pod.EnvFrom {
		coreEnvFromSource := corev1.EnvFromSource{}
		if ef.SecretRef != nil {
			coreEnvFromSource.SecretRef = &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: ef.SecretRef.LocalObjectReference.Name,
				},
			}
		}
		if ef.ConfigMapRef != nil {
			coreEnvFromSource.ConfigMapRef = &corev1.ConfigMapEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: ef.ConfigMapRef.LocalObjectReference.Name,
				},
			}
		}
		envFrom = append(envFrom, coreEnvFromSource)
	}

	return sparkv1beta2.SparkPodSpec{
		Cores:  &(pod.Resource.Cpu),
		Memory: &(pod.Resource.Memory),
		GPU: &sparkv1beta2.GPUSpec{
			Name:     pod.Resource.GpuSku,
			Quantity: int64(pod.Resource.Gpu),
		},
		Env:            envVars,
		EnvFrom:        envFrom,
		ServiceAccount: serviceAccount,
	}
}
