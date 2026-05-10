# SQL Key Concepts and Terms

Michelangelo uses SQL for platform metadata, not for storing training datasets or feature values. The ingester syncs Kubernetes custom resources into MySQL so API and operations workflows can query metadata without depending only on etcd.

Use this page when you need to understand the schema files, table naming, query patterns, and storage terms used by Michelangelo's SQL-backed metadata layer.

## SQL Surfaces

| Surface | Location | Purpose |
|---------|----------|---------|
| Helm schema | `helm/michelangelo/files/schema/mysql-init-schema.sql` | Schema bundled into the Helm chart and mounted into the API server schema-init container |
| Standalone ingester schema | `scripts/ingester/ingester_schema.sql` | Schema used by ingester setup scripts and jobs |
| Schema init Job | `scripts/ingester/ingester_schema_job.yaml` | Kubernetes Job that waits for MySQL and creates the ingester tables |
| Local init script | `scripts/ingester/init_ingester_db.sh` | Shell helper for initializing a reachable MySQL instance |
| Runtime SQL code | `go/storage/mysql/mysql.go` | MySQL implementation for upserts, reads, list queries, labels, annotations, and soft deletes |

## Core Terms

| Term | Meaning |
|------|---------|
| Metadata storage | The optional SQL-backed store for Michelangelo custom resource metadata |
| Ingester | Controller that watches Michelangelo CRDs and writes their metadata to MySQL |
| CRD table | Main table for one Kubernetes custom resource kind, such as `model` or `pipelinerun` |
| Side table | Per-kind table for labels or annotations, such as `model_labels` or `model_annotations` |
| Indexed field | A CRD field copied into a dedicated SQL column for efficient lookup |
| Soft delete | Delete behavior that sets `delete_time` instead of removing the row |
| Resource version | Kubernetes `metadata.resourceVersion`, stored as `res_version` for reconciliation ordering |
| Proto column | Serialized protobuf representation of the object, stored in `proto` |
| JSON column | Full JSON representation of the object, stored in `json` |

## Schema Model

Each supported CRD kind has three tables:

| Table | Example | Stores |
|-------|---------|--------|
| Main table | `model` | Object identity, timestamps, serialized payloads, and indexed fields |
| Labels table | `model_labels` | Kubernetes labels for each object UID |
| Annotations table | `model_annotations` | Kubernetes annotations for each object UID |

The schema currently covers these 13 resource kinds:

| CRD Kind | Main Table |
|----------|------------|
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

That produces 39 tables total: 13 main tables, 13 label tables, and 13 annotation tables.

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
| PipelineRun to Pipeline | `pipelinerun.pipeline_namespace`, `pipelinerun.pipeline_name` |
| PipelineRun to Revision | `pipelinerun.revision_namespace`, `pipelinerun.revision_name` |
| TriggerRun to Pipeline | `triggerrun.pipeline_namespace`, `triggerrun.pipeline_name` |
| TriggerRun to Revision | `triggerrun.revision_namespace`, `triggerrun.revision_name` |
| Model to ModelFamily | `model.model_family_namespace`, `model.model_family_name` |
| Revision to base resource | `revision.base_resource_namespace`, `revision.base_resource_name`, `revision.base_type` |

The schema does not define SQL foreign key constraints. Consistency is maintained by Kubernetes reconciliation and ingester writes.

## Main Table Columns

Every main table shares a common base shape:

| Column | Purpose |
|--------|---------|
| `uid` | Kubernetes object UID and primary key |
| `group_ver` | API group/version for the object |
| `namespace` | Kubernetes namespace |
| `name` | Kubernetes object name |
| `res_version` | Kubernetes resource version |
| `create_time` | Object creation timestamp |
| `update_time` | Last observed update timestamp |
| `delete_time` | Soft-delete timestamp, or `NULL` for active rows |
| `proto` | Serialized protobuf object |
| `json` | Full JSON object |

Main tables also include CRD-specific indexed columns. Examples include `model.algorithm`, `model.owner`, `pipeline.owner`, `pipelinerun.state`, `deployment.state`, and `inferenceserver.state`.

## Indexed Fields

Indexed fields are duplicated from CRD payloads into SQL columns so callers do not need to scan or extract from the `json` column. The generated CRD code exposes these fields through `GetIndexedKeyValuePairs()`, and the ingester passes them to the MySQL storage layer during upsert.

Common indexed fields:

| Main Table | Indexed Fields |
|------------|----------------|
| `model` | `algorithm`, `training_framework`, `owner`, `source`, `model_kind`, `package_type`, revision and report references |
| `pipeline` | `owner`, `pipeline_type` |
| `pipelinerun` | pipeline reference, revision reference, resume reference, `state`, `actor`, `end_time`, `exception_type` |
| `deployment` | `state`, `target_definition_type`, current revision reference, `deletion_requested_timestamp` |
| `inferenceserver` | `state` |
| `project` | `tier` |
| `revision` | base resource reference, `base_type`, `commit_branch`, `git_ref`, `owner` |
| `triggerrun` | pipeline reference, revision reference, `state`, `auto_flip` |

Use indexed columns for filters that appear in normal API or operations paths. Use the `json` column only when no indexed column exists and the query is diagnostic or low-volume.

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

```sql
SELECT namespace, name, state, update_time
FROM pipelinerun
WHERE state = 'FAILED'
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

### Inspect a Soft-Deleted Object

```sql
SELECT namespace, name, delete_time
FROM pipeline
WHERE delete_time IS NOT NULL
ORDER BY delete_time DESC;
```

## Write Patterns

The ingester owns writes to these tables. Application code should use the Michelangelo API or Kubernetes CRDs rather than writing SQL directly.

The storage layer uses `INSERT ... ON DUPLICATE KEY UPDATE` for main table writes. Labels and annotations are replaced on each upsert by deleting existing side-table rows for the object UID and inserting the current key/value pairs.

Deletes are soft deletes. The row remains in the main table with `delete_time` set, which preserves metadata for audits and delayed cleanup workflows.

## SQL File Conventions

- Main table names are lowercase CRD kind names, for example `ModelFamily` becomes `modelfamily`.
- Side tables use `<main_table>_labels` and `<main_table>_annotations`.
- Column names use snake case.
- Identifier names are quoted with backticks in schema files.
- Schema files should be idempotent and use `CREATE TABLE IF NOT EXISTS`.
- New queryable CRD fields should be added as indexed fields in the protobuf options and generated SQL, not only queried from the `json` column.

## Related Docs

- [Ingester Controller: Configuration and Operations](./ingester-configuration.md)
- [Ingester Controller: Architecture and Implementation](../contributing/ingester-internals.md)
