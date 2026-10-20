package watch

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/zapr"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	"github.com/uber-go/tally"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	fcache "k8s.io/client-go/tools/cache/testing"

	"github.com/michelangelo-ai/michelangelo/go/components/jobs/client"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/client/clientmocks"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/cluster"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/cluster/clustermocks"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// fakeController implements cache.Controller for testing.
type fakeController struct {
	runFunc func(stopCh <-chan struct{})
}

func (f *fakeController) Run(stopCh <-chan struct{}) {
	if f.runFunc != nil {
		f.runFunc(stopCh)
	}
}

// RunWithContext is the context-aware form of Run that cache.Controller gained
// in client-go v0.34. It delegates to the same runFunc so both entry points
// behave identically for tests.
func (f *fakeController) RunWithContext(ctx context.Context) {
	f.Run(ctx.Done())
}

func (f *fakeController) HasSynced() bool                 { return true }
func (f *fakeController) LastSyncResourceVersion() string { return "" }

// blockingController returns a fake controller whose Run, like a real
// informer's, only returns once its stop channel is closed, and a channel
// that is closed when Run returns.
func blockingController() (*fakeController, <-chan struct{}) {
	returned := make(chan struct{})
	return &fakeController{
		runFunc: func(stopCh <-chan struct{}) {
			defer close(returned)
			<-stopCh
		},
	}, returned
}

var _testCluster = v2pb.Cluster{
	TypeMeta: metav1.TypeMeta{
		Kind:       "Cluster",
		APIVersion: "michelangelo.uber.com/v2beta1",
	},
	ObjectMeta: metav1.ObjectMeta{
		Name:            "testCluster",
		Namespace:       constants.ClustersNamespace,
		ResourceVersion: "999",
	},
	Spec: v2pb.ClusterSpec{
		Region: "phx",
		Zone:   "phx5",
		Cluster: &v2pb.ClusterSpec_Kubernetes{
			Kubernetes: &v2pb.KubernetesSpec{
				Rest: &v2pb.ConnectionSpec{
					Host: "https://k8s-apiserver-kubernetes-batch01.phx5.uber.internal",
					Port: "port",
				},
			},
		},
	},
}

// newTestCluster returns a copy of _testCluster with the given name.
func newTestCluster(name string) *v2pb.Cluster {
	c := _testCluster.DeepCopy()
	c.Name = name
	return c
}

// newTestWatcher returns a watcher over the given cluster cache and federated
// client, and the scope its metrics are recorded in. Its logger is a no-op:
// watch goroutines log after a test has stopped them, and logging to t once
// the test has finished panics.
func newTestWatcher(
	t *testing.T,
	clusterCache cluster.RegisteredClustersCache,
	federatedClient client.FederatedClient,
	watcherParams ...*client.WatcherParams,
) (*watcher, tally.TestScope) {
	t.Helper()
	scope := tally.NewTestScope("test", map[string]string{})
	w := NewFederatedWatcher(FederatedWatcherParams{
		ClusterCache:    clusterCache,
		FederatedClient: federatedClient,
		Logger:          zapr.NewLogger(zap.NewNop()),
		WatcherParams:   watcherParams,
		Scope:           scope,
	})
	return w.(*watcher), scope
}

// expectSync makes the next sync see ready as the ready clusters and
// registered as every registered cluster.
func expectSync(m *clustermocks.MockRegisteredClustersCache, ready, registered []*v2pb.Cluster) {
	m.EXPECT().GetClusters(cluster.ReadyClusters).Return(ready)
	m.EXPECT().GetClusters(cluster.AllClusters).Return(registered)
}

// counter returns the value of a watcher counter, 0 if it was never emitted.
func counter(scope tally.TestScope, name string) int64 {
	c, ok := scope.Snapshot().Counters()["test.watcher."+name+"+"]
	if !ok {
		return 0
	}
	return c.Value()
}

// waitClosed fails the test unless ch is closed within wait.ForeverTestTimeout.
func waitClosed(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(wait.ForeverTestTimeout):
		t.Fatalf("timed out waiting for %s", what)
	}
}

// requireOpen fails the test if ch is closed.
func requireOpen(t *testing.T, ch <-chan struct{}, msg string) {
	t.Helper()
	select {
	case <-ch:
		t.Fatal(msg)
	default:
	}
}

type test struct {
	cluster v2pb.Cluster
	changed bool
	msg     string
}

func TestSync(t *testing.T) {
	tt := []test{
		{
			cluster: _testCluster,
			changed: false,
			msg:     "no change",
		},
		{
			cluster: v2pb.Cluster{
				ObjectMeta: _testCluster.ObjectMeta,
				Spec: v2pb.ClusterSpec{
					Cluster: &v2pb.ClusterSpec_Kubernetes{
						Kubernetes: &v2pb.KubernetesSpec{
							Rest: &v2pb.ConnectionSpec{
								Host: _testCluster.Spec.GetKubernetes().Rest.Host,
								Port: "NewPort",
							},
						},
					},
				},
			},
			changed: true,
			msg:     "port change",
		},
		{
			cluster: v2pb.Cluster{
				ObjectMeta: _testCluster.ObjectMeta,
				Spec: v2pb.ClusterSpec{
					Cluster: &v2pb.ClusterSpec_Kubernetes{
						Kubernetes: &v2pb.KubernetesSpec{
							Rest: &v2pb.ConnectionSpec{
								Host: "NewHost",
								Port: _testCluster.Spec.GetKubernetes().Rest.Port,
							},
						},
					},
				},
			},
			changed: true,
			msg:     "host change",
		},
	}

	for _, test := range tt {
		t.Run(test.msg, func(t *testing.T) {
			var wg sync.WaitGroup
			defer wg.Wait()

			gctrl := gomock.NewController(t)

			mockClusterCache := clustermocks.NewMockRegisteredClustersCache(gctrl)
			expectSync(mockClusterCache, []*v2pb.Cluster{&_testCluster}, []*v2pb.Cluster{&_testCluster})

			w := setupWatcher(t, test, &wg, gctrl)
			federatedWatcher := w.(*watcher)
			federatedWatcher.clusterCache = mockClusterCache
			defer federatedWatcher.stopAll()

			federatedWatcher.sync()

			// test that clusterInfoMap has _testCluster
			_, ok := federatedWatcher.clusterInfoMap.Load(_testCluster.Name)
			require.True(t, ok)

			// change the cluster
			expectSync(mockClusterCache, []*v2pb.Cluster{&test.cluster}, []*v2pb.Cluster{&test.cluster})

			federatedWatcher.sync()

			// test cluster update
			clusterInfo, ok := federatedWatcher.clusterInfoMap.Load(_testCluster.Name)
			require.True(t, ok)

			cl := clusterInfo.(*watchInfo).cluster
			require.Equal(t, test.cluster.Name, cl.Name)
			require.Equal(t, test.cluster.Spec.GetKubernetes().Rest.Host, cl.Spec.GetKubernetes().Rest.Host)
			require.Equal(t, test.cluster.Spec.GetKubernetes().Rest.Port, cl.Spec.GetKubernetes().Rest.Port)
		})
	}
}

func setupWatcher(t *testing.T, test test, wg *sync.WaitGroup, gctrl *gomock.Controller) FederatedWatcher {
	t.Helper()
	times := 1 // the first call
	if test.changed {
		// if the test mimics a change it would trigger another sync.
		times++
	}

	mockClient := clientmocks.NewMockFederatedClient(gctrl)
	mockClient.EXPECT().Watcher(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ []*client.WatcherParams, _ *v2pb.Cluster) ([]*client.ResourceWatcher, error) {
			wg.Add(1)
			return []*client.ResourceWatcher{
				{
					Controller: &fakeController{
						// like a real informer, run until stopped
						runFunc: func(stopCh <-chan struct{}) {
							defer wg.Done()
							<-stopCh
						},
					},
				},
			}, nil
		},
	).Times(times)

	testScope := tally.NewTestScope("test", map[string]string{})

	federatedWatcher := NewFederatedWatcher(
		FederatedWatcherParams{
			Logger:          zapr.NewLogger(zap.NewNop()),
			FederatedClient: mockClient,
			Scope:           testScope,
		})

	return federatedWatcher
}

// A watch controller that panics is recovered and counted, and flagged so
// that the next sync rebuilds it rather than leaving its resource unwatched.
func TestStartWatchControllerRecoverer(t *testing.T) {
	w, scope := newTestWatcher(t, nil, nil)

	ctrl := &fakeController{
		runFunc: func(_ <-chan struct{}) {
			panic(errors.New("test error"))
		},
	}

	stopCh := make(chan struct{})
	exited := make(chan struct{})

	w.startWatchGoRoutine(&client.ResourceWatcher{
		Controller: ctrl,
	}, stopCh, exited, "testCluster")

	require.Equal(t, int64(1), counter(scope, _failureWatchPanicMetricName))
	require.Equal(t, int64(0), counter(scope, _failureWatchExitWithoutPanicMetricName))
	select {
	case <-exited:
	default:
		t.Fatal("a panicked watch must be flagged for rebuild")
	}
}

// watch_exit_without_panic alerts on a watch that died on its own; a watch
// that sync stopped on purpose is neither counted nor flagged for rebuild.
func TestWatchExitWithoutPanicMetric(t *testing.T) {
	t.Run("stopped by sync", func(t *testing.T) {
		w, scope := newTestWatcher(t, nil, nil)
		ctrl, _ := blockingController()
		stopCh := make(chan struct{})
		exited := make(chan struct{})
		close(stopCh)

		w.startWatchGoRoutine(&client.ResourceWatcher{Controller: ctrl}, stopCh, exited, "testCluster")

		require.Equal(t, int64(0), counter(scope, _failureWatchExitWithoutPanicMetricName))
		requireOpen(t, exited, "a watch stopped on purpose must not be rebuilt")
	})

	t.Run("returned with its stop channel open", func(t *testing.T) {
		w, scope := newTestWatcher(t, nil, nil)
		stopCh := make(chan struct{})
		exited := make(chan struct{})

		w.startWatchGoRoutine(&client.ResourceWatcher{Controller: &fakeController{}}, stopCh, exited, "testCluster")

		require.Equal(t, int64(1), counter(scope, _failureWatchExitWithoutPanicMetricName))
		require.Equal(t, int64(0), counter(scope, _failureWatchPanicMetricName))
		select {
		case <-exited:
		default:
			t.Fatal("a watch that returned on its own must be flagged for rebuild")
		}
	})
}

// A panic in an event handler must not take the watch down: that event is
// lost, but the informer keeps delivering the ones after it.
func TestHandlerPanicKeepsWatchAlive(t *testing.T) {
	source := fcache.NewFakeControllerSource()
	seen := make(chan string, 10)
	handler := cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			pod := obj.(*corev1.Pod)
			if pod.Name == "a" {
				panic("handler bug")
			}
			seen <- pod.Name
		},
	}

	g := gomock.NewController(t)
	mockClusterCache := clustermocks.NewMockRegisteredClustersCache(g)
	expectSync(mockClusterCache, []*v2pb.Cluster{&_testCluster}, []*v2pb.Cluster{&_testCluster})

	// what client.Watcher builds, over a fake source instead of an API server
	var informer cache.Controller
	mockClient := clientmocks.NewMockFederatedClient(g)
	mockClient.EXPECT().Watcher(gomock.Any(), gomock.Any()).DoAndReturn(
		func(params []*client.WatcherParams, _ *v2pb.Cluster) ([]*client.ResourceWatcher, error) {
			store, controller := cache.NewInformerWithOptions(cache.InformerOptions{
				ListerWatcher: source,
				ObjectType:    params[0].ObjType,
				Handler:       params[0].ResourceEventHandler,
			})
			informer = controller
			return []*client.ResourceWatcher{{Store: store, Controller: controller}}, nil
		})

	w, scope := newTestWatcher(t, mockClusterCache, mockClient, &client.WatcherParams{
		ResourceName:         "pods",
		ObjType:              &corev1.Pod{},
		ResourceEventHandler: handler,
	})
	w.sync()
	defer w.stopAll()

	// Add the pods once the initial, empty list is done, so that both arrive
	// over the watch in order: "a" first, then "b".
	ctx, cancel := context.WithTimeout(context.Background(), wait.ForeverTestTimeout)
	defer cancel()
	require.True(t, cache.WaitForCacheSync(ctx.Done(), informer.HasSynced))
	source.Add(&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "default"}})
	source.Add(&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "b", Namespace: "default"}})

	select {
	case name := <-seen:
		require.Equal(t, "b", name)
	case <-time.After(wait.ForeverTestTimeout):
		t.Fatal("the watch stopped delivering events after its handler panicked")
	}
	require.Equal(t, int64(1), counter(scope, _failureWatchPanicMetricName))

	value, ok := w.clusterInfoMap.Load(_testCluster.Name)
	require.True(t, ok)
	require.False(t, value.(*watchInfo).hasExited())
}

// A watch controller that dies on its own leaves its resource unwatched until
// the next sync rebuilds the cluster's watches.
func TestSyncRebuildsExitedWatch(t *testing.T) {
	tests := []struct {
		name string
		run  func(stopCh <-chan struct{})
	}{
		{
			name: "controller panicked",
			run:  func(<-chan struct{}) { panic("test panic") },
		},
		{
			name: "controller returned",
			run:  func(<-chan struct{}) {},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := gomock.NewController(t)
			mockClusterCache := clustermocks.NewMockRegisteredClustersCache(g)

			rebuilt, rebuiltReturned := blockingController()
			mockClient := clientmocks.NewMockFederatedClient(g)
			gomock.InOrder(
				mockClient.EXPECT().Watcher(gomock.Any(), gomock.Any()).Return(
					[]*client.ResourceWatcher{{Controller: &fakeController{runFunc: tt.run}}}, nil),
				mockClient.EXPECT().Watcher(gomock.Any(), gomock.Any()).Return(
					[]*client.ResourceWatcher{{Controller: rebuilt}}, nil),
			)

			w, _ := newTestWatcher(t, mockClusterCache, mockClient)

			expectSync(mockClusterCache, []*v2pb.Cluster{&_testCluster}, []*v2pb.Cluster{&_testCluster})
			w.sync()

			value, ok := w.clusterInfoMap.Load(_testCluster.Name)
			require.True(t, ok)
			dead := value.(*watchInfo)
			waitClosed(t, dead.exited[0], "the dead watch to be flagged")

			expectSync(mockClusterCache, []*v2pb.Cluster{&_testCluster}, []*v2pb.Cluster{&_testCluster})
			w.sync()

			value, ok = w.clusterInfoMap.Load(_testCluster.Name)
			require.True(t, ok)
			require.NotSame(t, dead, value.(*watchInfo))
			require.False(t, value.(*watchInfo).hasExited())

			w.stopAll()
			waitClosed(t, rebuiltReturned, "the rebuilt watch to stop")
		})
	}
}

// A watch outlives its cluster's spells of not being ready: a rebuilt
// informer would start empty and miss every delete made in between.
func TestSyncKeepsWatchOfNotReadyCluster(t *testing.T) {
	g := gomock.NewController(t)
	mockClusterCache := clustermocks.NewMockRegisteredClustersCache(g)

	// a single watch over the whole test
	ctrl, returned := blockingController()
	mockClient := clientmocks.NewMockFederatedClient(g)
	mockClient.EXPECT().Watcher(gomock.Any(), gomock.Any()).Return(
		[]*client.ResourceWatcher{{Controller: ctrl}}, nil)

	w, _ := newTestWatcher(t, mockClusterCache, mockClient)
	registered := []*v2pb.Cluster{&_testCluster}

	// registered but not ready yet: not watched
	expectSync(mockClusterCache, nil, registered)
	w.sync()
	_, ok := w.clusterInfoMap.Load(_testCluster.Name)
	require.False(t, ok)

	// ready: watched
	expectSync(mockClusterCache, registered, registered)
	w.sync()
	value, ok := w.clusterInfoMap.Load(_testCluster.Name)
	require.True(t, ok)
	watched := value.(*watchInfo)

	// not ready again: the same watch keeps running
	expectSync(mockClusterCache, nil, registered)
	w.sync()
	value, ok = w.clusterInfoMap.Load(_testCluster.Name)
	require.True(t, ok)
	require.Same(t, watched, value.(*watchInfo))
	requireOpen(t, watched.resourceWatchers[0].StopCh, "a not-ready cluster's watch must keep running")

	// ready once more: still the same watch
	expectSync(mockClusterCache, registered, registered)
	w.sync()
	value, ok = w.clusterInfoMap.Load(_testCluster.Name)
	require.True(t, ok)
	require.Same(t, watched, value.(*watchInfo))

	w.stopAll()
	waitClosed(t, returned, "the watch to stop")
}

// Only a cluster that is no longer registered loses its watch.
func TestSyncRemovesDeregisteredCluster(t *testing.T) {
	removed := newTestCluster("removed")
	kept := newTestCluster("kept")

	g := gomock.NewController(t)
	mockClusterCache := clustermocks.NewMockRegisteredClustersCache(g)

	removedCtrl, removedReturned := blockingController()
	keptCtrl, keptReturned := blockingController()
	mockClient := clientmocks.NewMockFederatedClient(g)
	mockClient.EXPECT().Watcher(gomock.Any(), removed).Return(
		[]*client.ResourceWatcher{{Controller: removedCtrl}}, nil)
	mockClient.EXPECT().Watcher(gomock.Any(), kept).Return(
		[]*client.ResourceWatcher{{Controller: keptCtrl}}, nil)

	w, _ := newTestWatcher(t, mockClusterCache, mockClient)

	both := []*v2pb.Cluster{removed, kept}
	expectSync(mockClusterCache, both, both)
	w.sync()

	// removed is deregistered; kept is still registered, though not ready
	expectSync(mockClusterCache, nil, []*v2pb.Cluster{kept})
	w.sync()

	waitClosed(t, removedReturned, "the deregistered cluster's watch to stop")
	_, ok := w.clusterInfoMap.Load(removed.Name)
	require.False(t, ok)

	value, ok := w.clusterInfoMap.Load(kept.Name)
	require.True(t, ok)
	requireOpen(t, value.(*watchInfo).resourceWatchers[0].StopCh, "a registered cluster's watch must keep running")

	w.stopAll()
	waitClosed(t, keptReturned, "the kept cluster's watch to stop")
}

// client.Watcher writes each cluster's client into the params it is given,
// so every cluster must get its own copy, with the recovering handler.
func TestSyncGivesEachClusterItsOwnWatcherParams(t *testing.T) {
	shared := &client.WatcherParams{
		ResourceName:         "pods",
		ResourceEventHandler: cache.ResourceEventHandlerFuncs{},
	}
	a := newTestCluster("a")
	b := newTestCluster("b")

	g := gomock.NewController(t)
	mockClusterCache := clustermocks.NewMockRegisteredClustersCache(g)
	expectSync(mockClusterCache, []*v2pb.Cluster{a, b}, []*v2pb.Cluster{a, b})

	var got [][]*client.WatcherParams
	mockClient := clientmocks.NewMockFederatedClient(g)
	mockClient.EXPECT().Watcher(gomock.Any(), gomock.Any()).DoAndReturn(
		func(params []*client.WatcherParams, _ *v2pb.Cluster) ([]*client.ResourceWatcher, error) {
			// as client.Watcher does
			for _, p := range params {
				p.Client = &rest.RESTClient{}
			}
			got = append(got, params)
			return nil, nil
		}).Times(2)

	w, _ := newTestWatcher(t, mockClusterCache, mockClient, shared)
	w.sync()

	require.Nil(t, shared.Client, "the shared params must not be written to")
	require.Len(t, got, 2)
	require.NotSame(t, got[0][0], got[1][0])
	for _, params := range got {
		require.Len(t, params, 1)
		require.NotSame(t, shared, params[0])
		require.Equal(t, shared.ResourceName, params[0].ResourceName)
		require.IsType(t, &recoveringHandler{}, params[0].ResourceEventHandler)
	}
}

func TestSyncError(t *testing.T) {
	tt := []struct {
		existingCluster *v2pb.Cluster
		newCluster      *v2pb.Cluster
		msg             string
	}{
		{
			existingCluster: &v2pb.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "existingCluster",
				},
			},
			newCluster: &_testCluster,
			msg:        "error in adding cluster",
		},
		{
			existingCluster: &_testCluster,
			newCluster: &v2pb.Cluster{
				ObjectMeta: _testCluster.ObjectMeta,
				Spec: v2pb.ClusterSpec{
					Cluster: &v2pb.ClusterSpec_Kubernetes{
						Kubernetes: &v2pb.KubernetesSpec{
							Rest: &v2pb.ConnectionSpec{
								Host: _testCluster.Spec.GetKubernetes().Rest.Host,
								Port: "NewPort",
							},
						},
					},
				},
			},
			msg: "error in updating cluster",
		},
	}

	for _, test := range tt {
		t.Run(test.msg, func(t *testing.T) {
			g := gomock.NewController(t)
			mockClusterCache := clustermocks.NewMockRegisteredClustersCache(g)

			ctrl, returned := blockingController()
			mockClient := clientmocks.NewMockFederatedClient(g)
			gomock.InOrder(
				mockClient.EXPECT().Watcher(gomock.Any(), gomock.Any()).Return(nil, errors.New("test error")),
				mockClient.EXPECT().Watcher(gomock.Any(), gomock.Any()).Return(
					[]*client.ResourceWatcher{{Controller: ctrl}}, nil),
			)

			federatedWatcher, scope := newTestWatcher(t, mockClusterCache, mockClient)

			federatedWatcher.clusterInfoMap.Store(test.existingCluster.Name, &watchInfo{
				cluster: test.existingCluster,
			})

			expectSync(mockClusterCache, []*v2pb.Cluster{test.newCluster}, []*v2pb.Cluster{test.newCluster})
			federatedWatcher.sync()

			require.Equal(t, int64(1), counter(scope, _failureWatchAddMetricName))
			_, ok := federatedWatcher.clusterInfoMap.Load(test.newCluster.Name)
			require.False(t, ok, "a cluster whose watch failed must not look watched")

			// so the next sync retries it
			expectSync(mockClusterCache, []*v2pb.Cluster{test.newCluster}, []*v2pb.Cluster{test.newCluster})
			federatedWatcher.sync()

			_, ok = federatedWatcher.clusterInfoMap.Load(test.newCluster.Name)
			require.True(t, ok)
			require.Equal(t, int64(1), counter(scope, _failureWatchAddMetricName))

			federatedWatcher.stopAll()
			waitClosed(t, returned, "the watch to stop")
		})
	}
}

func TestNewFederatedWatcherWithoutScope(t *testing.T) {
	// A nil Scope disables metrics instead of panicking in the constructor.
	require.NotPanics(t, func() {
		w := NewFederatedWatcher(FederatedWatcherParams{Logger: zapr.NewLogger(zap.NewNop())})
		w.(*watcher).metrics.MetricsScope.Counter(_failureWatchAddMetricName).Inc(1)
	})
}

func TestStart(t *testing.T) {
	g := gomock.NewController(t)
	synced := make(chan struct{})
	mockClusterCache := clustermocks.NewMockRegisteredClustersCache(g)
	mockClusterCache.EXPECT().GetClusters(cluster.AllClusters).Return([]*v2pb.Cluster{})
	mockClusterCache.EXPECT().GetClusters(cluster.ReadyClusters).DoAndReturn(
		func(cluster.FilterType) []*v2pb.Cluster {
			close(synced)
			return []*v2pb.Cluster{}
		})

	federatedWatcher, _ := newTestWatcher(t, mockClusterCache, nil)
	federatedWatcher.period = time.Hour // one sync is enough

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		federatedWatcher.Start(ctx)
	}()

	waitClosed(t, synced, "the first sync")
	cancel()
	waitClosed(t, done, "Start to return once its context is done")

	// Only the first Start has any effect. A second sync loop would query the
	// cluster cache again, which the mock does not expect, and would never
	// return on this context.
	federatedWatcher.Start(context.Background())
}

// Cancelling Start's context stops every watch it started, not just the sync loop.
func TestStartStopsWatchesOnCancel(t *testing.T) {
	g := gomock.NewController(t)
	mockClusterCache := clustermocks.NewMockRegisteredClustersCache(g)
	expectSync(mockClusterCache, []*v2pb.Cluster{&_testCluster}, []*v2pb.Cluster{&_testCluster})

	synced := make(chan struct{})
	ctrl, returned := blockingController()
	mockClient := clientmocks.NewMockFederatedClient(g)
	mockClient.EXPECT().Watcher(gomock.Any(), gomock.Any()).DoAndReturn(
		func([]*client.WatcherParams, *v2pb.Cluster) ([]*client.ResourceWatcher, error) {
			close(synced)
			return []*client.ResourceWatcher{{Controller: ctrl}}, nil
		})

	w, _ := newTestWatcher(t, mockClusterCache, mockClient)
	w.period = time.Hour // one sync is enough

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		w.Start(ctx)
	}()

	// the sync that adds the watch runs to completion before Start sees the cancel
	waitClosed(t, synced, "the first sync to add the watch")
	cancel()
	waitClosed(t, done, "Start to return once its context is done")
	waitClosed(t, returned, "the watch to stop")

	empty := true
	w.clusterInfoMap.Range(func(_, _ interface{}) bool {
		empty = false
		return false
	})
	require.True(t, empty, "a stopped watcher must not keep any cluster")
}

func TestShouldClusterBeUpdatedInWatcherCache(t *testing.T) {
	withRest := func(host, port string) *v2pb.Cluster {
		return &v2pb.Cluster{Spec: v2pb.ClusterSpec{Cluster: &v2pb.ClusterSpec_Kubernetes{
			Kubernetes: &v2pb.KubernetesSpec{Rest: &v2pb.ConnectionSpec{Host: host, Port: port}},
		}}}
	}
	noKubernetes := &v2pb.Cluster{}
	noRest := &v2pb.Cluster{Spec: v2pb.ClusterSpec{Cluster: &v2pb.ClusterSpec_Kubernetes{
		Kubernetes: &v2pb.KubernetesSpec{},
	}}}

	tests := []struct {
		name   string
		stored *v2pb.Cluster
		latest *v2pb.Cluster
		want   bool
	}{
		{name: "same endpoint", stored: withRest("h", "1"), latest: withRest("h", "1"), want: false},
		{name: "host change", stored: withRest("h", "1"), latest: withRest("h2", "1"), want: true},
		{name: "port change", stored: withRest("h", "1"), latest: withRest("h", "2"), want: true},
		{name: "no kubernetes spec on either side", stored: noKubernetes, latest: noKubernetes, want: false},
		{name: "no rest spec on either side", stored: noRest, latest: noRest, want: false},
		{name: "kubernetes spec removed", stored: withRest("h", "1"), latest: noKubernetes, want: true},
		{name: "kubernetes spec added", stored: noKubernetes, latest: withRest("h", "1"), want: true},
		{name: "rest spec removed", stored: withRest("h", "1"), latest: noRest, want: true},
		{name: "rest spec added", stored: noRest, latest: withRest("h", "1"), want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, shouldClusterBeUpdatedInWatcherCache(tt.stored, tt.latest))
		})
	}
}
