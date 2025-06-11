package watch

import (
	"context"
	"fmt"
	"sync"
	"time"

	federatedClient "code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/client"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/cluster"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/constants"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/metrics"
	"github.com/go-logr/logr"
	"github.com/uber-go/tally"
	"k8s.io/apimachinery/pkg/util/wait"
	v2beta1pb "michelangelo/api/v2beta1"
)

const _defaultSyncPeriod = 10 * time.Second

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
	FederatedClient *federatedClient.Client
	Logger          logr.Logger
	WatcherParams   []*federatedClient.WatcherParams
	Scope           tally.Scope
}

// watcher implements the FederatedWatcher
type watcher struct {
	log       logr.Logger
	metrics   *metrics.ControllerMetrics
	startOnce sync.Once
	period    time.Duration

	clusterCache    cluster.RegisteredClustersCache
	federatedClient *federatedClient.Client
	clusterInfoMap  sync.Map
	watches         []*federatedClient.WatcherParams
}

type watchInfo struct {
	resourceWatchers []*federatedClient.ResourceWatcher
	cluster          *v2beta1pb.Cluster
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
	wait.Forever(func() {
		select {
		case <-ctx.Done():
			fmt.Printf("exiting watcher:%+v\n", ctx.Err())
			return
		default:
		}

		r.log.Info("syncing watches")

		err := r.sync()
		if err != nil {
			r.log.Error(err, "failed to sync watches, will retry", "period", r.period.String())
		}

	}, r.period)
}

// sync sets up watches based on the watcher params across all clusters.
func (r *watcher) sync() error {
	clusters := r.clusterCache.GetClusters(cluster.AllClusters)

	clusterNames := make(map[string]struct{})
	for _, cluster := range clusters {
		clusterNames[cluster.Name] = struct{}{}

		// sync watch for any new cluster
		clusterInfo, ok := r.clusterInfoMap.Load(cluster.Name)
		if !ok {
			r.log.Info("setting up watch for new cluster", "name", cluster.Name)
			err := r.addNewClusterToCache(r.watches, cluster)
			if err != nil {
				return err
			}
			continue
		}

		// check if the cluster has been updated
		wi := clusterInfo.(watchInfo)
		if shouldClusterBeUpdatedInWatcherCache(wi.cluster, cluster) {
			r.log.Info("updating watch for existing cluster", "name", cluster.Name)
			err := r.updateClusterInCache(wi, r.watches, cluster)
			if err != nil {
				return err
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

	return nil
}

func (r *watcher) updateClusterInCache(
	info watchInfo,
	watcherParams []*federatedClient.WatcherParams,
	cluster *v2beta1pb.Cluster) error {
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
	watcherParams []*federatedClient.WatcherParams,
	cluster *v2beta1pb.Cluster) error {
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

var _failureWatchPanicMetricName = "watch_panic"

func (r *watcher) startWatchController(watchInfo *federatedClient.ResourceWatcher, clusterName string) {
	// Make sure to initialize the channel outside of the goroutine. We do this because there could be a
	// race between starting the controller and an update in the cluster cache. In the update case, we will
	// try to close the channel. And closing a nil channel will panic.
	watchInfo.StopCh = make(chan struct{})
	// start the controller in a goroutine with a recoverer to handle any panics from the controller.
	go r.startWatchGoRoutine(watchInfo, clusterName)
}

// DO NOT call directly. Only caller should be startWatchController.
// This is separated into a method to enable unit testing.
func (r *watcher) startWatchGoRoutine(watchInfo *federatedClient.ResourceWatcher, clusterName string) {
	defer func() {
		if rvr := recover(); rvr != nil {
			// watch controller run panicked
			// log the error
			r.log.Error(fmt.Errorf("%+v", rvr), "Watch controller exited with panic", "clusterName", clusterName)
			// emit a metrics that can be alerted on
			r.metrics.MetricsScope.Counter(_failureWatchPanicMetricName).Inc(1)
		}
	}()

	r.log.Info("Starting watch controller", "clusterName", clusterName)
	watchInfo.Controller.Run(watchInfo.StopCh)
}

func shouldClusterBeUpdatedInWatcherCache(
	stored *v2beta1pb.Cluster, latest *v2beta1pb.Cluster) bool {
	return stored.Spec.GetKubernetes().Rest.Host != latest.Spec.GetKubernetes().Rest.Host ||
		stored.Spec.GetKubernetes().Rest.Port != latest.Spec.GetKubernetes().Rest.Port
}
