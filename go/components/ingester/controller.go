package ingester

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/michelangelo-ai/michelangelo/go/api"
	"github.com/michelangelo-ai/michelangelo/go/storage"
	"github.com/michelangelo-ai/michelangelo/go/storage/blobstorage"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	// defaultRequeuePeriod is the fallback requeue period when none is configured.
	defaultRequeuePeriod = 30 * time.Second
)

// Config holds configuration for the ingester controller.
type Config struct {
	// ConcurrentReconciles is the global default number of concurrent reconciliations
	ConcurrentReconciles int `yaml:"concurrentReconciles"`
	// RequeuePeriod is the global default period for requeuing reconciliations
	RequeuePeriod time.Duration `yaml:"requeuePeriod"`
	// ConcurrentReconcilesMap allows per-kind concurrency overrides
	ConcurrentReconcilesMap map[string]int `yaml:"concurrentReconcilesMap"`
	// RequeuePeriodMap allows per-kind requeue period overrides
	RequeuePeriodMap map[string]time.Duration `yaml:"requeuePeriodMap"`
	// DeletionDelay is the time to wait after DeletionTimestamp before removing the ingester finalizer.
	DeletionDelay time.Duration `yaml:"deletionDelay"`
}

// GetControllerConfig returns the resolved config for a specific CRD kind,
// falling back to global defaults when no per-kind override is set.
func (c Config) GetControllerConfig(kind string) Config {
	concurrency := c.ConcurrentReconciles
	requeuePeriod := c.RequeuePeriod

	if val, ok := c.ConcurrentReconcilesMap[kind]; ok {
		concurrency = val
	}
	if val, ok := c.RequeuePeriodMap[kind]; ok {
		requeuePeriod = val
	}

	return Config{
		ConcurrentReconciles: concurrency,
		RequeuePeriod:        requeuePeriod,
	}
}

// Reconciler reconciles a generic CRD object with metadata storage and blob storage.
type Reconciler struct {
	client.Client
	Log             logr.Logger
	Scheme          *runtime.Scheme
	TargetKind      client.Object
	MetadataStorage storage.MetadataStorage
	// BlobStorage is optional. When non-nil, objects are also uploaded to blob storage on sync.
	BlobStorage storage.BlobStorage
	Config      Config
}

// Reconcile is the main reconciliation loop.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	log := r.Log.WithValues("namespace", req.Namespace, "name", req.Name)
	log.Info("Reconciling object")

	object := r.TargetKind.DeepCopyObject().(client.Object)

	if err := r.Get(ctx, req.NamespacedName, object); err != nil {
		if client.IgnoreNotFound(err) == nil {
			log.Info("Object not found, may have been deleted")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to fetch object")
		return ctrl.Result{}, err
	}

	// K8s-initiated deletion: wait for DeletionDelay then remove finalizer.
	if !object.GetDeletionTimestamp().IsZero() {
		return r.handleDeletion(ctx, log, object)
	}

	// Annotation-driven deletion: sync → delete from storage → delete from etcd.
	if isDeletingAnnotationSet(object) {
		return r.handleDeletionAnnotation(ctx, log, object)
	}

	// Immutable objects: sync to storage then evict from etcd.
	if isImmutable(object) || isImmutableKind(object) {
		return r.handleImmutableObject(ctx, log, object)
	}

	return r.handleSync(ctx, log, object)
}

// handleSync upserts the object into metadata storage (and blob storage if configured).
func (r *Reconciler) handleSync(ctx context.Context, log logr.Logger, object client.Object) (ctrl.Result, error) {
	log.Info("Syncing object to metadata storage")

	indexedFields := r.getIndexedFields(object)

	if err := blobstorage.HandleUpdate(ctx, object, r.MetadataStorage, false, indexedFields, r.BlobStorage); err != nil {
		log.Error(err, "Failed to sync object")
		return ctrl.Result{RequeueAfter: r.getRequeuePeriod()}, err
	}

	log.Info("Successfully synced object")
	return ctrl.Result{}, nil
}

// handleDeletion waits for the configured DeletionDelay, then removes the ingester finalizer
// so k8s can proceed with garbage collection. Deletion from metadata/blob storage is handled
// by handleDeletionAnnotation (which runs before k8s deletion in the normal flow).
func (r *Reconciler) handleDeletion(ctx context.Context, log logr.Logger, object client.Object) (ctrl.Result, error) {
	log.Info("Object is being deleted in k8s")

	if !ctrlutil.ContainsFinalizer(object, api.IngesterFinalizer) {
		return ctrl.Result{}, nil
	}

	// Bypass delay if the object already has the deleting annotation (already cleaned up from storage).
	if isDeletingAnnotationSet(object) {
		return r.removeFinalizer(ctx, log, object)
	}

	// Enforce DeletionDelay so downstream readers have time to act on the deletion.
	if r.Config.DeletionDelay > 0 {
		expectedDeletionTime := object.GetDeletionTimestamp().Time.Add(r.Config.DeletionDelay)
		delta := expectedDeletionTime.Sub(time.Now())
		if delta > 0 {
			log.Info(fmt.Sprintf("Deletion scheduled after %v", delta))
			return ctrl.Result{Requeue: true, RequeueAfter: delta}, nil
		}
	}

	return r.removeFinalizer(ctx, log, object)
}

// handleDeletionAnnotation syncs the object, deletes it from storage, removes the finalizer,
// and finally deletes it from etcd.
func (r *Reconciler) handleDeletionAnnotation(ctx context.Context, log logr.Logger, object client.Object) (ctrl.Result, error) {
	log.Info("Object marked for deletion via annotation")

	gvk := object.GetObjectKind().GroupVersionKind()
	typeMeta := &metav1.TypeMeta{
		Kind:       gvk.Kind,
		APIVersion: gvk.GroupVersion().String(),
	}

	if err := blobstorage.HandleDelete(ctx, typeMeta, object, r.MetadataStorage, r.BlobStorage); err != nil {
		log.Error(err, "Failed to delete object from storage")
		return ctrl.Result{RequeueAfter: r.getRequeuePeriod()}, err
	}

	ctrlutil.RemoveFinalizer(object, api.IngesterFinalizer)
	if err := r.Update(ctx, object); err != nil {
		log.Error(err, "Failed to remove finalizer")
		return ctrl.Result{RequeueAfter: r.getRequeuePeriod()}, err
	}

	if err := r.Delete(ctx, object); err != nil {
		log.Error(err, "Failed to delete object from K8s")
		return ctrl.Result{RequeueAfter: r.getRequeuePeriod()}, err
	}

	log.Info("Successfully deleted object")
	return ctrl.Result{}, nil
}

// handleImmutableObject syncs the object to storage then evicts it from etcd.
func (r *Reconciler) handleImmutableObject(ctx context.Context, log logr.Logger, object client.Object) (ctrl.Result, error) {
	log.Info("Object is immutable, syncing to storage and removing from K8s/ETCD")

	indexedFields := r.getIndexedFields(object)

	if err := blobstorage.HandleUpdate(ctx, object, r.MetadataStorage, false, indexedFields, r.BlobStorage); err != nil {
		log.Error(err, "Failed to sync immutable object to storage")
		return ctrl.Result{RequeueAfter: r.getRequeuePeriod()}, err
	}

	ctrlutil.RemoveFinalizer(object, api.IngesterFinalizer)
	if err := r.Update(ctx, object); err != nil {
		log.Error(err, "Failed to remove finalizer")
		return ctrl.Result{RequeueAfter: r.getRequeuePeriod()}, err
	}

	if err := r.Delete(ctx, object); err != nil {
		log.Error(err, "Failed to delete immutable object from K8s")
		return ctrl.Result{RequeueAfter: r.getRequeuePeriod()}, err
	}

	log.Info("Successfully moved immutable object to storage only")
	return ctrl.Result{}, nil
}

// removeFinalizer removes the ingester finalizer and updates the object in k8s.
func (r *Reconciler) removeFinalizer(ctx context.Context, log logr.Logger, object client.Object) (ctrl.Result, error) {
	log.Info("Removing ingester finalizer")
	ctrlutil.RemoveFinalizer(object, api.IngesterFinalizer)
	if err := r.Update(ctx, object); err != nil {
		log.Error(err, "Failed to remove finalizer")
		return ctrl.Result{RequeueAfter: r.getRequeuePeriod()}, err
	}
	return ctrl.Result{}, nil
}

// SetupWithManager registers the controller with the given Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	kind := r.TargetKind.GetObjectKind().GroupVersionKind().Kind

	concurrentReconciles := r.Config.ConcurrentReconciles
	if concurrentReconciles <= 0 {
		concurrentReconciles = 1
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(r.TargetKind).
		Named(fmt.Sprintf("ingester_%s", kind)).
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

func (r *Reconciler) getIndexedFields(object client.Object) []storage.IndexedField {
	if indexedObj, ok := object.(storage.IndexedObject); ok {
		return indexedObj.GetIndexedKeyValuePairs()
	}
	return nil
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

func isImmutableKind(object client.Object) bool {
	type immutableKinder interface {
		IsImmutableKind() bool
	}
	if ik, ok := object.(immutableKinder); ok {
		return ik.IsImmutableKind()
	}
	return false
}

// getObjectTypeMeta returns TypeMeta for an object by inspecting its GVK.
func getObjectTypeMeta(object client.Object) *metav1.TypeMeta {
	gvk := object.GetObjectKind().GroupVersionKind()
	return &metav1.TypeMeta{
		Kind:       gvk.Kind,
		APIVersion: gvk.GroupVersion().String(),
	}
}

// suppress unused warning — used by handler helpers via runtime.Object
var _ = (*runtime.Scheme)(nil)
