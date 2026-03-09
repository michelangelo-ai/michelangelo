# Ingester Controller: Design Document

**Branch**: `hkriplani/feat-ingester`
**Date**: 2026-03-09
**Status**: Validated in sandbox (see [sandbox validation report](ingester-sandbox-validation.md))

---

## Overview

The **Ingester** is a generic Kubernetes controller that watches all 13 Michelangelo CRDs and durably syncs them into MySQL. Its purpose is to decouple metadata storage from etcd: objects created through the Michelangelo API Server are kept in sync with a relational database, enabling rich querying, long-term retention, and eventual etcd offload for historical/immutable records.

```
                 ┌─────────────────┐
  kubectl/gRPC → │  API Server     │ → creates CR in K8s + adds finalizer
                 └────────┬────────┘
                          │ K8s event
                 ┌────────▼────────┐
                 │  Ingester       │ → upserts to MySQL
                 │  Controller     │ → removes finalizer on delete
                 └────────┬────────┘
                          │ SQL
                 ┌────────▼────────┐
                 │     MySQL       │ 13 tables + labels + annotations
                 └─────────────────┘
```

---

## 1. Finalizer Implementation

### 1.1 The Finalizer

The ingester uses a single Kubernetes finalizer to guarantee that no CR is deleted from etcd before it has been soft-deleted in MySQL:

```go
// go/api/api.go
IngesterFinalizer = "michelangelo/Ingester"
```

Kubernetes will not actually remove an object from the API server until all finalizers have been stripped. The ingester uses this guarantee to ensure MySQL is always updated before etcd loses the record.

### 1.2 Finalizer Injection (API Server)

The API Server adds the finalizer during the `Create` handler, before the object is written to etcd:

```go
// go/api/handler/handler.go:546-547
ctrlRTUtil.AddFinalizer(objMeta.(ctrlRTClient.Object), api.IngesterFinalizer)
```

This means every CRD object that passes through the API Server carries the `michelangelo/Ingester` finalizer from birth. Objects created with `kubectl apply` that bypass the API Server handler will not have the finalizer and will not be tracked by the ingester.

### 1.3 Finalizer Removal (Ingester Controller)

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

### 1.4 Annotation-Based Deletion

Because the ingester finalizer blocks `kubectl delete` from completing until MySQL is updated, the API Server uses a different deletion path: it sets an annotation instead of issuing a K8s delete directly.

```go
// go/api/api.go
DeletingAnnotation = "michelangelo/Deleting"
```

When the API Server receives a delete request and metadata storage is enabled:
```go
// go/api/handler/handler.go:253,293
annotation[api.DeletingAnnotation] = "true"
```

The ingester detects this annotation, soft-deletes from MySQL, removes the finalizer, then issues the K8s delete:

```
annotation set → ingester detects → MySQL soft-delete → remove finalizer → K8s delete
```

This path ensures the API Server's delete request completes instantly from the caller's perspective while the ingester handles the MySQL cleanup asynchronously.

### 1.5 Immutable Objects

The `michelangelo/Immutable` annotation marks objects whose spec will never change again (e.g., completed PipelineRuns, archived Models). The ingester:

1. Upserts the object to MySQL one final time.
2. Removes the finalizer.
3. Deletes the object from K8s/etcd.

The object continues to exist in MySQL only, permanently freeing etcd memory.

```go
// go/api/api.go
ImmutableAnnotation = "michelangelo/Immutable"
```

### 1.6 Reconcile Decision Tree

```
Reconcile(object)
    │
    ├── object not found in K8s ──→ no-op (already gone)
    │
    ├── DeletionTimestamp set ──→ handleDeletion()
    │       └── grace period expired?
    │               ├── no  → requeue after 30s
    │               └── yes → MySQL.Delete() → RemoveFinalizer → done
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

---

## 2. MySQL Storage Architecture

### 2.1 Schema Layout

For each of the 13 CRDs, there are 3 MySQL tables:

| Table type | Naming | Purpose |
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

### 2.2 Main Table Schema

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

### 2.3 Upsert Strategy

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

### 2.4 Indexed Fields

CRDs that implement `storage.IndexedObject` expose `GetIndexedKeyValuePairs()` to return fields that are stored in dedicated indexed columns. This allows MySQL queries like `WHERE algorithm = 'xgboost'` without JSON extraction.

Example for `Model`:
```go
func (m *Model) GetIndexedKeyValuePairs() []storage.IndexedField {
    return []storage.IndexedField{
        {Key: "algorithm", Value: m.Spec.Algorithm},
    }
}
```

---

## 3. Controller Registration and Opt-In Design

### 3.1 Opt-In via Dependency Injection

The ingester module is registered in the `controllermgr` but only activates when MySQL config is present. It uses `fx` optional dependencies:

```go
// go/components/ingester/module.go
type registerParams struct {
    fx.In
    Manager         ctrl.Manager
    MetadataStorage storage.MetadataStorage `optional:"true"`
    Config          Config                  `optional:"true"`
    Logger          *zap.Logger
}

func register(p registerParams) error {
    if p.MetadataStorage == nil {
        p.Logger.Info("Metadata storage not configured, skipping ingester setup")
        return nil
    }
    // register one Reconciler per CRD
}
```

When `michelangelo-controllermgr-config` does not include a `mysql:` stanza, `MetadataStorage` is `nil` and the ingester silently skips setup. No other code changes are required to enable or disable it.

### 3.2 Configuration

The ingester is configured via the controllermgr ConfigMap:

```yaml
# michelangelo-controllermgr-config
mysql:
  enabled: true
  host: mysql
  port: 3306
  user: root
  password: root
  database: michelangelo
  maxOpenConns: 25
  maxIdleConns: 5
  connMaxLifetime: 5m

ingester:
  concurrentReconciles: 1
  requeuePeriod: 30s
```

### 3.3 Controller Setup

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

---

## 4. Migration Strategy

### 4.1 Enabling the Ingester on a Running Cluster

The ingester is designed to be enabled without downtime. Existing objects in etcd that predate the feature will be picked up automatically on the controller's first list-and-watch cycle.

**Steps to enable on an existing cluster**:

1. **Apply the schema init Job** to create the 39 MySQL tables:
   ```bash
   kubectl apply -f scripts/ingester/ingester-schema-init-job.yaml
   kubectl wait --for=condition=complete job/ingester-schema-init --timeout=120s
   ```

2. **Update the controllermgr ConfigMap** to add MySQL credentials:
   ```bash
   kubectl edit configmap michelangelo-controllermgr-config
   # Add mysql: and ingester: stanzas
   ```

3. **Restart the controllermgr**:
   ```bash
   kubectl rollout restart deployment michelangelo-controllermgr
   ```

4. **Verify controllers registered** (13 log lines expected):
   ```bash
   kubectl logs -l app=michelangelo-controllermgr | grep "Ingester controller registered"
   ```

5. **Verify backfill**: All existing objects will be reconciled once on startup. Check MySQL counts:
   ```bash
   for table in project modelfamily model pipeline pipelinerun inferenceserver \
                revision cluster raycluster rayjob triggerrun deployment sparkjob; do
     COUNT=$(kubectl exec pod/mysql -- mysql -uroot -proot michelangelo -sN \
       -e "SELECT COUNT(*) FROM ${table} WHERE delete_time IS NULL;" 2>/dev/null)
     echo "${table}: ${COUNT}"
   done
   ```

### 4.2 Existing Objects Without the Finalizer

Objects created before the ingester was enabled will not have the `michelangelo/Ingester` finalizer. The ingester still syncs them to MySQL (the `handleSync` path does not require a finalizer). However, when these objects are deleted via `kubectl delete`, the ingester will not intercept the deletion because there is no finalizer to block it.

**Trade-off**: Historical objects created before the ingester are synced to MySQL but may not receive deletion events. Soft-delete records will remain with `delete_time IS NULL` even after the object is gone from etcd.

**Mitigation options** (not yet implemented):
- A backfill controller that periodically compares etcd state to MySQL and soft-deletes orphaned rows.
- Require all delete operations to go through the API Server (which sets the `DeletingAnnotation`), bypassing the need for a finalizer on old objects.

### 4.3 Schema Evolution

Adding a new indexed column to an existing table requires a schema migration. The current schema init Job is idempotent for table creation (`CREATE TABLE IF NOT EXISTS`) but does not apply `ALTER TABLE` migrations.

**Recommended approach for schema changes**:
1. Add the new column in a separate migration Job.
2. Update `GetIndexedKeyValuePairs()` to populate the new field.
3. Trigger a reconcile of all affected objects (e.g., by bumping a resource version annotation) to populate the new column retroactively.

### 4.4 Disabling the Ingester

To disable the ingester without data loss:
1. Remove the `mysql:` stanza from the controllermgr ConfigMap.
2. Restart the controllermgr. The ingester module will detect `MetadataStorage == nil` and skip setup.
3. MySQL data is preserved. The ingester can be re-enabled later and will re-sync from the current etcd state.

---

## 5. Code Examples

### 5.1 Full Reconcile Loop

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

### 5.2 Sync to MySQL

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

### 5.3 Deletion via Finalizer

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

### 5.4 API Server Finalizer Injection

```go
// handler.go (Create handler)
ctrlRTUtil.AddFinalizer(objMeta.(ctrlRTClient.Object), api.IngesterFinalizer)
// then write to K8s
```

### 5.5 API Server Annotation-Based Delete

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

### 5.6 Adding a New CRD to the Ingester

To add a new CRD (e.g., `BatchJob`) to the ingester:

1. Add it to `AllCRDObjects` in `proto/api/v2/crd_objects.go`:
   ```go
   var AllCRDObjects = []runtime.Object{
       // ... existing
       &BatchJob{},
   }
   ```

2. Optionally implement `IndexedObject` on the type:
   ```go
   func (b *BatchJob) GetIndexedKeyValuePairs() []storage.IndexedField {
       return []storage.IndexedField{
           {Key: "job_type", Value: b.Spec.JobType},
       }
   }
   ```

3. Add the MySQL table in the schema init SQL:
   ```sql
   CREATE TABLE IF NOT EXISTS batchjob (
       uid          VARCHAR(64) NOT NULL PRIMARY KEY,
       -- ... standard columns
       job_type     VARCHAR(128),
       INDEX idx_namespace_name (namespace, name)
   );
   CREATE TABLE IF NOT EXISTS batchjob_labels ( ... );
   CREATE TABLE IF NOT EXISTS batchjob_annotations ( ... );
   ```

No changes to the ingester controller or module are needed — the loop over `AllCRDObjects` handles it automatically.

---

## 6. Testing Finalizers

### 6.1 Unit Tests

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

### 6.2 Running Unit Tests

```bash
bazel test //go/components/ingester/...
# or
go test ./go/components/ingester/... -v
```

### 6.3 Integration / E2E Testing

The sandbox validation (`docs/ingester-sandbox-validation.md`) serves as the integration test suite. The steps are fully reproducible:

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

### 6.4 Testing Finalizer Behavior Specifically

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

---

## 7. Known Limitations and Open Issues

| Issue | Severity | Notes |
|-------|----------|-------|
| SparkJob double-panic in business controller | High | Pre-existing bug in `spark/job/client/client.go:185`. Crashes controllermgr, preventing SparkJob MySQL sync. Fix required in SparkJob controller. |
| Pre-existing objects lack finalizer | Medium | Objects created before ingester was enabled won't get deletion events via finalizer. Soft-delete orphan cleanup not yet implemented. |
| `DeleteCollection` not implemented | Medium | Returns error. Required for namespace-scoped bulk deletes. |
| `QueryByTemplateID` not implemented | Low | Placeholder for template-based queries. |
| `Backfill` not implemented | Low | Placeholder for historical data migration. |
| Label selector in `List` not implemented | Low | SQL label filter not yet wired up. |
| `directUpdate` not implemented | Low | Optimistic concurrency update path placeholder. |
| No schema migration support | Medium | Schema init Job is create-only. `ALTER TABLE` for new columns requires manual intervention. |

---

## 8. Architecture Summary

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
