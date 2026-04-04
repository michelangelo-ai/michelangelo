# Ingester Controller: Architecture and Implementation

This guide explains the internal design of the Ingester controller for developers who need to understand, extend, or modify the ingester code.

## Finalizer Implementation

The ingester uses Kubernetes finalizers to guarantee safe deletions: MySQL is always updated before an object is removed from etcd.

### The Finalizer

A single finalizer blocks deletion:

```go
// go/api/api.go
IngesterFinalizer = "michelangelo/Ingester"
```

Kubernetes guarantees that objects are not removed from the API server until all finalizers are stripped. The ingester uses this to ensure MySQL is always updated before etcd loses the record.

### Finalizer Injection (API Server)

The API Server injects the finalizer during object creation, before writing to etcd:

```go
// go/api/handler/handler.go:546-547
ctrlRTUtil.AddFinalizer(objMeta.(ctrlRTClient.Object), api.IngesterFinalizer)
```

**Key invariant**: Every CRD object created through the API Server carries the `michelangelo/Ingester` finalizer from birth. Objects created with `kubectl apply` that bypass the API Server handler will not have the finalizer and will not be tracked by deletion.

### Finalizer Removal (Ingester Controller)

The ingester removes the finalizer only after MySQL has been successfully updated:

```go
// go/components/ingester/controller.go:134-138
ctrlutil.RemoveFinalizer(object, api.IngesterFinalizer)
if err := r.Update(ctx, object); err != nil {
    log.Error(err, "Failed to remove finalizer")
    return ctrl.Result{RequeueAfter: r.getRequeuePeriod()}, err
}
```

If MySQL is unreachable or the delete fails, the finalizer remains in place and the object stays in etcd. The controller retries every `requeuPeriod` (default: 30 seconds) until the operation succeeds.

### Annotation-Based Deletion

Because the ingester finalizer blocks `kubectl delete` from completing, the API Server uses an alternative deletion path: it sets an annotation instead of issuing a Kubernetes delete directly.

```go
// go/api/api.go
DeletingAnnotation = "michelangelo/Deleting"
```

When the API Server receives a delete request and metadata storage is enabled:

```go
// go/api/handler/handler.go:253,293
annotation[api.DeletingAnnotation] = "true"
```

The ingester detects this annotation and handles the deletion asynchronously:

```
annotation set → ingester detects → MySQL soft-delete → remove finalizer → K8s delete
```

This path ensures the API Server's delete request completes instantly from the caller's perspective while the ingester handles the MySQL cleanup asynchronously.

### Immutable Objects

The `michelangelo/Immutable` annotation marks objects whose spec will never change again (e.g., completed PipelineRuns, archived Models). The ingester:

1. Upserts the object to MySQL one final time.
2. Removes the finalizer.
3. Deletes the object from K8s/etcd.

The object continues to exist in MySQL only, permanently freeing etcd memory.

```go
// go/api/api.go
ImmutableAnnotation = "michelangelo/Immutable"
```

### Reconcile Decision Tree

```
Reconcile(object)
    │
    ├── object not found in K8s ──→ no-op (already gone)
    │
    ├── DeletionTimestamp set ──→ handleDeletion()
    │       └── MySQL.Delete() → RemoveFinalizer → done
    │
    ├── annotation michelangelo/Deleting = "true" ──→ handleDeletionAnnotation()
    │       └── MySQL.Delete() → RemoveFinalizer → K8s.Delete() → done
    │
    ├── annotation michelangelo/Immutable = "true" ──→ handleImmutableObject()
    │       └── MySQL.Upsert() → RemoveFinalizer → K8s.Delete() → done
    │
    └── (normal) ──→ handleSync()
            └── MySQL.Upsert(proto + JSON + indexed fields + labels + annotations) → done
```

## MySQL Storage Architecture

### Schema Layout

For each of the 13 CRDs, there are 3 MySQL tables:

| Table Type | Naming | Purpose |
|-----------|--------|---------|
| Main | `<kind>` | Core object data (uid, name, namespace, JSON, proto, indexed fields) |
| Labels | `<kind>_labels` | Key-value label pairs per object UID |
| Annotations | `<kind>_annotations` | Key-value annotation pairs per object UID |

The 13 CRDs and their table names (derived by `strings.ToLower(kind)`):

| CRD Kind | Table Name |
|----------|-----------|
| Project | `project` |
| ModelFamily | `modelfamily` |
| Model | `model` |
| Pipeline | `pipeline` |
| PipelineRun | `pipelinerun` |
| InferenceServer | `inferenceserver` |
| Revision | `revision` |
| Cluster | `cluster` |
| RayCluster | `raycluster` |
| RayJob | `rayjob` |
| TriggerRun | `triggerrun` |
| Deployment | `deployment` |
| SparkJob | `sparkjob` |

**Total: 39 tables** (13 × 3)

### Main Table Schema

```sql
CREATE TABLE model (
    uid           VARCHAR(64)  NOT NULL,   -- K8s UID (primary key)
    group_ver     VARCHAR(128),            -- APIVersion string
    namespace     VARCHAR(256),
    name          VARCHAR(256),
    res_version   BIGINT,                  -- K8s ResourceVersion
    create_time   DATETIME(6),
    update_time   DATETIME(6),
    delete_time   DATETIME(6),             -- NULL = active, non-NULL = soft-deleted
    proto         LONGBLOB,               -- serialized protobuf
    json          JSON,                   -- full object as JSON
    -- CRD-specific indexed fields, e.g.:
    algorithm     VARCHAR(128),           -- for Model
    PRIMARY KEY (uid),
    INDEX idx_namespace_name (namespace, name),
    INDEX idx_delete_time (delete_time)
);
```

Soft deletes are used: `DELETE` sets `delete_time` rather than removing the row. All queries filter `WHERE delete_time IS NULL` for live objects.

### Upsert Strategy

The ingester uses `INSERT ... ON DUPLICATE KEY UPDATE` (MySQL upsert):

```sql
INSERT INTO model (uid, group_ver, namespace, name, res_version,
                   create_time, update_time, proto, json, algorithm)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
    res_version  = VALUES(res_version),
    update_time  = VALUES(update_time),
    proto        = VALUES(proto),
    json         = VALUES(json),
    algorithm    = VALUES(algorithm);
```

Labels and annotations are replaced fully on every upsert (delete all existing rows for the UID, re-insert from current state).

### Indexed Fields

CRDs that implement `storage.IndexedObject` expose `GetIndexedKeyValuePairs()` to return fields that are stored in dedicated indexed columns. This allows MySQL queries without JSON extraction.

Example for `Model`:

```go
func (m *Model) GetIndexedKeyValuePairs() []storage.IndexedField {
    return []storage.IndexedField{
        {Key: "algorithm", Value: m.Spec.Algorithm},
    }
}
```

## Code Examples

### Full Reconcile Loop

```go
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    log := r.Log.WithValues("namespace", req.Namespace, "name", req.Name)
    log.Info("Reconciling object")

    object := r.TargetKind.DeepCopyObject().(client.Object)

    if err := r.Get(ctx, req.NamespacedName, object); err != nil {
        if client.IgnoreNotFound(err) == nil {
            return ctrl.Result{}, nil  // already gone, nothing to do
        }
        return ctrl.Result{}, err
    }

    if !object.GetDeletionTimestamp().IsZero() {
        return r.handleDeletion(ctx, log, object)      // K8s delete in progress
    }
    if isDeletingAnnotationSet(object) {
        return r.handleDeletionAnnotation(ctx, log, object)  // API Server delete
    }
    if isImmutable(object) {
        return r.handleImmutableObject(ctx, log, object)     // evict from etcd
    }
    return r.handleSync(ctx, log, object)                     // normal upsert
}
```

### Sync to MySQL

```go
func (r *Reconciler) handleSync(ctx context.Context, log logr.Logger, object client.Object) (ctrl.Result, error) {
    var indexedFields []storage.IndexedField
    if indexedObj, ok := object.(storage.IndexedObject); ok {
        indexedFields = indexedObj.GetIndexedKeyValuePairs()
    }

    if err := r.MetadataStorage.Upsert(ctx, object, false, indexedFields); err != nil {
        return ctrl.Result{RequeueAfter: r.getRequeuePeriod()}, err
    }
    return ctrl.Result{}, nil
}
```

### Deletion via Finalizer

```go
func (r *Reconciler) handleDeletion(ctx context.Context, log logr.Logger, object client.Object) (ctrl.Result, error) {
    if !ctrlutil.ContainsFinalizer(object, api.IngesterFinalizer) {
        return ctrl.Result{}, nil  // finalizer already gone
    }

    gvk := object.GetObjectKind().GroupVersionKind()
    typeMeta := &metav1.TypeMeta{Kind: gvk.Kind, APIVersion: gvk.GroupVersion().String()}

    if err := r.MetadataStorage.Delete(ctx, typeMeta, object.GetNamespace(), object.GetName()); err != nil {
        return ctrl.Result{RequeueAfter: r.getRequeuePeriod()}, err
    }

    ctrlutil.RemoveFinalizer(object, api.IngesterFinalizer)
    if err := r.Update(ctx, object); err != nil {
        return ctrl.Result{RequeueAfter: r.getRequeuePeriod()}, err
    }
    return ctrl.Result{}, nil
}
```

### API Server Finalizer Injection

```go
// handler.go (Create handler)
ctrlRTUtil.AddFinalizer(objMeta.(ctrlRTClient.Object), api.IngesterFinalizer)
// then write to K8s
```

### API Server Annotation-Based Delete

```go
// handler.go (Delete handler)
if metadataStorageEnabled {
    annotations := obj.GetAnnotations()
    if annotations == nil {
        annotations = make(map[string]string)
    }
    annotations[api.DeletingAnnotation] = "true"
    obj.SetAnnotations(annotations)
    return r.Update(ctx, obj)  // triggers ingester reconcile
}
// else: normal K8s delete
```

## Testing

### Unit Tests

The controller is tested using `controller-runtime`'s fake client and `testify/mock`. All 4 reconcile flows have dedicated tests in `go/components/ingester/controller_test.go`.

**Test pattern**:

```go
func TestReconciler_HandleDeletion(t *testing.T) {
    scheme := runtime.NewScheme()
    _ = v2.AddToScheme(scheme)

    now := metav1.Now()
    gracePeriod := int64(0)  // simulate expired grace period

    model := &v2.Model{
        ObjectMeta: metav1.ObjectMeta{
            Name:                       "test-model",
            Namespace:                  "default",
            DeletionTimestamp:          &now,
            DeletionGracePeriodSeconds: &gracePeriod,
            Finalizers:                 []string{api.IngesterFinalizer},
        },
    }

    fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(model).Build()

    mockStorage := new(MockMetadataStorage)
    mockStorage.On("Delete", mock.Anything, mock.Anything, "default", "test-model").Return(nil)

    reconciler := &Reconciler{
        Client:          fakeClient,
        MetadataStorage: mockStorage,
        // ...
    }

    result, err := reconciler.Reconcile(context.Background(), req)
    require.NoError(t, err)
    mockStorage.AssertCalled(t, "Delete", ...)
}
```

**Tests covered**:

| Test | Scenario | Assertions |
|------|----------|-----------|
| `TestReconciler_HandleSync` | Normal object, no special annotations | `Upsert` called once |
| `TestReconciler_HandleDeletion` | `DeletionTimestamp` set, grace period expired | `Delete` called, finalizer removed |
| `TestReconciler_HandleDeletionAnnotation` | `michelangelo/Deleting = "true"` annotation | `Delete` called, K8s object gone |
| `TestReconciler_HandleImmutableObject` | `michelangelo/Immutable = "true"` annotation | `Upsert` called, K8s object gone |
| `TestReconciler_ObjectNotFound` | Object deleted before reconcile runs | No storage calls |
| `TestHelperFunctions` | `isDeletingAnnotationSet`, `isImmutable`, `getRequeuePeriod` | Return correct values |

### Running Unit Tests

```bash
bazel test //go/components/ingester/...
# or
go test ./go/components/ingester/... -v
```

### Integration / E2E Testing

The [sandbox validation guide](../operator-guides/ingester-sandbox-validation.md) serves as the integration test suite. The steps are fully reproducible:

```bash
# 1. Recreate sandbox
python3 python/michelangelo/cli/sandbox/sandbox.py create

# 2. Verify schema
kubectl exec pod/mysql -- mysql -uroot -proot michelangelo -e "SHOW TABLES;"

# 3. Apply test CRs
kubectl apply -f scripts/ingester/ingester-test-crs/

# 4. Verify MySQL rows
for table in project modelfamily model pipeline pipelinerun inferenceserver \
             revision cluster raycluster rayjob triggerrun deployment; do
  COUNT=$(kubectl exec pod/mysql -- mysql -uroot -proot michelangelo -sN \
    -e "SELECT COUNT(*) FROM ${table} WHERE namespace='ingester-test';" 2>/dev/null)
  echo "${table}: ${COUNT}"
done

# 5. Apply updates and verify res_version increments
kubectl patch model ingester-test-model -n ingester-test --type=merge \
  -p '{"spec":{"algorithm":"lightgbm"}}'
kubectl exec pod/mysql -- mysql -uroot -proot michelangelo -sN \
  -e "SELECT algorithm FROM model WHERE namespace='ingester-test';"
# Expected: lightgbm
```

### Testing Finalizer Behavior Specifically

**Test: finalizer blocks K8s deletion until MySQL is updated**

1. Create a CR (finalizer is injected by API Server).
2. Verify finalizer present: `kubectl get model test -o jsonpath='{.metadata.finalizers}'`
3. Issue delete via `kubectl delete model test`.
4. Observe: object enters `Terminating` state (DeletionTimestamp set, finalizer blocking).
5. Observe ingester logs: `"Object is being deleted"` → `"Grace period expired, deleting from metadata storage"`.
6. Observe MySQL: `delete_time` populated.
7. Observe: finalizer removed, object disappears from K8s.

**Test: annotation-based delete path**

1. Create a CR.
2. Delete via API Server (sets `michelangelo/Deleting = "true"` annotation).
3. Observe: object is NOT in `Terminating` state (no DeletionTimestamp yet).
4. Observe ingester logs: `"Object marked for deletion via annotation"`.
5. Observe: MySQL soft-deleted, then K8s object deleted.

**Test: MySQL unavailable — finalizer holds**

1. Scale down MySQL (or block network access).
2. Delete a CR.
3. Observe: ingester logs error `"Failed to delete from metadata storage"` with requeue.
4. Object remains in `Terminating` state.
5. Restore MySQL → ingester retries → MySQL updated → finalizer removed → object gone.

## Controller Registration and Opt-In Design

### Opt-In via Dependency Injection

The ingester activates through a two-gate check in `go/cmd/controllermgr/ingester_providers.go`:

```go
func provideMetadataStorage(
    storageConfig storage.MetadataStorageConfig,
    mysqlConfig baseconfig.MySQLConfig,
    scheme *runtime.Scheme,
) (storage.MetadataStorage, error) {
    // Gate 1: metadataStorage.enableMetadataStorage must be true
    if !storage.EnableMetadataStorage(&storageConfig) {
        return nil, nil
    }
    // Gate 2: mysql config must have host/user/database or enabled: true
    if !mysqlConfigEnabled(mysqlConfig) {
        return nil, fmt.Errorf("metadata storage is enabled but mysql config is empty")
    }
    return mysqlstorage.NewMetadataStorage(mysqlConfig.ToMySQLConfig(), scheme)
}

func mysqlConfigEnabled(config baseconfig.MySQLConfig) bool {
    if config.Enabled {
        return true
    }
    return config.Host != "" || config.Database != "" || config.User != ""
}
```

When `provideMetadataStorage` returns `nil`, the ingester module detects it and skips setup:

```go
// go/components/ingester/module.go
func register(p registerParams) error {
    if p.MetadataStorage == nil {
        p.Logger.Info("Metadata storage not configured, skipping ingester setup")
        return nil
    }
    // register one Reconciler per CRD
}
```

No other code changes are required to enable or disable the ingester.

### Controller Setup

One `Reconciler` is registered per CRD kind, watching only that specific type:

```go
ctrl.NewControllerManagedBy(mgr).
    For(r.TargetKind).                         // watch only this CRD type
    Named(fmt.Sprintf("ingester_%s", kind)).   // unique controller name
    WithOptions(controller.Options{
        MaxConcurrentReconciles: concurrentReconciles,
    }).
    Complete(r)
```

With 13 CRDs and `concurrentReconciles: 1`, there are 13 independent work queues, each processing one object at a time.

## Architecture Summary

```
┌──────────────────────────────────────────────────────────────────┐
│                        controllermgr                             │
│                                                                  │
│  fx.Options(                                                     │
│    ingester.Module,          ← registers all 13 reconcilers      │
│    provideMetadataStorage,   ← MySQL connection (optional)       │
│    provideIngesterConfig,    ← concurrency + requeue config      │
│  )                                                               │
│                                                                  │
│  ┌────────────────────────────────────────────────────────────┐  │
│  │  ingester.Reconciler (×13, one per CRD kind)              │  │
│  │                                                            │  │
│  │  Watches: Model, ModelFamily, Pipeline, PipelineRun,       │  │
│  │           Deployment, InferenceServer, Project, Revision,  │  │
│  │           Cluster, RayCluster, RayJob, SparkJob,           │  │
│  │           TriggerRun                                        │  │
│  │                                                            │  │
│  │  On event → Reconcile() → handleSync / handleDeletion /    │  │
│  │             handleDeletionAnnotation / handleImmutable      │  │
│  └────────────────────────┬───────────────────────────────────┘  │
└───────────────────────────┼──────────────────────────────────────┘
                            │
              storage.MetadataStorage interface
                            │
              ┌─────────────▼────────────┐
              │   mysql.mysqlMetadataStorage │
              │                          │
              │   Upsert()  → INSERT ON  │
              │              DUPLICATE   │
              │              KEY UPDATE  │
              │   Delete()  → soft-delete│
              │   GetByName/ID()         │
              │   List()                 │
              └──────────────────────────┘
                            │
              ┌─────────────▼────────────┐
              │         MySQL            │
              │   39 tables              │
              │   (13 main +             │
              │    13 _labels +          │
              │    13 _annotations)      │
              └──────────────────────────┘
```

**Key design properties**:
- **Opt-in**: No MySQL config = ingester silently disabled. Zero impact on existing deployments.
- **Generic**: One controller implementation handles all 13 CRDs.
- **Safe deletions**: Finalizer guarantees MySQL is updated before etcd record is removed.
- **Resilient**: Failed MySQL operations trigger requeue with backoff. Object stays in K8s until MySQL confirms.
- **Idempotent**: Upsert is safe to call multiple times with the same object.

## Known Limitations and Issues

| Issue | Severity | Notes |
|-------|----------|-------|
| SparkJob double-panic in business controller | High | Pre-existing bug in `go/components/spark/job/client/client.go:185`. Crashes controllermgr, preventing SparkJob MySQL sync. Fix required in SparkJob controller. |
| Pre-existing objects lack finalizer | Medium | Objects created before ingester was enabled won't get deletion events via finalizer. Soft-delete orphan cleanup not yet implemented. |
| `DeleteCollection` not implemented | Medium | Returns error. Required for namespace-scoped bulk deletes. |
| `QueryByTemplateID` not implemented | Low | Placeholder for template-based queries. |
| `Backfill` not implemented | Low | Placeholder for historical data migration. |
| Label selector in `List` not implemented | Low | SQL label filter not yet wired up. |
| `directUpdate` not implemented | Low | Optimistic concurrency update path placeholder. |
| No schema migration support | Medium | Schema init Job is create-only. `ALTER TABLE` for new columns requires manual intervention. |

---

## Next Steps

- Review [Ingester Configuration and Operations](../operator-guides/ingester-configuration.md) for operational documentation
- Check the [sandbox validation guide](../operator-guides/ingester-sandbox-validation.md) for testing procedures
- See the code at `go/components/ingester/` for the full implementation
