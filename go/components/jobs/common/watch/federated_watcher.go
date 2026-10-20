package watch

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/uber-go/tally"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"

	"github.com/michelangelo-ai/michelangelo/go/components/jobs/client"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/cluster"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/metrics"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

const _defaultSyncPeriod = 10 * time.Second

var (
	_failureWatchPanicMetricName            = "watch_panic"
	_failureWatchExitWithoutPanicMetricName = "watch_exit_without_panic"
	_failureWatchAddMetricName              = "watch_add_failure"
)

// FederatedWatcher setups watches across all the registered clusters.
//
// Whenever a new cluster is registered with JC and ready, an informer is
// created for it based on the watcher params. Informers are stopped
// when a cluster is deleted; a cluster that is only temporarily not
// ready keeps its informers.
//
// The cluster controller
// keeps an eye on the clusters and thus the clusters in ETCD are up-to-date.
type FederatedWatcher interface {
	// Start starts the eager syncing of watches. It returns once ctx is
	// done, after signalling every watch it started to stop. It does not
	// wait for an event handler that is already running to finish.
	Start(ctx context.Context)
}

// FederatedWatcherParams are the params to instantiate a new federated watcher.
type FederatedWatcherParams struct {
	ClusterCache    cluster.RegisteredClustersCache
	FederatedClient client.FederatedClient
	Logger          logr.Logger
	WatcherParams   []*client.WatcherParams
	Scope           tally.Scope
}

// watcher implements the FederatedWatcher
type watcher struct {
	log       logr.Logger
	metrics   *metrics.ControllerMetrics
	startOnce sync.Once
	period    time.Duration

	clusterCache    cluster.RegisteredClustersCache
	federatedClient client.FederatedClient
	clusterInfoMap  sync.Map
	watches         []*client.WatcherParams
}

type watchInfo struct {
	resourceWatchers []*client.ResourceWatcher
	// exited has a channel per resource watcher, closed if its controller
	// stops while its StopCh is still open.
	exited  []<-chan struct{}
	cluster *v2pb.Cluster
}

// stop closes the StopCh of each of the cluster's resource watchers.
func (wi *watchInfo) stop() {
	for _, rw := range wi.resourceWatchers {
		close(rw.StopCh)
	}
}

// hasExited reports whether any of the cluster's watch controllers stopped
// on its own, leaving its resource unwatched.
func (wi *watchInfo) hasExited() bool {
	for _, exited := range wi.exited {
		select {
		case <-exited:
			return true
		default:
		}
	}
	return false
}

// NewFederatedWatcher returns a new federated watcher.
func NewFederatedWatcher(p FederatedWatcherParams) FederatedWatcher {
	scope := p.Scope
	if scope == nil {
		scope = tally.NoopScope
	}
	return &watcher{
		clusterCache:    p.ClusterCache,
		federatedClient: p.FederatedClient,
		log:             p.Logger,
		clusterInfoMap:  sync.Map{},
		period:          _defaultSyncPeriod,
		watches:         p.WatcherParams,
		metrics:         &metrics.ControllerMetrics{MetricsScope: scope.SubScope("watcher")},
	}
}

// Start starts the eager syncing of watches. It returns once ctx is done,
// after signalling every watch it started to stop. It does not wait for an
// event handler that is already running to finish.
func (r *watcher) Start(ctx context.Context) {
	r.startOnce.Do(func() {
		r.start(ctx)
	})
}

func (r *watcher) start(ctx context.Context) {
	wait.Until(func() {
		r.log.Info("syncing watches")

		r.sync()

	}, r.period, ctx.Done())

	// the sync loop has stopped, so nothing can add a watch after this
	r.stopAll()

	r.log.Info("exiting watcher")
}

// stopAll stops the watches of every cluster and forgets the clusters.
func (r *watcher) stopAll() {
	r.clusterInfoMap.Range(func(key, value interface{}) bool {
		value.(*watchInfo).stop()
		r.clusterInfoMap.Delete(key)
		return true
	})
}

// sync sets up watches based on the watcher params across all clusters.
func (r *watcher) sync() {
	// only set up watches for ready clusters
	clusters := r.clusterCache.GetClusters(cluster.ReadyClusters)

	// but only remove the watch of a cluster that is no longer registered. A
	// cluster can be briefly not ready (the cluster controller re-adds it
	// without a status after a spec edit), and a rebuilt informer starts
	// empty, so objects deleted in that gap would never reach OnDelete.
	clusterNames := make(map[string]struct{})
	for _, registered := range r.clusterCache.GetClusters(cluster.AllClusters) {
		clusterNames[registered.Name] = struct{}{}
	}

	for _, cluster := range clusters {
		clusterNames[cluster.Name] = struct{}{}

		// sync watch for any new cluster
		clusterInfo, ok := r.clusterInfoMap.Load(cluster.Name)
		if !ok {
			r.log.Info("setting up watch for new cluster", "name", cluster.Name)
			err := r.addNewClusterToCache(cluster)
			if err != nil {
				// continue to the next cluster if there is an error
				r.log.Error(err, "failed to add watch for new cluster", "name", cluster.Name)
				// emit a metrics that can be alerted on
				r.metrics.MetricsScope.Counter(_failureWatchAddMetricName).Inc(1)
			}
			continue
		}

		// check if the cluster has been updated or one of its watches died
		wi := clusterInfo.(*watchInfo)
		endpointChanged := shouldClusterBeUpdatedInWatcherCache(wi.cluster, cluster)
		watchExited := wi.hasExited()
		if endpointChanged || watchExited {
			r.log.Info("updating watch for existing cluster", "name", cluster.Name,
				"endpointChanged", endpointChanged, "watchExited", watchExited)
			err := r.updateClusterInCache(wi, cluster)
			if err != nil {
				// continue to the next cluster if there is an error
				r.log.Error(err, "failed to add watch for updated cluster", "name", cluster.Name)
				// emit a metrics that can be alerted on
				r.metrics.MetricsScope.Counter(_failureWatchAddMetricName).Inc(1)
			}
		}
	}

	// remove watch for any cluster no longer registered
	r.clusterInfoMap.Range(func(key, value interface{}) bool {
		name := key.(string)
		info := value.(*watchInfo)

		if _, ok := clusterNames[name]; !ok {
			r.log.Info("Removing watch for cluster", "name", name)
			info.stop()

			r.clusterInfoMap.Delete(name)
		}
		return true
	})
}

func (r *watcher) updateClusterInCache(
	info *watchInfo,
	cluster *v2pb.Cluster) error {
	info.stop()
	r.clusterInfoMap.Delete(cluster.Name)

	timer := r.metrics.MetricsScope.Timer(constants.WatcherLatency).Start()
	resourceWatchers, err := r.federatedClient.Watcher(r.watcherParams(cluster.Name), cluster)
	timer.Stop()

	if err != nil {
		return err
	}

	newInfo := &watchInfo{
		resourceWatchers: resourceWatchers,
		cluster:          cluster,
	}
	for _, rw := range resourceWatchers {
		newInfo.exited = append(newInfo.exited, r.startWatchController(rw, cluster.Name))
	}

	r.clusterInfoMap.Store(cluster.Name, newInfo)

	r.log.Info("Updated cluster in the watch list", "clusterName", cluster.Name)
	return nil
}

func (r *watcher) addNewClusterToCache(
	cluster *v2pb.Cluster) error {
	timer := r.metrics.MetricsScope.Timer(constants.WatcherLatency).Start()
	resourceWatchers, err := r.federatedClient.Watcher(r.watcherParams(cluster.Name), cluster)
	timer.Stop()

	if err != nil {
		return err
	}

	info := &watchInfo{
		resourceWatchers: resourceWatchers,
		cluster:          cluster,
	}
	for _, rw := range resourceWatchers {
		info.exited = append(info.exited, r.startWatchController(rw, cluster.Name))
	}
	r.clusterInfoMap.Store(cluster.Name, info)

	r.log.Info("Added cluster to the watch list", "clusterName", cluster.Name)
	return nil
}

// watcherParams returns a fresh copy of the watcher params for one cluster's
// watches. client.Watcher sets each param's Client to that cluster's client,
// so the shared params must never be handed to it. Each copy's handler is
// wrapped so that a panic in it cannot take the watch down.
func (r *watcher) watcherParams(clusterName string) []*client.WatcherParams {
	params := make([]*client.WatcherParams, 0, len(r.watches))
	for _, wp := range r.watches {
		p := *wp
		p.ResourceEventHandler = &recoveringHandler{
			handler: wp.ResourceEventHandler,
			log:     r.log.WithValues("clusterName", clusterName, "resource", wp.ResourceName),
			metrics: r.metrics,
		}
		params = append(params, &p)
	}
	return params
}

// startWatchController starts the controller of a resource watcher. The
// returned channel is closed if the controller stops on its own.
func (r *watcher) startWatchController(watchInfo *client.ResourceWatcher, clusterName string) <-chan struct{} {
	// Make sure to initialize the channel outside of the goroutine. We do this because there could be a
	// race between starting the controller and an update in the cluster cache. In the update case, we will
	// try to close the channel. And closing a nil channel will panic.
	stopCh := make(chan struct{})
	watchInfo.StopCh = stopCh
	exited := make(chan struct{})
	// start the controller in a goroutine with a recoverer to handle any panics from the controller.
	// passing stopCh as a parameter to prevent a data race
	// between startWatchController() writing to watchInfo.StopCh and startWatchGoRoutine() reading it
	go r.startWatchGoRoutine(watchInfo, stopCh, exited, clusterName)
	return exited
}

// DO NOT call directly. Only caller should be startWatchController.
// This is separated into a method to enable unit testing.
func (r *watcher) startWatchGoRoutine(
	watchInfo *client.ResourceWatcher, stopCh, exited chan struct{}, clusterName string) {
	defer func() {
		rvr := recover()
		if rvr != nil {
			// watch controller run panicked
			// log the error
			r.log.Error(fmt.Errorf("%+v", rvr), "Watch controller exited with panic", "clusterName", clusterName)
			// emit a metrics that can be alerted on
			r.metrics.MetricsScope.Counter(_failureWatchPanicMetricName).Inc(1)
		}

		// check if the channel is closed
		select {
		case <-stopCh:
			// sync stopped the watch: its cluster was removed or the watch replaced
			r.log.Info("Stop channel is closed", "clusterName", clusterName)
		default:
			// Run only returns on its own if the controller died, which leaves
			// the resource unwatched
			r.log.Info("Stop channel is not closed", "clusterName", clusterName)
			if rvr == nil {
				r.log.Info("Watch controller exited without panic", "clusterName", clusterName)
				// emit a metric that can be alerted on
				r.metrics.MetricsScope.Counter(_failureWatchExitWithoutPanicMetricName).Inc(1)
			}
			// have the next sync rebuild the cluster's watches
			close(exited)
		}
	}()

	r.log.Info("Starting watch controller", "clusterName", clusterName)
	watchInfo.Controller.Run(stopCh)
}

// recoveringHandler recovers a panic in a watch's event handler. Left alone,
// the panic would unwind the informer's processing loop while its reflector
// keeps listing into a queue that nothing drains. Recovered, only that event
// is lost; the informer's periodic resync re-delivers any object it still has.
type recoveringHandler struct {
	handler cache.ResourceEventHandler
	log     logr.Logger
	metrics *metrics.ControllerMetrics
}

func (h *recoveringHandler) OnAdd(obj interface{}, isInInitialList bool) {
	defer h.recoverPanic()
	h.handler.OnAdd(obj, isInInitialList)
}

func (h *recoveringHandler) OnUpdate(oldObj, newObj interface{}) {
	defer h.recoverPanic()
	h.handler.OnUpdate(oldObj, newObj)
}

func (h *recoveringHandler) OnDelete(obj interface{}) {
	defer h.recoverPanic()
	h.handler.OnDelete(obj)
}

func (h *recoveringHandler) recoverPanic() {
	if rvr := recover(); rvr != nil {
		h.log.Error(fmt.Errorf("%+v", rvr), "Watch event handler panicked", "stack", string(debug.Stack()))
		// emit a metrics that can be alerted on
		h.metrics.MetricsScope.Counter(_failureWatchPanicMetricName).Inc(1)
	}
}

func shouldClusterBeUpdatedInWatcherCache(
	stored *v2pb.Cluster, latest *v2pb.Cluster) bool {
	storedRest := stored.Spec.GetKubernetes().GetRest()
	latestRest := latest.Spec.GetKubernetes().GetRest()
	if storedRest == nil || latestRest == nil {
		return storedRest != latestRest
	}
	return storedRest.Host != latestRest.Host ||
		storedRest.Port != latestRest.Port
}
