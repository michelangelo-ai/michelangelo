package sparkjob

import (
	"context"
	"reflect"
	"time"

	"github.com/go-logr/logr"
	sparkv1beta2 "github.com/kubeflow/spark-operator/api/v1beta2"
	sparkclientset "github.com/kubeflow/spark-operator/pkg/client/clientset/versioned"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/michelangelo-ai/michelangelo/go/api/utils"
	"github.com/michelangelo-ai/michelangelo/go/base/env"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

const requeueAfter = 10 * time.Second

type Reconciler struct {
	client.Client
	SparkClient sparkclientset.Interface
	env         env.Context
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	res := ctrl.Result{}

	var sparkJob v2pb.SparkJob
	if err := r.Get(ctx, req.NamespacedName, &sparkJob); err != nil {
		if utils.IsNotFoundError(err) {
			return res, nil
		}
		res.RequeueAfter = requeueAfter
		return res, err
	}
	original := sparkJob.DeepCopy()

	status, message, err := r.getJobStatus(ctx, logger, &sparkJob)
	if err != nil {
		logger.Info("SparkApplication not found, creating new one")
		if err := r.createJob(ctx, logger, &sparkJob); err != nil {
			logger.Error(err, "failed to create SparkApplication")
			sparkJob.Status.StatusConditions = nil
			sparkJob.Status.JobUrl = ""
			sparkJob.Status.ApplicationId = ""
			res.RequeueAfter = requeueAfter
			return res, err
		}
		sparkJob.Status.JobUrl = ""
		sparkJob.Status.ApplicationId = ""
		res.RequeueAfter = requeueAfter
	} else if status != nil {
		logger.Info("Found SparkApplication", "ID", sparkJob.Status.ApplicationId, "status", *status)
		sparkJob.Status.JobUrl = message
		sparkJob.Status.ApplicationId = *status
		res.RequeueAfter = requeueAfter
	}

	if !reflect.DeepEqual(original, sparkJob) {
		if err := r.Status().Update(ctx, &sparkJob); err != nil {
			logger.Error(err, "failed to update SparkJob status")
			res.RequeueAfter = requeueAfter
			return res, err
		}
	}

	logger.Info("SparkJob reconciled", "name", sparkJob.Name, "namespace", sparkJob.Namespace)

	return res, nil
}

func (r *Reconciler) Register(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v2pb.SparkJob{}).
		Complete(r)
}

// createJob creates a new Spark job
func (r *Reconciler) createJob(ctx context.Context, log logr.Logger, job *v2pb.SparkJob) error {
	spec := job.Spec

	app := &sparkv1beta2.SparkApplication{
		ObjectMeta: metav1.ObjectMeta{
			Name:      job.Name,
			Namespace: job.Namespace,
		},
		Spec: sparkv1beta2.SparkApplicationSpec{
			Type:                sparkv1beta2.SparkApplicationTypePython,
			SparkVersion:        spec.SparkVersion,
			Mode:                sparkv1beta2.DeployModeCluster,
			Image:               &spec.Driver.Pod.Image,
			ImagePullPolicy:     &spec.Driver.Pod.ImagePullingPolicy,
			MainClass:           stringPtr(spec.MainClass),
			MainApplicationFile: stringPtr(spec.MainApplicationFile),
			Arguments:           spec.MainArgs,
			SparkConf:           spec.SparkConf,
			Driver: sparkv1beta2.DriverSpec{
				SparkPodSpec: r.toSparkPodSpec(spec.Driver.Pod),
			},
			Executor: sparkv1beta2.ExecutorSpec{
				SparkPodSpec: r.toSparkPodSpec(spec.Executor.Pod),
				Instances:    int32Ptr(spec.Executor.Instances),
			},
		},
	}

	if spec.Deps != nil {
		app.Spec.Deps = sparkv1beta2.Dependencies{
			Jars:    spec.Deps.Jars,
			Files:   spec.Deps.Files,
			PyFiles: spec.Deps.PyFiles,
		}
	}

	created, err := r.SparkClient.SparkoperatorV1beta2().
		SparkApplications(job.Namespace).
		Create(ctx, app, metav1.CreateOptions{})
	if err != nil {
		log.Error(err, "Failed to create SparkApplication")
		return err
	}

	job.Status.ApplicationId = string(created.UID)
	job.Status.JobUrl = created.Status.DriverInfo.WebUIIngressAddress
	log.Info("Created SparkApplication", "id", job.Status.ApplicationId, "jobUrl", job.Status.JobUrl)
	return nil
}

// getJobStatus retrieves the status of the Spark job
func (r *Reconciler) getJobStatus(ctx context.Context, logger logr.Logger, job *v2pb.SparkJob) (*string, string, error) {
	app, err := r.SparkClient.SparkoperatorV1beta2().SparkApplications(job.Namespace).Get(ctx, job.Name, metav1.GetOptions{})
	if err != nil {
		return nil, "", err
	}

	appID := app.Status.AppState.State
	url := app.Status.DriverInfo.WebUIIngressAddress

	job.Status.ApplicationId = string(app.UID)
	job.Status.JobUrl = url

	return stringPtr(string(appID)), url, nil
}

// toSparkPodSpec converts a PodSpec from the v2pb package to a SparkPodSpec
func (r *Reconciler) toSparkPodSpec(pod *v2pb.PodSpec) sparkv1beta2.SparkPodSpec {
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

	return sparkv1beta2.SparkPodSpec{
		Cores:  int32Ptr(pod.Resource.Cpu),
		Memory: stringPtr(pod.Resource.Memory),
		GPU: &sparkv1beta2.GPUSpec{
			Name:     pod.Resource.GpuSku,
			Quantity: int64(pod.Resource.Gpu),
		},
		Env: envVars,
	}
}

func stringPtr(s string) *string {
	return &s
}

func int32Ptr(s int32) *int32 {
	return &s
}
