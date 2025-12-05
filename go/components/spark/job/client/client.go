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

// SparkClient implements the Spark job client interface for creating and managing
// Spark applications in Kubernetes.
//
// The client communicates with the Spark Operator by creating and monitoring
// SparkApplication custom resources. It handles the conversion from SparkJob
// specifications to Spark Operator format and extracts status information.
type SparkClient struct {
	K8sClient      rest.Interface         // REST client for sparkoperator.k8s.io/v1beta2 API
	ParameterCodec runtime.ParameterCodec // Codec for query parameter encoding
}

// Compile-time assertion that SparkClient implements job.Client interface.
var _ job.Client = &SparkClient{}

// CreateJob creates a new SparkApplication from a SparkJob specification.
//
// This method:
//  1. Converts SparkJob spec to SparkApplication format
//  2. Configures driver and executor pods with resource requirements
//  3. Sets up dependencies (JARs, Python files, etc.)
//  4. Creates the SparkApplication via the Spark Operator API
//  5. Updates the SparkJob status with application ID and URL
//
// The created SparkApplication triggers the Spark Operator to provision
// driver and executor pods for running the Spark job.
//
// Returns an error if SparkApplication creation fails.
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

// GetJobStatus retrieves the current status of a SparkApplication.
//
// This method queries the Spark Operator for the SparkApplication resource and
// extracts its current state, web UI URL, and any error messages.
//
// Returns (in order):
//   - State string pointer: Current application state (e.g., "SUBMITTED", "RUNNING", "COMPLETED", "FAILED")
//   - Job URL: Web UI ingress address for accessing Spark UI
//   - Error message: Error message if the application failed
//   - Error: Error if status retrieval fails (e.g., not found, permission denied)
//
// The method also updates the SparkJob status with the application ID and URL.
func (r SparkClient) GetJobStatus(ctx context.Context, logger logr.Logger, job *v2pb.SparkJob) (*string, string, string, error) {
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
		return nil, "", "", err
	}

	state := result.Status.AppState.State
	url := result.Status.DriverInfo.WebUIIngressAddress
	errorMessage := result.Status.AppState.ErrorMessage

	job.Status.ApplicationId = string(result.UID)
	job.Status.JobUrl = url

	stateStr := string(state)
	return &stateStr, url, errorMessage, nil
}

// toSparkPodSpec converts a Michelangelo PodSpec to Spark Operator SparkPodSpec.
//
// This method transforms the generic pod specification from the Michelangelo API
// into the format required by the Spark Operator, including:
//   - Resource requirements (CPU, memory, GPU)
//   - Environment variables and config maps
//   - Service account configuration
//
// The serviceAccount parameter is optional and only set for driver pods.
//
// Returns a configured SparkPodSpec for use in SparkApplication.
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
