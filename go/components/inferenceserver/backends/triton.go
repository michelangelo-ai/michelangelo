package backends

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

var _ Backend = &tritonBackend{}

const (
	// defaultTritonImage is the serving image for Triton-backed InferenceServers
	// that name none themselves, and is the only place the default is recorded;
	// inferenceServer.triton.defaultImage exists to override it, not to restate
	// it. Built from docker/triton-serving.Dockerfile, which adds the ML
	// framework dependencies the stock Triton image omits. The tag is pinned to a
	// specific build rather than floating, so an InferenceServer's runtime only
	// changes through an explicit edit.
	defaultTritonImage = "ghcr.io/michelangelo-ai/triton-serving:sha-e9b9955"

	// k8sProgressDeadlineExceeded is the Kubernetes DeploymentCondition reason string
	// that signals a rolling update has stalled. Named constant prevents silent
	// breakage if the comparison string drifts from the Kubernetes API.
	k8sProgressDeadlineExceeded = "ProgressDeadlineExceeded"

	// tritonHTTPPort is the container port Triton's HTTP API listens on. Model status
	// probes address pods directly on this port, bypassing the Service.
	tritonHTTPPort = 8000

	// tritonReasonUnloaded is the reason Triton attaches to an UNAVAILABLE repository
	// entry that was unloaded on request rather than by a failed load.
	tritonReasonUnloaded = "unloaded"

	// tritonSpecHashAnnotation records the hash of the pod template the backend last
	// applied to a Triton Deployment. CreateServer compares it with the template it would
	// build today and updates the Deployment when they differ, so probe and argument
	// changes reach servers that already exist.
	tritonSpecHashAnnotation = "inferenceserver.michelangelo.ai/spec-hash"

	// tritonModelConfigMountPath is where the inference server's model config ConfigMap is
	// mounted inside the Triton container. The readiness probe reads it to learn which
	// models a replica must serve before it may receive traffic.
	tritonModelConfigMountPath = "/etc/michelangelo/model-config"

	// TritonReadinessModelAware gates a replica's readiness on every serving model in the
	// model config being loaded, so a replica that is missing a model, or is still loading
	// one after a restart, is withheld from the inference Service. This is the default.
	TritonReadinessModelAware = "model-aware"
	// TritonReadinessServer gates readiness on Triton's own /v2/health/ready only.
	TritonReadinessServer = "server"
	// TritonReadinessNone configures no readiness probe.
	TritonReadinessNone = "none"
)

// errProxyDenied marks an API-server proxy request that was rejected for lack of
// permission. It is a deployment problem (missing pods/proxy RBAC), not a model state,
// so callers surface it as an error instead of waiting for the model.
var errProxyDenied = errors.New("api server refused the pod proxy request")

// tritonReadinessScript is the model-aware readiness probe. It compares the serving
// entries of the mounted model config with Triton's repository index and fails while any
// serving model is not READY. Entries without a phase were written before phases existed
// and count as serving. Runs with the python3 that ships in Triton's py3 images.
const tritonReadinessScript = `import json, sys, urllib.request
try:
    with open("` + tritonModelConfigMountPath + `/model-list.json") as f:
        entries = json.load(f)
except (OSError, ValueError):
    entries = []
required = {
    e["name"]
    for e in entries
    if isinstance(e, dict) and e.get("name") and (e.get("phase") or "serving") == "serving"
}
try:
    req = urllib.request.Request(
        "http://127.0.0.1:8000/v2/repository/index",
        data=b"{}",
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    with urllib.request.urlopen(req, timeout=5) as resp:
        index = json.load(resp)
except Exception as exc:
    print("triton unreachable:", exc)
    sys.exit(1)
ready = {m.get("name") for m in index if isinstance(m, dict) and m.get("state") == "READY"}
missing = sorted(required - ready)
if missing:
    print("serving models not loaded:", ", ".join(missing))
    sys.exit(1)
`

// tritonRepositoryEntry is one element of Triton's POST /v2/repository/index response.
// A model that is present in the repository but was never loaded has only a name.
type tritonRepositoryEntry struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	State   string `json:"state"`
	Reason  string `json:"reason"`
}

// TritonOption customizes a tritonBackend.
type TritonOption func(*tritonBackend)

// WithReadinessProbe selects the readiness probe mode for Triton pods: one of
// TritonReadinessModelAware (default), TritonReadinessServer or TritonReadinessNone.
func WithReadinessProbe(mode string) TritonOption {
	return func(b *tritonBackend) {
		b.readinessProbe = mode
	}
}

// ProbeTiming overrides one probe's timing. Zero keeps the probe's built-in value.
type ProbeTiming struct {
	PeriodSeconds    int32
	TimeoutSeconds   int32
	FailureThreshold int32
}

// apply returns probe with the non-zero timing fields overridden. A nil probe stays nil.
func (t ProbeTiming) apply(probe *corev1.Probe) *corev1.Probe {
	if probe == nil {
		return nil
	}
	if t.PeriodSeconds > 0 {
		probe.PeriodSeconds = t.PeriodSeconds
	}
	if t.TimeoutSeconds > 0 {
		probe.TimeoutSeconds = t.TimeoutSeconds
	}
	if t.FailureThreshold > 0 {
		probe.FailureThreshold = t.FailureThreshold
	}
	return probe
}

// ProbeTimings holds the timing overrides for each Triton pod probe.
type ProbeTimings struct {
	Startup   ProbeTiming
	Liveness  ProbeTiming
	Readiness ProbeTiming
}

// WithProbeTimings overrides the timing of the Triton pod probes. Zero fields keep the
// built-in values.
func WithProbeTimings(timings ProbeTimings) TritonOption {
	return func(b *tritonBackend) {
		b.probeTimings = timings
	}
}

const (
	// defaultDrainPreStopSeconds is how long a terminating pod keeps running after it left the
	// Service endpoints, so the gateway stops sending it new requests before Triton gets SIGTERM.
	defaultDrainPreStopSeconds int32 = 10
	// defaultDrainExitTimeoutSeconds is how long Triton waits for in-flight inference requests
	// to finish after SIGTERM before it exits (Triton's --exit-timeout-secs).
	defaultDrainExitTimeoutSeconds int32 = 30
	// drainGraceMarginSeconds is added to the drain budget when sizing the pod's termination
	// grace period, so the kubelet never SIGKILLs a pod that is still draining inside its budget.
	drainGraceMarginSeconds int32 = 5
)

// Drain tunes how a Triton pod shuts down when it is replaced, for example when a pod
// template change rolls the Deployment. Zero fields keep the built-in values.
type Drain struct {
	// PreStopSeconds delays SIGTERM so endpoint removal propagates first.
	PreStopSeconds int32
	// ExitTimeoutSeconds bounds how long Triton waits for in-flight requests after SIGTERM.
	ExitTimeoutSeconds int32
}

// WithDrain overrides the pod shutdown drain. Zero fields keep the built-in values.
func WithDrain(drain Drain) TritonOption {
	return func(b *tritonBackend) {
		b.drain = drain
	}
}

// resolved returns the drain with built-in values filled in for zero fields.
func (d Drain) resolved() Drain {
	if d.PreStopSeconds <= 0 {
		d.PreStopSeconds = defaultDrainPreStopSeconds
	}
	if d.ExitTimeoutSeconds <= 0 {
		d.ExitTimeoutSeconds = defaultDrainExitTimeoutSeconds
	}
	return d
}

// Triton Server Management
type tritonBackend struct {
	// defaultImage is the operator-configured image. Empty means defaultTritonImage.
	defaultImage string
	// readinessProbe is the readiness probe mode. Empty means TritonReadinessModelAware.
	readinessProbe string
	// probeTimings overrides the built-in probe timings.
	probeTimings ProbeTimings
	// drain tunes pod shutdown so replacing a pod does not drop in-flight requests.
	drain Drain
}

func NewTritonBackend(defaultImage string, opts ...TritonOption) *tritonBackend {
	backend := &tritonBackend{defaultImage: defaultImage}
	for _, opt := range opts {
		opt(backend)
	}
	return backend
}

func (b *tritonBackend) CreateServer(ctx context.Context, logger *zap.Logger, kubeClient client.Client, inferenceServer *v2pb.InferenceServer) (*ServerStatus, error) {
	// Create or reconcile the Deployment
	if err := b.ensureTritonDeployment(ctx, logger, kubeClient, inferenceServer); err != nil {
		return nil, fmt.Errorf("failed to create Deployment for %s/%s: %w",
			inferenceServer.Namespace, inferenceServer.Name, err)
	}

	// Create Service
	if err := b.createTritonService(ctx, logger, kubeClient, inferenceServer); err != nil {
		return nil, fmt.Errorf("failed to create Service for %s/%s: %w",
			inferenceServer.Namespace, inferenceServer.Name, err)
	}

	return &ServerStatus{
		State:     v2pb.INFERENCE_SERVER_STATE_CREATING,
		Endpoints: []string{fmt.Sprintf("http://%s.%s.svc.cluster.local:80", generateK8sServiceName(inferenceServer.Name), inferenceServer.Namespace)},
	}, nil
}

func (b *tritonBackend) GetServerStatus(ctx context.Context, logger *zap.Logger, kubeClient client.Client, inferenceServerName string, namespace string) (*ServerStatus, error) {
	deploymentName := generateK8sDeploymentName(inferenceServerName)

	// Check deployment exists
	deployment := &appsv1.Deployment{}
	deploymentKey := client.ObjectKey{Name: deploymentName, Namespace: namespace}

	if err := kubeClient.Get(ctx, deploymentKey, deployment); err != nil {
		// When deployment doesn't exist, return CREATE_PENDING to indicate resources need to be created
		return &ServerStatus{
			State: v2pb.INFERENCE_SERVER_STATE_CREATE_PENDING,
		}, nil
	}

	// Check if service exists
	service := &corev1.Service{}
	serviceKey := client.ObjectKey{Name: generateK8sServiceName(inferenceServerName), Namespace: namespace}

	if err := kubeClient.Get(ctx, serviceKey, service); err != nil {
		// Service doesn't exist, return CREATE_PENDING to indicate resources need to be created
		return &ServerStatus{
			State: v2pb.INFERENCE_SERVER_STATE_CREATE_PENDING,
		}, nil
	}

	// Determine state from deployment status and conditions
	state := b.getStateFromDeployment(logger, deployment, deploymentName)

	return &ServerStatus{
		State: state,
		Endpoints: []string{
			fmt.Sprintf("http://%s.%s.svc.cluster.local:80", generateK8sServiceName(inferenceServerName), namespace),
		},
	}, nil
}

func (b *tritonBackend) DeleteServer(ctx context.Context, logger *zap.Logger, kubeClient client.Client, inferenceServerName string, namespace string) error {
	// Delete Deployment
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      generateK8sDeploymentName(inferenceServerName),
			Namespace: namespace,
		},
	}
	if err := kubeClient.Delete(ctx, deployment); err != nil {
		logger.Warn("failed to delete deployment",
			zap.Error(err),
			zap.String("operation", "delete_server"),
			zap.String("namespace", namespace),
			zap.String("inferenceServer", inferenceServerName))
	}

	// Delete Service
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      generateK8sServiceName(inferenceServerName),
			Namespace: namespace,
		},
	}
	if err := kubeClient.Delete(ctx, service); err != nil {
		logger.Warn("failed to delete service",
			zap.Error(err),
			zap.String("operation", "delete_server"),
			zap.String("namespace", namespace),
			zap.String("inferenceServer", inferenceServerName))
	}

	return nil
}

func (b *tritonBackend) IsHealthy(ctx context.Context, logger *zap.Logger, kubeClient client.Client, inferenceServerName string, namespace string) (bool, error) {
	// Check Kubernetes resource status instead of HTTP endpoints
	// Get the Triton deployment status from Kubernetes
	deploymentName := generateK8sDeploymentName(inferenceServerName)

	deployment := &appsv1.Deployment{}
	err := kubeClient.Get(ctx, client.ObjectKey{Name: deploymentName, Namespace: namespace}, deployment)
	if err != nil {
		return false, fmt.Errorf("failed to get deployment %s/%s: %w", namespace, deploymentName, err)
	}

	// Check deployment conditions following Uber's pattern
	for _, condition := range deployment.Status.Conditions {
		if condition.Type == appsv1.DeploymentAvailable {
			if condition.Status == corev1.ConditionTrue {
				// Also check if pods are ready (additional safety check)
				if deployment.Status.ReadyReplicas > 0 && deployment.Status.ReadyReplicas == deployment.Status.Replicas {
					return true, nil
				} else {
					logger.Warn("Triton deployment available but pods not ready",
						zap.String("operation", "health_check"),
						zap.String("namespace", namespace),
						zap.String("server", inferenceServerName),
						zap.Int("readyReplicas", int(deployment.Status.ReadyReplicas)),
						zap.Int("totalReplicas", int(deployment.Status.Replicas)))
					return false, nil
				}
			} else {
				logger.Warn("Triton deployment not available",
					zap.String("operation", "health_check"),
					zap.String("namespace", namespace),
					zap.String("server", inferenceServerName),
					zap.String("reason", condition.Reason),
					zap.String("message", condition.Message))
				return false, nil
			}
		}
	}

	logger.Warn("Triton deployment status unclear",
		zap.String("operation", "health_check"),
		zap.String("namespace", namespace),
		zap.String("server", inferenceServerName))
	return false, nil
}

// GetModelStatus lists the Triton pods behind the inference server and asks each one for
// its repository index through the API server's pod proxy. Going pod by pod, instead of
// through the Service, is what lets the deployment controller require every replica to
// have the model before routing traffic to it.
func (b *tritonBackend) GetModelStatus(ctx context.Context, logger *zap.Logger, kubeClient client.Client, httpClient *http.Client, apiServerURL string, inferenceServerName string, namespace string, modelName string) (*ModelStatus, error) {
	logger.Info("Checking Triton model status", zap.String("model", modelName), zap.String("server", inferenceServerName))

	deploymentName := generateK8sDeploymentName(inferenceServerName)
	deployment := &appsv1.Deployment{}
	if err := kubeClient.Get(ctx, client.ObjectKey{Name: deploymentName, Namespace: namespace}, deployment); err != nil {
		return nil, fmt.Errorf("failed to get deployment %s/%s: %w", namespace, deploymentName, err)
	}
	desired := int32(1)
	if deployment.Spec.Replicas != nil {
		desired = *deployment.Spec.Replicas
	}

	pods := &corev1.PodList{}
	if err := kubeClient.List(ctx, pods, client.InNamespace(namespace), client.MatchingLabels{"app": deploymentName}); err != nil {
		return nil, fmt.Errorf("failed to list pods of deployment %s/%s: %w", namespace, deploymentName, err)
	}

	status := &ModelStatus{Desired: desired}
	for i := range pods.Items {
		pod := &pods.Items[i]
		if pod.DeletionTimestamp != nil {
			// A terminating pod is already being removed from the Service's endpoints.
			continue
		}
		replica, err := b.replicaModelStatus(ctx, httpClient, apiServerURL, pod, modelName)
		if err != nil {
			return nil, err
		}
		if replica.State != ModelLoadStateReady {
			logger.Warn("Triton model not ready on replica",
				zap.String("model", modelName),
				zap.String("pod", pod.Name),
				zap.String("state", string(replica.State)),
				zap.String("reason", replica.Reason))
		}
		status.Replicas = append(status.Replicas, replica)
	}
	sort.Slice(status.Replicas, func(i, j int) bool {
		return status.Replicas[i].Replica < status.Replicas[j].Replica
	})
	return status, nil
}

// replicaModelStatus classifies one pod's load state for the model. Only a refused proxy
// request is returned as an error: everything else that keeps the index from being read
// (pod still starting, Triton not listening yet) is a LOADING state that will resolve.
func (b *tritonBackend) replicaModelStatus(ctx context.Context, httpClient *http.Client, apiServerURL string, pod *corev1.Pod, modelName string) (ReplicaModelStatus, error) {
	replica := ReplicaModelStatus{Replica: pod.Name, State: ModelLoadStateLoading}
	if pod.Status.Phase != corev1.PodRunning {
		replica.Reason = fmt.Sprintf("pod is %s", pod.Status.Phase)
		return replica, nil
	}
	replica.Running = true

	entries, err := b.repositoryIndex(ctx, httpClient, apiServerURL, pod.Namespace, pod.Name)
	if err != nil {
		if errors.Is(err, errProxyDenied) {
			return replica, fmt.Errorf("model status probe for pod %s/%s: %w", pod.Namespace, pod.Name, err)
		}
		replica.Reason = err.Error()
		return replica, nil
	}

	replica.State, replica.Reason = classifyTritonRepositoryEntries(entries, modelName)
	return replica, nil
}

// repositoryIndex fetches Triton's repository index from one pod via the API server's pod
// proxy. The index is the only Triton endpoint that distinguishes a model that is still
// loading from one whose load failed.
func (b *tritonBackend) repositoryIndex(ctx context.Context, httpClient *http.Client, apiServerURL string, namespace string, podName string) ([]tritonRepositoryEntry, error) {
	indexURL := fmt.Sprintf(
		"%s/api/v1/namespaces/%s/pods/%s:%d/proxy/v2/repository/index",
		apiServerURL, namespace, podName, tritonHTTPPort,
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, indexURL, strings.NewReader("{}"))
	if err != nil {
		return nil, fmt.Errorf("build repository index request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("repository index request failed: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		var entries []tritonRepositoryEntry
		if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
			return nil, fmt.Errorf("decode repository index: %w", err)
		}
		return entries, nil
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, fmt.Errorf("%w: %s", errProxyDenied, resp.Status)
	default:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("repository index returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
}

// classifyTritonRepositoryEntries folds the index entries for one model into a load state.
// Any READY version makes the model READY. UNAVAILABLE with a reason other than "unloaded"
// is a failed load, since Triton records the load error there; "unloaded", LOADING,
// UNLOADING, and an entry with no state (never loaded) are all still in progress.
func classifyTritonRepositoryEntries(entries []tritonRepositoryEntry, modelName string) (ModelLoadState, string) {
	found := false
	failureReason := ""
	pendingReason := "model not yet loaded"
	for _, entry := range entries {
		if entry.Name != modelName {
			continue
		}
		found = true
		switch entry.State {
		case "READY":
			return ModelLoadStateReady, ""
		case "LOADING", "UNLOADING":
			pendingReason = strings.ToLower(entry.State)
		case "UNAVAILABLE":
			if entry.Reason != "" && entry.Reason != tritonReasonUnloaded {
				failureReason = entry.Reason
			} else {
				pendingReason = tritonReasonUnloaded
			}
		}
	}
	if failureReason != "" {
		return ModelLoadStateFailed, failureReason
	}
	if !found {
		return ModelLoadStateLoading, "model not in repository"
	}
	return ModelLoadStateLoading, pendingReason
}

// ensureTritonDeployment creates the Triton Deployment, or updates an existing one whose
// recorded spec hash differs from the template the backend builds today. The hash, rather
// than a field-by-field comparison, is what makes this stable against API-server
// defaulting of the stored template.
func (b *tritonBackend) ensureTritonDeployment(ctx context.Context, logger *zap.Logger, kubeClient client.Client, inferenceServer *v2pb.InferenceServer) error {
	desired := b.desiredTritonDeployment(inferenceServer)
	deploymentName := desired.Name

	existing := &appsv1.Deployment{}
	err := kubeClient.Get(ctx, client.ObjectKey{Name: deploymentName, Namespace: inferenceServer.Namespace}, existing)
	if err == nil {
		desiredHash := desired.Annotations[tritonSpecHashAnnotation]
		if existing.Annotations[tritonSpecHashAnnotation] == desiredHash {
			logger.Info("Deployment already up to date, skipping", zap.String("name", deploymentName))
			return nil
		}
		logger.Info("Updating Triton Deployment to the current pod template",
			zap.String("name", deploymentName),
			zap.String("previousSpecHash", existing.Annotations[tritonSpecHashAnnotation]),
			zap.String("specHash", desiredHash))
		existing.Spec.Replicas = desired.Spec.Replicas
		existing.Spec.Template = desired.Spec.Template
		if existing.Annotations == nil {
			existing.Annotations = map[string]string{}
		}
		existing.Annotations[tritonSpecHashAnnotation] = desiredHash
		if err := kubeClient.Update(ctx, existing); err != nil {
			logger.Error("failed to update Triton Deployment",
				zap.Error(err),
				zap.String("operation", "update_triton_deployment"),
				zap.String("namespace", inferenceServer.Namespace),
				zap.String("deployment", deploymentName))
			return fmt.Errorf("failed to update Triton Deployment %s/%s: %w",
				inferenceServer.Namespace, deploymentName, err)
		}
		return nil
	}

	if err := kubeClient.Create(ctx, desired); err != nil {
		logger.Error("failed to create Triton Deployment",
			zap.Error(err),
			zap.String("operation", "create_triton_deployment"),
			zap.String("namespace", inferenceServer.Namespace),
			zap.String("deployment", deploymentName))
		return fmt.Errorf("failed to create Triton Deployment %s/%s: %w",
			inferenceServer.Namespace, deploymentName, err)
	}
	return nil
}

// desiredTritonDeployment builds the Deployment the backend wants for the inference
// server, with the spec hash annotation already set.
func (b *tritonBackend) desiredTritonDeployment(inferenceServer *v2pb.InferenceServer) *appsv1.Deployment {
	deploymentName := generateK8sDeploymentName(inferenceServer.Name)

	replicas := inferenceServer.Spec.InitSpec.NumInstances
	if replicas == 0 {
		replicas = 1
	}

	servingImage := inferenceServer.Spec.InitSpec.GetServingSpec().GetImage()
	hostPathType := corev1.HostPathDirectoryOrCreate
	modelConfigOptional := true

	drain := b.drain.resolved()
	terminationGracePeriod := int64(drain.PreStopSeconds + drain.ExitTimeoutSeconds + drainGraceMarginSeconds)

	template := corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				"app": deploymentName,
			},
		},
		Spec: corev1.PodSpec{
			TerminationGracePeriodSeconds: &terminationGracePeriod,
			ImagePullSecrets:              tritonImagePullSecrets(servingImage),
			Containers: []corev1.Container{
				{
					Name:            "triton",
					Image:           b.buildTritonImage(inferenceServer),
					ImagePullPolicy: tritonImagePullPolicy(servingImage),
					Ports: []corev1.ContainerPort{
						{ContainerPort: tritonHTTPPort, Name: "http"},
						{ContainerPort: 8001, Name: "grpc"},
						{ContainerPort: 8002, Name: "metrics"},
					},
					Resources: buildResourceRequirements(inferenceServer.Spec.InitSpec),
					Args: []string{
						"tritonserver",
						"--model-store=/mnt/models",
						"--grpc-port=8001",
						"--http-port=8000",
						"--allow-grpc=true",
						"--allow-http=true",
						"--allow-metrics=true",
						"--metrics-port=8002",
						"--model-control-mode=explicit",
						"--strict-model-config=false",
						// Keep /v2/health/ready a server-level signal. Model readiness is
						// decided per replica by the readiness probe from the model config, so
						// one failed load must not take the whole server out of "ready" for
						// the sync daemon and other server-level callers.
						"--strict-readiness=false",
						"--exit-on-error=true",
						// On SIGTERM wait for in-flight requests instead of Triton's default.
						fmt.Sprintf("--exit-timeout-secs=%d", drain.ExitTimeoutSeconds),
						"--log-error=true",
						"--log-warning=true",
						"--log-verbose=0",
					},
					// Hold SIGTERM until the pod has left the Service endpoints, so the
					// gateway stops routing to it before Triton starts shutting down.
					Lifecycle: &corev1.Lifecycle{
						PreStop: &corev1.LifecycleHandler{
							Exec: &corev1.ExecAction{Command: []string{"sleep", strconv.Itoa(int(drain.PreStopSeconds))}},
						},
					},
					StartupProbe:   b.tritonStartupProbe(),
					LivenessProbe:  b.tritonLivenessProbe(),
					ReadinessProbe: b.tritonReadinessProbe(),
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "workdir",
							MountPath: "/mnt/models",
						},
						{
							Name:      "model-config",
							MountPath: tritonModelConfigMountPath,
							ReadOnly:  true,
						},
					},
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: "workdir",
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{
							Path: fmt.Sprintf("/var/lib/michelangelo/models/%s", inferenceServer.Name),
							Type: &hostPathType,
						},
					},
				},
				{
					Name: "model-config",
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: fmt.Sprintf("%s-model-config", inferenceServer.Name),
							},
							// The ConfigMap is created by the InferenceServer controller's
							// model config actor, which may run after the pods are scheduled.
							Optional: &modelConfigOptional,
						},
					},
				},
			},
		},
	}

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName,
			Namespace: inferenceServer.Namespace,
			Annotations: map[string]string{
				tritonSpecHashAnnotation: tritonSpecHash(replicas, template),
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": deploymentName,
				},
			},
			Template: template,
		},
	}
}

// tritonSpecHash hashes the parts of the Deployment spec the backend owns.
func tritonSpecHash(replicas int32, template corev1.PodTemplateSpec) string {
	raw, err := json.Marshal(struct {
		Replicas int32                  `json:"replicas"`
		Template corev1.PodTemplateSpec `json:"template"`
	}{Replicas: replicas, Template: template})
	if err != nil {
		// The template is built from plain structs and cannot fail to marshal; an
		// empty hash only ever forces an update, which is the safe direction.
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// tritonStartupProbe waits for Triton's HTTP server to come up before liveness applies,
// which gives large images and GPU initialization time to start.
func (b *tritonBackend) tritonStartupProbe() *corev1.Probe {
	return b.probeTimings.Startup.apply(&corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{Path: "/v2/health/live", Port: intstr.FromInt(tritonHTTPPort)},
		},
		PeriodSeconds:    5,
		TimeoutSeconds:   3,
		FailureThreshold: 60,
	})
}

// tritonLivenessProbe restarts a Triton container whose HTTP server has stopped answering.
func (b *tritonBackend) tritonLivenessProbe() *corev1.Probe {
	return b.probeTimings.Liveness.apply(&corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{Path: "/v2/health/live", Port: intstr.FromInt(tritonHTTPPort)},
		},
		PeriodSeconds:    10,
		TimeoutSeconds:   5,
		FailureThreshold: 6,
	})
}

// tritonReadinessProbe returns the readiness probe for the configured mode, with any
// configured timing overrides applied.
func (b *tritonBackend) tritonReadinessProbe() *corev1.Probe {
	return b.probeTimings.Readiness.apply(b.defaultReadinessProbe())
}

// defaultReadinessProbe returns the readiness probe for the configured mode with its
// built-in timing.
func (b *tritonBackend) defaultReadinessProbe() *corev1.Probe {
	switch b.readinessProbe {
	case TritonReadinessNone:
		return nil
	case TritonReadinessServer:
		return &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{Path: "/v2/health/ready", Port: intstr.FromInt(tritonHTTPPort)},
			},
			PeriodSeconds:    10,
			TimeoutSeconds:   5,
			FailureThreshold: 3,
		}
	default:
		return &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				Exec: &corev1.ExecAction{Command: []string{"python3", "-c", tritonReadinessScript}},
			},
			PeriodSeconds:    10,
			TimeoutSeconds:   8,
			FailureThreshold: 3,
		}
	}
}

func (b *tritonBackend) createTritonService(ctx context.Context, logger *zap.Logger, kubeClient client.Client, inferenceServer *v2pb.InferenceServer) error {
	serviceName := generateK8sServiceName(inferenceServer.Name)

	// Check if Service already exists
	existing := &corev1.Service{}
	err := kubeClient.Get(ctx, client.ObjectKey{Name: serviceName, Namespace: inferenceServer.Namespace}, existing)
	if err == nil {
		// Service already exists, log and return success
		logger.Info("Service already exists, skipping creation", zap.String("name", serviceName))
		return nil
	}

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: inferenceServer.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app": generateK8sDeploymentName(inferenceServer.Name),
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       80,
					TargetPort: intstr.FromInt(tritonHTTPPort),
					Protocol:   corev1.ProtocolTCP,
				},
				{
					Name:       "grpc",
					Port:       8001,
					TargetPort: intstr.FromInt(8001),
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Type: corev1.ServiceTypeClusterIP,
		},
	}

	if err := kubeClient.Create(ctx, service); err != nil {
		logger.Error("failed to create Triton Service",
			zap.Error(err),
			zap.String("operation", "create_triton_service"),
			zap.String("namespace", inferenceServer.Namespace),
			zap.String("service", serviceName))
		return fmt.Errorf("failed to create Triton Service %s/%s: %w",
			inferenceServer.Namespace, serviceName, err)
	}
	return nil
}

// getStateFromDeployment determines the server state by checking the Kubernetes Deployment status.
func (b *tritonBackend) getStateFromDeployment(logger *zap.Logger, deployment *appsv1.Deployment, deploymentName string) v2pb.InferenceServerState {
	desiredReplicas := int32(1)
	if deployment.Spec.Replicas != nil {
		desiredReplicas = *deployment.Spec.Replicas
	}

	// Check deployment conditions
	var availableCondition, progressingCondition *appsv1.DeploymentCondition
	for i := range deployment.Status.Conditions {
		cond := &deployment.Status.Conditions[i]
		switch cond.Type {
		case appsv1.DeploymentAvailable:
			availableCondition = cond
		case appsv1.DeploymentProgressing:
			progressingCondition = cond
		}
	}

	logger.Debug("Deployment status",
		zap.String("deployment", deploymentName),
		zap.Int32("desiredReplicas", desiredReplicas),
		zap.Int32("readyReplicas", deployment.Status.ReadyReplicas),
		zap.Int32("availableReplicas", deployment.Status.AvailableReplicas),
		zap.Int32("updatedReplicas", deployment.Status.UpdatedReplicas))

	// Check if deployment has failed (Progressing condition is False with a failure reason)
	if progressingCondition != nil && progressingCondition.Status == corev1.ConditionFalse {
		if progressingCondition.Reason == k8sProgressDeadlineExceeded {
			logger.Warn("Deployment progress deadline exceeded",
				zap.String("deployment", deploymentName),
				zap.String("message", progressingCondition.Message))
			return v2pb.INFERENCE_SERVER_STATE_FAILED
		}
	}

	// Check if deployment is available and all replicas are ready
	if availableCondition != nil && availableCondition.Status == corev1.ConditionTrue {
		if deployment.Status.ReadyReplicas >= desiredReplicas && desiredReplicas > 0 {
			return v2pb.INFERENCE_SERVER_STATE_SERVING
		}
	}

	// Deployment is still progressing
	return v2pb.INFERENCE_SERVER_STATE_CREATING
}

func buildResourceRequirements(initSpec *v2pb.InitSpec) corev1.ResourceRequirements {
	requests := corev1.ResourceList{}
	limits := corev1.ResourceList{}

	// resourceSpec is optional, so it is read through the nil-safe getters.
	resourceSpec := initSpec.GetResourceSpec()

	if cpu := resourceSpec.GetCpu(); cpu > 0 {
		requests[corev1.ResourceCPU] = parseQuantity(fmt.Sprintf("%d", cpu))
		limits[corev1.ResourceCPU] = parseQuantity(fmt.Sprintf("%d", cpu))
	}

	if memory := resourceSpec.GetMemory(); memory != "" {
		requests[corev1.ResourceMemory] = parseQuantity(memory)
		limits[corev1.ResourceMemory] = parseQuantity(memory)
	}

	if gpu := resourceSpec.GetGpu(); gpu > 0 {
		requests["nvidia.com/gpu"] = parseQuantity(fmt.Sprintf("%d", gpu))
		limits["nvidia.com/gpu"] = parseQuantity(fmt.Sprintf("%d", gpu))
	}

	return corev1.ResourceRequirements{
		Requests: requests,
		Limits:   limits,
	}
}

func parseQuantity(value string) resource.Quantity {
	qty, _ := resource.ParseQuantity(value)
	return qty
}

func generateK8sDeploymentName(inferenceServerName string) string {
	return fmt.Sprintf("triton-%s", inferenceServerName)
}

func generateK8sServiceName(inferenceServerName string) string {
	return fmt.Sprintf("%s-inference-service", inferenceServerName)
}

// buildTritonImage resolves the container image in order of precedence: the spec,
// the operator-configured default, then defaultTritonImage.
func (b *tritonBackend) buildTritonImage(inferenceServer *v2pb.InferenceServer) string {
	if uri := inferenceServer.Spec.InitSpec.GetServingSpec().GetImage().GetUri(); uri != "" {
		return uri
	}
	if b.defaultImage != "" {
		return b.defaultImage
	}
	return defaultTritonImage
}

// tritonImagePullPolicy maps the spec's pull policy onto the Kubernetes enum,
// defaulting to IfNotPresent.
func tritonImagePullPolicy(image *v2pb.ServingImage) corev1.PullPolicy {
	switch image.GetImagePullPolicy() {
	case string(corev1.PullAlways):
		return corev1.PullAlways
	case string(corev1.PullNever):
		return corev1.PullNever
	default:
		return corev1.PullIfNotPresent
	}
}

// tritonImagePullSecrets names the registry credentials the kubelet uses to pull
// a private image.
func tritonImagePullSecrets(image *v2pb.ServingImage) []corev1.LocalObjectReference {
	names := image.GetImagePullSecrets()
	if len(names) == 0 {
		return nil
	}
	refs := make([]corev1.LocalObjectReference, 0, len(names))
	for _, name := range names {
		refs = append(refs, corev1.LocalObjectReference{Name: name})
	}
	return refs
}
