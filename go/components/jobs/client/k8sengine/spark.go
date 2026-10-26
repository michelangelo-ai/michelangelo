package k8sengine

import (
	"fmt"
	"strings"

	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	sparkv1beta2 "github.com/michelangelo-ai/michelangelo/go/thirdparty/k8s-crds/apis/sparkoperator.k8s.io/v1beta2"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sptr "k8s.io/utils/ptr"
)

const (
	// sparkDriverServiceAccount is the service account driver pods run as. The
	// driver needs it to create and watch its own executors; executors need
	// nothing. It must exist in SparkLocalNamespace on every compute cluster
	// that runs Spark jobs. The spark-operator Helm chart names the service
	// account it creates after the release, so it creates this one only when
	// installed as release spark-operator; under any other release name, set
	// the chart's spark.serviceAccount.name to this value.
	sparkDriverServiceAccount = "spark-operator-spark"

	// sparkDriverRole and sparkExecutorRole are the role names handed to
	// injectCollectorSidecar. They are Spark's analogues of Ray's "Head" and
	// "Worker" and land on the collector as RAY_ROLE. Neither is "Head", so
	// neither picks up the head-only dashboard-poll env vars.
	sparkDriverRole   = "Driver"
	sparkExecutorRole = "Executor"

	// sparkJobFailedMessage is the Succeeded condition's message for a failed
	// application whose operator state carries no error text. The standalone
	// Spark controller used the same default.
	sparkJobFailedMessage = "Spark job failed"
)

// sparkApplicationType infers the SparkApplicationType from the main application
// file's extension. SparkApplicationSpec.Type is required by the CRD and
// v2pb.SparkJobSpec carries no equivalent field, so the extension is the only
// signal available. Mirrors go/components/spark/job/client/client.go.
func sparkApplicationType(mainApplicationFile string) sparkv1beta2.SparkApplicationType {
	switch {
	case strings.HasSuffix(mainApplicationFile, ".py"):
		return sparkv1beta2.PythonApplicationType
	case strings.HasSuffix(mainApplicationFile, ".R"), strings.HasSuffix(mainApplicationFile, ".r"):
		return sparkv1beta2.RApplicationType
	default:
		return sparkv1beta2.JavaApplicationType
	}
}

// mapSparkJob translates a global SparkJob into the SparkApplication projected
// into the assigned compute cluster.
//
// Unlike Ray — where the RayCluster is the scheduled unit and the RayJob merely
// rides along — a Spark job is itself what gets scheduled and admitted, so this
// follows mapRayCluster's shape, not mapRay's: build the sub-specs, inject the
// collector sidecar, resolve the Kueue LocalQueue (propagating the error),
// stamp the control-plane labels on the pod specs, then assemble the CR.
//
// The translation itself is adapted from the deprecated standalone,
// single-cluster path in go/components/spark/job/client/client.go, which is
// not imported from here: the jobs control plane depends only on
// spark/sparkoperator, a leaf package holding the CRD's scheme and REST client,
// not on the deprecated standalone packages, and the standalone's pod
// conversion is a method on a REST-client-carrying struct. Every pointer
// traversal below is nil-guarded; the standalone's is not, and a mapper that
// panics on a sparse CR would take the control-plane process down with it.
func (m Mapper) mapSparkJob(sparkJob *v2pb.SparkJob, cluster *v2pb.Cluster) (runtime.Object, error) {
	if sparkJob == nil {
		return nil, fmt.Errorf("sparkJob cannot be nil")
	}

	spec := sparkJob.GetSpec()
	driverPod := spec.GetDriver().GetPod()
	executorPod := spec.GetExecutor().GetPod()

	serviceAccount := sparkDriverServiceAccount
	driver := sparkv1beta2.DriverSpec{
		SparkPodSpec: toSparkPodSpec(driverPod, &serviceAccount),
	}
	executor := sparkv1beta2.ExecutorSpec{
		SparkPodSpec: toSparkPodSpec(executorPod, nil),
	}
	// The CRD requires Instances >= 1, so an unset or zero count has to stay nil
	// and let the operator apply its own default rather than be rejected.
	if instances := spec.GetExecutor().GetInstances(); instances > 0 {
		executor.Instances = &instances
	}

	// The application-level image applies to the driver and every executor. It
	// comes from the driver pod, or from the executor pod when the driver names
	// none. An executor image that differs from it is set on the executor
	// itself, where the operator lets it override the application's. The CRD has
	// no per-pod pull policy, so only the application-level one, the driver
	// pod's, is carried.
	appImage := driverPod.GetImage()
	if appImage == "" {
		appImage = executorPod.GetImage()
	}
	if executorImage := executorPod.GetImage(); executorImage != "" && executorImage != appImage {
		executor.Image = &executorImage
	}

	// SparkPodSpec has no Volumes of its own — volumes are declared once on the
	// application and mounted per pod — so the sidecar's emptyDir comes back
	// here to be hoisted, and driver and executor contribute the same one.
	//
	// The collector is Ray's: it collects from a raylet, which a Spark pod does
	// not run, so on Spark pods it uploads nothing yet. It is injected all the
	// same, through the injector Ray's pods use; Spark log persistence is not
	// implemented yet.
	var volumes []corev1.Volume
	if m.LogPersistence.Enabled {
		volumes = mergeVolumes(volumes, m.injectSparkCollectorSidecar(&driver.SparkPodSpec, sparkDriverRole))
		volumes = mergeVolumes(volumes, m.injectSparkCollectorSidecar(&executor.SparkPodSpec, sparkExecutorRole))
	}

	queueName, err := m.kueueQueueName(sparkJob.GetLabels(), sparkJob.GetNamespace(), cluster)
	if err != nil {
		return nil, err
	}

	// The control-plane labels go on the driver and executor pod specs too, so
	// the pods carry them whatever the operator copies from the application.
	// Spark Operator 2.x copies the application's labels onto the driver and
	// executor pods as well, so these are a second route, not the only one;
	// nothing in the control plane watches Spark pods yet.
	// controlPlaneLabels is called twice so the two pod specs do not share one
	// map. The Kueue queue label is deliberately not among them: on a pod it
	// would expose the pods to Kueue's pod integration as separately
	// admissible workloads. Leaving it off the pod specs is not enough on its
	// own, since the operator would copy it from the application: Spark
	// Operator 2.4.0 is the first release that skips kueue.x-k8s.io/* labels
	// when it copies them, which is one reason a Kueue-managed cluster needs it.
	driver.Labels = m.controlPlaneLabels(sparkJob.GetNamespace(), constants.MAOwnerSparkLabelValue)
	executor.Labels = m.controlPlaneLabels(sparkJob.GetNamespace(), constants.MAOwnerSparkLabelValue)

	// The job's own UID goes on the application: it is what lets the client and
	// the watcher tell this job's SparkApplication from a same-named one that
	// another job left behind in the shared member namespace, or that was
	// created outside Michelangelo. The mapper does not put it on the pod specs;
	// Spark Operator 2.x copies it onto the pods with the application's other
	// labels, and nothing reads it there. It is taken from the CR itself, never
	// from the source labels, so a job author cannot forge it. An object that
	// never went through the API server has no UID, and the label is left off:
	// an empty value would claim nothing either, since an application without
	// the job's non-empty UID in this label is no job's own.
	objectLabels := m.objectLabels(sparkJob.GetNamespace(), constants.MAOwnerSparkLabelValue, queueName)
	if uid := sparkJob.GetUID(); uid != "" {
		objectLabels[constants.JobUIDLabelKey] = string(uid)
	}

	// MainApplicationFile has no omitempty in the CRD, so it is always set.
	mainApplicationFile := spec.GetMainApplicationFile()

	sparkApplication := &sparkv1beta2.SparkApplication{
		TypeMeta: metav1.TypeMeta{
			Kind:       SparkApplicationKind,
			APIVersion: SparkAPIVersion,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      sparkJob.GetName(),
			Namespace: SparkLocalNamespace,
			Labels:    objectLabels,
		},
		Spec: sparkv1beta2.SparkApplicationSpec{
			Type:                sparkApplicationType(mainApplicationFile),
			SparkVersion:        spec.GetSparkVersion(),
			Mode:                sparkv1beta2.ClusterMode,
			Image:               strPtrOrNil(appImage),
			ImagePullPolicy:     strPtrOrNil(driverPod.GetImagePullingPolicy()),
			MainClass:           strPtrOrNil(spec.GetMainClass()),
			MainApplicationFile: &mainApplicationFile,
			Arguments:           spec.GetMainArgs(),
			SparkConf:           spec.GetSparkConf(),
			Volumes:             volumes,
			Driver:              driver,
			Executor:            executor,
			// Without a TTL the operator never reaps a finished application.
			// A fresh pointer per application, so no two share one.
			TimeToLiveSeconds: k8sptr.To(SparkApplicationTTLSeconds),
		},
	}

	if deps := spec.GetDeps(); deps != nil {
		sparkApplication.Spec.Deps = sparkv1beta2.Dependencies{
			Jars:    deps.GetJars(),
			Files:   deps.GetFiles(),
			PyFiles: deps.GetPyFiles(),
		}
	}

	return sparkApplication, nil
}

// toSparkPodSpec converts a Michelangelo PodSpec into the CRD's SparkPodSpec.
//
// Every field is optional and every traversal nil-guarded: a sparse CR yields a
// sparse SparkPodSpec, never a panic. Cores and GPU are omitted rather than set
// to zero because the CRD declares Minimum=1 on Cores and a zero-quantity GPU
// would still emit a resource limit for a device the job never asked for.
//
// A pod that asks for a GPU also gets the NVIDIA GPU toleration, for the
// reason tolerateNVIDIAGPUTaint gives for Ray: the scheduler rather than the
// caller picks the compute cluster, so the caller cannot know whether that
// cluster taints its GPU nodes. It holds all the more here, because the v2
// PodSpec has no tolerations, so a SparkJob cannot add one itself. A pod that
// asks for none is left alone, so the taint keeps reserving those nodes.
func toSparkPodSpec(pod *v2pb.PodSpec, serviceAccount *string) sparkv1beta2.SparkPodSpec {
	sparkPodSpec := sparkv1beta2.SparkPodSpec{ServiceAccount: serviceAccount}
	if pod == nil {
		return sparkPodSpec
	}

	if resources := pod.GetResource(); resources != nil {
		if cores := resources.GetCpu(); cores > 0 {
			sparkPodSpec.Cores = &cores
		}
		if memory := resources.GetMemory(); memory != "" {
			sparkPodSpec.Memory = &memory
		}
		if gpu := resources.GetGpu(); gpu > 0 {
			// GPUSpec.Name is the extended-resource name the operator puts in the
			// pod's limits. gpu_sku is a GPU model such as "nvidia-tesla-v100",
			// never a resource name, and the Uniflow tasks leave it empty (the
			// operator drops a GPUSpec with no name). The control plane accounts
			// every GPU as nvidia.com/gpu, so that is the name here, SKU or not.
			sparkPodSpec.GPU = &sparkv1beta2.GPUSpec{
				Name:     string(constants.ResourceNvidiaGPU),
				Quantity: int64(gpu),
			}
			// A fresh slice per pod, so editing one pod's tolerations cannot
			// reach the other's.
			sparkPodSpec.Tolerations = []corev1.Toleration{nvidiaGPUToleration}
		}
	}

	if env := pod.GetEnv(); len(env) > 0 {
		envVars := make([]corev1.EnvVar, 0, len(env))
		for _, e := range env {
			envVars = append(envVars, corev1.EnvVar{Name: e.GetName(), Value: e.GetValue()})
		}
		sparkPodSpec.Env = envVars
	}

	if envFrom := pod.GetEnvFrom(); len(envFrom) > 0 {
		envFromSources := make([]corev1.EnvFromSource, 0, len(envFrom))
		for _, ef := range envFrom {
			source := corev1.EnvFromSource{Prefix: ef.GetPrefix()}
			// proto3 cannot tell an unset optional from false, so only an explicit
			// true is carried and nil leaves the source required. Optional matters
			// more across clusters than within one: a source present on the host
			// cluster may be absent on the member, and a required one then keeps
			// the pod from starting.
			if ref := ef.GetSecretRef(); ref != nil {
				source.SecretRef = &corev1.SecretEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: ref.GetLocalObjectReference().GetName()},
				}
				if ref.GetOptional() {
					source.SecretRef.Optional = k8sptr.To(true)
				}
			}
			if ref := ef.GetConfigMapRef(); ref != nil {
				source.ConfigMapRef = &corev1.ConfigMapEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: ref.GetLocalObjectReference().GetName()},
				}
				if ref.GetOptional() {
					source.ConfigMapRef.Optional = k8sptr.To(true)
				}
			}
			envFromSources = append(envFromSources, source)
		}
		sparkPodSpec.EnvFrom = envFromSources
	}

	return sparkPodSpec
}

// injectSparkCollectorSidecar runs the shared collector injection against a
// SparkPodSpec and returns the volumes it wants hoisted onto the application.
//
// injectCollectorSidecar speaks PodTemplateSpec: it mutates every existing
// container's env and volume mounts, appends the collector, and declares a
// volume. SparkPodSpec is flatter — one implied container, no Volumes — so the
// pod spec is lifted into a stand-in template and the results read back out.
// The stand-in carries the container-level SecurityContext because
// collectorSecurityContext matches the sidecar's identity to Containers[0], and
// the pod-level one because it falls back to that.
func (m Mapper) injectSparkCollectorSidecar(sparkPodSpec *sparkv1beta2.SparkPodSpec, role string) []corev1.Volume {
	template := corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name:            strings.ToLower(role),
				Env:             sparkPodSpec.Env,
				VolumeMounts:    sparkPodSpec.VolumeMounts,
				SecurityContext: sparkPodSpec.SecurityContext,
			}},
			SecurityContext: sparkPodSpec.PodSecurityContext,
		},
	}

	injectCollectorSidecar(&template, m.LogPersistence, role)

	mainContainer := template.Spec.Containers[0]
	sparkPodSpec.Env = mainContainer.Env
	sparkPodSpec.VolumeMounts = mainContainer.VolumeMounts
	sparkPodSpec.Sidecars = append(sparkPodSpec.Sidecars, template.Spec.Containers[1:]...)

	return template.Spec.Volumes
}

// mergeVolumes appends added to existing, skipping names already present.
// Driver and executor each ask for the same log volume; the application may
// declare it only once.
func mergeVolumes(existing, added []corev1.Volume) []corev1.Volume {
	for _, volume := range added {
		duplicate := false
		for _, present := range existing {
			if present.Name == volume.Name {
				duplicate = true
				break
			}
		}
		if !duplicate {
			existing = append(existing, volume)
		}
	}
	return existing
}

// strPtrOrNil returns nil for the empty string so optional CRD fields stay
// absent rather than being serialized as "".
func strPtrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// convertSparkApplicationStatusToGlobal projects a SparkApplication's status
// back into the global representation.
//
// v2pb.SparkJobStatus has no state enum, so the application state travels in
// StatusConditions — the control plane's real state machine: AppState.State
// becomes the condition reason and AppState.ErrorMessage its message. The
// condition types, Succeeded and SparkAppRunning, are the ones the Spark
// controller published before federation; their reasons are now the
// operator's states.
//
// This is a projection, not an update: it builds the condition list from the
// state in hand on every call, so a Succeeded condition is always present and
// can never be left asserting a state the application has since left. Six
// SparkApplicationStatus fields have no global counterpart and are dropped.
func convertSparkApplicationStatusToGlobal(sparkApplication *sparkv1beta2.SparkApplication) *v2pb.SparkJobStatus {
	if sparkApplication == nil {
		status := &v2pb.SparkJobStatus{}
		setSparkCondition(&status.StatusConditions, constants.SucceededCondition,
			apipb.CONDITION_STATUS_UNKNOWN, "", string(sparkv1beta2.UnknownState))
		return status
	}

	appState := sparkApplication.Status.AppState
	reason := string(appState.State)
	message := appState.ErrorMessage

	status := &v2pb.SparkJobStatus{
		// The CRD's own application ID, set by the Spark distribution from
		// spark.app.id. The standalone path reports the CR's UID instead, which
		// identifies the Kubernetes object rather than the Spark application.
		ApplicationId: sparkApplication.Status.SparkApplicationID,
		JobUrl:        sparkApplication.Status.DriverInfo.WebUIIngressAddress,
	}

	switch appState.State {
	case sparkv1beta2.CompletedState:
		setSparkCondition(&status.StatusConditions, constants.SucceededCondition,
			apipb.CONDITION_STATUS_TRUE, message, reason)
	case sparkv1beta2.FailedState, sparkv1beta2.FailedSubmissionState:
		// SUBMISSION_FAILED is terminal here although the operator moves it on to
		// FAILED at its next reconcile (the mapper sets no restart policy, so a
		// failed submission is never retried). The outcome is the same either
		// way, and the job is frozen at the first terminal event, so the recorded
		// reason stays SUBMISSION_FAILED. The operator does not always say why an
		// application failed, and an empty message would show up downstream as a
		// bare "FAILED:".
		if message == "" {
			message = sparkJobFailedMessage
		}
		setSparkCondition(&status.StatusConditions, constants.SucceededCondition,
			apipb.CONDITION_STATUS_FALSE, message, reason)
	default:
		// Not terminal yet: New, SUBMITTED, RUNNING, PENDING_RERUN,
		// INVALIDATING, SUCCEEDING, FAILING, UNKNOWN, and any state the
		// vendored types do not name, such as Spark Operator 2.4.0's
		// SUSPENDING, SUSPENDED and RESUMING, which an application Kueue
		// holds back passes through. FAILING is transitional, the mirror of
		// SUCCEEDING: the operator moves it on to FAILED or PENDING_RERUN, and
		// that next event decides the outcome.
		setSparkCondition(&status.StatusConditions, constants.SucceededCondition,
			apipb.CONDITION_STATUS_UNKNOWN, message, reason)
	}

	if appState.State == sparkv1beta2.RunningState {
		setSparkCondition(&status.StatusConditions, constants.SparkAppRunningCondition,
			apipb.CONDITION_STATUS_TRUE, message, reason)
	}

	return status
}

// setSparkCondition upserts a condition by type. Its message-before-reason
// argument order follows the setCondition helper of the Spark controller before
// federation, which no longer exists.
func setSparkCondition(conditions *[]*apipb.Condition, conditionType string, status apipb.ConditionStatus, message, reason string) {
	for _, condition := range *conditions {
		if condition.Type == conditionType {
			condition.Status = status
			condition.Message = message
			condition.Reason = reason
			return
		}
	}
	*conditions = append(*conditions, &apipb.Condition{
		Type:    conditionType,
		Status:  status,
		Message: message,
		Reason:  reason,
	})
}
