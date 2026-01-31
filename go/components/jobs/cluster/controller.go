package cluster

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/michelangelo-ai/michelangelo/go/api"
	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	apiutils "github.com/michelangelo-ai/michelangelo/go/api/utils"
	"github.com/michelangelo-ai/michelangelo/go/components/controllerutil"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/client"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/metrics"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"github.com/uber-go/tally"
	"go.uber.org/fx"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	ctrl "sigs.k8s.io/controller-runtime"
	k8scontrollerutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	_clusterDeleteFinalizer  = "clusters.michelangelo.uber.com/finalizer"
	_clusterMonitoringPeriod = 30 * time.Second
)

// Reconciler reconciles a Cluster object.
type Reconciler struct {
	api.Handler
	apiHandlerFactory apiHandler.Factory
	log               logr.Logger
	metrics           *metrics.ControllerMetrics

	clusterClient client.FederatedClient

	// clusterDataMap is a mapping of ClusterName and the cluster specific details.
	clusterDataMap *clusterMap
}

// Params for controller constructor
type Params struct {
	fx.In
	ClusterClient     client.FederatedClient
	Scope             tally.Scope
	APIHandlerFactory apiHandler.Factory
}

// Result is the output of the module.
type Result struct {
	fx.Out

	Reconciler              *Reconciler
	RegisteredClustersCache RegisteredClustersCache
}

const _controllerName = "cluster"

// NewReconciler returns a new reconciler.
func NewReconciler(p Params) Result {
	reconciler := &Reconciler{
		apiHandlerFactory: p.APIHandlerFactory,
		clusterClient:     p.ClusterClient,
		clusterDataMap:    &clusterMap{},
		metrics:           metrics.NewControllerMetrics(p.Scope, _controllerName),
	}

	return Result{
		Reconciler:              reconciler,
		RegisteredClustersCache: reconciler,
	}
}

var _ RegisteredClustersCache = (*Reconciler)(nil)

// GetCluster returns the cluster with the given name, if found.
func (r *Reconciler) GetCluster(name string) *v2pb.Cluster {
	data := r.clusterDataMap.get(name)
	if data != nil {
		return data.cachedObj
	}
	return nil
}

// GetClusters returns a list of all clusters based on the filter.
func (r *Reconciler) GetClusters(filter FilterType) []*v2pb.Cluster {
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

	reconcileSuccess := false
	defer func() {
		if reconcileSuccess {
			r.metrics.MetricsScope.Counter(_clusterSuccessCountMetricName).Inc(1)
			timer.Stop()
		}
	}()

	if req.Namespace != constants.ClustersNamespace {
		r.metrics.MetricsScope.Tagged(map[string]string{constants.
			FailureReasonKey: _clusterErrorInNamespace}).
			Counter(_clusterFailedCountMetricName).Inc(1)
		return ctrl.Result{}, fmt.Errorf(
			"cluster must only belong to the namespace %s", constants.ClustersNamespace)
	}

	// retrieve the cluster
	var cluster v2pb.Cluster
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
		if !k8scontrollerutil.ContainsFinalizer(&cluster, _clusterDeleteFinalizer) {
			k8scontrollerutil.AddFinalizer(&cluster, _clusterDeleteFinalizer)
			if err := r.Update(ctx, &cluster, &metav1.UpdateOptions{}); err != nil {
				r.metrics.MetricsScope.Tagged(map[string]string{constants.
					FailureReasonKey: _clusterErrorWhileUpdate}).
					Counter(_clusterFailedCountMetricName).Inc(1)
				return ctrl.Result{}, err
			}
		}
	} else {
		log.Info("Cluster controller observed a cluster deletion")
		if k8scontrollerutil.ContainsFinalizer(&cluster, _clusterDeleteFinalizer) {
			// our finalizer is present
			r.delCluster(&cluster)
			k8scontrollerutil.RemoveFinalizer(&cluster, _clusterDeleteFinalizer)
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
	r.delCluster(&cluster)
	r.addCluster(&cluster, log)

	reconcileSuccess = true
	return ctrl.Result{}, nil
}

func (r *Reconciler) isClusterSpecSame(cachedObj *v2pb.Cluster, newObj *v2pb.Cluster) bool {
	return cachedObj.Spec.GetKubernetes().GetRest().GetHost() == newObj.Spec.GetKubernetes().GetRest().GetHost() &&
		cachedObj.Spec.GetKubernetes().GetRest().GetPort() == newObj.Spec.GetKubernetes().GetRest().GetPort() &&
		cachedObj.Spec.GetRegion() == newObj.Spec.GetRegion() &&
		cachedObj.Spec.GetZone() == newObj.Spec.GetZone() &&
		cachedObj.Spec.GetDc() == newObj.Spec.GetDc() &&
		cachedObj.Spec.GetSla() == newObj.Spec.GetSla()
}

func (r *Reconciler) delCluster(obj *v2pb.Cluster) {
	r.clusterDataMap.delete(obj.Name)
}

func (r *Reconciler) addCluster(obj *v2pb.Cluster, log logr.Logger) {
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
	clusterStatusRunnable := controllerutil.LeaderOnlyRunnable(r.periodicallyMonitorCluster)
	mgr.Add(clusterStatusRunnable)

	// Create a controller for the cluster resource
	return ctrl.NewControllerManagedBy(mgr).
		For(&v2pb.Cluster{}).
		Complete(r)
}

// monitor cluster periodically,
func (r *Reconciler) periodicallyMonitorCluster(ctx context.Context) error {
	wait.UntilWithContext(ctx, r.updateClusterInfo, _clusterMonitoringPeriod)
	return nil
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
	cluster *v2pb.Cluster,
	wg *sync.WaitGroup,
) {
	defer wg.Done()

	log := r.log.WithValues("cluster", cluster.GetName())
	log.V(1).Info("Updating cluster status")

	// this should only happen on the leader node
	if err := r.updateClusterHealth(cluster); err != nil {
		log.Error(err, "Failed to update the health for cluster")
		return
	}
}

// Update status by getting the health state from "/healthz".
func (r *Reconciler) updateClusterHealth(cluster *v2pb.Cluster) error {
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

	// Use thread-safe method to update both internal status and cached cluster object status
	clusterData.UpdateClusterAndStatus(currentClusterStatus)

	if err = r.UpdateStatus(ctx, cluster, &metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("failed to update the status of cluster err:%v", err)
	}

	return nil
}
