---
last_verified: 2026-10-14
---

# SQL Key Concepts and Terms

This page is the SQL reference for operators querying Michelangelo AI's metadata database — schema layout, indexed columns, safe query patterns, and known storage limitations. Michelangelo AI uses SQL for platform metadata, not for storing training datasets or feature values. The ingester syncs Kubernetes custom resources into MySQL so API and operations workflows can query metadata without depending only on etcd.

**Audience:** Platform operators running Michelangelo AI's ingester and metadata DB in production.

**Prerequisites:** The ingester must be deployed and connected to MySQL. See [Ingester Controller: Configuration and Operations](./components/ingester-configuration.md) if you haven't done this yet.

**You'll reach for this page when:**
- Writing a diagnostic SQL query to investigate a stuck `pipeline_run` or missing model
- Deciding which columns are indexed before adding a filter to an ad-hoc query
- Debugging a label filter that returns unexpected results
- Reading the schema files to understand what the ingester writes

## SQL Surfaces

| Surface | Location | Purpose |
|---------|----------|---------|
| Helm schema | `helm/michelangelo/files/schema/mysql-init-schema.sql` | Schema bundled into the Helm chart and mounted into the API server schema-init container |
| Standalone ingester schema | `scripts/ingester/ingester_schema.sql` | The file actually referenced by `ingester_schema_job.yaml` and `init_ingester_db.sh` at runtime. This is the authoritative copy for ingester setup. |
| Complete ingester schema | `scripts/ingester/complete_ingester_schema.sql` | An older convenience copy (13 kinds). No automation references this file directly and it has not been kept in sync with `ingester_schema.sql`. |
| Schema init Job | `scripts/ingester/ingester_schema_job.yaml` | Kubernetes Job that waits for MySQL and creates the ingester tables |
| Local init script | `scripts/ingester/init_ingester_db.sh` | Shell helper for initializing a reachable MySQL instance |
| Runtime SQL code | `go/storage/mysql/mysql.go` | MySQL implementation for upserts, reads, list queries, labels, annotations, and soft deletes |

`ingester_schema.sql` and the Helm schema define the same 15 resource kinds and differ only in comments and whitespace. `complete_ingester_schema.sql` is out of date (it defines only 13 kinds, without `cached_output` and `evaluation_report`, and uses older index names), so do not use it for new installs. No automation keeps these files in sync — when you change one, update the others by hand.

## Core Terms

| Term | Meaning |
|------|---------|
| Metadata storage | The optional SQL-backed store for Michelangelo AI custom resource metadata |
| Ingester | Controller that watches Michelangelo AI CRDs and writes their metadata to MySQL |
| CRD table | Main table for one Kubernetes custom resource kind, such as `model` or `pipeline_run` |
| Side table | Per-kind table for labels or annotations, such as `model_labels` or `model_annotations` |
| Extracted column | A CRD field copied into a dedicated SQL column so callers can read it without parsing the `json` payload. Extracted columns are not necessarily indexed — see [Extracted Columns and SQL Indexes](#extracted-columns-and-sql-indexes) |
| SQL index | An explicit `KEY` declared on a column (or column tuple) in the schema, which lets MySQL satisfy filters on those columns without a full table scan |
| Soft delete | Delete behavior that sets `delete_time` instead of removing the row |
| Resource version | Kubernetes `metadata.resourceVersion`, stored as `res_version` for reconciliation ordering |
| Proto column | Serialized protobuf representation of the object, stored in `proto` |
| JSON column | Full JSON representation of the object, stored in `json` |

## Schema Model

Each supported CRD kind has three tables:

| Table | Example | Stores |
|-------|---------|--------|
| Main table | `model` | Object identity, timestamps, serialized payloads, and extracted columns |
| Labels table | `model_labels` | Kubernetes labels for each object UID |
| Annotations table | `model_annotations` | Kubernetes annotations for each object UID |

The schema currently covers 15 resource kinds: Project, ModelFamily, Model, Pipeline, PipelineRun, InferenceServer, Revision, Cluster, RayCluster, RayJob, TriggerRun, Deployment, SparkJob, CachedOutput, and EvaluationReport. See [Extracted Columns and SQL Indexes](#extracted-columns-and-sql-indexes) for the per-kind column detail.

That produces 45 tables total: 15 main tables, 15 label tables, and 15 annotation tables.

## Table Relationships

The schema uses object UIDs to connect main tables to their side tables:

```text
<kind>
  uid
  namespace
  name
  ...
    |
    | <kind>.uid = <kind>_labels.obj_uid
    v
<kind>_labels

<kind>
  uid
  namespace
  name
  ...
    |
    | <kind>.uid = <kind>_annotations.obj_uid
    v
<kind>_annotations
```

Cross-resource relationships are stored as denormalized namespace/name columns instead of foreign keys. For example:

| Relationship | Columns |
|--------------|---------|
| PipelineRun to Pipeline | `pipeline_run.pipeline_namespace`, `pipeline_run.pipeline_name` |
| PipelineRun to Revision | `pipeline_run.revision_namespace`, `pipeline_run.revision_name` |
| TriggerRun to Pipeline | `trigger_run.pipeline_namespace`, `trigger_run.pipeline_name` |
| TriggerRun to Revision | `trigger_run.revision_namespace`, `trigger_run.revision_name` |
| Model to ModelFamily | `model.model_family_namespace`, `model.model_family_name` |
| Revision to base resource | `revision.base_resource_namespace`, `revision.base_resource_name`, `revision.base_type` |

The schema does not define SQL foreign key constraints. Consistency is maintained by Kubernetes reconciliation and ingester writes.

:::note Label and annotation value sizes
In the side tables, label `value` columns are typed `VARCHAR(63)` while annotation `value` columns are typed `TEXT`. This matches Kubernetes, which limits label values to 63 characters but allows much longer annotation values.
:::

## Main Table Columns

Every main table shares a common base shape:

| Column | Purpose |
|--------|---------|
| `uid` | Kubernetes object UID and primary key |
| `group_ver` | API group/version for the object |
| `namespace` | Kubernetes namespace |
| `name` | Kubernetes object name |
| `res_version` | Kubernetes resource version |
| `create_time` | Object creation timestamp, sourced from the Kubernetes resource |
| `update_time` | Wall-clock time of the last ingester upsert (`time.Now().UTC()` at write time), not a field copied from the Kubernetes resource |
| `delete_time` | Soft-delete timestamp, or `NULL` for active rows |
| `proto` | Serialized protobuf object |
| `json` | Full JSON object |

Main tables also include CRD-specific extracted columns. Examples include `model.algorithm`, `model.description`, `model.owner`, `pipeline.owner`, `pipeline_run.state`, `deployment.state`, and `inference_server.state`. Whether a given extracted column also has a SQL index depends on the table — see [Extracted Columns and SQL Indexes](#extracted-columns-and-sql-indexes).

## Extracted Columns and SQL Indexes

The schema treats two related ideas as separate concerns. Knowing which is which decides whether a query is cheap or scans the whole table.

**Extracted columns** are CRD fields the ingester copies from the protobuf payload into dedicated SQL columns at upsert time. With an extracted column you can read or filter on the field directly in SQL without parsing the `json` column — but the filter is not necessarily fast.

**SQL indexes** are explicit `KEY` declarations on a column or column tuple. Filters on indexed columns can use the index; filters on non-indexed columns require a full table scan even when the column is extracted.

Every main table has a `PRIMARY KEY` on `uid`. Beyond that, only the columns listed below have a SQL index today.

| Main Table | Indexes (besides the primary key on `uid`) |
|------------|---------------------------------------------|
| `model` | `(namespace, name)`, `create_time`, `algorithm`, `owner` |
| `model_family` | `(namespace, name)`, `create_time` |
| `pipeline` | `(namespace, name)`, `create_time`, `owner` |
| `pipeline_run` | `(namespace, name)`, `create_time`, `(pipeline_namespace, pipeline_name)`, `state` |
| `deployment` | `(namespace, name)`, `create_time`, `state` |
| `inference_server` | `(namespace, name)`, `create_time`, `state` |
| `project` | `(namespace, name)`, `create_time` |
| `revision` | `(namespace, name)`, `create_time`, `(base_resource_namespace, base_resource_name)` |
| `cluster` | `(namespace, name)`, `create_time` |
| `ray_cluster` | `(namespace, name)`, `create_time` |
| `ray_job` | `(namespace, name)`, `create_time` |
| `spark_job` | `(namespace, name)`, `create_time` |
| `trigger_run` | `(namespace, name)`, `create_time`, `(pipeline_namespace, pipeline_name)`, `state` |
| `cached_output` | `(namespace, name)`, `create_time` |
| `evaluation_report` | `(namespace, name)`, `create_time` |

Side tables (`<kind>_labels`, `<kind>_annotations`) have a `PRIMARY KEY` on `id` and indexes on `obj_uid`. Label tables additionally index `(key, value)`; annotation tables do not (the `value` is `TEXT`).

Many extracted columns are not indexed. For example, `pipeline_run` extracts `actor`, `end_time`, `exception_type`, and several namespace/name reference pairs in addition to the indexed `state` column — filtering by any of these is supported but requires a table scan. Common extracted columns by table:

| Main Table | Extracted Columns Beyond the Common Base |
|------------|------------------------------------------|
| `model` | `algorithm`, `training_framework`, `owner`, `source`, `description`, `model_kind`, `package_type`, `revision_id`, `src_pipeline_run_namespace`/`src_pipeline_run_name`, `model_family_namespace`/`model_family_name`, plus four eval-report namespace/name pairs (`feature_eval_report`, `performance_eval_report`, `feature_quality_report`, `explainability_report`) |
| `model_family` | `model_family_name` |
| `pipeline` | `owner`, `pipeline_type` |
| `pipeline_run` | `pipeline_namespace`/`pipeline_name`, `revision_namespace`/`revision_name`, `resume_pipeline_run_namespace`/`resume_pipeline_run_name`, `state`, `actor`, `end_time`, `exception_type` |
| `deployment` | `state`, `target_definition_type`, `current_revision_namespace`/`current_revision_name`, `deletion_requested_timestamp` |
| `inference_server` | `state` |
| `project` | `tier` |
| `revision` | `base_resource_namespace`/`base_resource_name`, `base_type`, `commit_branch`, `git_ref`, `owner` |
| `trigger_run` | `pipeline_namespace`/`pipeline_name`, `revision_namespace`/`revision_name`, `state`, `auto_flip` |
| `cluster`, `ray_cluster`, `ray_job`, `spark_job`, `cached_output`, `evaluation_report` | None beyond the common base columns |

When you write a query, prefer filters on indexed columns. Filters on extracted-but-unindexed columns still work, but expect linear scan cost. Filters that have to dig into the `json` column should be reserved for one-off diagnostic queries.

## Current Storage-Layer Limitations

Several `MetadataStorage` operations are stubbed in `go/storage/mysql/mysql.go` and return an `Unimplemented` error. Operators relying on these paths should know what does and does not work today:

| Operation | Behavior |
|-----------|----------|
| `DeleteCollection` | Returns `DeleteCollection not yet implemented`. Use `Delete` per object instead. |
| `QueryByTemplateID` | Returns `QueryByTemplateID not yet implemented`. |
| `Backfill` | Returns `Backfill not yet implemented`. |

`Upsert` with `direct = true` is implemented: it updates only the labels, annotations, and resource version (rewriting the stored `proto` and `json` blobs), with optimistic concurrency on the resource version. `List` applies Kubernetes label and field selector strings (`=`, `==`, `in`, `exists`, `!key`, `notin` for labels) as well as the structured criterion filters.

## Query Patterns

### Fetch a Live Object by Namespace and Name

```sql
SELECT proto
FROM model
WHERE namespace = 'default'
  AND name = 'my-model'
  AND delete_time IS NULL;
```

### List Live Objects by State

The `state` column holds the proto enum name (for example `PIPELINE_RUN_STATE_FAILED`).

```sql
SELECT namespace, name, state, update_time
FROM pipeline_run
WHERE state = 'PIPELINE_RUN_STATE_FAILED'
  AND delete_time IS NULL
ORDER BY update_time DESC;
```

### Join Labels for Filtering

```sql
SELECT m.namespace, m.name, m.update_time
FROM model AS m
JOIN model_labels AS l
  ON l.obj_uid = m.uid
WHERE l.`key` = 'team'
  AND l.`value` = 'fraud'
  AND m.delete_time IS NULL;
```

Note: the storage layer's `List` API also supports Kubernetes label selectors, but when you query the database directly, join the label table as above.

### Inspect a Soft-Deleted Object

```sql
SELECT namespace, name, delete_time
FROM pipeline
WHERE delete_time IS NOT NULL
ORDER BY delete_time DESC;
```

## Write Patterns

:::warning
The ingester owns writes to these tables. Application code should use the Michelangelo AI API or Kubernetes CRDs rather than writing SQL directly.
:::

Each ingester upsert overwrites the row's payload, timestamps, and indexed columns; labels and annotations are replaced wholesale.

Deletes are soft deletes. The row remains in the main table with `delete_time` set, which preserves metadata for audits and delayed cleanup workflows.

## SQL File Conventions

- Main table names are the snake_case CRD kind names, for example `ModelFamily` becomes `model_family` and `PipelineRun` becomes `pipeline_run`.
- Side tables use `<main_table>_labels` and `<main_table>_annotations`.
- Column names use snake case.
- Identifiers are quoted with backticks in schema files.

## Related Docs

- [Ingester Controller: Configuration and Operations](./components/ingester-configuration.md)
- [Ingester Controller: Architecture and Implementation](../contributing/ingester-internals.md)
