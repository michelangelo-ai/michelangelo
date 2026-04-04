# Ingester Controller: Configuration and Operations

The **Ingester** is a Kubernetes controller that syncs all Michelangelo CRDs into MySQL, decoupling metadata storage from etcd. This guide covers deploying, configuring, and operating the ingester on production clusters.

## Architecture Overview

The ingester maintains consistency between Kubernetes and MySQL:

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

Every CRD object created through the API Server is automatically synced to MySQL. When deleted, the ingester ensures MySQL is updated before the object is removed from Kubernetes.

## MySQL Storage

### Schema Layout

For each of the 13 CRDs, the ingester creates 3 MySQL tables:

| Table Type | Purpose |
|-----------|---------|
| Main (e.g., `model`) | Core object data (uid, name, namespace, JSON, proto, indexed fields) |
| Labels (e.g., `model_labels`) | Key-value label pairs per object |
| Annotations (e.g., `model_annotations`) | Key-value annotation pairs per object |

**Total: 39 tables** (13 CRDs × 3 table types)

Supported CRDs: Project, ModelFamily, Model, Pipeline, PipelineRun, InferenceServer, Revision, Cluster, RayCluster, RayJob, TriggerRun, Deployment, SparkJob

### Main Table Schema Example

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

All deletions are soft-deletes: `DELETE` sets `delete_time` rather than removing the row. Live object queries use `WHERE delete_time IS NULL`.

## Configuration

### Prerequisites

- Kubernetes cluster with Michelangelo API Server
- MySQL 5.7+ accessible from the controllermgr pod
- Schema init Job (provides 39 MySQL tables)

### Enabling the Ingester

The ingester activates through two configuration gates in the controllermgr ConfigMap:

**Gate 1:** `metadataStorage.enableMetadataStorage` must be `true`
**Gate 2:** MySQL connection details must be provided

Both gates are required. If either is missing or disabled, the ingester silently stays disabled with zero impact on the rest of the controllermgr.

### ConfigMap Configuration

Edit the controllermgr ConfigMap to add or update these stanzas:

```yaml
# michelangelo-controllermgr-config

# Gate 1: Enable metadata storage
metadataStorage:
  enableMetadataStorage: true
  deletionDelay: 10s
  enableResourceVersionCache: false

# Gate 2: MySQL connection details
mysql:
  host: mysql
  port: 3306
  user: root
  password: root
  database: michelangelo
  maxOpenConns: 25
  maxIdleConns: 5
  connMaxLifetime: 5m

# Optional: ingester-specific concurrency settings
ingester:
  concurrentReconciles: 2
  requeuePeriod: 30s
```

| Setting | Purpose | Default |
|---------|---------|---------|
| `enableMetadataStorage` | Master switch for the feature | `false` |
| `deletionDelay` | Grace period for soft-deletes | `10s` |
| `enableResourceVersionCache` | Cache K8s resource versions (experimental) | `false` |
| `host` | MySQL hostname or IP | (required) |
| `port` | MySQL port | `3306` |
| `user` | MySQL user account | (required) |
| `password` | MySQL password | (required) |
| `database` | MySQL database name | (required) |
| `maxOpenConns` | Max concurrent connections | `25` |
| `maxIdleConns` | Max idle connections to keep | `5` |
| `connMaxLifetime` | Max connection lifetime | `5m` |
| `concurrentReconciles` | Parallel reconcilers per CRD | `2` |
| `requeuePeriod` | Retry interval on failure | `30s` |

:::info
With 13 CRDs and `concurrentReconciles: 2`, you get 13 independent work queues, each with 2 parallel workers.
:::

## Migration: Enabling on a Running Cluster

The ingester is designed to be enabled without downtime. Existing objects in Kubernetes will be picked up automatically on the controller's first startup.

### Step-by-Step Enablement

#### 1. Create MySQL Schema

Apply the schema init Job to create the 39 tables:

```bash
kubectl apply -f scripts/ingester/ingester-schema-init-job.yaml
kubectl wait --for=condition=complete job/ingester-schema-init --timeout=120s
```

#### 2. Update the Controllermgr ConfigMap

Add both required stanzas (`metadataStorage` and `mysql`):

```bash
kubectl edit configmap michelangelo-controllermgr-config
```

Add the configuration shown in the [ConfigMap Configuration](#configmap-configuration) section above.

#### 3. Restart the Controllermgr

Restart the controller manager to pick up the new configuration:

```bash
# If controllermgr is a Deployment:
kubectl rollout restart deployment michelangelo-controllermgr

# If controllermgr is a bare Pod (e.g., sandbox):
kubectl delete pod michelangelo-controllermgr
```

#### 4. Verify Controllers Registered

Check that all 13 controllers registered successfully:

```bash
kubectl logs -l app=michelangelo-controllermgr | grep "Ingester controller registered"
```

You should see 13 log lines (one per CRD kind).

#### 5. Verify Backfill

All existing objects will be reconciled once on startup. Verify MySQL was populated:

```bash
for table in project modelfamily model pipeline pipelinerun inferenceserver \
             revision cluster raycluster rayjob triggerrun deployment sparkjob; do
  COUNT=$(kubectl exec pod/mysql -- mysql -uroot -proot michelangelo -sN \
    -e "SELECT COUNT(*) FROM ${table} WHERE delete_time IS NULL;" 2>/dev/null)
  echo "${table}: ${COUNT}"
done
```

All counts should be non-zero (assuming you have existing objects).

## Operational Guidance

### Deletion Behavior

When an object is deleted through the API Server or kubectl, the ingester ensures MySQL is updated before the object is removed from Kubernetes:

1. User issues delete command
2. Kubernetes sets `DeletionTimestamp` on the object (or API Server sets deletion annotation)
3. Ingester detects change and soft-deletes from MySQL (sets `delete_time`)
4. Ingester removes the finalizer
5. Kubernetes deletes the object from etcd

This two-phase delete guarantees no data loss.

### Immutable Objects

Objects marked with the `michelangelo/Immutable` annotation are moved to MySQL only:

1. Ingester updates MySQL one final time
2. Finalizer is removed
3. Object is deleted from Kubernetes

Immutable objects (like completed PipelineRuns) no longer consume etcd memory.

### Pre-Existing Objects

Objects created before the ingester was enabled will NOT have the ingester finalizer. They are still synced to MySQL, but may not be intercepted during deletion via `kubectl delete`.

**Workaround**: Ensure all deletions go through the API Server, which sets the deletion annotation that the ingester monitors.

### Schema Evolution

Adding a new indexed column to an existing table requires a schema migration:

1. Create and apply an `ALTER TABLE` migration Job
2. Update the code to populate the new indexed field
3. Trigger a reconcile of affected objects (e.g., by bumping a resource version annotation) to backfill the new column

:::warning
The schema init Job is create-only (`CREATE TABLE IF NOT EXISTS`). It does not apply `ALTER TABLE` migrations automatically.
:::

### Disabling the Ingester

To disable the ingester without data loss:

1. Remove the `mysql:` stanza from the controllermgr ConfigMap
2. Restart the controllermgr
3. The ingester module will detect missing MySQL config and skip setup
4. MySQL data is preserved; you can re-enable the ingester later

The controllermgr will continue operating normally with the ingester disabled.

## Monitoring and Troubleshooting

### Verify Ingester is Running

Check controller logs for startup messages:

```bash
kubectl logs -l app=michelangelo-controllermgr | grep -i ingester
```

You should see messages like:
- `"Ingester controller registered for <Kind>"`
- `"Metadata storage enabled"`
- `"MySQL connection established"`

### Check MySQL Connectivity

Verify the controllermgr can reach MySQL:

```bash
kubectl exec -it deployment/michelangelo-controllermgr -- \
  mysql -h<mysql-host> -u<user> -p<password> -e "SELECT 1;"
```

### Monitor Sync Status

Query MySQL to verify objects are syncing:

```bash
# Check Model count
kubectl exec pod/mysql -- mysql -uroot -proot michelangelo -sN \
  -e "SELECT COUNT(*) FROM model WHERE delete_time IS NULL;"

# Check for recent updates
kubectl exec pod/mysql -- mysql -uroot -proot michelangelo -sN \
  -e "SELECT namespace, name, update_time FROM model ORDER BY update_time DESC LIMIT 5;"
```

### Detect Requeue Issues

If objects are not syncing, check for requeue errors in logs:

```bash
kubectl logs -l app=michelangelo-controllermgr | grep -i "requeue\|error" | tail -20
```

Requeue errors typically indicate:
- MySQL is unreachable
- Database credentials are wrong
- Permission issues on the MySQL connection

## Known Limitations and Issues

| Issue | Severity | Impact |
|-------|----------|--------|
| **SparkJob double-panic crash** | High | Controllermgr may crash when syncing SparkJob, preventing ALL MySQL sync. Requires fix in SparkJob controller code. |
| Pre-existing objects lack finalizer | Medium | Objects created before ingester enable won't be intercepted during `kubectl delete`. Mitigation: use API Server for all deletes. |
| `DeleteCollection` not implemented | Medium | Namespace-scoped bulk deletes return error. Use individual object deletes. |
| No schema migration support | Medium | New indexed columns require manual `ALTER TABLE` and backfill. |
| Label selector in `List` not implemented | Low | SQL label filtering not yet wired. Use JSON extraction in queries. |

### SparkJob Double-Panic Issue

**Status:** Pre-existing bug in `go/components/spark/job/client/client.go:185`

**Symptom:** Controllermgr crashes with panic when processing SparkJob objects

**Mitigation:** Do not enable the ingester until SparkJob controller is fixed, or disable SparkJob reconciliation in your controllermgr config

**Fix required:** Upstream fix to SparkJob controller error handling

---

## Verified Behavior

When the ingester is correctly enabled:

1. **All 13 controllers register** at startup (one per CRD kind)
2. **Immediate sync on creation** — objects appear in MySQL within milliseconds
3. **Immediate sync on update** — `res_version` and `update_time` advance in MySQL after every change
4. **Full JSON stored** — complete object JSON in the `json` column
5. **Indexed fields stored** — `algorithm`, `ray_version`, `entrypoint`, etc. in dedicated indexed columns
6. **Labels synced** — label changes reflected in `*_labels` companion tables
7. **Opt-in disabled by default** — ingester only runs when MySQL config is present in `michelangelo-controllermgr-config`
8. **SparkJob blocked** — pre-existing nil pointer panic in the SparkJob business controller prevents sync (unrelated to ingester)

---

## Next Steps

- Review [Ingester Internals](../contributing/ingester-internals.md) for developer documentation
- Monitor logs and MySQL after enablement to verify steady-state operation
