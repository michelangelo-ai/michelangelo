---
last_verified: 2026-10-14
---

# Ingester Controller: Configuration and Operations

The **Ingester** is a Kubernetes controller that syncs all Michelangelo AI CRDs into MySQL, decoupling metadata storage from etcd. This guide covers deploying, configuring, and operating the ingester on production clusters.

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
                 │     MySQL       │ 15 tables + labels + annotations
                 └─────────────────┘
```

Every CRD object created through the API Server is automatically synced to MySQL. When deleted, the ingester ensures MySQL is updated before the object is removed from Kubernetes.

For schema terminology, table naming, indexed fields, and common query patterns, see the [SQL Key Concepts](../sql-key-concepts-and-terms.md) reference — in particular [Extracted Columns and SQL Indexes](../sql-key-concepts-and-terms.md#extracted-columns-and-sql-indexes) and [Query Patterns](../sql-key-concepts-and-terms.md#query-patterns).

## MySQL Storage

### Schema Layout

For each of the 15 CRDs, the ingester creates 3 MySQL tables:

| Table Type | Purpose |
|-----------|---------|
| Main (e.g., `model`) | Core object data (uid, name, namespace, JSON, proto, indexed fields) |
| Labels (e.g., `model_labels`) | Key-value label pairs per object |
| Annotations (e.g., `model_annotations`) | Key-value annotation pairs per object |

**Total: 45 tables** (15 CRDs × 3 table types)

Supported CRDs: Project, ModelFamily, Model, Pipeline, PipelineRun, InferenceServer, Revision, Cluster, RayCluster, RayJob, TriggerRun, Deployment, SparkJob, CachedOutput, EvaluationReport

Table names are the snake_case kind names (for example `model_family`, `pipeline_run`, `inference_server`).

### Main Table Schema Example

```sql
-- helm/michelangelo/files/schema/mysql-init-schema.sql (abridged)
CREATE TABLE IF NOT EXISTS `model` (
    `uid` VARCHAR(255) NOT NULL,           -- K8s UID (primary key)
    `group_ver` VARCHAR(255) NOT NULL,     -- APIVersion string
    `namespace` VARCHAR(255) NOT NULL,
    `name` VARCHAR(255) NOT NULL,
    `res_version` BIGINT UNSIGNED NOT NULL,-- K8s ResourceVersion
    `create_time` DATETIME NOT NULL,
    `update_time` DATETIME,
    `delete_time` DATETIME,                -- NULL = active, non-NULL = soft-deleted
    `proto` MEDIUMBLOB,                    -- serialized protobuf
    `json` JSON,                           -- full object as JSON
    -- CRD-specific extracted fields, e.g. for Model:
    `algorithm` VARCHAR(255),
    -- ...
    PRIMARY KEY (`uid`),
    KEY `model_namespace_name` (`namespace`, `name`),
    KEY `model_create_time` (`create_time`),
    KEY `model_algorithm` (`algorithm`),
    KEY `model_owner` (`owner`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
```

All deletions are soft-deletes: `DELETE` sets `delete_time` rather than removing the row. Live object queries use `WHERE delete_time IS NULL`.

## Configuration

:::caution SparkJob pod specs without a resource
`SparkClient.toSparkPodSpec` in `go/components/spark/job/client/client.go` dereferences `pod.Resource` (for example `&(pod.Resource.Cpu)`) without a nil check, so a SparkJob whose driver or executor pod spec has no `resource` can panic the SparkJob controller. This is independent of the ingester, but a panic that takes down the controller manager also stops MySQL sync for every kind. Make sure every SparkJob you create sets `resource` on its driver and executor. See [Known Limitations](#known-limitations-and-issues).
:::

### Prerequisites

- Kubernetes cluster with Michelangelo AI API Server
- MySQL 5.7+ accessible from the controllermgr pod
- The 45-table schema applied to MySQL (the Helm chart's API server applies it automatically through its `schema-init` init container)

### Enabling the Ingester

The ingester activates through two configuration requirements in the controllermgr ConfigMap:

**Gate 1:** `metadataStorage.enableMetadataStorage` must be `true`
**Gate 2:** MySQL connection details must be provided (at least one of `mysql.enabled`, `host`, `database`, or `user`)

If `enableMetadataStorage` is `false` (the default), the ingester stays disabled with zero impact on the rest of the controllermgr. If it is `true` but the `mysql` stanza is empty, the controller manager fails at startup with `metadata storage is enabled but mysql config is empty`.

### Helm Values

With the Helm chart you do not edit the ConfigMap by hand. The chart renders the `metadataStorage`, `mysql`, and `ingester` stanzas from these values:

```yaml
metadataStorage:            # shared MySQL connection (also used by the API server)
  host: mysql.example.internal
  port: 3306
  database: michelangelo
  user: root
  existingSecret: michelangelo-metadata-storage   # Secret with a `password` key

controllermgr:
  metadataStorage:
    enable: true                       # Gate 1 (default: false)
    deletionDelay: 10s
    enableResourceVersionCache: false
  ingester:
    concurrentReconciles: 2
    requeuePeriod: 30s
    concurrentReconcilesMap:           # per-kind overrides
      PipelineRun: 10
```

The chart does not render `maxOpenConns`, `maxIdleConns`, or `connMaxLifetime`; the controller manager falls back to the defaults below. Also set `apiserver.metadataStorage.enable: true` if you want the API server to read from MySQL.

### ConfigMap Configuration

The rendered controllermgr ConfigMap (`michelangelo-controllermgr-config`) contains these stanzas, which you can also write by hand for non-Helm deployments:

```yaml
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
  password: ${MYSQL_PASSWORD}
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
| `port` | MySQL port | `3306` (chart default) |
| `user` | MySQL user account | `root` (chart default) |
| `password` | MySQL password | (required; the chart injects it from the metadata-storage Secret) |
| `database` | MySQL database name | `michelangelo` (chart default) |
| `maxOpenConns` | Max concurrent connections | `25` (applied when unset or 0) |
| `maxIdleConns` | Max idle connections to keep | `5` |
| `connMaxLifetime` | Max connection lifetime | `5m` |
| `concurrentReconciles` | Parallel reconcilers per CRD | `2` |
| `requeuePeriod` | Retry interval on failure | `30s` |

:::info
With 15 CRDs and `concurrentReconciles: 2`, you get 15 independent work queues, each with 2 parallel workers. Use `ingester.concurrentReconcilesMap` and `requeuePeriodMap` to override per kind.
:::

## Migration: Enabling on a Running Cluster

The ingester is designed to be enabled without downtime. Existing objects in Kubernetes will be picked up automatically on the controller's first startup.

### Step-by-Step Enablement

#### 1. Create MySQL Schema

**Helm installs:** the API server's `schema-init` init container applies `helm/michelangelo/files/schema/mysql-init-schema.sql` (all 45 tables, idempotent) every time an API server pod starts, so no separate step is needed. Confirm with:

```bash
kubectl logs deployment/michelangelo-apiserver -c schema-init
```

**Non-Helm installs:** create a ConfigMap from `scripts/ingester/ingester_schema.sql` and run the schema Job:

```bash
kubectl create configmap ingester-schema-sql \
  --from-file=ingester_schema.sql=scripts/ingester/ingester_schema.sql
kubectl apply -f scripts/ingester/ingester_schema_job.yaml
kubectl wait --for=condition=complete job/ingester-schema-init --timeout=120s
```

The Job in `scripts/ingester/ingester_schema_job.yaml` connects to host `mysql` as `root`/`root`; edit it for your environment. (Its log messages still say "13 CRDs (39 total tables)", but it applies the full `ingester_schema.sql`.) Do not use `ingester-schema-init-job.yaml` or `ingester-schema-init-job-complete.yaml`; they embed older, incomplete schemas.

If the Job fails:

```bash
# Inspect why it failed
kubectl describe job/ingester-schema-init
kubectl logs job/ingester-schema-init

# Common causes: MySQL unreachable, wrong credentials, insufficient privileges
# After fixing the root cause, delete the failed Job and reapply:
kubectl delete job ingester-schema-init
kubectl apply -f scripts/ingester/ingester_schema_job.yaml
kubectl wait --for=condition=complete job/ingester-schema-init --timeout=120s
```

#### 2. Update the Controllermgr Configuration

With Helm, set `controllermgr.metadataStorage.enable: true` and the `metadataStorage.*` connection values (see [Helm Values](#helm-values)) and run `helm upgrade`. Without Helm, edit the ConfigMap and add both required stanzas (`metadataStorage` and `mysql`):

```bash
kubectl edit configmap michelangelo-controllermgr-config
```

Add the configuration shown in the [ConfigMap Configuration](#configmap-configuration) section above.

#### 3. Restart the Controllermgr

Restart the controller manager to pick up the new configuration (`helm upgrade` does this automatically when the ConfigMap changes the pod template; otherwise restart manually):

```bash
# If controllermgr is a Deployment:
kubectl rollout restart deployment michelangelo-controllermgr

# If controllermgr is a bare Pod (e.g., sandbox):
kubectl delete pod michelangelo-controllermgr
```

#### 4. Verify Controllers Registered

Check that all 15 controllers registered successfully:

```bash
kubectl logs -l app.kubernetes.io/component=controllermgr | grep "Ingester controller registered"
```

You should see 15 log lines (one per CRD kind).

#### 5. Verify Backfill

All existing objects will be reconciled once on startup. Verify MySQL was populated:

```bash
for table in project model_family model pipeline pipeline_run inference_server \
             revision cluster ray_cluster ray_job trigger_run deployment spark_job \
             cached_output evaluation_report; do
  COUNT=$(kubectl exec pod/mysql -- mysql -uroot -proot michelangelo -sN \
    -e "SELECT COUNT(*) FROM ${table} WHERE delete_time IS NULL;" 2>/dev/null)
  echo "${table}: ${COUNT}"
done
```

Counts should be non-zero for every kind that has existing objects.

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

Objects created before the ingester was enabled will NOT have the ingester finalizer. They are still synced to MySQL on the controller's first startup, but are not intercepted during deletion via `kubectl delete`. If a user deletes such an object directly through `kubectl`, Kubernetes removes it from etcd immediately — the ingester never sees the deletion — leaving behind an orphan row in MySQL with `delete_time IS NULL`. Over time this causes MySQL to drift out of sync with etcd.

Note this is narrower than it may sound: it only applies to objects that never had the ingester finalizer added in the first place. Once an object carries the finalizer (the normal case for anything the ingester has reconciled at least once), a direct `kubectl delete` (or GitOps/foreground-GC deletion) is intercepted correctly — `handleCascadeDeletion` soft-deletes the MySQL row for non-opted-in kinds (and retains the final state for kinds opted into the cascade `RetainPolicy`) before the finalizer is removed, so recreating the object under the same name/namespace does not orphan the old row.

Two mitigations are available for the remaining pre-existing-object gap:

1. **Backfill controller (not yet implemented):** A periodic reconciliation controller that compares MySQL rows against etcd and soft-deletes any rows whose corresponding etcd object no longer exists. This is the cleanest long-term solution but requires additional engineering work.

2. **Require all deletes through the API Server:** The API Server sets the `michelangelo/Deleting` annotation instead of issuing a direct Kubernetes delete, which the ingester detects and handles correctly even without a finalizer. Enforce this operationally by restricting direct `kubectl delete` access to CRD objects.

Until a backfill controller is in place, option 2 is the recommended operational workaround for objects predating the ingester finalizer.

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

1. Set `metadataStorage.enableMetadataStorage` to `false` (Helm: `controllermgr.metadataStorage.enable: false`)
2. Restart the controllermgr
3. The ingester module sees no metadata storage and skips setup
4. MySQL data is preserved; you can re-enable the ingester later

Do not just remove the `mysql:` stanza while `enableMetadataStorage` is still `true`: the controller manager would fail at startup.

The controllermgr will continue operating normally with the ingester disabled.

## Monitoring and Troubleshooting

### Verify Ingester is Running

Check controller logs for startup messages:

```bash
kubectl logs -l app.kubernetes.io/component=controllermgr | grep -i ingester
```

You should see one `Ingester controller registered successfully` message per CRD kind (the `kind` field identifies each). If metadata storage is enabled but the MySQL config is empty, the controller manager exits at startup with `metadata storage is enabled but mysql config is empty`.

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
kubectl logs -l app.kubernetes.io/component=controllermgr | grep -i "requeue\|error" | tail -20
```

Requeue errors typically indicate:
- MySQL is unreachable
- Database credentials are wrong
- Permission issues on the MySQL connection

## Known Limitations and Issues

| Issue | Severity | Impact |
|-------|----------|--------|
| **SparkJob nil `resource` panic** | High | A SparkJob pod spec without `resource` can panic the SparkJob controller (independent of the ingester); if that takes down the controller manager, ALL MySQL sync stops. |
| Pre-existing objects lack finalizer | Medium | Objects created before ingester enable won't be intercepted during `kubectl delete`. Mitigation: use API Server for all deletes. |
| `DeleteCollection` not implemented | Medium | Namespace-scoped bulk deletes return error. Use individual object deletes. |
| No schema migration support | Medium | New indexed columns require manual `ALTER TABLE` and backfill. |

### SparkJob nil resource panic

**Status:** Open in `go/components/spark/job/client/client.go` (`toSparkPodSpec`)

**Symptom:** A panic in the SparkJob controller when a driver or executor `PodSpec` has no `resource`

**Mitigation:** Always set `resource` (cpu, memory) on SparkJob driver and executor pod specs

**Fix required:** Nil-check `pod.Resource` in `toSparkPodSpec`

---

## Verified Behavior

When the ingester is correctly enabled:

1. **All 15 controllers register** at startup (one per CRD kind)
2. **Immediate sync on creation** — objects appear in MySQL within milliseconds
3. **Immediate sync on update** — `res_version` and `update_time` advance in MySQL after every change
4. **Full JSON stored** — complete object JSON in the `json` column
5. **Extracted fields stored** — `algorithm`, `owner`, `state`, etc. in dedicated columns (see [SQL Key Concepts](../sql-key-concepts-and-terms.md#extracted-columns-and-sql-indexes))
6. **Labels synced** — label changes reflected in `*_labels` companion tables
7. **Opt-in disabled by default** — ingester only runs when `metadataStorage.enableMetadataStorage` is `true` and MySQL config is present in `michelangelo-controllermgr-config`

---

## Next Steps

Once the ingester is running, verify steady-state behavior: all 15 CRD kinds appear in MySQL, object counts match etcd, and `update_time` advances on changes. Then:

- **Monitor**: Set up alerting on requeue errors — elevated `workqueue_retries_total` for ingester controllers indicates MySQL connectivity issues. See [Monitoring](../operations/monitoring.md).
- **Restrict deletes**: Enforce all CRD deletes through the Michelangelo AI API Server (not raw `kubectl delete`) to prevent orphan rows from pre-existing objects.
- **Review internals**: See [Ingester Internals](../../contributing/ingester-internals.md) for developer documentation on extending the ingester or adding new CRD kinds.
