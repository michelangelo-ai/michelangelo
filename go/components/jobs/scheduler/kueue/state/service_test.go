package state

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	maconfig "github.com/michelangelo-ai/michelangelo/go/base/config"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/compute/computemocks"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

const (
	_timeout = 10 * time.Second
	_tick    = 10 * time.Millisecond
)

// _queueResources maps the Kueue queue kinds to their resources.
var _queueResources = map[string]string{"ClusterQueue": "clusterqueues", "LocalQueue": "localqueues"}

// fakeKueue is a fake Kueue API server for one kueue.x-k8s.io version. It lists
// the queues it holds, streams their changes to open watches from the resource
// version they ask for, and fails or holds the requests it is told to.
type fakeKueue struct {
	version string
	server  *httptest.Server
	// stop ends open watches and held lists, for the server to close.
	stop chan struct{}

	mu sync.Mutex
	rv int
	// objects are the queues by resource, then store key.
	objects map[string]map[string]*unstructured.Unstructured
	// events are the changes by resource, in order.
	events map[string][]fakeEvent
	// watches are the open watches by resource.
	watches map[string]map[*fakeWatch]bool
	// failures are the HTTP statuses to fail requests with, by "<verb> <resource>".
	failures map[string]int
	// requests counts the requests by "<verb> <resource>".
	requests map[string]int
	// listGate, when set, holds lists until it is closed.
	listGate chan struct{}
	// unexpected are the requests the server does not serve.
	unexpected []string
}

type fakeEvent struct {
	rv    int
	frame []byte
}

type fakeWatch struct {
	frames chan []byte
	// end is closed to end the watch.
	end chan struct{}
}

func newFakeKueue(t *testing.T, version string, objs ...*unstructured.Unstructured) *fakeKueue {
	f := &fakeKueue{
		version:  version,
		stop:     make(chan struct{}),
		objects:  map[string]map[string]*unstructured.Unstructured{"clusterqueues": {}, "localqueues": {}},
		events:   map[string][]fakeEvent{},
		watches:  map[string]map[*fakeWatch]bool{"clusterqueues": {}, "localqueues": {}},
		failures: map[string]int{},
		requests: map[string]int{},
	}
	for _, obj := range objs {
		f.objects[_queueResources[obj.GetKind()]][storeKey(obj)] = obj.DeepCopy()
	}
	f.server = httptest.NewServer(f)
	t.Cleanup(func() {
		close(f.stop)
		f.server.Close()
		f.mu.Lock()
		defer f.mu.Unlock()
		assert.Empty(t, f.unexpected, "requests the fake Kueue server does not serve")
	})
	return f
}

func storeKey(obj *unstructured.Unstructured) string {
	if obj.GetNamespace() == "" {
		return obj.GetName()
	}
	return obj.GetNamespace() + "/" + obj.GetName()
}

func (f *fakeKueue) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	prefix := "/apis/" + kueueGroup + "/" + f.version + "/"
	resource := map[string]string{
		prefix + "clusterqueues":                  "clusterqueues",
		prefix + "namespaces/default/localqueues": "localqueues",
	}[r.URL.Path]
	verb := "list"
	if r.URL.Query().Get("watch") == "true" {
		verb = "watch"
	}

	f.mu.Lock()
	if resource == "" || r.Method != http.MethodGet {
		f.unexpected = append(f.unexpected, r.Method+" "+r.URL.String())
		f.mu.Unlock()
		writeStatus(w, http.StatusNotFound)
		return
	}
	f.requests[verb+" "+resource]++
	code := f.failures[verb+" "+resource]
	f.mu.Unlock()

	switch {
	case code != 0:
		writeStatus(w, code)
	case verb == "watch":
		f.serveWatch(w, r, resource)
	default:
		f.serveList(w, r, resource)
	}
}

// writeStatus fails a request with a Status, as the API server does.
func writeStatus(w http.ResponseWriter, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(metav1.Status{
		TypeMeta: metav1.TypeMeta{Kind: "Status", APIVersion: "v1"},
		Status:   metav1.StatusFailure,
		Message:  http.StatusText(code),
		Code:     int32(code),
	})
}

func (f *fakeKueue) serveList(w http.ResponseWriter, r *http.Request, resource string) {
	f.mu.Lock()
	gate := f.listGate
	f.mu.Unlock()
	if gate != nil {
		select {
		case <-gate:
		case <-r.Context().Done():
			return
		case <-f.stop:
			return
		}
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	keys := make([]string, 0, len(f.objects[resource]))
	for key := range f.objects[resource] {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	items := make([]interface{}, 0, len(keys))
	for _, key := range keys {
		items = append(items, f.objects[resource][key].Object)
	}
	kind := map[string]string{"clusterqueues": "ClusterQueueList", "localqueues": "LocalQueueList"}[resource]
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"apiVersion": kueueGroup + "/" + f.version,
		"kind":       kind,
		"metadata":   map[string]interface{}{"resourceVersion": strconv.Itoa(f.rv)},
		"items":      items,
	})
}

func (f *fakeKueue) serveWatch(w http.ResponseWriter, r *http.Request, resource string) {
	fw := &fakeWatch{frames: make(chan []byte, 64), end: make(chan struct{})}
	f.mu.Lock()
	if from, err := strconv.Atoi(r.URL.Query().Get("resourceVersion")); err == nil {
		for _, e := range f.events[resource] {
			if e.rv > from {
				fw.frames <- e.frame
			}
		}
	}
	f.watches[resource][fw] = true
	f.mu.Unlock()
	defer func() {
		f.mu.Lock()
		delete(f.watches[resource], fw)
		f.mu.Unlock()
	}()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	flusher := w.(http.Flusher)
	flusher.Flush()
	for {
		select {
		case frame := <-fw.frames:
			_, _ = w.Write(frame)
			flusher.Flush()
		case <-fw.end:
			return
		case <-r.Context().Done():
			return
		case <-f.stop:
			return
		}
	}
}

// put adds or replaces a queue, and streams the change.
func (f *fakeKueue) put(t *testing.T, obj *unstructured.Unstructured) {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	resource, key := _queueResources[obj.GetKind()], storeKey(obj)
	eventType := watch.Added
	if _, ok := f.objects[resource][key]; ok {
		eventType = watch.Modified
	}
	obj = obj.DeepCopy()
	f.rv++
	obj.SetResourceVersion(strconv.Itoa(f.rv))
	f.objects[resource][key] = obj
	f.emitLocked(t, resource, eventType, obj)
}

// remove deletes a queue, and streams the change.
func (f *fakeKueue) remove(t *testing.T, resource, key string) {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	obj := f.objects[resource][key].DeepCopy()
	delete(f.objects[resource], key)
	f.rv++
	obj.SetResourceVersion(strconv.Itoa(f.rv))
	f.emitLocked(t, resource, watch.Deleted, obj)
}

func (f *fakeKueue) emitLocked(t *testing.T, resource string, eventType watch.EventType, obj *unstructured.Unstructured) {
	raw, err := obj.MarshalJSON()
	require.NoError(t, err)
	frame, err := json.Marshal(metav1.WatchEvent{Type: string(eventType), Object: runtime.RawExtension{Raw: raw}})
	require.NoError(t, err)
	frame = append(frame, '\n')
	f.events[resource] = append(f.events[resource], fakeEvent{rv: f.rv, frame: frame})
	for fw := range f.watches[resource] {
		fw.frames <- frame
	}
}

// endWatches ends the open watches of the resource, as the API server does
// when a watch times out.
func (f *fakeKueue) endWatches(resource string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for fw := range f.watches[resource] {
		close(fw.end)
		delete(f.watches[resource], fw)
	}
}

// fail fails the requests, "<verb> <resource>", with the HTTP status; 0 stops
// failing them.
func (f *fakeKueue) fail(request string, code int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if code == 0 {
		delete(f.failures, request)
		return
	}
	f.failures[request] = code
}

// setAbsent has the server answer every request as an API server without
// Kueue does, or stop.
func (f *fakeKueue) setAbsent(absent bool) {
	code := 0
	if absent {
		code = http.StatusNotFound
	}
	for _, request := range []string{"list clusterqueues", "watch clusterqueues", "list localqueues", "watch localqueues"} {
		f.fail(request, code)
	}
}

func (f *fakeKueue) holdLists() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.listGate = make(chan struct{})
}

func (f *fakeKueue) releaseLists() {
	f.mu.Lock()
	defer f.mu.Unlock()
	close(f.listGate)
	f.listGate = nil
}

func (f *fakeKueue) requestCount(request string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.requests[request]
}

func (f *fakeKueue) openWatches() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.watches["clusterqueues"]) + len(f.watches["localqueues"])
}

// newTestService returns a Kueue state service of the Kueue backend at the API
// version, reading a fake manager cache that holds the objects. Its factory
// builds a cluster's REST config for the server at the cluster's host, and
// counts the builds.
func newTestService(t *testing.T, apiVersion string, objs ...client.Object) (*Service, *atomic.Int32) {
	builds := &atomic.Int32{}
	factory := computemocks.NewMockFactory(gomock.NewController(t))
	factory.EXPECT().GetRestConfigForCluster(gomock.Any()).DoAndReturn(func(c *v2pb.Cluster) (*rest.Config, error) {
		builds.Add(1)
		return &rest.Config{Host: c.Spec.GetKubernetes().GetRest().GetHost()}, nil
	}).AnyTimes()
	cfg := maconfig.SchedulerConfig{Backend: kueueBackend, Kueue: maconfig.KueueConfig{APIVersion: apiVersion}}
	svc, err := newService(cfg, factory, newCache(t, interceptor.Funcs{}, objs...), newMapper(false), logr.Discard())
	require.NoError(t, err)
	return svc, builds
}

// runService runs the service as the manager does, until the test ends.
func runService(t *testing.T, svc *Service) {
	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		assert.NoError(t, svc.run(ctx))
	}()
	t.Cleanup(func() {
		cancel()
		requireClosed(t, stopped)
		svc.mu.Lock()
		var watches []*clusterWatch
		for _, w := range svc.watches {
			watches = append(watches, w)
		}
		svc.mu.Unlock()
		for _, w := range watches {
			if w.started() {
				requireClosed(t, w.done)
			}
		}
	})
}

func requireClosed(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(_timeout):
		require.FailNow(t, "not closed")
	}
}

func requireSynced(t *testing.T, svc *Service, cluster string) {
	t.Helper()
	require.Eventually(t, func() bool { return svc.Synced(cluster) }, _timeout, _tick)
}

// requireView waits for the view of the LocalQueue on cluster-1 to satisfy cond.
func requireView(t *testing.T, svc *Service, localQueue string, cond func(View) bool) {
	t.Helper()
	require.Eventually(t, func() bool {
		view, err := svc.Lookup("cluster-1", localQueue)
		return err == nil && cond(view)
	}, _timeout, _tick)
}

// kueueClusterAt returns a Kueue-managed cluster whose API server is at host.
func kueueClusterAt(name, host, tokenTag string) *v2pb.Cluster {
	return &v2pb.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "ma-system"},
		Spec: v2pb.ClusterSpec{
			SchedulerType: v2pb.SCHEDULER_TYPE_KUEUE,
			Cluster: &v2pb.ClusterSpec_Kubernetes{Kubernetes: &v2pb.KubernetesSpec{
				Rest: &v2pb.ConnectionSpec{Host: host, TokenTag: tokenTag},
			}},
		},
	}
}

// localQueue returns the active LocalQueue fixture of the version, renamed and
// feeding the given spec.clusterQueue.
func localQueue(t *testing.T, version, name string, clusterQueue interface{}) *unstructured.Unstructured {
	lq := loadFixture(t, version, "localqueue-active")
	lq.SetName(name)
	require.NoError(t, unstructured.SetNestedField(lq.Object, clusterQueue, "spec", "clusterQueue"))
	return lq
}

func pendingWorkloads(n int32) func(View) bool {
	return func(v View) bool { return v.ClusterQueue != nil && v.ClusterQueue.PendingWorkloads == n }
}

func TestServiceLookup(t *testing.T) {
	fk := newFakeKueue(t, "v1beta2",
		loadFixture(t, "v1beta2", "clusterqueue-single-flavor"),
		loadFixture(t, "v1beta2", "localqueue-active"),
		loadFixture(t, "v1beta2", "localqueue-inactive"),
		localQueue(t, "v1beta2", "ma-team-d", "missing-queue"),
		localQueue(t, "v1beta2", "ma-team-e", int64(5)),
	)
	svc, _ := newTestService(t, "")
	runService(t, svc)
	svc.Watch(kueueClusterAt("cluster-1", fk.server.URL, "token"))
	requireSynced(t, svc, "cluster-1")

	tests := []struct {
		name        string
		clusterName string
		localQueue  string
		// want is the view but its ClusterQueue, which wantCQ names.
		want    View
		wantCQ  string
		wantErr string
	}{
		{
			name:       "an active LocalQueue",
			localQueue: "ma-team-a",
			want:       View{Synced: true, Exists: true, Active: true},
			wantCQ:     "cluster-queue",
		},
		{
			name:       "an inactive LocalQueue",
			localQueue: "ma-team-b",
			want:       View{Synced: true, Exists: true},
			wantCQ:     "cluster-queue",
		},
		{name: "no such LocalQueue", localQueue: "ma-team-c", want: View{Synced: true}},
		{name: "no such ClusterQueue", localQueue: "ma-team-d", want: View{Synced: true, Exists: true, Active: true}},
		{
			name:       "a LocalQueue that cannot be read",
			localQueue: "ma-team-e",
			wantErr:    "LocalQueue default/ma-team-e: spec.clusterQueue is int64, not a string",
		},
		{name: "a cluster not watched", clusterName: "cluster-2", localQueue: "ma-team-a"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clusterName := tt.clusterName
			if clusterName == "" {
				clusterName = "cluster-1"
			}
			view, err := svc.Lookup(clusterName, tt.localQueue)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				assert.Equal(t, View{}, view)
				return
			}
			require.NoError(t, err)
			cq := view.ClusterQueue
			view.ClusterQueue = nil
			assert.Equal(t, tt.want, view)
			if tt.wantCQ == "" {
				assert.Nil(t, cq)
				return
			}
			require.NotNil(t, cq)
			assert.Equal(t, tt.wantCQ, cq.Name)
			assert.Equal(t, 1, cq.FlavorCount)
			assert.Equal(t, int32(3), cq.PendingWorkloads)
			requireResources(t, map[corev1.ResourceName]string{"cpu": "64", "memory": "256Gi", "nvidia.com/gpu": "8"}, cq.Nominal)
		})
	}
	assert.False(t, svc.Synced("cluster-2"))
}

func TestServiceWatchEventsUpdateTheViews(t *testing.T) {
	fk := newFakeKueue(t, "v1beta2",
		loadFixture(t, "v1beta2", "clusterqueue-single-flavor"),
		loadFixture(t, "v1beta2", "localqueue-active"),
	)
	svc, _ := newTestService(t, "")
	runService(t, svc)
	svc.Watch(kueueClusterAt("cluster-1", fk.server.URL, "token"))
	requireSynced(t, svc, "cluster-1")
	requireView(t, svc, "ma-team-a", pendingWorkloads(3))

	cq := loadFixture(t, "v1beta2", "clusterqueue-single-flavor")
	require.NoError(t, unstructured.SetNestedField(cq.Object, int64(7), "status", "pendingWorkloads"))
	fk.put(t, cq)
	requireView(t, svc, "ma-team-a", pendingWorkloads(7))

	fk.put(t, localQueue(t, "v1beta2", "ma-team-c", "cluster-queue"))
	requireView(t, svc, "ma-team-c", func(v View) bool { return v.Exists && v.ClusterQueue != nil })

	fk.remove(t, "localqueues", "default/ma-team-a")
	requireView(t, svc, "ma-team-a", func(v View) bool { return v.Synced && !v.Exists })
}

func TestServiceWatchErrorUnsyncsTheClusterUntilItRelists(t *testing.T) {
	fk := newFakeKueue(t, "v1beta2",
		loadFixture(t, "v1beta2", "clusterqueue-single-flavor"),
		loadFixture(t, "v1beta2", "localqueue-active"),
	)
	svc, _ := newTestService(t, "")
	runService(t, svc)
	svc.Watch(kueueClusterAt("cluster-1", fk.server.URL, "token"))
	requireSynced(t, svc, "cluster-1")

	// a change arrives over the ClusterQueue watch
	cq := loadFixture(t, "v1beta2", "clusterqueue-single-flavor")
	require.NoError(t, unstructured.SetNestedField(cq.Object, int64(7), "status", "pendingWorkloads"))
	fk.put(t, cq)
	requireView(t, svc, "ma-team-a", pendingWorkloads(7))

	// the watch ends and cannot be restarted, and lists hang
	fk.holdLists()
	fk.fail("watch clusterqueues", http.StatusForbidden)
	fk.endWatches("clusterqueues")
	require.Eventually(t, func() bool { return !svc.Synced("cluster-1") }, _timeout, _tick)
	assert.True(t, svc.watch("cluster-1").clusterQueues.informer.HasSynced(), "the informer itself never unsyncs")
	view, err := svc.Lookup("cluster-1", "ma-team-a")
	require.NoError(t, err)
	assert.Equal(t, View{}, view)

	// relisting, but the list has not completed
	require.Eventually(t, func() bool { return fk.requestCount("list clusterqueues") >= 2 }, _timeout, _tick)
	assert.False(t, svc.Synced("cluster-1"))

	fk.fail("watch clusterqueues", 0)
	fk.releaseLists()
	requireSynced(t, svc, "cluster-1")
	requireView(t, svc, "ma-team-a", pendingWorkloads(7))
}

// TestServiceLastKnown follows the LocalQueues as last listed and watched
// through a cluster's first list, a broken LocalQueue watch and a restart with
// a new client.
func TestServiceLastKnown(t *testing.T) {
	fk := newFakeKueue(t, "v1beta2",
		loadFixture(t, "v1beta2", "clusterqueue-single-flavor"),
		loadFixture(t, "v1beta2", "localqueue-active"),
	)
	svc, _ := newTestService(t, "")
	runService(t, svc)
	requireLastKnown := func(t *testing.T, localQueue string, wantExists, wantKnown bool) {
		t.Helper()
		exists, known := svc.LastKnown("cluster-1", localQueue)
		require.Equal(t, wantExists, exists, "exists")
		require.Equal(t, wantKnown, known, "known")
	}

	// the first list hangs
	fk.holdLists()
	svc.Watch(kueueClusterAt("cluster-1", fk.server.URL, "token"))
	require.Eventually(t, func() bool { return fk.requestCount("list localqueues") >= 1 }, _timeout, _tick)
	requireLastKnown(t, "ma-team-a", false, false)

	fk.releaseLists()
	requireSynced(t, svc, "cluster-1")
	requireLastKnown(t, "ma-team-a", true, true)
	requireLastKnown(t, "ma-team-c", false, true)
	exists, known := svc.LastKnown("cluster-2", "ma-team-a")
	assert.False(t, exists || known, "a cluster not watched")

	// a LocalQueue is created over the watch
	fk.put(t, localQueue(t, "v1beta2", "ma-team-c", "cluster-queue"))
	requireView(t, svc, "ma-team-c", func(v View) bool { return v.Exists })

	// the LocalQueue watch ends and cannot be restarted, and lists hang
	fk.holdLists()
	fk.fail("watch localqueues", http.StatusForbidden)
	fk.endWatches("localqueues")
	require.Eventually(t, func() bool { return !svc.Synced("cluster-1") }, _timeout, _tick)
	view, err := svc.Lookup("cluster-1", "ma-team-a")
	require.NoError(t, err)
	assert.Equal(t, View{}, view)
	requireLastKnown(t, "ma-team-a", true, true)
	requireLastKnown(t, "ma-team-c", true, true)
	requireLastKnown(t, "ma-team-d", false, true)

	// restarted with a new client, whose list hangs
	old := svc.watch("cluster-1")
	svc.Rewatch("cluster-1")
	requireClosed(t, old.done)
	requireLastKnown(t, "ma-team-a", false, false)

	fk.fail("watch localqueues", 0)
	fk.releaseLists()
	requireSynced(t, svc, "cluster-1")
	requireLastKnown(t, "ma-team-a", true, true)
	requireLastKnown(t, "ma-team-c", true, true)

	// a stored object that is not a LocalQueue cannot be read
	require.NoError(t, svc.watch("cluster-1").localQueues.informer.GetStore().Add(
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "ma-team-x", Namespace: "default"}}))
	requireLastKnown(t, "ma-team-x", false, false)
}

func TestServiceKueueAbsent(t *testing.T) {
	fk := newFakeKueue(t, "v1beta2",
		loadFixture(t, "v1beta2", "clusterqueue-single-flavor"),
		loadFixture(t, "v1beta2", "localqueue-active"),
	)
	fk.setAbsent(true)
	svc, _ := newTestService(t, "")
	runService(t, svc)
	svc.Watch(kueueClusterAt("cluster-1", fk.server.URL, "token"))

	// listed again, with backoff
	require.Eventually(t, func() bool {
		return fk.requestCount("list clusterqueues") >= 2 && fk.requestCount("list localqueues") >= 2
	}, _timeout, _tick)
	assert.False(t, svc.Synced("cluster-1"))
	view, err := svc.Lookup("cluster-1", "ma-team-a")
	require.NoError(t, err)
	assert.Equal(t, View{}, view)

	// Kueue installed
	fk.setAbsent(false)
	requireSynced(t, svc, "cluster-1")
	requireView(t, svc, "ma-team-a", pendingWorkloads(3))
}

func TestServiceUnwatch(t *testing.T) {
	fk := newFakeKueue(t, "v1beta2",
		loadFixture(t, "v1beta2", "clusterqueue-single-flavor"),
		loadFixture(t, "v1beta2", "localqueue-active"),
	)
	svc, _ := newTestService(t, "")
	runService(t, svc)
	svc.Watch(kueueClusterAt("cluster-1", fk.server.URL, "token"))
	requireSynced(t, svc, "cluster-1")
	w := svc.watch("cluster-1")

	svc.Unwatch("cluster-1")
	requireClosed(t, w.done)
	require.Eventually(t, func() bool { return fk.openWatches() == 0 }, _timeout, _tick)
	assert.Nil(t, svc.watch("cluster-1"))
	assert.False(t, svc.Synced("cluster-1"))
	view, err := svc.Lookup("cluster-1", "ma-team-a")
	require.NoError(t, err)
	assert.Equal(t, View{}, view)

	svc.Unwatch("cluster-1")
}

func TestServiceWatchWithNewCredentialsReplacesTheWatch(t *testing.T) {
	oldServer := newFakeKueue(t, "v1beta2",
		loadFixture(t, "v1beta2", "clusterqueue-single-flavor"),
		loadFixture(t, "v1beta2", "localqueue-active"),
	)
	newServer := newFakeKueue(t, "v1beta2",
		loadFixture(t, "v1beta2", "clusterqueue-multi-flavor"),
		loadFixture(t, "v1beta2", "localqueue-active"),
	)
	svc, builds := newTestService(t, "")
	runService(t, svc)
	svc.Watch(kueueClusterAt("cluster-1", oldServer.server.URL, "token-1"))
	requireView(t, svc, "ma-team-a", func(v View) bool { return v.ClusterQueue != nil && v.ClusterQueue.FlavorCount == 1 })
	old := svc.watch("cluster-1")

	svc.Watch(kueueClusterAt("cluster-1", newServer.server.URL, "token-2"))
	requireClosed(t, old.done)
	require.Eventually(t, func() bool { return oldServer.openWatches() == 0 }, _timeout, _tick)
	requireView(t, svc, "ma-team-a", func(v View) bool { return v.ClusterQueue != nil && v.ClusterQueue.FlavorCount == 3 })
	assert.Equal(t, "token-2", svc.watch("cluster-1").cluster.Spec.GetKubernetes().GetRest().GetTokenTag())
	assert.EqualValues(t, 2, builds.Load())
}

func TestServiceRewatch(t *testing.T) {
	fk := newFakeKueue(t, "v1beta2",
		loadFixture(t, "v1beta2", "clusterqueue-single-flavor"),
		loadFixture(t, "v1beta2", "localqueue-active"),
	)
	svc, builds := newTestService(t, "")
	runService(t, svc)
	c := kueueClusterAt("cluster-1", fk.server.URL, "token")
	svc.Watch(c)
	// the service watches a copy of the cluster
	c.Spec.GetKubernetes().Rest.Host = "http://127.0.0.1:1"
	requireSynced(t, svc, "cluster-1")
	assert.EqualValues(t, 1, builds.Load())
	old := svc.watch("cluster-1")

	svc.Rewatch("cluster-1")
	requireClosed(t, old.done)
	requireSynced(t, svc, "cluster-1")
	assert.EqualValues(t, 2, builds.Load(), "restarted with a new client")
	assert.Same(t, old.cluster, svc.watch("cluster-1").cluster)

	svc.Rewatch("cluster-2")
	assert.Nil(t, svc.watch("cluster-2"))
	assert.EqualValues(t, 2, builds.Load())
}

func TestServiceRunStartsTheWatches(t *testing.T) {
	fk := newFakeKueue(t, "v1beta2",
		loadFixture(t, "v1beta2", "clusterqueue-single-flavor"),
		loadFixture(t, "v1beta2", "localqueue-active"),
	)
	svc, builds := newTestService(t, "")
	svc.Watch(kueueClusterAt("cluster-1", fk.server.URL, "token"))
	w := svc.watch("cluster-1")
	assert.False(t, w.started(), "started before the manager started the service")
	assert.False(t, svc.Synced("cluster-1"))
	assert.Zero(t, builds.Load())

	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		assert.NoError(t, svc.run(ctx))
	}()
	t.Cleanup(func() {
		cancel()
		<-stopped
	})
	requireSynced(t, svc, "cluster-1")

	// the manager stops
	cancel()
	requireClosed(t, stopped)
	requireClosed(t, w.done)
	require.Eventually(t, func() bool { return fk.openWatches() == 0 }, _timeout, _tick)
}

func TestServiceV1beta1(t *testing.T) {
	fk := newFakeKueue(t, "v1beta1",
		loadFixture(t, "v1beta1", "clusterqueue-cohort"),
		localQueue(t, "v1beta1", "ma-team-a", "research-queue"),
	)
	svc, _ := newTestService(t, "v1beta1")
	runService(t, svc)
	svc.Watch(kueueClusterAt("cluster-1", fk.server.URL, "token"))
	requireSynced(t, svc, "cluster-1")

	view, err := svc.Lookup("cluster-1", "ma-team-a")
	require.NoError(t, err)
	require.NotNil(t, view.ClusterQueue)
	assert.Equal(t, "research-queue", view.ClusterQueue.Name)
	assert.Equal(t, "research", view.ClusterQueue.Cohort)
	assert.False(t, view.Scorable(), "the ClusterQueue shares quota in a cohort")
}

func TestServiceDemand(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestService(t, "", assignedJob("job-1", "cluster-1", 1))
	svc.RecordAssignment(newJob("job-2", 1), "cluster-1", _now)

	got, err := svc.InFlight(ctx, _kueueCluster, _now)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"job-1", "job-2"}, jobNames(got))
	requireAsk(t, resources("cpu", "12", "memory", "48Gi", "nvidia.com/gpu", "2"), got.Ask)

	ask, err := svc.Ask(newJob("job-3", 1), _kueueCluster)
	require.NoError(t, err)
	requireAsk(t, resources("cpu", "6", "memory", "24Gi", "nvidia.com/gpu", "1"), ask)
}

func TestServiceUnderAnotherBackend(t *testing.T) {
	for _, backend := range []string{"", "default"} {
		t.Run("backend "+backend, func(t *testing.T) {
			// no factory call is expected: the service sends nothing to the
			// clusters
			factory := computemocks.NewMockFactory(gomock.NewController(t))
			cfg := maconfig.SchedulerConfig{Backend: backend, Kueue: maconfig.KueueConfig{APIVersion: "v1"}}
			svc, err := newService(cfg, factory, newCache(t, interceptor.Funcs{}, assignedJob("job-1", "cluster-1", 1)),
				newMapper(false), logr.Discard())
			require.NoError(t, err, "the Kueue API version is read under the Kueue backend only")

			svc.Watch(kueueClusterAt("cluster-1", "http://127.0.0.1:1", "token"))
			svc.Rewatch("cluster-1")
			assert.Nil(t, svc.watch("cluster-1"))
			assert.False(t, svc.Synced("cluster-1"))
			view, err := svc.Lookup("cluster-1", "ma-team-a")
			require.NoError(t, err)
			assert.Equal(t, View{}, view)
			svc.Unwatch("cluster-1")

			svc.RecordAssignment(newJob("job-2", 1), "cluster-1", _now)
			assert.Empty(t, svc.tracker.recorded)
			got, err := svc.InFlight(context.Background(), _kueueCluster, _now)
			require.NoError(t, err)
			assert.Empty(t, got.Jobs)
			assert.NotNil(t, got.Ask)
			assert.Empty(t, got.Ask)
		})
	}
}

func TestNewServiceAPIVersion(t *testing.T) {
	tests := []struct {
		name        string
		backend     string
		apiVersion  string
		wantEnabled bool
		wantVersion string
		wantErr     string
	}{
		{name: "Kueue backend, default version", backend: "kueue", wantEnabled: true, wantVersion: "v1beta2"},
		{name: "Kueue backend, v1beta2", backend: "kueue", apiVersion: "v1beta2", wantEnabled: true, wantVersion: "v1beta2"},
		{name: "Kueue backend, v1beta1", backend: "kueue", apiVersion: "v1beta1", wantEnabled: true, wantVersion: "v1beta1"},
		{
			name:       "Kueue backend, unsupported version",
			backend:    "kueue",
			apiVersion: "v1",
			wantErr:    `unsupported jobs.scheduler.kueue.apiVersion "v1" (expected v1beta1 or v1beta2)`,
		},
		{
			name:       "Kueue backend, retired version",
			backend:    "kueue",
			apiVersion: "v1alpha1",
			wantErr:    `unsupported jobs.scheduler.kueue.apiVersion "v1alpha1"`,
		},
		{name: "default backend, unsupported version", backend: "default", apiVersion: "v1"},
		{name: "no backend, unsupported version", apiVersion: "v1alpha1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := maconfig.SchedulerConfig{Backend: tt.backend, Kueue: maconfig.KueueConfig{APIVersion: tt.apiVersion}}
			svc, err := newService(cfg, nil, nil, nil, logr.Discard())
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantEnabled, svc.enabled)
			if tt.wantEnabled {
				assert.Equal(t, tt.wantVersion, svc.apiVersion)
			}
		})
	}
}
