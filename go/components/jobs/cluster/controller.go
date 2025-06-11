package cluster

import (
	"context"
	"fmt"
	"sync"
	"time"

	infraCrds "code.uber.internal/infra/compute/k8s-crds/apis/compute.uber.com/v1beta1"
	clusterclient "code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/client"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/constants"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/skus"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/metrics"
	ctypes "code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/types"
	"github.com/go-logr/logr"
	"github.com/michelangelo-ai/michelangelo/go/api"
	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	apiutils "github.com/michelangelo-ai/michelangelo/go/api/utils"
	"github.com/uber-go/tally"
	"go.uber.org/fx"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	v2beta1pb "michelangelo/api/v2beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	_clusterDeleteFinalizer  = "clusters.michelangelo.uber.com/finalizer"
	_clusterMonitoringPeriod = 30 * time.Second
)

// Reconciler reconciles a object
// This controller is not leader elected and would run on all instances of the controller manager.
// This is a read only controller and does not make any changes to the cluster.
type Reconciler struct {
	api.Handler
	apiHandlerFactory apiHandler.Factory
	log               logr.Logger
	metrics           *metrics.ControllerMetrics

	clusterClient *clusterclient.Client

	// clusterDataMap is a mapping of ClusterName and the cluster specific details.
	clusterDataMap     *clusterMap
	resourcePoolsCache *resourcePoolCache
	skuConfigCache     *skuConfigCache
}

// Params for controller constructor
type Params struct {
	fx.In
	ClusterClient     *clusterclient.Client
	ResourcePoolCache ResourcePoolCache
	SkuConfigCache    skus.SkuConfigCache
	Scope             tally.Scope
	APIHandlerFactory apiHandler.Factory
}

// Result is the output of the module.
type Result struct {
	fx.Out

	Reconciler              ctypes.Reconciler `group:"reconciler"`
	RegisteredClustersCache RegisteredClustersCache
}

const _controllerName = "cluster"

// NewReconciler returns a new reconciler.
func NewReconciler(p Params) Result {
	reconciler := &Reconciler{
		apiHandlerFactory:  p.APIHandlerFactory,
		clusterClient:      p.ClusterClient,
		resourcePoolsCache: p.ResourcePoolCache.(*resourcePoolCache),
		skuConfigCache:     p.SkuConfigCache.(*skuConfigCache),
		clusterDataMap:     &clusterMap{},
		metrics:            metrics.NewControllerMetrics(p.Scope, _controllerName),
	}

	return Result{
		Reconciler:              reconciler,
		RegisteredClustersCache: reconciler,
	}
}

var _ RegisteredClustersCache = (*Reconciler)(nil)

// GetCluster returns the cluster with the given name, if found.
func (r *Reconciler) GetCluster(name string) *v2beta1pb.Cluster {
	data := r.clusterDataMap.get(name)
	if data != nil {
		return data.cachedObj
	}
	return nil
}

// GetClusters returns a list of all clusters based on the filter.
func (r *Reconciler) GetClusters(filter FilterType) []*v2beta1pb.Cluster {
	return r.clusterDataMap.getClustersByFilter(filter)
}

const (
	_clusterErrorInNamespace   = "error_in_namespace"
	_clusterErrorWhileRetrieve = "error_while_retrieving_cluster"
	_clusterErrorWhileUpdate   = "error_while_updating_cluster"

	_clusterInitiatedCountMetricName    = "reconcile_count"
	_clusterReconcileDurationMetricName = "success_reconcile_duration"
	_clusterFailedCountMetricName       = "failed_count"
	_clusterSuccessCountMetricName      = "success_count"
)

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	log := r.log.WithValues(_controllerName, req.NamespacedName.String())

	r.metrics.MetricsScope.Counter(_clusterInitiatedCountMetricName).Inc(1)
	timer := r.metrics.MetricsScope.Timer(_clusterReconcileDurationMetricName).Start()

	var reconcileSuccess = false
	defer func() {
		if reconcileSuccess {
			r.metrics.MetricsScope.Counter(_clusterSuccessCountMetricName).Inc(1)
			timer.Stop()
		}
		return
	}()

	// TODO: This check should go in the API validation hook.
	// https://t3.uberinternal.com/browse/MA-17444
	if req.Namespace != constants.ClustersNamespace {
		r.metrics.MetricsScope.Tagged(map[string]string{constants.
			FailureReasonKey: _clusterErrorInNamespace}).
			Counter(_clusterFailedCountMetricName).Inc(1)
		return ctrl.Result{}, fmt.Errorf(
			"cluster must only belong to the namespace %s", constants.ClustersNamespace)
	}

	// retrieve the cluster
	var cluster v2beta1pb.Cluster
	if err := r.Get(ctx, req.NamespacedName.Namespace, req.NamespacedName.Name, &metav1.GetOptions{},
		&cluster); err != nil {
		if apiutils.IsNotFoundError(err) {
			return ctrl.Result{}, nil
		}
		r.metrics.MetricsScope.Tagged(map[string]string{constants.
			FailureReasonKey: _clusterErrorWhileRetrieve}).
			Counter(_clusterFailedCountMetricName).Inc(1)
		return ctrl.Result{}, err
	}

	// examine DeletionTimestamp to determine if object is under deletion
	if cluster.ObjectMeta.DeletionTimestamp.IsZero() {
		// The object is not being deleted, so if it does not have our finalizer,
		// then lets add the finalizer and update the object. This is equivalent
		// registering our finalizer.
		if !controllerutil.ContainsFinalizer(&cluster, _clusterDeleteFinalizer) {
			controllerutil.AddFinalizer(&cluster, _clusterDeleteFinalizer)
			if err := r.Update(ctx, &cluster, &metav1.UpdateOptions{}); err != nil {
				r.metrics.MetricsScope.Tagged(map[string]string{constants.
					FailureReasonKey: _clusterErrorWhileUpdate}).
					Counter(_clusterFailedCountMetricName).Inc(1)
				return ctrl.Result{}, err
			}
		}
	} else {
		log.Info("Cluster controller observed a cluster deletion")
		if controllerutil.ContainsFinalizer(&cluster, _clusterDeleteFinalizer) {
			// our finalizer is present
			// TODO : move to a single transaction
			r.delCluster(&cluster)
			r.resourcePoolsCache.deletePoolsByCluster(&cluster)
			controllerutil.RemoveFinalizer(&cluster, _clusterDeleteFinalizer)
			if err := r.Update(ctx, &cluster, &metav1.UpdateOptions{}); err != nil {
				r.metrics.MetricsScope.Tagged(map[string]string{constants.
					FailureReasonKey: _clusterErrorWhileUpdate}).
					Counter(_clusterFailedCountMetricName).Inc(1)
				return ctrl.Result{}, err
			}
		}

		reconcileSuccess = true

		// Stop reconciliation as the item is being deleted
		log.Info("Cluster deleted")
		return ctrl.Result{}, nil
	}

	data := r.clusterDataMap.get(cluster.Name)
	if data == nil {
		log.Info("Cluster controller observed a new cluster")
		r.addCluster(&cluster, log)
		log.Info("Cluster added")

		reconcileSuccess = true
		return ctrl.Result{}, nil
	}

	if r.isClusterSpecSame(data.cachedObj, &cluster) {
		reconcileSuccess = true
		return ctrl.Result{}, nil
	}

	log.Info("Cluster controller observed an update", "cached_object", data.cachedObj, "new_object", &cluster)
	// re-add
	// TODO : move to a single transaction
	r.delCluster(&cluster)
	r.addCluster(&cluster, log)

	reconcileSuccess = true
	return ctrl.Result{}, nil
}

func (r *Reconciler) isClusterSpecSame(cachedObj *v2beta1pb.Cluster, newObj *v2beta1pb.Cluster) bool {
	return cachedObj.Spec.GetKubernetes().GetRest().GetHost() == newObj.Spec.GetKubernetes().GetRest().GetHost() &&
		cachedObj.Spec.GetKubernetes().GetRest().GetPort() == newObj.Spec.GetKubernetes().GetRest().GetPort() &&
		cachedObj.Spec.GetRegion() == newObj.Spec.GetRegion() &&
		cachedObj.Spec.GetZone() == newObj.Spec.GetZone() &&
		cachedObj.Spec.GetDc() == newObj.Spec.GetDc() &&
		cachedObj.Spec.GetSla() == newObj.Spec.GetSla()
}

func (r *Reconciler) delCluster(obj *v2beta1pb.Cluster) {
	r.clusterDataMap.delete(obj.Name)
}

func (r *Reconciler) addCluster(obj *v2beta1pb.Cluster, log logr.Logger) {
	clusterData := r.clusterDataMap.get(obj.Name)
	if clusterData != nil {
		log.Info("Cluster existed with this name, skipping adding", "cluster_name", obj.Name)
		return
	}

	r.clusterDataMap.add(obj.Name, &Data{cachedObj: obj.DeepCopy()})
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.log = mgr.GetLogger().
		WithName(_controllerName)
	apiHandler, err := r.apiHandlerFactory.GetAPIHandler(mgr.GetClient())
	if err != nil {
		return err
	}
	r.Handler = apiHandler

	// Update cluster health status only on the leader node
	clusterStatusRunnable := ctypes.LeaderOnlyRunnable(r.periodicallyMonitorCluster)
	mgr.Add(clusterStatusRunnable)

	// Update resource pools cache on every node
	resourcePoolsCacheRunnable := ctypes.NonLeaderRunnable(r.periodicallyUpdateResourcePoolsCache)
	mgr.Add(resourcePoolsCacheRunnable)

	// Create a controller for the cluster resource
	controller, err := controller.NewUnmanaged(_controllerName, mgr, controller.Options{
		Reconciler: ctypes.MakeSafeReconciler(r, _controllerName),
	})
	if err != nil {
		return fmt.Errorf("failed to create controller: %w", err)
	}

	// Watch for changes to the cluster resource on all nodes
	cont := &ctypes.NonLeaderReconciler{Controller: controller}
	err = cont.Watch(&source.Kind{Type: &v2beta1pb.Cluster{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return fmt.Errorf("failed to watch clusters: %w", err)
	}

	return mgr.Add(cont)
}

// monitor cluster periodically,
func (r *Reconciler) periodicallyMonitorCluster(ctx context.Context) error {
	wait.UntilWithContext(ctx, r.updateClusterInfo, _clusterMonitoringPeriod)
	return nil
}

func (r *Reconciler) periodicallyUpdateResourcePoolsCache(ctx context.Context) error {
	wait.UntilWithContext(ctx, r.updateResourcePoolsCache, _clusterMonitoringPeriod)
	return nil
}

func (r *Reconciler) updateResourcePoolsCache(ctx context.Context) {
	clusters := r.clusterDataMap.getClustersByFilter(AllClusters)

	var wg sync.WaitGroup
	for _, cluster := range clusters {
		wg.Add(1)
		go r.updateIndividualClusterResourcePoolsCache(cluster, &wg)
	}
	wg.Wait()
}

func (r *Reconciler) updateClusterInfo(ctx context.Context) {
	clusters := r.clusterDataMap.getClustersByFilter(AllClusters)

	var wg sync.WaitGroup
	for _, cluster := range clusters {
		wg.Add(1)
		go r.updateIndividualClusterInfo(cluster, &wg)
	}

	wg.Wait()
}

// updateClusterStatus checks cluster health and updates their status.
// For healthy clusters, we fetch their resource quota snapshot.
func (r *Reconciler) updateIndividualClusterInfo(
	cluster *v2beta1pb.Cluster,
	wg *sync.WaitGroup) {
	defer wg.Done()

	log := r.log.WithValues("cluster", cluster.GetName())
	log.V(1).Info("Updating cluster status")

	// this should only happen on the leader node
	if err := r.updateClusterHealth(cluster); err != nil {
		log.Error(err, "Failed to update the health for cluster")
		return
	}
}

func (r *Reconciler) updateIndividualClusterResourcePoolsCache(
	cluster *v2beta1pb.Cluster,
	wg *sync.WaitGroup) error {
	defer wg.Done()
	log := r.log.WithValues("cluster", cluster.GetName())
	log.V(1).Info("Updating resource pools cache")

	if err := r.updateResourcePools(cluster); err != nil {
		log.Error(err, "Failed to update the resource pools for cluster")
	}

	if err := r.updateGPUSkuCache(cluster); err != nil {
		log.Error(err, "Failed to update the GPU config map for cluster")
	}

	log.V(1).Info("Resource pools cache updated")
	return nil
}

// Update status by getting the health state from "/healthz".
func (r *Reconciler) updateClusterHealth(cluster *v2beta1pb.Cluster) error {
	clusterData := r.clusterDataMap.get(cluster.Name)
	if clusterData == nil {
		return fmt.Errorf("failed to retrieve stored data for cluster")
	}

	r.log.V(1).Info("Updating individual cluster health status", "cluster", cluster.GetName())

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	currentClusterStatus, err := r.clusterClient.GetClusterStatus(ctx, cluster)
	if err != nil {
		return err
	}

	clusterData.clusterStatus = currentClusterStatus
	cluster.Status = *currentClusterStatus
	if err = r.UpdateStatus(ctx, cluster, &metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("failed to update the status of cluster err:%v", err)
	}

	return nil
}

const (
	_sparkZonalClusterLabelKey   = "spark-zonal-cluster"
	_sparkZonalClusterLabelValue = "true"
)

func (r *Reconciler) updateResourcePools(cluster *v2beta1pb.Cluster) error {
	// If it is a zonal cluster used by Spark jobs, then do not fetch their resource pool information.
	// This is because we use these clusters purely to monitor the Spark jobs. We do not use them
	// for routing. When we do use the job scheduler for Spark jobs at a later point, we will use the
	// regional API servers for fetching the resource pools. Therefore, these zonal clusters will never be used
	// for routing jobs.
	if v, ok := cluster.Labels[_sparkZonalClusterLabelKey]; ok && v == _sparkZonalClusterLabelValue {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	clusterStatus := cluster.GetStatus()

	log := r.log.WithValues("cluster", cluster.GetName()).
		WithValues("clusterStatus", clusterStatus)
	log.V(1).Info("Fetching individual cluster resource pools")

	pools, err := func() (infraCrds.ResourcePoolList, error) {
		timer := r.metrics.MetricsScope.Timer(constants.GetResourcePoolsLatency).Start()
		defer timer.Stop()
		pools, err := r.clusterClient.GetResourcePools(ctx, cluster)
		if err != nil {
			return infraCrds.ResourcePoolList{}, err
		}
		return pools, nil
	}()
	if err != nil {
		return err
	}

	log.V(1).Info("Cluster resource pools found", "num_resource_pools", len(pools.Items))
	r.resourcePoolsCache.cleanup(cluster, pools)

	if !isClusterReady(&clusterStatus) {
		for _, pool := range pools.Items {
			// Delete the pool to the cache for unhealthy cluster
			log.V(1).Info("Deleting resource pool for unhealthy cluster", "pool", pool.GetName())
			r.resourcePoolsCache.delete(pool, cluster)
		}
		return nil
	}

	for _, pool := range pools.Items {
		// Add the pool to the cache
		log.V(1).Info("Adding/Updating resource pool", "pool", pool.GetName())
		r.resourcePoolsCache.addOrUpdate(pool, cluster)
	}
	return nil
}

func (r *Reconciler) updateGPUSkuCache(cluster *v2beta1pb.Cluster) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	clusterStatus := cluster.GetStatus()

	log := r.log.WithValues("cluster", cluster.GetName()).
		WithValues("clusterStatus", clusterStatus)
	log.V(1).Info("Fetching individual cluster config maps")

	configMaps, err := func() (corev1.ConfigMapList, error) {
		timer := r.metrics.MetricsScope.Timer(constants.GetSkuConfigMapLatency).Start()
		defer timer.Stop()
		configMaps, err := r.clusterClient.GetSkuConfigMaps(ctx, cluster)
		if err != nil {
			return corev1.ConfigMapList{}, err
		}
		return configMaps, nil
	}()
	if err != nil {
		return fmt.Errorf("error in fetching sku config maps for cluster %s. err: %v", cluster.Name, err)
	}

	// We do not need to worry about cleaning up unhealthy clusters because the scheduler will ensure to not
	// schedule jobs on them.
	r.skuConfigCache.addSkuMaps(configMaps.Items, cluster.Name)

	return nil
}
