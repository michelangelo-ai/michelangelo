---
last_verified: 2026-10-14
---

# Ingester Controller: Architecture and Implementation

This guide explains the internal design of the Ingester controller for developers who need to understand, extend, or modify the ingester code.

## Overview

The Ingester is a generic Kubernetes controller that watches all 15 Michelangelo AI CRDs and durably syncs them into MySQL. Its purpose is to decouple metadata storage from etcd: Michelangelo AI's API Server and query layer can read from MySQL (faster, richer query capabilities) rather than depending solely on etcd. One `Reconciler` instance runs per CRD kind, watching only objects of that type and upserting them into a dedicated MySQL table on every create, update, or delete event.

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

When metadata storage is enabled, the API Server injects the finalizer during object creation, before writing to etcd:

```go
// go/api/handler/handler.go (handleMetadataStorageForCreate)
ctrlRTUtil.AddFinalizer(objMeta.(ctrlRTClient.Object), api.IngesterFinalizer)
```

**Key invariant**: Every CRD object created through the API Server carries the `michelangelo/Ingester` finalizer from birth. Objects created with `kubectl apply` that bypass the API Server handler will not have the finalizer and will not be tracked by deletion.

### Finalizer Removal (Ingester Controller)

The ingester removes the finalizer only after MySQL has been successfully updated:

```go
// go/components/ingester/controller.go (handleDeletion)
ctrlutil.RemoveFinalizer(object, api.IngesterFinalizer)
if err := r.Update(ctx, object); err != nil {
    log.Error(err, "Failed to remove finalizer")
    return ctrl.Result{RequeueAfter: r.getRequeuePeriod()}, err
}
```

If MySQL is unreachable or the delete fails, the finalizer remains in place and the object stays in etcd. The controller retries after the configured requeue period (`RequeuePeriod`, default: 30 seconds) until the operation succeeds.

### Annotation-Based Deletion

Because the ingester finalizer blocks `kubectl delete` from completing, the API Server uses an alternative deletion path: it sets an annotation instead of issuing a Kubernetes delete directly.

```go
// go/api/api.go
DeletingAnnotation = "michelangelo/Deleting"
```

When the API Server receives a delete request and metadata storage is enabled:

```go
// go/api/handler/handler.go (Delete handler)
annotation[api.DeletingAnnotation] = "true"
```

If the caller supplied a delete propagation policy (for example Foreground for cascade deletion), the API Server also records it in the `michelangelo/DeletePropagation` annotation so the ingester can honor it on the real Kubernetes delete.

The ingester detects the `Deleting` annotation and handles the deletion asynchronously:

```
annotation set → ingester detects → MySQL soft-delete → K8s delete (finalizer still present, so K8s sets DeletionTimestamp) → next reconcile removes finalizer → object gone
```

This path ensures the API Server's delete request completes instantly from the caller's perspective while the ingester handles the MySQL cleanup asynchronously.

### Immutable Objects

The `michelangelo/Immutable` annotation marks objects whose spec will never change again (e.g., completed PipelineRuns, archived Models). Kinds that report `IsImmutableKind()` are treated the same way without the annotation. The ingester:

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
Reconcile(object)            (30s context timeout)
    │
    ├── object not found in K8s ──→ no-op (already gone)
    │
    ├── DeletionTimestamp set ──→ handleDeletion()
    │       ├── ingester finalizer absent ──→ no-op
    │       ├── no michelangelo/Deleting annotation (kubectl / GC delete) ──→ handleCascadeDeletion()
    │       │       ├── kind not in RetainPolicy ──→ MySQL.Delete() (soft-delete)
    │       │       └── kind in RetainPolicy ──→ upsert final state; wait while a drain finalizer is present
    │       └── RemoveFinalizer → done
    │
    ├── annotation michelangelo/Deleting = "true" ──→ handleDeletionAnnotation()
    │       └── MySQL.Delete() → K8s.Delete() (finalizer kept) → next reconcile hits handleDeletion()
    │
    ├── michelangelo/MetadataStoragePrimaryKey annotation missing ──→ set it to the UID, Update, return
    │       (the Update triggers a new reconcile that continues below)
    │
    ├── annotation michelangelo/Immutable = "true" or IsImmutableKind() ──→ handleImmutableObject()
    │       └── MySQL.Upsert() → RemoveFinalizer → K8s.Delete() → done
    │
    └── (normal) ──→ handleSync()
            └── MySQL.Upsert(proto + JSON + indexed fields + labels + annotations) → done
```

## MySQL Storage Architecture

### Schema Layout

For each of the 15 CRDs, there are 3 MySQL tables:

| Table Type | Naming | Purpose |
|-----------|--------|---------|
| Main | `<kind_snake_case>` | Core object data (uid, name, namespace, JSON, proto, indexed fields) |
| Labels | `<kind_snake_case>_labels` | Key-value label pairs per object UID |
| Annotations | `<kind_snake_case>_annotations` | Key-value annotation pairs per object UID |

The 15 CRDs and their table names (the kind converted to snake_case by `utils.ToSnakeCase`):

| CRD Kind | Table Name |
|----------|-----------|
| Project | `project` |
| ModelFamily | `model_family` |
| Model | `model` |
| Pipeline | `pipeline` |
| PipelineRun | `pipeline_run` |
| InferenceServer | `inference_server` |
| Revision | `revision` |
| Cluster | `cluster` |
| RayCluster | `ray_cluster` |
| RayJob | `ray_job` |
| TriggerRun | `trigger_run` |
| Deployment | `deployment` |
| SparkJob | `spark_job` |
| CachedOutput | `cached_output` |
| EvaluationReport | `evaluation_report` |

**Total: 45 tables** (15 × 3). The DDL lives in `scripts/ingester/ingester_schema.sql` (the same schema ships in `helm/michelangelo/files/schema/mysql-init-schema.sql` and the sandbox's `mysql-ingester.yaml`).

### Main Table Schema

```sql
CREATE TABLE IF NOT EXISTS `model` (
    `uid`         VARCHAR(255) NOT NULL,    -- primary key: michelangelo/MetadataStoragePrimaryKey annotation, else K8s UID
    `group_ver`   VARCHAR(255) NOT NULL,    -- APIVersion string
    `namespace`   VARCHAR(255) NOT NULL,
    `name`        VARCHAR(255) NOT NULL,
    `res_version` BIGINT UNSIGNED NOT NULL, -- K8s ResourceVersion
    `create_time` DATETIME NOT NULL,
    `update_time` DATETIME,
    `delete_time` DATETIME,                 -- NULL = active, non-NULL = soft-deleted
    `proto`       MEDIUMBLOB,               -- serialized protobuf
    `json`        JSON,                     -- full object as JSON
    -- CRD-specific indexed fields, e.g.:
    `algorithm`   VARCHAR(255),             -- for Model
    PRIMARY KEY (`uid`),
    KEY `model_namespace_name` (`namespace`, `name`),
    KEY `model_create_time` (`create_time`),
    KEY `model_algorithm` (`algorithm`)
    -- (abbreviated: the real table has more indexed columns and keys)
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

Labels and annotations are replaced fully on every upsert (delete all existing rows for the object's primary key, re-insert from current state), inside the same transaction as the main row.

### Indexed Fields

CRDs that implement `storage.IndexedObject` expose `GetIndexedKeyValuePairs()` to return fields that are stored in dedicated indexed columns. This allows MySQL queries without JSON extraction. The method is generated into `proto-go/api/v2/*.pb.go` from the proto field annotations.

Example for `Model` (abbreviated; the generated method returns more fields):

```go
func (m *Model) GetIndexedKeyValuePairs() []storage.IndexedField {
    return []storage.IndexedField{
        {Key: "algorithm", Value: m.Spec.Algorithm},
        // ... src_pipeline_run_namespace, src_pipeline_run_name, and other indexed columns
    }
}
```

## Code Examples

### Full Reconcile Loop

```go
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
    defer cancel()

    log := r.log.WithValues("namespace", req.Namespace, "name", req.Name)

    object := r.targetKind.DeepCopyObject().(client.Object)

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

    // Pin the MySQL primary key to the UID once; return early after the write.
    if updated, err := r.ensureMetadataStoragePrimaryKey(ctx, object); err != nil {
        return ctrl.Result{RequeueAfter: r.getRequeuePeriod()}, err
    } else if updated {
        return ctrl.Result{}, nil
    }

    if isImmutable(object) || isImmutableKind(object) {
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

    // Non-apiserver delete (kubectl, GitOps, cascade GC): there is no Deleting
    // annotation, so MySQL has not been updated yet. Retain or soft-delete first.
    if !isDeletingAnnotationSet(object) {
        wait, err := r.handleCascadeDeletion(ctx, log, object)
        if err != nil {
            return ctrl.Result{RequeueAfter: r.getRequeuePeriod()}, err
        }
        if wait {
            // Drain in flight: keep the ingester finalizer and requeue.
            return ctrl.Result{RequeueAfter: r.getRequeuePeriod()}, nil
        }
    }

    ctrlutil.RemoveFinalizer(object, api.IngesterFinalizer)
    if err := r.Update(ctx, object); err != nil {
        return ctrl.Result{RequeueAfter: r.getRequeuePeriod()}, err
    }
    return ctrl.Result{}, nil
}
```

For an API Server delete (annotation present), `handleDeletionAnnotation` has already soft-deleted the MySQL row before issuing the Kubernetes delete, so `handleDeletion` only removes the finalizer.

### API Server Finalizer Injection

```go
// handler.go (Create handler)
ctrlRTUtil.AddFinalizer(objMeta.(ctrlRTClient.Object), api.IngesterFinalizer)
// then write to K8s
```

### API Server Annotation-Based Delete

```go
// handler.go (Delete handler)
if metadata storage is enabled {
    // fetch the live object; immutable objects follow a separate path
    annotation := tmpObj.GetAnnotations()
    annotation[api.DeletingAnnotation] = "true"
    // also records api.DeletePropagationAnnotation when the caller set a propagation policy
    return handler.k8sHandler.Update(ctx, tmpObj, &metav1.UpdateOptions{})  // triggers ingester reconcile
}
// else: normal K8s delete
```

## Testing

### Unit Tests

The controller is tested using `controller-runtime`'s fake client and a `testify/mock`-based `MockMetadataStorage`. All reconcile flows have dedicated tests in `go/components/ingester/controller_test.go`.

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

    // Model is not in the cascade RetainPolicy, so a plain delete soft-deletes the row.
    mockStorage := new(MockMetadataStorage)
    mockStorage.On("Delete", mock.Anything, mock.Anything, "default", "test-model").Return(nil)

    reconciler := NewReconciler(fakeClient, logr.Discard(), scheme, &v2.Model{}, mockStorage)

    result, err := reconciler.Reconcile(context.Background(), req)
    require.NoError(t, err)
    mockStorage.AssertCalled(t, "Delete", ...)
}
```

**Tests covered**:

| Test | Scenario | Assertions |
|------|----------|-----------|
| `TestReconciler_HandleSync` | Normal object, no special annotations | `Upsert` called once |
| `TestReconciler_HandleDeletion` | `DeletionTimestamp` set, no `Deleting` annotation, kind not retained | `Delete` called, finalizer removed |
| `TestHandleDeletion_OnlyRemovesFinalizer` | `DeletionTimestamp` set with `Deleting` annotation | No second `Delete`; finalizer removed |
| `TestReconciler_HandleDeletionAnnotation` | `michelangelo/Deleting = "true"` annotation | `Delete` called, K8s object gone |
| `TestReconciler_HandleImmutableObject` | `michelangelo/Immutable = "true"` annotation | `Upsert` called, K8s object gone |
| `TestReconciler_HandleImmutableKind` | Kind reports `IsImmutableKind()` | `Upsert` called, K8s object gone |
| `TestReconciler_SetsMetadataStoragePrimaryKey_WhenAbsent` | Primary-key annotation missing | Annotation set to the UID, no `Upsert` yet |
| `TestReconciler_ObjectNotFound` | Object deleted before reconcile runs | No storage calls |
| `TestHelperFunctions` | `isDeletingAnnotationSet`, `isImmutable`, `getRequeuePeriod` | Return correct values |
| `TestDeleteOptionsFromAnnotations` | `michelangelo/DeletePropagation` annotation | Propagation policy delete option is built |

### Running Unit Tests

```bash
bazel test //go/components/ingester/...
# or
(cd go && go test ./components/ingester/... -v)
```

### Integration / E2E Testing

The integration test suite uses test CRs from `scripts/ingester/ingester-test-crs/`. The steps are fully reproducible:

```bash
# 1. Recreate sandbox
cd python && poetry run ma sandbox create && cd ..

# 2. Verify schema
kubectl exec pod/mysql -- mysql -uroot -proot michelangelo -e "SHOW TABLES;"

# 3. Apply test CRs
kubectl apply -f scripts/ingester/ingester-test-crs/

# 4. Verify MySQL rows
for table in project model_family model pipeline pipeline_run inference_server \
             revision cluster ray_cluster ray_job spark_job trigger_run deployment; do
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
5. Observe ingester logs: `"Object is being deleted"` → `"Successfully removed finalizer"`. (A plain `kubectl delete` has no `Deleting` annotation, so `handleDeletion` soft-deletes the row via `handleCascadeDeletion` first for kinds outside the `RetainPolicy`.)
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
3. Observe: ingester logs an error (`"Failed to delete object from metadata storage"` for `kubectl delete`, `"Failed to delete from metadata storage"` for an API Server delete) with requeue.
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
    return mysqlstorage.NewMetadataStorage(mysqlConfig.ToMySQLConfig(), scheme, nil)
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
        p.Logger.Info("Metadata storage not configured, skipping ingester controller setup")
        return nil
    }
    // register one Reconciler per CRD in v2pb.CrdObjects
}
```

No other code changes are required to enable or disable the ingester.

### Controller Setup

One `Reconciler` is registered per CRD kind (every entry in `v2pb.CrdObjects`), watching only that specific type:

```go
ctrl.NewControllerManagedBy(mgr).
    For(r.targetKind).                         // watch only this CRD type
    Named(fmt.Sprintf("ingester_%s", kind)).   // unique controller name
    WithOptions(controller.Options{
        MaxConcurrentReconciles: concurrentReconciles,
    }).
    Complete(r)
```

With 15 CRDs and `concurrentReconciles: 1`, there are 15 independent work queues, each processing one object at a time.

## Architecture Summary

```
┌──────────────────────────────────────────────────────────────────┐
│                        controllermgr                             │
│                                                                  │
│  fx.Options(                                                     │
│    ingester.Module,          ← registers all 15 reconcilers      │
│    provideMetadataStorage,   ← MySQL connection (optional)       │
│    provideIngesterConfig,    ← concurrency + requeue config      │
│  )                                                               │
│                                                                  │
│  ┌────────────────────────────────────────────────────────────┐  │
│  │  ingester.Reconciler (×15, one per CRD kind)              │  │
│  │                                                            │  │
│  │  Watches: Model, ModelFamily, Pipeline, PipelineRun,       │  │
│  │           Deployment, InferenceServer, Project, Revision,  │  │
│  │           Cluster, RayCluster, RayJob, SparkJob,           │  │
│  │           TriggerRun, CachedOutput, EvaluationReport        │  │
│  │                                                            │  │
│  │  On event → Reconcile() → handleSync / handleDeletion /    │  │
│  │             handleDeletionAnnotation / handleImmutable      │  │
│  └────────────────────────┬───────────────────────────────────┘  │
└───────────────────────────┼──────────────────────────────────────┘
                            │
              storage.MetadataStorage interface
                            │
              ┌─────────────▼────────────┐
              │   mysqlMetadataStorage   │
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
              │   45 tables              │
              │   (15 main +             │
              │    15 _labels +          │
              │    15 _annotations)      │
              └──────────────────────────┘
```

**Key design properties**:
- **Opt-in**: No MySQL config = ingester silently disabled. Zero impact on existing deployments.
- **Generic**: One controller implementation handles all 15 CRDs.
- **Safe deletions**: Finalizer guarantees MySQL is updated before etcd record is removed.
- **Resilient**: Failed MySQL operations trigger requeue with backoff. Object stays in K8s until MySQL confirms.
- **Idempotent**: Upsert is safe to call multiple times with the same object.

## Known Limitations and Issues

| Issue | Severity | Notes |
|-------|----------|-------|
| Pre-existing objects lack finalizer | Medium | Objects created before ingester was enabled won't get deletion events via finalizer. Soft-delete orphan cleanup not yet implemented. |
| `DeleteCollection` not implemented | Medium | Returns `Unimplemented`. Required for namespace-scoped bulk deletes. |
| `QueryByTemplateID` not implemented | Low | Placeholder for template-based queries. |
| `Backfill` not implemented | Low | Returns `Unimplemented`; placeholder for historical data migration. |
| No schema migration support | Medium | Schema init Job is create-only (`CREATE TABLE IF NOT EXISTS`). `ALTER TABLE` for new columns requires manual intervention. |

## Cascade Delete: Retain Strategy

When a Pipeline is [cascade-deleted](../operator-guides/cascade-delete.md), its PipelineRuns and TriggerRuns are removed via Kubernetes foreground garbage collection. These kinds use a **retain strategy** — the ingester must preserve their final state in MySQL before allowing the CR to be removed from etcd.

Whether a kind retains on cascade is **not** a feature flag and the ingester hard-codes no kind names. Instead an injected per-kind **`RetainPolicy`** (`cascadedelete.RetainPolicy`) tells the ingester whether to retain a given kind's final state on a non-apiserver delete. The opt-in set is supplied at the controller-manager composition root — currently `{PipelineRun, TriggerRun}` via `cascadedelete.NewStaticRetainPolicy("PipelineRun", "TriggerRun")`. Kinds that do not opt in keep their existing deletion behavior.

### How it works

During a cascade deletion, each opted-in run carries two finalizers:

1. A **drain finalizer** (`pipelineruns.michelangelo.uber.com/drain` or `triggerruns.michelangelo.uber.com/drain`) — owned by the child's business controller, blocks removal until the in-flight workflow is cancelled.
2. The **ingester finalizer** (`michelangelo/Ingester`) — owned by the ingester, blocks removal until MySQL is updated.

The ingester's `handleCascadeDeletion` path runs only for kinds the injected `RetainPolicy` opts in, and works as follows:

- **While a non-ingester finalizer (the drain finalizer) is present**: the ingester keeps its own finalizer in place and refreshes MySQL with the CR's current state on every reconcile. This ensures MySQL reflects the latest status (e.g., transitioning from `RUNNING` to `KILLED`) as the drain progresses.
- **Once the drain finalizer is removed**: the workflow has been cleanly terminated. The ingester upserts the final CR state into MySQL, removes its own finalizer, and the CR is deleted from etcd.

For kinds the `RetainPolicy` does **not** opt in, `handleCascadeDeletion` soft-deletes the MySQL row (so a recreated object with the same name does not leave an orphaned row behind) and the ingester then removes its finalizer.

### No independent timeout

The ingester enforces no timeout of its own; it waits for the drain finalizer's lifecycle. A wedged drain is unblocked by the **child's own** [24-hour safety timeout](../operator-guides/cascade-delete.md#safety-timeout) — keyed off that child's `deletionTimestamp` — not by any Pipeline-level clock and not by the ingester.

## Adding a cascade child kind

The cascade machinery lives in the `go/cascadedelete` package and is consumed by thin, per-kind adapters. The scope is **deliberately** `Pipeline → {PipelineRun, TriggerRun}` only — the Pipeline controller itself carries zero cascade code. If you ever need to add a new child kind, do all of the following:

1. **Implement a `cascadedelete.DrainTarget` adapter** for the new kind in its controller. The adapter wraps a single fetched child and persists via the controller's `api.Handler` (not `client.Client`); the shared driver `cascadedelete.RunDrainStep` calls its five methods (`RequestCancel`, `Progress`, `MarkKilled`, `ForceKill`, `CompleteDrain`), reading the run's terminal/started state from the `DrainState` you pass it. Drive the drain from the controller's `Reconcile` when a `deletionTimestamp` is set.
2. **Add the kind's local constants** in that controller package — `drainFinalizer` (the byte-exact finalizer string, e.g. `<kind>s.michelangelo.uber.com/drain`) and `metricKind` (the metric label value). These CRD-specific facts must **not** live in `go/cascadedelete`.
3. **Register the API hook** to stamp the Pipeline ownerReference at creation (via `cascadedelete.StampOwnerRefOnCreate`) — the canonical, permanent path. As a transitional migration for objects predating the hook, also stamp the ownerReference in the controller during reconciliation. Install the drain finalizer **before** the ownerRef stamp.
4. **Add the kind to the `RetainPolicy` provider** in `go/cmd/controllermgr/main.go` — extend `cascadedelete.NewStaticRetainPolicy("PipelineRun", "TriggerRun", …)` so the ingester retains the new kind's final state in MySQL on cascade. Kind names live only at this composition root, never in `go/cascadedelete` or the ingester.

Keep `go/cascadedelete` free of concrete kind names and of the strings `pipeline`/`trigger` in non-test code; per-kind specifics belong in the consumers, and the *set* of retain kinds belongs in the controller-manager composition root (`cascadeRetainKinds` in `go/cmd/controllermgr/main.go`; `TestRetainPolicyDriftGuard` pins the expected set).

---

## Next Steps

- Review [Ingester Configuration and Operations](../operator-guides/components/ingester-configuration.md) for operational documentation
- Reference test CRs in `scripts/ingester/ingester-test-crs/` for integration testing examples
- See the code at `go/components/ingester/` for the full implementation
- [Go Key Concepts and Terms](dev/go/key-concepts-and-terms.md) — package map, key types, and patterns for the broader Go backend
