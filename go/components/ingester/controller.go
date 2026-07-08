package ingester

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/michelangelo-ai/michelangelo/go/api"
	"github.com/michelangelo-ai/michelangelo/go/cascadedelete"
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
	// ConcurrentReconciles is the global default number of concurrent reconciliations
	ConcurrentReconciles int `yaml:"concurrentReconciles"`
	// RequeuePeriod is the global default period for requeuing reconciliations
	RequeuePeriod time.Duration `yaml:"requeuePeriod"`
	// ConcurrentReconcilesMap allows per-kind concurrency overrides
	ConcurrentReconcilesMap map[string]int `yaml:"concurrentReconcilesMap"`
	// RequeuePeriodMap allows per-kind requeue period overrides
	RequeuePeriodMap map[string]time.Duration `yaml:"requeuePeriodMap"`
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

// Option configures a Reconciler. Use the WithXxx functions below.
type Option func(*Reconciler)

// WithConfig sets controller configuration (concurrency limits and requeue period).
// If not provided, NewReconciler uses conservative defaults:
//   - 1 concurrent reconcile per controller
//   - 30s requeue period (defaultRequeuePeriod)
//
// Callers that need per-kind overrides should use Config.GetControllerConfig before passing.
func WithConfig(cfg Config) Option {
	return func(r *Reconciler) { r.config = cfg }
}

// WithRetainPolicy injects the per-kind cascade retain opt-in. For opted-in kinds,
// a non-apiserver delete (cascade GC, kubectl, GitOps) retains the object's final
// state in MySQL instead of leaving a stale row. Other kinds keep today's behavior.
// If not provided, no kind retains (NewStaticRetainPolicy with an empty set).
func WithRetainPolicy(rp cascadedelete.RetainPolicy) Option {
	return func(r *Reconciler) { r.retain = rp }
}

// Reconciler reconciles a generic CRD object with metadata storage.
//
// All fields are unexported. Exported struct fields become permanent public API surface —
// external packages can depend on them directly, making future removal a breaking change.
// Use NewReconciler() to construct instances.
type Reconciler struct {
	client.Client
	log             logr.Logger
	scheme          *runtime.Scheme
	targetKind      client.Object
	metadataStorage storage.MetadataStorage
	config          Config
	// retain answers whether a kind's final state must be retained in MySQL on a
	// non-apiserver delete. Defaults to an empty (no-retain) policy.
	retain cascadedelete.RetainPolicy
}

// NewReconciler creates a new Reconciler for the given CRD target kind.
//
// The c, log, scheme, targetKind, and metadataStorage parameters are required.
// Use WithConfig to set concurrency and requeue configuration; the default is
// 1 concurrent reconcile and a 30s requeue period.
func NewReconciler(
	c client.Client,
	log logr.Logger,
	scheme *runtime.Scheme,
	targetKind client.Object,
	metadataStorage storage.MetadataStorage,
	opts ...Option,
) *Reconciler {
	r := &Reconciler{
		Client:          c,
		log:             log,
		scheme:          scheme,
		targetKind:      targetKind,
		metadataStorage: metadataStorage,
		config:          Config{ConcurrentReconciles: 1, RequeuePeriod: defaultRequeuePeriod},
		retain:          cascadedelete.NewStaticRetainPolicy(),
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Reconcile is the main reconciliation loop
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	log := r.log.WithValues("namespace", req.Namespace, "name", req.Name)
	log.Info("Reconciling object")

	// Create a new instance of the target kind
	object := r.targetKind.DeepCopyObject().(client.Object)

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

	// Check if object is immutable (either by kind or annotation)
	if isImmutable(object) || isImmutableKind(object) {
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
	if err := r.metadataStorage.Upsert(ctx, object, false, indexedFields); err != nil {
		log.Error(err, "Failed to upsert object to metadata storage")
		return ctrl.Result{RequeueAfter: r.getRequeuePeriod()}, err
	}

	log.Info("Successfully synced object to metadata storage")
	return ctrl.Result{}, nil
}

// handleDeletion handles an object that K8s has marked with a DeletionTimestamp.
//
// On the normal apiserver delete the row is already soft-deleted/retained via the
// DeletingAnnotation path, so this only removes the ingester finalizer. On a non-apiserver
// delete (foreground GC, kubectl/GitOps) the annotation is absent; handleCascadeDeletion then
// retains the final state of opted-in kinds before the finalizer is removed.
func (r *Reconciler) handleDeletion(ctx context.Context, log logr.Logger, object client.Object) (ctrl.Result, error) {
	log.Info("Object is being deleted")

	// Check if our finalizer is present
	if !ctrlutil.ContainsFinalizer(object, api.IngesterFinalizer) {
		log.Info("Finalizer not present, nothing to do")
		return ctrl.Result{}, nil
	}

	// Non-apiserver delete (no DeletingAnnotation: foreground GC, kubectl/GitOps):
	// retain opted-in kinds before the finalizer is removed; no-op otherwise.
	if !isDeletingAnnotationSet(object) {
		wait, err := r.handleCascadeDeletion(ctx, log, object)
		if err != nil {
			return ctrl.Result{RequeueAfter: r.getRequeuePeriod()}, err
		}
		if wait {
			// Drain in flight: keep the ingester finalizer and requeue (no timeout —
			// see handleCascadeDeletion).
			return ctrl.Result{RequeueAfter: r.getRequeuePeriod()}, nil
		}
		// Terminal cases fall through to the shared finalizer removal below.
	}

	// Remove our finalizer so K8s can finish deleting the object.
	ctrlutil.RemoveFinalizer(object, api.IngesterFinalizer)
	if err := r.Update(ctx, object); err != nil {
		log.Error(err, "Failed to remove finalizer")
		return ctrl.Result{RequeueAfter: r.getRequeuePeriod()}, err
	}

	log.Info("Successfully removed finalizer")
	return ctrl.Result{}, nil
}

// handleCascadeDeletion reconciles the MySQL end-state for an object deleted
// directly (no DeletingAnnotation), scoped to opted-in kinds via the RetainPolicy.
// It never touches the ingester finalizer or issues a delete — GC owns that. It
// returns wait=true only while an opted-in object still carries a non-ingester
// (drain) finalizer, so the caller keeps the ingester finalizer and requeues;
// there is deliberately no independent ingester timeout (unwedging a stuck drain
// is the drain safety timeout's job). On drain completion it upserts the final
// state (retain).
func (r *Reconciler) handleCascadeDeletion(ctx context.Context, log logr.Logger, object client.Object) (wait bool, err error) {
	// TODO(#943): gvks[0] may be non-deterministic when a type is registered under
	// multiple versions. See issue for planned multi-GVK selection strategy.
	gvks, _, err := r.scheme.ObjectKinds(object)
	if err != nil || len(gvks) == 0 {
		return false, fmt.Errorf("failed to get GVK for %T: %w", object, err)
	}

	if !r.retain.RetainOnCascade(gvks[0].Kind) {
		// Not a cascade-retain kind: the caller just removes the ingester finalizer
		// (unchanged behavior).
		return false, nil
	}

	if hasNonIngesterFinalizer(object) {
		// Drain in flight: refresh MySQL with the in-progress state and wait
		// (bounded by the drain finalizer's lifecycle).
		if err := r.upsertToStorage(ctx, object); err != nil {
			log.Error(err, "Failed to refresh draining object in metadata storage")
			return false, err
		}
		log.Info("Cascade drain in progress; keeping ingester finalizer and requeuing")
		return true, nil
	}

	// Drain complete (finalizer gone / never present): capture the final state.
	log.Info("Retaining final state for cascade-deleted object")
	return false, r.upsertToStorage(ctx, object)
}

// hasNonIngesterFinalizer reports whether the object carries any finalizer other
// than the ingester's. For opted-in kinds the only finalizers are the ingester's
// and the drain one, so this signals an in-flight drain.
func hasNonIngesterFinalizer(object client.Object) bool {
	for _, f := range object.GetFinalizers() {
		if f != api.IngesterFinalizer {
			return true
		}
	}
	return false
}

// upsertToStorage upserts the object to metadata storage, extracting indexed
// fields when the object implements storage.IndexedObject. It mirrors the upsert
// performed by handleSync/handleImmutableObject.
func (r *Reconciler) upsertToStorage(ctx context.Context, object client.Object) error {
	var indexedFields []storage.IndexedField
	if indexedObj, ok := object.(storage.IndexedObject); ok {
		indexedFields = indexedObj.GetIndexedKeyValuePairs()
	}
	return r.metadataStorage.Upsert(ctx, object, false, indexedFields)
}

// handleDeletionAnnotation handles objects marked with DeletingAnnotation.
// It deletes the object from metadata storage and from K8s. Because the ingester
// finalizer is still present when r.Delete is called, K8s sets a DeletionTimestamp
// rather than removing the object immediately; the subsequent handleDeletion reconcile
// then removes the finalizer. This single-pass design avoids double-deletes from MySQL.
func (r *Reconciler) handleDeletionAnnotation(ctx context.Context, log logr.Logger, object client.Object) (ctrl.Result, error) {
	log.Info("Object marked for deletion via annotation")

	// Delete from metadata storage first
	gvks, _, err := r.scheme.ObjectKinds(object)
	if err != nil || len(gvks) == 0 {
		return ctrl.Result{}, fmt.Errorf("failed to get GVK for %T: %w", object, err)
	}
	// TODO(#943): gvks[0] may be non-deterministic when a type is registered under multiple
	// versions. See issue for planned multi-GVK selection strategy.
	typeMeta := &metav1.TypeMeta{
		Kind:       gvks[0].Kind,
		APIVersion: gvks[0].GroupVersion().String(),
	}

	if err := r.metadataStorage.Delete(ctx, typeMeta, object.GetNamespace(), object.GetName()); err != nil {
		log.Error(err, "Failed to delete from metadata storage")
		return ctrl.Result{RequeueAfter: r.getRequeuePeriod()}, err
	}

	// Delete from K8s/ETCD. The finalizer is intentionally left in place so that
	// K8s sets a DeletionTimestamp instead of removing the object immediately;
	// handleDeletion will remove the finalizer on the next reconcile.
	if err := r.Delete(ctx, object, deleteOptionsFromAnnotations(object)...); err != nil {
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

	if err := r.metadataStorage.Upsert(ctx, object, false, indexedFields); err != nil {
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
	if err := r.Delete(ctx, object, deleteOptionsFromAnnotations(object)...); err != nil {
		log.Error(err, "Failed to delete immutable object from K8s")
		return ctrl.Result{RequeueAfter: r.getRequeuePeriod()}, err
	}

	log.Info("Successfully moved immutable object to metadata storage only")
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	gvks, _, err := r.scheme.ObjectKinds(r.targetKind)
	if err != nil || len(gvks) == 0 {
		return fmt.Errorf("failed to get GVK for %T: %w", r.targetKind, err)
	}
	// TODO(#943): scheme.ObjectKinds may return multiple GVKs (e.g. v1 and v1beta1 for the same
	// type). We currently take gvks[0] which may be non-deterministic. Add explicit multi-GVK
	// selection strategy (prefer storage version, or error if multiple exist).
	kind := gvks[0].Kind
	controllerName := fmt.Sprintf("ingester_%s", kind)

	concurrentReconciles := r.config.ConcurrentReconciles
	if concurrentReconciles <= 0 {
		concurrentReconciles = 1
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(r.targetKind).
		Named(controllerName).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: concurrentReconciles,
		}).
		Complete(r)
}

// Helper functions

func (r *Reconciler) getRequeuePeriod() time.Duration {
	if r.config.RequeuePeriod > 0 {
		return r.config.RequeuePeriod
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

// deleteOptionsFromAnnotations builds the controller-runtime delete options from the
// object's annotations. When the apiserver recorded a delete propagation policy on the
// metadata-storage delete path (api.DeletePropagationAnnotation), it is honored here so
// the real K8s delete uses the caller's propagation policy (e.g. Foreground for cascade
// deletion). Returns nil when no policy annotation is present.
func deleteOptionsFromAnnotations(object client.Object) []client.DeleteOption {
	annotations := object.GetAnnotations()
	if annotations == nil {
		return nil
	}
	policy, ok := annotations[api.DeletePropagationAnnotation]
	if !ok || policy == "" {
		return nil
	}
	return []client.DeleteOption{client.PropagationPolicy(metav1.DeletionPropagation(policy))}
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
