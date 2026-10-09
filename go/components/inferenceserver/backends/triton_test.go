package backends

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

const (
	tritonTestServer    = "test-server"
	tritonTestNamespace = "default"
	tritonTestModel     = "model-v1"
)

var tritonProxyPath = regexp.MustCompile(`^/api/v1/namespaces/([^/]+)/pods/([^:/]+):8000/proxy/v2/repository/index$`)

func newTritonScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	return scheme
}

func tritonPod(name string, phase corev1.PodPhase) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: tritonTestNamespace,
			Labels:    map[string]string{"app": generateK8sDeploymentName(tritonTestServer)},
		},
		Status: corev1.PodStatus{Phase: phase},
	}
}

func tritonDeployment(replicas int32) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: generateK8sDeploymentName(tritonTestServer), Namespace: tritonTestNamespace},
		Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
	}
}

// fakeAPIServer answers pod proxy requests for Triton's repository index. responses maps a
// pod name to the JSON body it returns; unknown pods get a 404 from the "API server".
func fakeAPIServer(t *testing.T, responses map[string]string, code int) (*httptest.Server, *[]string) {
	t.Helper()
	requested := &[]string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		match := tritonProxyPath.FindStringSubmatch(r.URL.Path)
		if match == nil {
			http.NotFound(w, r)
			return
		}
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, tritonTestNamespace, match[1])
		*requested = append(*requested, match[2])
		body, ok := responses[match[2]]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(code)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return server, requested
}

func TestTritonGetModelStatus(t *testing.T) {
	terminating := tritonPod("pod-d", corev1.PodRunning)
	now := metav1.Now()
	terminating.DeletionTimestamp = &now
	terminating.Finalizers = []string{"test.michelangelo.ai/keep"}
	otherApp := tritonPod("pod-e", corev1.PodRunning)
	otherApp.Labels = map[string]string{"app": "something-else"}

	kubeClient := fake.NewClientBuilder().WithScheme(newTritonScheme(t)).WithObjects(
		tritonDeployment(3),
		tritonPod("pod-b", corev1.PodRunning),
		tritonPod("pod-a", corev1.PodRunning),
		tritonPod("pod-c", corev1.PodPending),
		terminating,
		otherApp,
	).Build()
	server, requested := fakeAPIServer(t, map[string]string{
		"pod-a": `[{"name":"model-v1","version":"1","state":"READY"},{"name":"other","version":"1","state":"READY"}]`,
		"pod-b": `[{"name":"model-v1","version":"1","state":"UNAVAILABLE","reason":"failed to load: bad config"}]`,
	}, http.StatusOK)

	status, err := NewTritonBackend("").GetModelStatus(context.Background(), zap.NewNop(), kubeClient,
		server.Client(), server.URL, tritonTestServer, tritonTestNamespace, tritonTestModel)

	require.NoError(t, err)
	assert.Equal(t, int32(3), status.Desired)
	assert.Equal(t, []ReplicaModelStatus{
		{Replica: "pod-a", Running: true, State: ModelLoadStateReady},
		{Replica: "pod-b", Running: true, State: ModelLoadStateFailed, Reason: "failed to load: bad config"},
		{Replica: "pod-c", Running: false, State: ModelLoadStateLoading, Reason: "pod is Pending"},
	}, status.Replicas, "sorted by name; terminating and unrelated pods are ignored")
	assert.False(t, status.Ready())
	assert.Len(t, status.Failed(), 1)
	assert.ElementsMatch(t, []string{"pod-a", "pod-b"}, *requested, "only running pods are probed")
	assert.Contains(t, status.Summary(), "1/3")
}

func TestTritonGetModelStatus_AllReady(t *testing.T) {
	kubeClient := fake.NewClientBuilder().WithScheme(newTritonScheme(t)).WithObjects(
		tritonDeployment(2), tritonPod("pod-a", corev1.PodRunning), tritonPod("pod-b", corev1.PodRunning),
	).Build()
	ready := `[{"name":"model-v1","version":"1","state":"READY"}]`
	server, _ := fakeAPIServer(t, map[string]string{"pod-a": ready, "pod-b": ready}, http.StatusOK)

	status, err := NewTritonBackend("").GetModelStatus(context.Background(), zap.NewNop(), kubeClient,
		server.Client(), server.URL, tritonTestServer, tritonTestNamespace, tritonTestModel)

	require.NoError(t, err)
	assert.True(t, status.Ready())
	assert.Empty(t, status.Failed())
	replica, ok := status.Replica("pod-b")
	require.True(t, ok)
	assert.Equal(t, ModelLoadStateReady, replica.State)
}

func TestTritonGetModelStatus_FewerPodsThanDesiredIsNotReady(t *testing.T) {
	kubeClient := fake.NewClientBuilder().WithScheme(newTritonScheme(t)).WithObjects(
		tritonDeployment(2), tritonPod("pod-a", corev1.PodRunning),
	).Build()
	server, _ := fakeAPIServer(t, map[string]string{"pod-a": `[{"name":"model-v1","version":"1","state":"READY"}]`}, http.StatusOK)

	status, err := NewTritonBackend("").GetModelStatus(context.Background(), zap.NewNop(), kubeClient,
		server.Client(), server.URL, tritonTestServer, tritonTestNamespace, tritonTestModel)

	require.NoError(t, err)
	assert.False(t, status.Ready(), "a replica that has not been scheduled yet still has to load the model")
}

func TestTritonGetModelStatus_ProxyDeniedIsAnError(t *testing.T) {
	kubeClient := fake.NewClientBuilder().WithScheme(newTritonScheme(t)).WithObjects(
		tritonDeployment(1), tritonPod("pod-a", corev1.PodRunning),
	).Build()
	server, _ := fakeAPIServer(t, map[string]string{"pod-a": `{"kind":"Status","reason":"Forbidden"}`}, http.StatusForbidden)

	_, err := NewTritonBackend("").GetModelStatus(context.Background(), zap.NewNop(), kubeClient,
		server.Client(), server.URL, tritonTestServer, tritonTestNamespace, tritonTestModel)

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrProxyDenied), "a refused proxy is a misconfiguration, not a pending load: %v", err)
}

func TestTritonGetModelStatus_TritonNotListeningYetIsLoading(t *testing.T) {
	kubeClient := fake.NewClientBuilder().WithScheme(newTritonScheme(t)).WithObjects(
		tritonDeployment(1), tritonPod("pod-a", corev1.PodRunning),
	).Build()
	// The API server proxies to a container whose HTTP port is not open yet.
	server, _ := fakeAPIServer(t, map[string]string{"pod-a": "dial tcp: connection refused"}, http.StatusBadGateway)

	status, err := NewTritonBackend("").GetModelStatus(context.Background(), zap.NewNop(), kubeClient,
		server.Client(), server.URL, tritonTestServer, tritonTestNamespace, tritonTestModel)

	require.NoError(t, err)
	require.Len(t, status.Replicas, 1)
	assert.Equal(t, ModelLoadStateLoading, status.Replicas[0].State)
	assert.Contains(t, status.Replicas[0].Reason, "502")
}

func TestTritonGetModelStatus_DeploymentMissing(t *testing.T) {
	kubeClient := fake.NewClientBuilder().WithScheme(newTritonScheme(t)).Build()
	server, _ := fakeAPIServer(t, nil, http.StatusOK)

	_, err := NewTritonBackend("").GetModelStatus(context.Background(), zap.NewNop(), kubeClient,
		server.Client(), server.URL, tritonTestServer, tritonTestNamespace, tritonTestModel)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "triton-test-server")
}

func TestClassifyTritonRepositoryEntries(t *testing.T) {
	tests := []struct {
		name       string
		entries    []tritonRepositoryEntry
		wantState  ModelLoadState
		wantReason string
	}{
		{name: "not in the repository", entries: nil, wantState: ModelLoadStateLoading, wantReason: "model not in repository"},
		{name: "present but never loaded", entries: []tritonRepositoryEntry{{Name: tritonTestModel}}, wantState: ModelLoadStateLoading, wantReason: "model not yet loaded"},
		{name: "loading", entries: []tritonRepositoryEntry{{Name: tritonTestModel, State: "LOADING"}}, wantState: ModelLoadStateLoading, wantReason: "loading"},
		{name: "unloading", entries: []tritonRepositoryEntry{{Name: tritonTestModel, State: "UNLOADING"}}, wantState: ModelLoadStateLoading, wantReason: "unloading"},
		{name: "explicitly unloaded", entries: []tritonRepositoryEntry{{Name: tritonTestModel, State: "UNAVAILABLE", Reason: "unloaded"}}, wantState: ModelLoadStateLoading, wantReason: "unloaded"},
		{name: "unavailable without a reason", entries: []tritonRepositoryEntry{{Name: tritonTestModel, State: "UNAVAILABLE"}}, wantState: ModelLoadStateLoading, wantReason: "unloaded"},
		{name: "load failed", entries: []tritonRepositoryEntry{{Name: tritonTestModel, State: "UNAVAILABLE", Reason: "Internal: unable to load"}}, wantState: ModelLoadStateFailed, wantReason: "Internal: unable to load"},
		{name: "ready", entries: []tritonRepositoryEntry{{Name: tritonTestModel, Version: "1", State: "READY"}}, wantState: ModelLoadStateReady},
		{
			name: "any ready version wins over a failed one",
			entries: []tritonRepositoryEntry{
				{Name: tritonTestModel, Version: "1", State: "UNAVAILABLE", Reason: "bad version"},
				{Name: tritonTestModel, Version: "2", State: "READY"},
			},
			wantState: ModelLoadStateReady,
		},
		{
			name: "other models are ignored",
			entries: []tritonRepositoryEntry{
				{Name: "other", State: "READY"},
				{Name: tritonTestModel, State: "LOADING"},
			},
			wantState:  ModelLoadStateLoading,
			wantReason: "loading",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state, reason := classifyTritonRepositoryEntries(tt.entries, tritonTestModel)
			assert.Equal(t, tt.wantState, state)
			assert.Equal(t, tt.wantReason, reason)
		})
	}
}

func tritonInferenceServer(image string, replicas int32) *v2pb.InferenceServer {
	return &v2pb.InferenceServer{
		ObjectMeta: metav1.ObjectMeta{Name: tritonTestServer, Namespace: tritonTestNamespace},
		Spec: v2pb.InferenceServerSpec{
			BackendType: v2pb.BACKEND_TYPE_TRITON,
			InitSpec: &v2pb.InitSpec{
				NumInstances: replicas,
				ServingSpec: &v2pb.ServingSpec{
					Image: &v2pb.ServingImage{Source: &v2pb.ServingImage_Uri{Uri: image}},
				},
			},
		},
	}
}

func getTritonDeployment(t *testing.T, kubeClient client.Client) *appsv1.Deployment {
	t.Helper()
	deployment := &appsv1.Deployment{}
	require.NoError(t, kubeClient.Get(context.Background(),
		client.ObjectKey{Name: generateK8sDeploymentName(tritonTestServer), Namespace: tritonTestNamespace}, deployment))
	return deployment
}

func TestTritonCreateServerReconcilesTheDeployment(t *testing.T) {
	kubeClient := fake.NewClientBuilder().WithScheme(newTritonScheme(t)).Build()
	backend := NewTritonBackend("")
	ctx := context.Background()

	_, err := backend.CreateServer(ctx, zap.NewNop(), kubeClient, tritonInferenceServer("registry/triton:1", 2))
	require.NoError(t, err)
	created := getTritonDeployment(t, kubeClient)
	require.NotNil(t, created.Spec.Replicas)
	assert.Equal(t, int32(2), *created.Spec.Replicas)
	require.Len(t, created.Spec.Template.Spec.Containers, 1)
	assert.Equal(t, "registry/triton:1", created.Spec.Template.Spec.Containers[0].Image)
	firstHash := created.Annotations[tritonSpecHashAnnotation]
	assert.NotEmpty(t, firstHash)
	probe := created.Spec.Template.Spec.Containers[0].ReadinessProbe
	require.NotNil(t, probe)
	require.NotNil(t, probe.Exec, "the default readiness probe is model-aware")
	assert.Equal(t, "python3", probe.Exec.Command[0])

	// The same spec again is a no-op.
	_, err = backend.CreateServer(ctx, zap.NewNop(), kubeClient, tritonInferenceServer("registry/triton:1", 2))
	require.NoError(t, err)
	unchanged := getTritonDeployment(t, kubeClient)
	assert.Equal(t, created.ResourceVersion, unchanged.ResourceVersion, "an unchanged spec must not be rewritten")

	// A new image rolls the Deployment and records the new hash.
	_, err = backend.CreateServer(ctx, zap.NewNop(), kubeClient, tritonInferenceServer("registry/triton:2", 3))
	require.NoError(t, err)
	updated := getTritonDeployment(t, kubeClient)
	assert.Equal(t, "registry/triton:2", updated.Spec.Template.Spec.Containers[0].Image)
	assert.Equal(t, int32(3), *updated.Spec.Replicas)
	assert.NotEqual(t, firstHash, updated.Annotations[tritonSpecHashAnnotation])
	assert.NotEqual(t, created.ResourceVersion, updated.ResourceVersion)
}

// A Deployment created by a controller that predates the spec hash has no annotation and is
// brought up to the current template once; that is the one-time Triton restart on upgrade.
func TestTritonCreateServerAdoptsALegacyDeployment(t *testing.T) {
	legacy := tritonDeployment(1)
	legacy.Spec.Template = corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": legacy.Name}},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "triton", Image: "registry/triton:old"}}},
	}
	kubeClient := fake.NewClientBuilder().WithScheme(newTritonScheme(t)).WithObjects(legacy).Build()

	_, err := NewTritonBackend("").CreateServer(context.Background(), zap.NewNop(), kubeClient, tritonInferenceServer("registry/triton:1", 1))

	require.NoError(t, err)
	adopted := getTritonDeployment(t, kubeClient)
	assert.NotEmpty(t, adopted.Annotations[tritonSpecHashAnnotation])
	assert.Equal(t, "registry/triton:1", adopted.Spec.Template.Spec.Containers[0].Image)
	assert.NotNil(t, adopted.Spec.Template.Spec.Containers[0].ReadinessProbe)
}

func TestTritonReadinessProbeModes(t *testing.T) {
	assert.Nil(t, NewTritonBackend("", WithReadinessProbe(TritonReadinessNone)).tritonReadinessProbe())

	serverProbe := NewTritonBackend("", WithReadinessProbe(TritonReadinessServer)).tritonReadinessProbe()
	require.NotNil(t, serverProbe)
	require.NotNil(t, serverProbe.HTTPGet)
	assert.Equal(t, "/v2/health/ready", serverProbe.HTTPGet.Path)

	for _, mode := range []string{"", TritonReadinessModelAware} {
		modelAware := NewTritonBackend("", WithReadinessProbe(mode)).tritonReadinessProbe()
		require.NotNil(t, modelAware)
		require.NotNil(t, modelAware.Exec, "mode %q", mode)
		assert.Equal(t, []string{"python3", "-c", tritonReadinessScript}, modelAware.Exec.Command)
	}
}

func TestTritonProbeTimingOverrides(t *testing.T) {
	defaults := NewTritonBackend("")
	assert.Equal(t, int32(60), defaults.tritonStartupProbe().FailureThreshold)
	assert.Equal(t, int32(6), defaults.tritonLivenessProbe().FailureThreshold)
	assert.Equal(t, int32(8), defaults.tritonReadinessProbe().TimeoutSeconds)

	tuned := NewTritonBackend("", WithProbeTimings(ProbeTimings{
		Startup:   ProbeTiming{PeriodSeconds: 2, FailureThreshold: 300},
		Liveness:  ProbeTiming{TimeoutSeconds: 9},
		Readiness: ProbeTiming{PeriodSeconds: 20, TimeoutSeconds: 15, FailureThreshold: 5},
	}))
	startup := tuned.tritonStartupProbe()
	assert.Equal(t, int32(2), startup.PeriodSeconds)
	assert.Equal(t, int32(300), startup.FailureThreshold)
	assert.Equal(t, int32(3), startup.TimeoutSeconds, "unset fields keep the built-in value")
	liveness := tuned.tritonLivenessProbe()
	assert.Equal(t, int32(9), liveness.TimeoutSeconds)
	assert.Equal(t, int32(10), liveness.PeriodSeconds)
	readiness := tuned.tritonReadinessProbe()
	assert.Equal(t, int32(20), readiness.PeriodSeconds)
	assert.Equal(t, int32(15), readiness.TimeoutSeconds)
	assert.Equal(t, int32(5), readiness.FailureThreshold)

	// Timing overrides never create a probe the mode disabled.
	none := NewTritonBackend("", WithReadinessProbe(TritonReadinessNone),
		WithProbeTimings(ProbeTimings{Readiness: ProbeTiming{PeriodSeconds: 20}}))
	assert.Nil(t, none.tritonReadinessProbe())
}

func TestTritonSpecHashIsStable(t *testing.T) {
	template := corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "triton-x"}},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "triton", Image: "img:1"}}},
	}
	assert.Equal(t, tritonSpecHash(2, template), tritonSpecHash(2, template))
	assert.NotEqual(t, tritonSpecHash(2, template), tritonSpecHash(3, template))

	changed := template.DeepCopy()
	changed.Spec.Containers[0].Image = "img:2"
	assert.NotEqual(t, tritonSpecHash(2, template), tritonSpecHash(2, *changed))
}

func TestTritonDrain(t *testing.T) {
	podOf := func(backend *tritonBackend) corev1.PodSpec {
		return backend.desiredTritonDeployment(tritonInferenceServer("registry/triton:1", 1)).Spec.Template.Spec
	}
	drained := func(pod corev1.PodSpec, preStop string, exitTimeout string, grace int64) {
		t.Helper()
		container := pod.Containers[0]
		require.NotNil(t, container.Lifecycle)
		require.NotNil(t, container.Lifecycle.PreStop)
		assert.Equal(t, []string{"sleep", preStop}, container.Lifecycle.PreStop.Exec.Command)
		assert.Contains(t, container.Args, "--exit-timeout-secs="+exitTimeout)
		require.NotNil(t, pod.TerminationGracePeriodSeconds)
		assert.Equal(t, grace, *pod.TerminationGracePeriodSeconds)
	}

	// Built-in values: 10s preStop + 30s exit timeout + 5s margin.
	drained(podOf(NewTritonBackend("")), "10", "30", 45)

	// Overrides replace the built-ins and the grace period follows them.
	drained(podOf(NewTritonBackend("", WithDrain(Drain{PreStopSeconds: 3, ExitTimeoutSeconds: 120}))), "3", "120", 128)

	// A partial override keeps the other built-in value.
	drained(podOf(NewTritonBackend("", WithDrain(Drain{ExitTimeoutSeconds: 60}))), "10", "60", 75)

	// Changing the drain changes the pod template, so the Deployment is rolled once.
	defaults := NewTritonBackend("").desiredTritonDeployment(tritonInferenceServer("registry/triton:1", 1))
	tuned := NewTritonBackend("", WithDrain(Drain{PreStopSeconds: 3})).desiredTritonDeployment(tritonInferenceServer("registry/triton:1", 1))
	assert.NotEqual(t, defaults.Annotations[tritonSpecHashAnnotation], tuned.Annotations[tritonSpecHashAnnotation])
}
