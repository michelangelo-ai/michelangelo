package ingester

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/michelangelo-ai/michelangelo/go/api"
	"github.com/michelangelo-ai/michelangelo/go/storage"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	// Default reconcile period for requeuing
	defaultRequeuePeriod = 30 * time.Second
)

// Config holds configuration for the ingester controller
type Config struct {
	// ConcurrentReconciles is the number of concurrent reconciliations
	ConcurrentReconciles int `yaml:"concurrentReconciles"`
	// RequeuePeriod is the period for requeuing reconciliations
	RequeuePeriod time.Duration `yaml:"requeuePeriod"`
}

// Reconciler reconciles a generic CRD object with metadata storage
type Reconciler struct {
	client.Client
	Log             logr.Logger
	Scheme          *runtime.Scheme
	TargetKind      client.Object
	MetadataStorage storage.MetadataStorage
	Config          Config
}

// Reconcile is the main reconciliation loop
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("namespace", req.Namespace, "name", req.Name)
	log.Info("Reconciling object")

	// Create a new instance of the target kind
	object := r.TargetKind.DeepCopyObject().(client.Object)

	// Fetch the object from K8s
	if err := r.Get(ctx, req.NamespacedName, object); err != nil {
		if client.IgnoreNotFound(err) == nil {
			log.Info("Object not found, may have been deleted")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to fetch object")
		return ctrl.Result{}, err
	}

	// Check if object is being deleted
	if !object.GetDeletionTimestamp().IsZero() {
		return r.handleDeletion(ctx, log, object)
	}

	// Check if object is marked for deletion via annotation
	if isDeletingAnnotationSet(object) {
		return r.handleDeletionAnnotation(ctx, log, object)
	}

	// Check if object is immutable
	if isImmutable(object) {
		return r.handleImmutableObject(ctx, log, object)
	}

	// Normal reconciliation: sync to metadata storage
	return r.handleSync(ctx, log, object)
}

// handleSync syncs the object to metadata storage
func (r *Reconciler) handleSync(ctx context.Context, log logr.Logger, object client.Object) (ctrl.Result, error) {
	log.Info("Syncing object to metadata storage")

	// Extract indexed fields if object implements IndexedObject interface
	var indexedFields []storage.IndexedField
	if indexedObj, ok := object.(storage.IndexedObject); ok {
		indexedFields = indexedObj.GetIndexedKeyValuePairs()
	}

	// Upsert to metadata storage (includes all fields - no blob separation)
	if err := r.MetadataStorage.Upsert(ctx, object, false, indexedFields); err != nil {
		log.Error(err, "Failed to upsert object to metadata storage")
		return ctrl.Result{RequeueAfter: r.getRequeuePeriod()}, err
	}

	log.Info("Successfully synced object to metadata storage")
	return ctrl.Result{}, nil
}

// handleDeletion handles object deletion
func (r *Reconciler) handleDeletion(ctx context.Context, log logr.Logger, object client.Object) (ctrl.Result, error) {
	log.Info("Object is being deleted")

	// Check if our finalizer is present
	if !ctrlutil.ContainsFinalizer(object, api.IngesterFinalizer) {
		log.Info("Finalizer not present, nothing to do")
		return ctrl.Result{}, nil
	}

	log.Info("Deleting from metadata storage")

	// Delete from metadata storage
	gvk := object.GetObjectKind().GroupVersionKind()
	typeMeta := &metav1.TypeMeta{
		Kind:       gvk.Kind,
		APIVersion: gvk.GroupVersion().String(),
	}

	if err := r.MetadataStorage.Delete(ctx, typeMeta, object.GetNamespace(), object.GetName()); err != nil {
		log.Error(err, "Failed to delete from metadata storage")
		return ctrl.Result{RequeueAfter: r.getRequeuePeriod()}, err
	}

	// Remove our finalizer
	ctrlutil.RemoveFinalizer(object, api.IngesterFinalizer)
	if err := r.Update(ctx, object); err != nil {
		log.Error(err, "Failed to remove finalizer")
		return ctrl.Result{RequeueAfter: r.getRequeuePeriod()}, err
	}

	log.Info("Successfully removed finalizer")
	return ctrl.Result{}, nil
}

// handleDeletionAnnotation handles objects marked with DeletingAnnotation
func (r *Reconciler) handleDeletionAnnotation(ctx context.Context, log logr.Logger, object client.Object) (ctrl.Result, error) {
	log.Info("Object marked for deletion via annotation")

	// Delete from metadata storage first
	gvk := object.GetObjectKind().GroupVersionKind()
	typeMeta := &metav1.TypeMeta{
		Kind:       gvk.Kind,
		APIVersion: gvk.GroupVersion().String(),
	}

	if err := r.MetadataStorage.Delete(ctx, typeMeta, object.GetNamespace(), object.GetName()); err != nil {
		log.Error(err, "Failed to delete from metadata storage")
		return ctrl.Result{RequeueAfter: r.getRequeuePeriod()}, err
	}

	// Remove finalizer
	ctrlutil.RemoveFinalizer(object, api.IngesterFinalizer)
	if err := r.Update(ctx, object); err != nil {
		log.Error(err, "Failed to remove finalizer")
		return ctrl.Result{RequeueAfter: r.getRequeuePeriod()}, err
	}

	// Delete from K8s/ETCD
	if err := r.Delete(ctx, object); err != nil {
		log.Error(err, "Failed to delete from K8s")
		return ctrl.Result{RequeueAfter: r.getRequeuePeriod()}, err
	}

	log.Info("Successfully deleted object")
	return ctrl.Result{}, nil
}

// handleImmutableObject handles immutable objects
func (r *Reconciler) handleImmutableObject(ctx context.Context, log logr.Logger, object client.Object) (ctrl.Result, error) {
	log.Info("Object is immutable, removing from K8s/ETCD")

	// Ensure object is already in metadata storage
	var indexedFields []storage.IndexedField
	if indexedObj, ok := object.(storage.IndexedObject); ok {
		indexedFields = indexedObj.GetIndexedKeyValuePairs()
	}

	if err := r.MetadataStorage.Upsert(ctx, object, false, indexedFields); err != nil {
		log.Error(err, "Failed to ensure object is in metadata storage")
		return ctrl.Result{RequeueAfter: r.getRequeuePeriod()}, err
	}

	// Remove finalizer
	ctrlutil.RemoveFinalizer(object, api.IngesterFinalizer)
	if err := r.Update(ctx, object); err != nil {
		log.Error(err, "Failed to remove finalizer")
		return ctrl.Result{RequeueAfter: r.getRequeuePeriod()}, err
	}

	// Delete from K8s/ETCD (object now only exists in metadata storage)
	if err := r.Delete(ctx, object); err != nil {
		log.Error(err, "Failed to delete immutable object from K8s")
		return ctrl.Result{RequeueAfter: r.getRequeuePeriod()}, err
	}

	log.Info("Successfully moved immutable object to metadata storage only")
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	kind := r.TargetKind.GetObjectKind().GroupVersionKind().Kind
	controllerName := fmt.Sprintf("ingester_%s", kind)

	concurrentReconciles := r.Config.ConcurrentReconciles
	if concurrentReconciles <= 0 {
		concurrentReconciles = 1
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(r.TargetKind).
		Named(controllerName).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: concurrentReconciles,
		}).
		Complete(r)
}

// Helper functions

func (r *Reconciler) getRequeuePeriod() time.Duration {
	if r.Config.RequeuePeriod > 0 {
		return r.Config.RequeuePeriod
	}
	return defaultRequeuePeriod
}

func isDeletingAnnotationSet(object client.Object) bool {
	annotations := object.GetAnnotations()
	if annotations == nil {
		return false
	}
	return annotations[api.DeletingAnnotation] == "true"
}

func isImmutable(object client.Object) bool {
	annotations := object.GetAnnotations()
	if annotations == nil {
		return false
	}
	return annotations[api.ImmutableAnnotation] == "true"
}
