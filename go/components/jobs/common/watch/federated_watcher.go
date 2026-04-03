package watch

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/uber-go/tally"
	"k8s.io/apimachinery/pkg/util/wait"

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
// Whenever a new cluster is registered with JC, an informer is
// created for it based on the watcher params. Informers are stopped
// when a cluster is either put offline of deleted.
//
// The cluster controller
// keeps an eye on the clusters and thus the clusters in ETCD are up-to-date.
type FederatedWatcher interface {
	// Start starts the eager syncing of watches.
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
	cluster          *v2pb.Cluster
}

// NewFederatedWatcher returns a new federated watcher.
func NewFederatedWatcher(p FederatedWatcherParams) FederatedWatcher {
	return &watcher{
		clusterCache:    p.ClusterCache,
		federatedClient: p.FederatedClient,
		log:             p.Logger,
		clusterInfoMap:  sync.Map{},
		period:          _defaultSyncPeriod,
		watches:         p.WatcherParams,
		metrics:         &metrics.ControllerMetrics{MetricsScope: p.Scope.SubScope("watcher")},
	}
}

// Start starts the eager syncing of watches.
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

	r.log.Info("exiting watcher")
}

// sync sets up watches based on the watcher params across all clusters.
func (r *watcher) sync() {
	// only sync ready clusters
	clusters := r.clusterCache.GetClusters(cluster.ReadyClusters)

	clusterNames := make(map[string]struct{})
	for _, cluster := range clusters {
		clusterNames[cluster.Name] = struct{}{}

		// sync watch for any new cluster
		clusterInfo, ok := r.clusterInfoMap.Load(cluster.Name)
		if !ok {
			r.log.Info("setting up watch for new cluster", "name", cluster.Name)
			err := r.addNewClusterToCache(r.watches, cluster)
			if err != nil {
				// continue to the next cluster if there is an error
				r.log.Error(err, "failed to add watch for new cluster", "name", cluster.Name)
				// emit a metrics that can be alerted on
				r.metrics.MetricsScope.Counter(_failureWatchAddMetricName).Inc(1)
			}
			continue
		}

		// check if the cluster has been updated
		wi := clusterInfo.(watchInfo)
		if shouldClusterBeUpdatedInWatcherCache(wi.cluster, cluster) {
			r.log.Info("updating watch for existing cluster", "name", cluster.Name)
			err := r.updateClusterInCache(wi, r.watches, cluster)
			if err != nil {
				// continue to the next cluster if there is an error
				r.log.Error(err, "failed to add watch for updated cluster", "name", cluster.Name)
				// emit a metrics that can be alerted on
				r.metrics.MetricsScope.Counter(_failureWatchAddMetricName).Inc(1)
			}
		}
	}

	// remove watch for any cluster no longer in use
	r.clusterInfoMap.Range(func(key, value interface{}) bool {
		name := key.(string)
		info := value.(watchInfo)

		if _, ok := clusterNames[name]; !ok {
			r.log.Info("Removing watch for cluster", "name", name)
			for _, rw := range info.resourceWatchers {
				close(rw.StopCh)
			}

			r.clusterInfoMap.Delete(name)
		}
		return true
	})
}

func (r *watcher) updateClusterInCache(
	info watchInfo,
	watcherParams []*client.WatcherParams,
	cluster *v2pb.Cluster) error {
	for _, rw := range info.resourceWatchers {
		close(rw.StopCh)
	}
	r.clusterInfoMap.Delete(cluster.Name)

	timer := r.metrics.MetricsScope.Timer(constants.WatcherLatency).Start()
	resourceWatchers, err := r.federatedClient.Watcher(watcherParams, cluster)
	timer.Stop()

	if err != nil {
		return err
	}

	for _, rw := range resourceWatchers {
		r.startWatchController(rw, cluster.Name)
	}

	r.clusterInfoMap.Store(cluster.Name, watchInfo{
		resourceWatchers: resourceWatchers,
		cluster:          cluster,
	})

	r.log.Info("Updated cluster in the watch list", "clusterName", cluster.Name)
	return nil
}

func (r *watcher) addNewClusterToCache(
	watcherParams []*client.WatcherParams,
	cluster *v2pb.Cluster) error {
	timer := r.metrics.MetricsScope.Timer(constants.WatcherLatency).Start()
	resourceWatchers, err := r.federatedClient.Watcher(watcherParams, cluster)
	timer.Stop()

	if err != nil {
		return err
	}

	for _, rw := range resourceWatchers {
		r.startWatchController(rw, cluster.Name)
	}
	r.clusterInfoMap.Store(cluster.Name, watchInfo{
		resourceWatchers: resourceWatchers,
		cluster:          cluster,
	})

	r.log.Info("Added cluster to the watch list", "clusterName", cluster.Name)
	return nil
}

func (r *watcher) startWatchController(watchInfo *client.ResourceWatcher, clusterName string) {
	// Make sure to initialize the channel outside of the goroutine. We do this because there could be a
	// race between starting the controller and an update in the cluster cache. In the update case, we will
	// try to close the channel. And closing a nil channel will panic.
	stopCh := make(chan struct{})
	watchInfo.StopCh = stopCh
	// start the controller in a goroutine with a recoverer to handle any panics from the controller.
	// passing stopCh as a parameter to prevent a data race
	// between startWatchController() writing to watchInfo.StopCh and startWatchGoRoutine() reading it
	go r.startWatchGoRoutine(watchInfo, stopCh, clusterName)
}

// DO NOT call directly. Only caller should be startWatchController.
// This is separated into a method to enable unit testing.
func (r *watcher) startWatchGoRoutine(watchInfo *client.ResourceWatcher, stopCh chan struct{}, clusterName string) {
	defer func() {
		if rvr := recover(); rvr != nil {
			// watch controller run panicked
			// log the error
			r.log.Error(fmt.Errorf("%+v", rvr), "Watch controller exited with panic", "clusterName", clusterName)
			// emit a metrics that can be alerted on
			r.metrics.MetricsScope.Counter(_failureWatchPanicMetricName).Inc(1)
			return
		}

		// watch controller exited without panic
		r.log.Info("Watch controller exited without panic", "clusterName", clusterName)
		// emit a metric that can be alerted on
		r.metrics.MetricsScope.Counter(_failureWatchExitWithoutPanicMetricName).Inc(1)

		// check if the channel is closed
		select {
		case <-stopCh:
			r.log.Info("Stop channel is closed", "clusterName", clusterName)
		default:
			r.log.Info("Stop channel is not closed", "clusterName", clusterName)
		}
	}()

	r.log.Info("Starting watch controller", "clusterName", clusterName)
	watchInfo.Controller.Run(stopCh)
}

func shouldClusterBeUpdatedInWatcherCache(
	stored *v2pb.Cluster, latest *v2pb.Cluster) bool {
	return stored.Spec.GetKubernetes().Rest.Host != latest.Spec.GetKubernetes().Rest.Host ||
		stored.Spec.GetKubernetes().Rest.Port != latest.Spec.GetKubernetes().Rest.Port
}
