package cluster

// Integration tests across the RayCluster controller, the Kueue JobQueue, the
// assignment strategies and the Kueue state (RFC-20260731 Phases 0–2). Each
// runs controller manager processes over a fake API server, and in-memory
// compute clusters that the real REST clients reach through an in-process
// transport: no cluster, network or Docker is involved.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber-go/tally"
	"go.uber.org/fx"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kubescheme "k8s.io/client-go/kubernetes/scheme"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	api "github.com/michelangelo-ai/michelangelo/go/api"
	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	maconfig "github.com/michelangelo-ai/michelangelo/go/base/config"
	"github.com/michelangelo-ai/michelangelo/go/base/env"
	jobsclient "github.com/michelangelo-ai/michelangelo/go/components/jobs/client"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/client/k8sengine"
	jobscluster "github.com/michelangelo-ai/michelangelo/go/components/jobs/cluster"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	matypes "github.com/michelangelo-ai/michelangelo/go/components/jobs/common/types"
	jobsutils "github.com/michelangelo-ai/michelangelo/go/components/jobs/common/utils"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/compute"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/scheduler"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/scheduler/framework"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/scheduler/kueue"
	kueuestate "github.com/michelangelo-ai/michelangelo/go/components/jobs/scheduler/kueue/state"
	"github.com/michelangelo-ai/michelangelo/go/components/ray/kuberay"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

const (
	kueueA         = "kueue-a"
	kueueB         = "kueue-b"
	plainCluster   = "plain"
	noKueueCluster = "no-kueue"

	// fleetNamespace is the jobs' namespace, and so their project.
	fleetNamespace    = "proj"
	fleetClusterQueue = "cluster-queue"
	kueueAPIPath      = "/apis/kueue.x-k8s.io/v1beta2/"
)

var (
	// fleetLocalQueue is the jobs' LocalQueue: the default template applied
	// to their project.
	fleetLocalQueue = jobsutils.ResolveLocalQueueName(maconfig.KueueConfig{}, fleetNamespace)
	// rayClustersPath is where a compute cluster's KubeRay RayClusters live.
	rayClustersPath = "/apis/ray.io/v1/namespaces/" + k8sengine.RayLocalNamespace + "/rayclusters"
)

// computeCluster is an in-memory compute cluster that the real REST clients
// reach through its RoundTrip. It answers /healthz; KubeRay RayCluster
// creates, gets and deletes; and Kueue's reads: the ClusterQueue and
// LocalQueue lists the Kueue state watches, whose watches stay open without
// events, and the LocalQueue get. Errors carry the status codes and Status
// bodies an API server sends.
type computeCluster struct {
	name string
	// kueueManaged marks the Cluster that registers it as Kueue-managed.
	kueueManaged bool

	mu sync.Mutex
	// noKueue makes the Kueue API absent: every Kueue path returns 404.
	noKueue bool
	// stallLists keeps Kueue lists from returning, so the Kueue state never
	// syncs, while the LocalQueue get still answers.
	stallLists   bool
	unhealthy    bool
	clusterQueue map[string]any
	localQueue   map[string]any
	// failNext fails the next creates, in order; then failAlways, if set,
	// fails every create.
	failNext   []metav1.Status
	failAlways *metav1.Status
	creates    int
	kueueLists int
	objects    map[string]*unstructured.Unstructured
}

// newKueueCluster returns a Kueue-managed compute cluster with a
// single-flavor ClusterQueue of the given CPU quota, nothing reserved, fed by
// the jobs' active LocalQueue.
func newKueueCluster(name, cpu string) *computeCluster {
	c := &computeCluster{name: name, kueueManaged: true, objects: map[string]*unstructured.Unstructured{}}
	c.clusterQueue = map[string]any{
		"apiVersion": "kueue.x-k8s.io/v1beta2",
		"kind":       "ClusterQueue",
		"metadata":   map[string]any{"name": fleetClusterQueue, "resourceVersion": "1"},
		"spec": map[string]any{"resourceGroups": []any{map[string]any{
			"coveredResources": []any{"cpu", "memory"},
			"flavors": []any{map[string]any{"name": "default-flavor", "resources": []any{
				map[string]any{"name": "cpu", "nominalQuota": cpu},
				map[string]any{"name": "memory", "nominalQuota": "64Gi"},
			}}},
		}}},
		"status": map[string]any{"flavorsReservation": []any{}, "pendingWorkloads": 0},
	}
	c.localQueue = map[string]any{
		"apiVersion": "kueue.x-k8s.io/v1beta2",
		"kind":       "LocalQueue",
		"metadata": map[string]any{
			"name": fleetLocalQueue, "namespace": k8sengine.RayLocalNamespace, "resourceVersion": "1",
		},
		"spec":   map[string]any{"clusterQueue": fleetClusterQueue},
		"status": map[string]any{"conditions": []any{map[string]any{"type": "Active", "status": "True"}}},
	}
	return c
}

// newPlainCluster returns a compute cluster that Kueue does not manage.
func newPlainCluster(name string) *computeCluster {
	return &computeCluster{name: name, noKueue: true, objects: map[string]*unstructured.Unstructured{}}
}

// newNoKueueCluster returns a compute cluster marked Kueue-managed whose
// Kueue API is absent.
func newNoKueueCluster(name string) *computeCluster {
	c := newPlainCluster(name)
	c.kueueManaged = true
	return c
}

// cluster returns the Cluster that registers the compute cluster.
func (c *computeCluster) cluster() *v2pb.Cluster {
	cluster := &v2pb.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: c.name, Namespace: clusterNamespace},
		Spec: v2pb.ClusterSpec{
			Region: "phx",
			Cluster: &v2pb.ClusterSpec_Kubernetes{Kubernetes: &v2pb.KubernetesSpec{
				Rest: &v2pb.ConnectionSpec{Host: "https://" + c.name + ".compute.test", Port: "443"},
			}},
		},
	}
	if c.kueueManaged {
		cluster.Spec.SchedulerType = v2pb.SCHEDULER_TYPE_KUEUE
	}
	return cluster
}

// update changes the compute cluster's behavior.
func (c *computeCluster) update(change func(c *computeCluster)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	change(c)
}

// failCreates fails the next creates with the given errors, in order.
func (c *computeCluster) failCreates(statuses ...metav1.Status) {
	c.update(func(c *computeCluster) { c.failNext = append(c.failNext, statuses...) })
}

// putRayCluster stores a RayCluster of the given name that records the
// given owner annotations.
func (c *computeCluster) putRayCluster(name string, annotations map[string]string) {
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion("ray.io/v1")
	obj.SetKind("RayCluster")
	obj.SetNamespace(k8sengine.RayLocalNamespace)
	obj.SetName(name)
	obj.SetResourceVersion("1")
	obj.SetAnnotations(annotations)
	c.update(func(c *computeCluster) { c.objects[name] = obj })
}

// rayCluster returns the stored RayCluster of the given name, or nil.
func (c *computeCluster) rayCluster(name string) *unstructured.Unstructured {
	c.mu.Lock()
	defer c.mu.Unlock()
	if obj, ok := c.objects[name]; ok {
		return obj.DeepCopy()
	}
	return nil
}

// rayClusterNames returns the names of the stored RayClusters, sorted.
func (c *computeCluster) rayClusterNames() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	names := make([]string, 0, len(c.objects))
	for name := range c.objects {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (c *computeCluster) createCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.creates
}

func (c *computeCluster) kueueListCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.kueueLists
}

// RoundTrip serves a request. A stalled request returns when it is
// cancelled.
func (c *computeCluster) RoundTrip(req *http.Request) (*http.Response, error) {
	c.mu.Lock()
	resp, stall := c.serve(req)
	c.mu.Unlock()
	if stall {
		<-req.Context().Done()
		return nil, req.Context().Err()
	}
	return resp, nil
}

func (c *computeCluster) serve(req *http.Request) (resp *http.Response, stall bool) {
	switch path := req.URL.Path; {
	case path == "/healthz":
		if c.unhealthy {
			return statusResponse(req, apiErrors.NewServiceUnavailable("healthz check failed").ErrStatus), false
		}
		return response(req, http.StatusOK, "text/plain", []byte("ok")), false
	case strings.HasPrefix(path, kueueAPIPath):
		return c.serveKueue(req, strings.TrimPrefix(path, kueueAPIPath))
	case path == rayClustersPath && req.Method == http.MethodPost:
		return c.create(req), false
	case strings.HasPrefix(path, rayClustersPath+"/"):
		return c.serveRayCluster(req, strings.TrimPrefix(path, rayClustersPath+"/")), false
	}
	return statusResponse(req, apiErrors.NewNotFound(rayClustersResource, req.URL.Path).ErrStatus), false
}

// serveKueue serves the Kueue reads: the queue lists and watches, and the
// LocalQueue get.
func (c *computeCluster) serveKueue(req *http.Request, path string) (*http.Response, bool) {
	localQueues := "namespaces/" + k8sengine.RayLocalNamespace + "/localqueues"
	var list map[string]any
	switch path {
	case "clusterqueues":
		list = queueList("ClusterQueueList", c.clusterQueue)
	case localQueues:
		list = queueList("LocalQueueList", c.localQueue)
	case localQueues + "/" + fleetLocalQueue:
		if c.noKueue || c.localQueue == nil {
			return kueueNotFound(req), false
		}
		return jsonResponse(req, http.StatusOK, c.localQueue), false
	default:
		return kueueNotFound(req), false
	}
	watching := req.URL.Query().Get("watch") == "true"
	if !watching {
		c.kueueLists++
	}
	switch {
	case c.noKueue:
		return kueueNotFound(req), false
	case watching:
		resp := response(req, http.StatusOK, "application/json", nil)
		resp.Body = &openStream{ctx: req.Context(), closed: make(chan struct{})}
		return resp, false
	case c.stallLists:
		return nil, true
	}
	return jsonResponse(req, http.StatusOK, list), false
}

// create stores a RayCluster, unless a failure is due or one of its name
// exists.
func (c *computeCluster) create(req *http.Request) *http.Response {
	c.creates++
	switch {
	case len(c.failNext) > 0:
		status := c.failNext[0]
		c.failNext = c.failNext[1:]
		return statusResponse(req, status)
	case c.failAlways != nil:
		return statusResponse(req, *c.failAlways)
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return statusResponse(req, apiErrors.NewBadRequest(err.Error()).ErrStatus)
	}
	obj := &unstructured.Unstructured{}
	if err := obj.UnmarshalJSON(body); err != nil {
		return statusResponse(req, apiErrors.NewBadRequest(err.Error()).ErrStatus)
	}
	if _, ok := c.objects[obj.GetName()]; ok {
		return statusResponse(req, apiErrors.NewAlreadyExists(rayClustersResource, obj.GetName()).ErrStatus)
	}
	obj.SetResourceVersion("1")
	c.objects[obj.GetName()] = obj
	return jsonResponse(req, http.StatusCreated, obj.Object)
}

// serveRayCluster serves the get and the delete of a RayCluster.
func (c *computeCluster) serveRayCluster(req *http.Request, name string) *http.Response {
	obj, ok := c.objects[name]
	if !ok {
		return statusResponse(req, apiErrors.NewNotFound(rayClustersResource, name).ErrStatus)
	}
	switch req.Method {
	case http.MethodGet:
		return jsonResponse(req, http.StatusOK, obj.Object)
	case http.MethodDelete:
		delete(c.objects, name)
		return jsonResponse(req, http.StatusOK, map[string]any{"kind": "Status", "apiVersion": "v1", "status": "Success"})
	}
	return statusResponse(req, apiErrors.NewMethodNotSupported(rayClustersResource, req.Method).ErrStatus)
}

// queueList is a list of the given kind holding item, if any.
func queueList(kind string, item map[string]any) map[string]any {
	items := []any{}
	if item != nil {
		items = append(items, item)
	}
	return map[string]any{
		"apiVersion": "kueue.x-k8s.io/v1beta2", "kind": kind,
		"metadata": map[string]any{"resourceVersion": "1"}, "items": items,
	}
}

// kueueNotFound is what an API server without Kueue's resources answers on
// their paths.
func kueueNotFound(req *http.Request) *http.Response {
	return statusResponse(req, metav1.Status{
		Code: http.StatusNotFound, Reason: metav1.StatusReasonNotFound,
		Message: "the server could not find the requested resource",
	})
}

func statusResponse(req *http.Request, status metav1.Status) *http.Response {
	status.TypeMeta = metav1.TypeMeta{Kind: "Status", APIVersion: "v1"}
	status.Status = metav1.StatusFailure
	return jsonResponse(req, int(status.Code), status)
}

func jsonResponse(req *http.Request, code int, obj any) *http.Response {
	body, err := json.Marshal(obj)
	if err != nil {
		panic(err)
	}
	return response(req, code, "application/json", body)
}

func response(req *http.Request, code int, contentType string, body []byte) *http.Response {
	return &http.Response{
		Status:        fmt.Sprintf("%d %s", code, http.StatusText(code)),
		StatusCode:    code,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        http.Header{"Content-Type": []string{contentType}},
		Body:          io.NopCloser(bytes.NewReader(body)),
		ContentLength: int64(len(body)),
		Request:       req,
	}
}

// openStream is a watch response body that stays open, without events,
// until the watch stops.
type openStream struct {
	ctx    context.Context
	closed chan struct{}
	once   sync.Once
}

func (s *openStream) Read([]byte) (int, error) {
	select {
	case <-s.ctx.Done():
	case <-s.closed:
	}
	return 0, io.EOF
}

func (s *openStream) Close() error {
	s.once.Do(func() { close(s.closed) })
	return nil
}

// computeFleet is the compute.Factory of the in-memory compute clusters: it
// builds the real REST clients over each cluster's RoundTrip.
type computeFleet map[string]*computeCluster

var _ compute.Factory = computeFleet{}

func (f computeFleet) GetRestConfigForCluster(c *v2pb.Cluster) (*rest.Config, error) {
	cc, ok := f[c.GetName()]
	if !ok {
		return nil, fmt.Errorf("no compute cluster %q", c.GetName())
	}
	// a negative QPS turns client-side rate limiting off
	return &rest.Config{Host: "https://" + cc.name + ".compute.test", Transport: cc, QPS: -1}, nil
}

func (f computeFleet) GetClientSetForCluster(c *v2pb.Cluster) (*compute.ClientSet, error) {
	cfg, err := f.GetRestConfigForCluster(c)
	if err != nil {
		return nil, err
	}
	ray, err := kuberay.NewRestClient(cfg)
	if err != nil {
		return nil, err
	}
	core, err := corev1client.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	return &compute.ClientSet{Ray: ray, CoreV1: core.RESTClient()}, nil
}

func (computeFleet) Invalidate(*v2pb.ClusterSpec) {}

// webhookDown is what an API server answers a create with while KubeRay's
// admission webhook cannot be called.
func webhookDown() metav1.Status {
	return apiErrors.NewInternalError(errors.New(`failed calling webhook "mraycluster.kb.io": failed to call webhook: ` +
		`Post "https://kuberay-webhook-service.ray-system.svc:443/mutate-ray-io-v1-raycluster?timeout=10s": ` +
		`dial tcp 10.96.12.7:443: connect: connection refused`)).ErrStatus
}

// serverUnavailable is what an API server that cannot serve a create
// answers, e.g. while it restarts.
func serverUnavailable() metav1.Status {
	return apiErrors.NewServiceUnavailable("the server is currently unable to handle the request").ErrStatus
}

// fleetHarness runs controller manager processes over a fake API server and
// in-memory compute clusters, under a test clock. A process holds what one
// controller manager process does: its Kueue state, cluster controller, Kueue
// JobQueue and RayCluster controller. A new process over the same API objects
// and compute clusters is a leader failover.
type fleetHarness struct {
	t        *testing.T
	ctx      context.Context
	cfg      maconfig.SchedulerConfig
	mapper   matypes.Mapper
	computes computeFleet
	server   client.WithWatch
	// indexes are the field indexes the Kueue state registered on the
	// manager's cache, which the fake API server indexes RayClusters by.
	indexes []fleetIndex
	now     time.Time
	proc    *fleetProcess
}

type fleetIndex struct {
	obj     client.Object
	field   string
	extract client.IndexerFunc
}

type fleetProcess struct {
	state    *kueuestate.Service
	clusters *jobscluster.Reconciler
	rcc      *Reconciler
	cancel   context.CancelFunc
}

// newFleetHarness registers the compute clusters and starts a process under
// the given scheduler backend and assignment strategy.
func newFleetHarness(t *testing.T, backend, strategy string, clusters ...*computeCluster) *fleetHarness {
	t.Helper()
	jobsclient.RegisterDispatchMetrics()
	cfg := maconfig.SchedulerConfig{Backend: backend, AssignmentStrategy: strategy}
	h := &fleetHarness{
		t:        t,
		ctx:      log.IntoContext(context.Background(), logr.Discard()),
		cfg:      cfg,
		mapper:   k8sengine.NewMapper(k8sengine.LogPersistenceConfig{}, cfg).Mapper,
		computes: computeFleet{},
		// Condition timestamps are real; the test clock starts at the real
		// time, so jobs launched during a test count as just launched.
		now: time.Now().UTC().Truncate(time.Second),
	}
	for _, c := range clusters {
		h.computes[c.name] = c
	}
	h.startProcess()
	t.Cleanup(func() { h.proc.cancel() })
	for _, c := range clusters {
		require.NoError(t, h.server.Create(h.ctx, c.cluster()))
	}
	h.registerClusters()
	return h
}

// failover replaces the process: the new one starts with empty process-local
// state, as a new leader does.
func (h *fleetHarness) failover() {
	h.t.Helper()
	h.startProcess()
	h.registerClusters()
}

// startProcess starts a process, stopping the current one. The first process
// builds the fake API server, with the indexes its Kueue state registered.
func (h *fleetHarness) startProcess() {
	h.t.Helper()
	if h.proc != nil {
		h.proc.cancel()
	}
	ctx, cancel := context.WithCancel(h.ctx)
	p := &fleetProcess{cancel: cancel}
	h.proc = p
	p.state = h.startKueueState(ctx)
	if h.server == nil {
		h.buildServer()
	}

	handler := apiHandler.NewFakeAPIHandler(h.server)
	federated := jobsclient.NewClient(jobsclient.Params{
		Factory: h.computes, Logger: zap.NewNop(), Mapper: h.mapper, Helper: jobsclient.NewHelper(),
	})
	p.clusters = jobscluster.NewReconciler(jobscluster.Params{
		ClusterClient:  federated,
		ComputeFactory: h.computes,
		Scope:          tally.NoopScope,
		KueueWatches:   p.state,
	}).Reconciler
	p.clusters.Handler = handler // as SetupWithManager sets it

	var queue scheduler.JobQueue = &mockSchedulerQueue{} // the default scheduler is not under test
	if h.cfg.Backend == backendKueue {
		queue = h.kueueJobQueue(p, handler)
	}
	p.rcc = NewReconciler(logr.Discard(), nil, env.Context{}, queue, federated, p.clusters,
		WithSchedulerConfig(h.cfg), WithClusterReadiness(jobscluster.NewClusterReadiness(p.clusters)))
	// as Register sets them from the manager
	p.rcc.Handler = handler
	p.rcc.apiReader = h.server
	p.rcc.clusterReader = h.server
	p.rcc.now = h.clock
}

// startKueueState builds the Kueue state through its fx module, as the
// controller manager does, and runs what the module adds to the manager.
func (h *fleetHarness) startKueueState(ctx context.Context) *kueuestate.Service {
	h.t.Helper()
	mgr := &fleetManager{h: h}
	var svc *kueuestate.Service
	app := fx.New(
		fx.NopLogger,
		fx.Supply(h.cfg, logr.Discard()),
		fx.Provide(
			func() compute.Factory { return h.computes },
			func() manager.Manager { return mgr },
			fx.Annotate(func() matypes.Mapper { return h.mapper }, fx.ResultTags(`name:"k8sengineMapper"`)),
		),
		kueuestate.Module,
		fx.Populate(&svc),
	)
	require.NoError(h.t, app.Err())
	for _, r := range mgr.runnables {
		go func() { _ = r.Start(ctx) }()
	}
	return svc
}

// kueueJobQueue builds the Kueue JobQueue as the scheduler module does for
// the configured assignment strategy.
func (h *fleetHarness) kueueJobQueue(p *fleetProcess, handler api.Handler) scheduler.JobQueue {
	queues := kueue.NewLocalQueues(h.computes, h.cfg)
	var opts []kueue.Option
	switch h.cfg.AssignmentStrategy {
	case maconfig.AssignmentStrategyKueueAware, maconfig.AssignmentStrategyKueueAwareShadow:
		strategy := kueue.NewKueueAwareStrategy(kueue.KueueAwareParams{
			ClusterCache: p.clusters,
			LocalQueues:  queues,
			State:        p.state,
			Demand:       p.state,
			Config:       h.cfg,
			Logger:       logr.Discard(),
			Now:          h.clock,
		})
		shadow := h.cfg.AssignmentStrategy == maconfig.AssignmentStrategyKueueAwareShadow
		opts = append(opts, kueue.WithKueueAwareStrategy(strategy, shadow))
	}
	return kueue.NewKueueJobQueue(kueue.Params{
		Handler:            handler,
		AssignmentStrategy: framework.NewClusterOnlyAssignmentStrategy(p.clusters, logr.Discard()),
		ClusterCache:       p.clusters,
		LocalQueues:        queues,
		Config:             h.cfg,
		Scope:              tally.NoopScope,
		Logger:             logr.Discard(),
	}, opts...)
}

func (h *fleetHarness) buildServer() {
	scheme := runtime.NewScheme()
	require.NoError(h.t, kubescheme.AddToScheme(scheme))
	require.NoError(h.t, v2pb.AddToScheme(scheme))
	builder := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&v2pb.RayCluster{}, &v2pb.Cluster{})
	for _, index := range h.indexes {
		builder = builder.WithIndex(index.obj, index.field, index.extract)
	}
	h.server = builder.Build()
}

// registerClusters has the process's cluster controller add and probe every
// Cluster, and waits for the Kueue state of the Kueue-managed clusters that
// serve their queues.
func (h *fleetHarness) registerClusters() {
	h.t.Helper()
	for name, c := range h.computes {
		h.reconcileCluster(name)
		c.mu.Lock()
		served := c.kueueManaged && !c.noKueue && !c.stallLists
		c.mu.Unlock()
		if served && h.cfg.Backend == backendKueue {
			require.Eventually(h.t, func() bool { return h.proc.state.Synced(name) }, 10*time.Second, 5*time.Millisecond,
				"the Kueue state of cluster %s did not sync", name)
		}
	}
}

func (h *fleetHarness) reconcileCluster(name string) {
	h.t.Helper()
	_, err := h.proc.clusters.Reconcile(h.ctx, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: clusterNamespace, Name: name}})
	require.NoError(h.t, err)
}

// markNotReady fails the cluster's /healthz and has the cluster controller
// probe it again, as its periodic monitor would: an edit of the Cluster's
// spec makes it probe.
func (h *fleetHarness) markNotReady(name string) {
	h.t.Helper()
	h.computes[name].update(func(c *computeCluster) { c.unhealthy = true })
	cluster := &v2pb.Cluster{}
	require.NoError(h.t, h.server.Get(h.ctx, types.NamespacedName{Namespace: clusterNamespace, Name: name}, cluster))
	cluster.Spec.Region = "probed-again"
	require.NoError(h.t, h.server.Update(h.ctx, cluster))
	h.reconcileCluster(name)
	require.Equal(h.t, jobscluster.ReadinessNotReady, h.proc.clusters.GetClusterReadiness(name))
}

// waitUnsynced waits until the Kueue state has listed the cluster's queues
// and checks that the cluster's Kueue state is not synced.
func (h *fleetHarness) waitUnsynced(name string) {
	h.t.Helper()
	require.Eventually(h.t, func() bool { return h.computes[name].kueueListCount() >= 2 }, 10*time.Second, 5*time.Millisecond,
		"the Kueue state did not list the queues of cluster %s", name)
	assert.False(h.t, h.proc.state.Synced(name), "the Kueue state of cluster %s synced", name)
}

func (h *fleetHarness) clock() time.Time {
	return h.now
}

func (h *fleetHarness) advance(d time.Duration) {
	h.now = h.now.Add(d)
}

// submit creates a head-only RayCluster asking for one CPU, with the given
// labels.
func (h *fleetHarness) submit(name string, labels map[string]string) {
	h.t.Helper()
	rc := &v2pb.RayCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: fleetNamespace, Labels: labels,
			UID: types.UID(name + "-uid"), Generation: 1, CreationTimestamp: metav1.NewTime(h.now),
		},
		Spec: v2pb.RayClusterSpec{
			RayVersion: "2.9.0",
			Head: &v2pb.RayHeadSpec{
				ServiceType: "ClusterIP",
				Pod: &corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{
					Name:  "ray-head",
					Image: "rayproject/ray:2.9.0",
					Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{
						corev1.ResourceCPU: resource.MustParse("1"),
					}},
				}}}},
			},
		},
	}
	require.NoError(h.t, h.server.Create(h.ctx, rc))
}

// placeOn schedules the job on target, as the default scheduler would.
func (h *fleetHarness) placeOn(name, target string) {
	h.t.Helper()
	rc := h.get(name)
	rc.Status.Assignment = &v2pb.AssignmentInfo{Cluster: target}
	scheduled := jobsutils.GetCondition(&rc.Status.StatusConditions, ScheduledCondition, rc.Generation)
	jobsutils.UpdateCondition(scheduled, jobsutils.ConditionUpdateParams{Status: apipb.CONDITION_STATUS_TRUE, Generation: rc.Generation})
	require.NoError(h.t, h.server.Status().Update(h.ctx, rc))
}

func (h *fleetHarness) reconcile(name string) (ctrl.Result, error) {
	return h.proc.rcc.Reconcile(h.ctx, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: fleetNamespace, Name: name}})
}

func (h *fleetHarness) reconcileOK(name string) ctrl.Result {
	h.t.Helper()
	res, err := h.reconcile(name)
	require.NoError(h.t, err)
	return res
}

// reconcileUntilCreates reconciles the job, at most three times, until the
// compute clusters have seen the given number of creates in all, and returns
// the last result.
func (h *fleetHarness) reconcileUntilCreates(name string, creates int) ctrl.Result {
	h.t.Helper()
	var res ctrl.Result
	for i := 0; i < 3 && h.creates() < creates; i++ {
		res = h.reconcileOK(name)
	}
	require.Equal(h.t, creates, h.creates(), "creates of %s", name)
	return res
}

// reconcileUntilLaunched reconciles the job, at most three times, until it
// launches.
func (h *fleetHarness) reconcileUntilLaunched(name string) {
	h.t.Helper()
	for i := 0; i < 3 && launchedStatus(h.get(name)) != apipb.CONDITION_STATUS_TRUE; i++ {
		h.reconcileOK(name)
	}
	require.Equal(h.t, apipb.CONDITION_STATUS_TRUE, launchedStatus(h.get(name)), "%s did not launch", name)
}

func (h *fleetHarness) creates() int {
	var creates int
	for _, c := range h.computes {
		creates += c.createCount()
	}
	return creates
}

func (h *fleetHarness) get(name string) *v2pb.RayCluster {
	h.t.Helper()
	rc := &v2pb.RayCluster{}
	require.NoError(h.t, h.server.Get(h.ctx, types.NamespacedName{Namespace: fleetNamespace, Name: name}, rc))
	return rc
}

// dispatchState returns the dispatch record in the job's Launched condition.
func (h *fleetHarness) dispatchState(name string) jobsutils.DispatchState {
	h.t.Helper()
	state, err := jobsutils.DispatchStateFromCondition(jobsutils.FindCondition(h.get(name).Status.StatusConditions, LaunchedCondition))
	require.NoError(h.t, err)
	return state
}

// inFlight returns the names of the jobs that the process's Kueue state
// counts in flight to the cluster, sorted.
func (h *fleetHarness) inFlight(cluster string) []string {
	h.t.Helper()
	inFlight, err := h.proc.state.InFlight(h.ctx, h.proc.clusters.GetCluster(cluster), h.now)
	require.NoError(h.t, err)
	var names []string
	for _, job := range inFlight.Jobs {
		names = append(names, job.RayCluster.GetName())
	}
	sort.Strings(names)
	return names
}

// other returns the name of the compute cluster other than name, of two.
func (h *fleetHarness) other(name string) string {
	h.t.Helper()
	require.Len(h.t, h.computes, 2)
	for other := range h.computes {
		if other != name {
			return other
		}
	}
	return ""
}

// fleetManager is the part of the controller manager that the Kueue state
// uses: the cache it reads RayClusters from, the field indexer it registers
// its index with, and the runnables it adds.
type fleetManager struct {
	manager.Manager
	h         *fleetHarness
	runnables []manager.Runnable
}

func (m *fleetManager) GetCache() cache.Cache { return fleetCache{h: m.h} }

func (m *fleetManager) GetFieldIndexer() client.FieldIndexer { return m }

// IndexField records an index for the fake API server, which a later process
// finds built in.
func (m *fleetManager) IndexField(_ context.Context, obj client.Object, field string, extract client.IndexerFunc) error {
	if m.h.server == nil {
		m.h.indexes = append(m.h.indexes, fleetIndex{obj: obj, field: field, extract: extract})
	}
	return nil
}

func (m *fleetManager) Add(r manager.Runnable) error {
	m.runnables = append(m.runnables, r)
	return nil
}

// fleetCache reads the fake API server, as the manager's cache would.
type fleetCache struct {
	cache.Cache
	h *fleetHarness
}

func (c fleetCache) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	return c.h.server.Get(ctx, key, obj, opts...)
}

func (c fleetCache) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	return c.h.server.List(ctx, list, opts...)
}

// scheduledMessage returns the message of the job's Scheduled condition.
func scheduledMessage(rc *v2pb.RayCluster) string {
	return jobsutils.FindCondition(rc.Status.StatusConditions, ScheduledCondition).GetMessage()
}

// attemptsInWindow is the number of creates a job makes on a target whose
// creates keep failing: one every backoff, until one finds the window over.
func attemptsInWindow() int {
	attempts, elapsed := 1, time.Duration(0)
	for elapsed < jobsutils.DispatchWindow {
		elapsed += jobsutils.DispatchBackoff(attempts)
		attempts++
	}
	return attempts
}

// TestKueueAwareBurstSpreadsByInFlightDemandAcrossAFailover submits a burst
// of jobs under kueue_aware to two Kueue-managed clusters with skewed quota,
// and fails the leader over halfway: the new process, with an empty overlay,
// counts the jobs placed before the failover in flight from their persisted
// assignments, so the burst keeps spreading by quota.
func TestKueueAwareBurstSpreadsByInFlightDemandAcrossAFailover(t *testing.T) {
	h := newFleetHarness(t, backendKueue, maconfig.AssignmentStrategyKueueAware,
		newKueueCluster(kueueA, "4"), newKueueCluster(kueueB, "12"))

	// Each job asks for one CPU, and fits in either cluster's free quota.
	// Tier 1 ranks by slack, the CPU a cluster has left after the job over its
	// nominal quota, counting every earlier job in flight; ties break by
	// name. Ranked by snapshot alone, every job would go to kueue-b, 11/12
	// over 3/4.
	want := []string{
		kueueB, kueueB, kueueA, kueueB, kueueB, kueueB,
		// The seventh job, burst-06, ties kueue-a's 2/4 with kueue-b's
		// 6/12 only if the jobs placed before the failover still count.
		kueueA, kueueB, kueueB, kueueB, kueueA, kueueB,
	}
	var got []string
	for i := range want {
		if i == len(want)/2 {
			h.failover()
			assert.Equal(t, []string{"burst-02"}, h.inFlight(kueueA))
			assert.Equal(t, []string{"burst-00", "burst-01", "burst-03", "burst-04", "burst-05"}, h.inFlight(kueueB))
		}
		name := fmt.Sprintf("burst-%02d", i)
		h.submit(name, nil)
		h.reconcileUntilLaunched(name)
		rc := h.get(name)
		got = append(got, rc.Status.Assignment.GetCluster())
		assert.Equal(t, "cluster "+rc.Status.Assignment.GetCluster()+", tier 1", scheduledMessage(rc))
	}
	assert.Equal(t, want, got)

	// No pile-up: each cluster holds three quarters of its quota, and runs
	// the jobs placed on it, in their LocalQueue.
	for cluster, jobs := range map[string]int{kueueA: 3, kueueB: 9} {
		names := h.computes[cluster].rayClusterNames()
		assert.Len(t, names, jobs, "jobs on %s", cluster)
		for _, name := range names {
			assert.Equal(t, fleetLocalQueue, h.computes[cluster].rayCluster(name).GetLabels()[constants.KueueQueueNameLabelKey])
		}
	}
}

// TestWebhookUnavailabilityRetriesInPlaceThenLaunches fails a job's creates
// with a webhook-call 500 and then a 503, and lets the third succeed: the job
// retries in place on its Ready target, with the backoff and attempts
// persisted in Launched=False/ClusterCreationFailed, and then launches.
func TestWebhookUnavailabilityRetriesInPlaceThenLaunches(t *testing.T) {
	tests := []struct {
		name    string
		backend string
		target  *computeCluster
	}{
		{name: "kueue backend", backend: backendKueue, target: newKueueCluster(kueueA, "4")},
		{name: "default backend", backend: "", target: newPlainCluster(plainCluster)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newFleetHarness(t, tt.backend, maconfig.AssignmentStrategyClusterOnly, tt.target)
			tt.target.failCreates(webhookDown(), serverUnavailable())
			rejected := createRetries(t, jobsclient.DispatchRejected)
			start := h.clock()

			h.submit("webhook", nil)
			if tt.backend == "" {
				h.placeOn("webhook", tt.target.name)
			}
			assert.Equal(t, 5*time.Second, h.reconcileUntilCreates("webhook", 1).RequeueAfter)
			rc := h.get("webhook")
			assert.Equal(t, tt.target.name, rc.Status.Assignment.GetCluster())
			launched := jobsutils.FindCondition(rc.Status.StatusConditions, LaunchedCondition)
			assert.Equal(t, apipb.CONDITION_STATUS_FALSE, launched.GetStatus())
			assert.Equal(t, clusterCreationFailedReason, launched.GetReason())
			assert.Contains(t, launched.GetMessage(), "failed calling webhook")
			assert.Equal(t, jobsutils.DispatchState{WindowStart: start, Attempts: 1, NextAttemptAt: start.Add(5 * time.Second)},
				h.dispatchState("webhook"))
			assert.Equal(t, rejected+1, createRetries(t, jobsclient.DispatchRejected))

			// the attempt's own status event creates nothing until the backoff ends
			assert.Equal(t, 5*time.Second, h.reconcileOK("webhook").RequeueAfter)
			assert.Equal(t, 1, tt.target.createCount())

			h.advance(5 * time.Second)
			assert.Equal(t, 10*time.Second, h.reconcileOK("webhook").RequeueAfter)
			assert.Equal(t, 2, tt.target.createCount())
			rc = h.get("webhook")
			assert.Equal(t, tt.target.name, rc.Status.Assignment.GetCluster())
			launched = jobsutils.FindCondition(rc.Status.StatusConditions, LaunchedCondition)
			assert.Equal(t, apipb.CONDITION_STATUS_FALSE, launched.GetStatus())
			assert.Equal(t, clusterCreationFailedReason, launched.GetReason())
			assert.Contains(t, launched.GetMessage(), "unable to handle the request")
			assert.Equal(t, jobsutils.DispatchState{WindowStart: start, Attempts: 2, NextAttemptAt: start.Add(15 * time.Second)},
				h.dispatchState("webhook"))
			assert.Equal(t, rejected+2, createRetries(t, jobsclient.DispatchRejected))

			h.advance(10 * time.Second)
			h.reconcileOK("webhook")
			assert.Equal(t, 3, tt.target.createCount())
			rc = h.get("webhook")
			assert.Equal(t, tt.target.name, rc.Status.Assignment.GetCluster())
			launched = jobsutils.FindCondition(rc.Status.StatusConditions, LaunchedCondition)
			assert.Equal(t, apipb.CONDITION_STATUS_TRUE, launched.GetStatus())
			assert.Equal(t, clusterCreatedReason, launched.GetReason())
			assert.True(t, h.dispatchState("webhook").IsZero(), "the dispatch record outlived the launch")
			created := tt.target.rayCluster("webhook")
			require.NotNil(t, created)
			assert.Equal(t, map[string]string{
				constants.OwnerAnnotationKey:    fleetNamespace + "/webhook",
				constants.OwnerUIDAnnotationKey: "webhook-uid",
			}, created.GetAnnotations())
		})
	}
}

// TestKueueReplacesAJobOffATargetThatIsNotReady places a job, has its target
// go not Ready before the create, and checks that under the Kueue backend the
// job goes back through the Kueue JobQueue to the other cluster, without a
// create on its target.
func TestKueueReplacesAJobOffATargetThatIsNotReady(t *testing.T) {
	for _, strategy := range []string{maconfig.AssignmentStrategyClusterOnly, maconfig.AssignmentStrategyKueueAware} {
		t.Run(strategy, func(t *testing.T) {
			h := newFleetHarness(t, backendKueue, strategy, newKueueCluster(kueueA, "4"), newKueueCluster(kueueB, "4"))
			notReady := replacements(t, jobsclient.ReplacementReasonTargetNotReady)

			h.submit("moved", nil)
			h.reconcileOK("moved")
			from := h.get("moved").Status.Assignment.GetCluster()
			require.NotEmpty(t, from, "the job was not placed")
			to := h.other(from)

			h.markNotReady(from)
			h.reconcileOK("moved")
			rc := h.get("moved")
			assert.Nil(t, rc.Status.Assignment)
			assert.Equal(t, "re-placing from "+from+": target not ready", scheduledMessage(rc))
			state := h.dispatchState("moved")
			assert.Equal(t, []string{from}, state.ExcludedClusters)
			assert.Equal(t, 1, state.Replacements)
			assert.Equal(t, notReady+1, replacements(t, jobsclient.ReplacementReasonTargetNotReady))

			h.reconcileOK("moved")
			assert.Equal(t, to, h.get("moved").Status.Assignment.GetCluster())
			// the re-placement backs off before the next create
			h.advance(state.BackoffRemaining(h.clock()))
			h.reconcileUntilLaunched("moved")
			assert.Equal(t, to, h.get("moved").Status.Assignment.GetCluster())
			assert.Zero(t, h.computes[from].createCount(), "a create reached the target that is not Ready")
			assert.Equal(t, []string{"moved"}, h.computes[to].rayClusterNames())
		})
	}
}

// TestKueueReplacesAJobAtTheWindowEndExcludingItsTarget keeps a Ready
// target's webhook failing past the create window: under the Kueue backend
// the job is re-placed, its target recorded as excluded (RFC 0f, D53), and the
// fresh decision picks another cluster, unless the job is pinned or no other
// cluster is left.
func TestKueueReplacesAJobAtTheWindowEndExcludingItsTarget(t *testing.T) {
	tests := []struct {
		name     string
		clusters []string
		labels   map[string]string
		// wantBack is whether the job goes back to the cluster it left.
		wantBack bool
	}{
		{name: "goes to the other cluster", clusters: []string{kueueA, kueueB}},
		{
			name:     "a pinned job keeps its cluster",
			clusters: []string{kueueA, kueueB},
			labels:   map[string]string{constants.ClusterAffinityLabelKey: kueueA},
			wantBack: true,
		},
		{name: "with one cluster, goes back to it", clusters: []string{kueueA}, wantBack: true},
	}
	for _, strategy := range []string{maconfig.AssignmentStrategyClusterOnly, maconfig.AssignmentStrategyKueueAware} {
		for _, tt := range tests {
			t.Run(strategy+"/"+tt.name, func(t *testing.T) {
				var clusters []*computeCluster
				for _, name := range tt.clusters {
					c := newKueueCluster(name, "4")
					down := webhookDown()
					c.failAlways = &down
					clusters = append(clusters, c)
				}
				h := newFleetHarness(t, backendKueue, strategy, clusters...)
				windowExpired := replacements(t, jobsclient.ReplacementReasonWindowExpired)

				h.submit("stuck", tt.labels)
				h.reconcileOK("stuck")
				from := h.get("stuck").Status.Assignment.GetCluster()
				require.NotEmpty(t, from, "the job was not placed")
				if tt.labels != nil {
					require.Equal(t, kueueA, from, "the pin was not kept")
				}

				for i := 0; h.get("stuck").Status.Assignment != nil; i++ {
					require.Less(t, i, 2*attemptsInWindow(), "the job was not re-placed")
					h.advance(h.reconcileOK("stuck").RequeueAfter)
				}
				assert.Equal(t, attemptsInWindow(), h.computes[from].createCount())
				assert.Equal(t, "re-placing from "+from+": window expired", scheduledMessage(h.get("stuck")))
				state := h.dispatchState("stuck")
				assert.Equal(t, []string{from}, state.ExcludedClusters)
				assert.Equal(t, windowExpired+1, replacements(t, jobsclient.ReplacementReasonWindowExpired))

				h.reconcileOK("stuck")
				to := h.get("stuck").Status.Assignment.GetCluster()
				if tt.wantBack {
					assert.Equal(t, from, to)
				} else {
					assert.Equal(t, h.other(from), to)
				}
				for _, c := range clusters {
					c.update(func(c *computeCluster) { c.failAlways = nil })
				}
				h.advance(state.BackoffRemaining(h.clock()))
				h.reconcileUntilLaunched("stuck")
				assert.Equal(t, to, h.get("stuck").Status.Assignment.GetCluster())
				assert.Equal(t, []string{"stuck"}, h.computes[to].rayClusterNames())
			})
		}
	}
}

// TestKueueDispatchChecksTheOwnerOfAnExistingCluster has the job's create
// meet AlreadyExists (RFC 0f, D53): another job's object fails the job at
// once, without a retry or a re-placement; the job's own object, or one
// without an owner, is adopted, and the job launches.
func TestKueueDispatchChecksTheOwnerOfAnExistingCluster(t *testing.T) {
	tests := []struct {
		name         string
		owner        map[string]string
		wantLaunched bool
	}{
		{
			name:  "another job's cluster fails the job",
			owner: map[string]string{constants.OwnerAnnotationKey: "other/existing", constants.OwnerUIDAnnotationKey: "other-uid"},
		},
		{
			name:         "the job's own cluster is adopted",
			owner:        map[string]string{constants.OwnerAnnotationKey: fleetNamespace + "/existing", constants.OwnerUIDAnnotationKey: "existing-uid"},
			wantLaunched: true,
		},
		{name: "a cluster without an owner is adopted", wantLaunched: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, b := newKueueCluster(kueueA, "4"), newKueueCluster(kueueB, "4")
			// either cluster the job is placed on holds an object of its name
			a.putRayCluster("existing", tt.owner)
			b.putRayCluster("existing", tt.owner)
			h := newFleetHarness(t, backendKueue, maconfig.AssignmentStrategyKueueAware, a, b)
			retries := createRetries(t, jobsclient.DispatchRejected) + createRetries(t, jobsclient.DispatchAmbiguous)

			h.submit("existing", nil)
			h.reconcileUntilCreates("existing", 1)
			rc := h.get("existing")
			target := rc.Status.Assignment.GetCluster()
			require.NotEmpty(t, target)
			if tt.wantLaunched {
				assert.Equal(t, apipb.CONDITION_STATUS_TRUE, launchedStatus(rc))
			} else {
				assert.Equal(t, v2pb.RAY_CLUSTER_STATE_FAILED, rc.Status.State)
				succeeded := jobsutils.FindCondition(rc.Status.StatusConditions, SucceededCondition)
				assert.Equal(t, apipb.CONDITION_STATUS_FALSE, succeeded.GetStatus())
				assert.Equal(t, clusterCreationFailedReason, succeeded.GetReason())
				assert.Contains(t, succeeded.GetMessage(), "other/existing")
				assert.Equal(t, apipb.CONDITION_STATUS_TRUE,
					jobsutils.FindCondition(rc.Status.StatusConditions, ScheduledCondition).GetStatus(), "the job was re-placed")
				// the failed job finalizes without creating again
				h.reconcileOK("existing")
				assert.Equal(t, target, h.get("existing").Status.Assignment.GetCluster())
			}
			assert.Equal(t, 1, h.creates())
			assert.Equal(t, retries, createRetries(t, jobsclient.DispatchRejected)+createRetries(t, jobsclient.DispatchAmbiguous))
			// the existing object is left as it was
			assert.Equal(t, tt.owner, h.computes[target].rayCluster("existing").GetAnnotations())
		})
	}
}

// TestKueueAbsentClusterIsUnsyncedAndPlacementFallsBack marks a cluster as
// Kueue-managed while its Kueue API returns 404: its Kueue state never syncs,
// and placement goes on without it, by the decision table.
func TestKueueAbsentClusterIsUnsyncedAndPlacementFallsBack(t *testing.T) {
	t.Run("a cluster Kueue does not manage takes the job", func(t *testing.T) {
		h := newFleetHarness(t, backendKueue, maconfig.AssignmentStrategyKueueAware,
			newNoKueueCluster(noKueueCluster), newPlainCluster(plainCluster))
		h.waitUnsynced(noKueueCluster)

		h.submit("fallback", nil)
		h.reconcileUntilLaunched("fallback")
		rc := h.get("fallback")
		assert.Equal(t, plainCluster, rc.Status.Assignment.GetCluster())
		// cluster_only's fallback, which no tier ranks
		assert.Equal(t, "cluster "+plainCluster, scheduledMessage(rc))
		assert.Zero(t, h.computes[noKueueCluster].createCount())
		assert.NotContains(t, h.computes[plainCluster].rayCluster("fallback").GetLabels(), constants.KueueQueueNameLabelKey)
	})

	t.Run("a synced Kueue-managed cluster takes the job", func(t *testing.T) {
		h := newFleetHarness(t, backendKueue, maconfig.AssignmentStrategyKueueAware,
			newNoKueueCluster(noKueueCluster), newKueueCluster(kueueB, "4"))
		h.waitUnsynced(noKueueCluster)

		h.submit("ranked", nil)
		h.reconcileUntilLaunched("ranked")
		rc := h.get("ranked")
		assert.Equal(t, "cluster "+kueueB+", tier 1", scheduledMessage(rc))
		assert.Zero(t, h.computes[noKueueCluster].createCount())
	})

	for _, strategy := range []string{maconfig.AssignmentStrategyClusterOnly, maconfig.AssignmentStrategyKueueAware} {
		t.Run("alone, the job waits for its LocalQueue/"+strategy, func(t *testing.T) {
			h := newFleetHarness(t, backendKueue, strategy, newNoKueueCluster(noKueueCluster))
			h.waitUnsynced(noKueueCluster)

			h.submit("waiting", nil)
			for i := 0; i < 2; i++ {
				_, err := h.reconcile("waiting")
				assert.Error(t, err)
			}
			rc := h.get("waiting")
			assert.Nil(t, rc.Status.Assignment)
			enqueued := jobsutils.FindCondition(rc.Status.StatusConditions, EnqueuedCondition)
			assert.Equal(t, apipb.CONDITION_STATUS_FALSE, enqueued.GetStatus())
			assert.Equal(t, constants.KueueQueueNotFound, enqueued.GetReason())
			assert.Zero(t, h.creates())
		})
	}
}

// TestUnsyncedKueueStateRanksInClusterOnlyOrder keeps both clusters' Kueue
// lists from returning, so their Kueue state is not yet synced while their
// LocalQueue can be read: kueue_aware ranks them tier 3, in cluster_only's
// order, whatever their quota.
func TestUnsyncedKueueStateRanksInClusterOnlyOrder(t *testing.T) {
	a, b := newKueueCluster(kueueA, "4"), newKueueCluster(kueueB, "12")
	a.stallLists, b.stallLists = true, true
	h := newFleetHarness(t, backendKueue, maconfig.AssignmentStrategyKueueAware, a, b)
	h.waitUnsynced(kueueA)
	h.waitUnsynced(kueueB)

	h.submit("unsynced", nil)
	clusterOnly, found, _, err := framework.NewClusterOnlyAssignmentStrategy(h.proc.clusters, logr.Discard()).
		Select(h.ctx, framework.BatchRayCluster{RayCluster: h.get("unsynced")})
	require.NoError(t, err)
	require.True(t, found)

	h.reconcileUntilLaunched("unsynced")
	rc := h.get("unsynced")
	assert.Equal(t, clusterOnly.GetCluster(), rc.Status.Assignment.GetCluster())
	assert.Equal(t, "cluster "+clusterOnly.GetCluster()+", tier 3", scheduledMessage(rc))
}
