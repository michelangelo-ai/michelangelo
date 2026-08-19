---
sidebar_position: 10
sidebar_label: API Reference
---

# Jobs API Reference

This page documents the gRPC services that back job submission and lifecycle management in Michelangelo AI. Two services are defined in `proto/api/v2/`:

- **RayJobService** — create, inspect, and delete Ray training or batch inference jobs.
- **SparkJobService** — create, inspect, and delete Spark ETL and batch processing jobs.

Both services follow the same CRUD + `DeleteCollection` + `List` pattern. The `Required` column in each table reflects fields that must be supplied for a successful call, inferred from the proto structure (proto3 does not encode field necessity explicitly). Option structs sourced from the Kubernetes API machinery (`CreateOptions`, `GetOptions`, etc.) are always optional.

---

## RayJobService

RayJob Service defines the RayJob related methods, such as CRUD and list.

**Proto source:** `proto/api/v2/ray_job_svc.proto`

### CreateRayJob

`CreateRayJob(CreateRayJobRequest) → CreateRayJobResponse`

Create a new RayJob with the given spec.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `ray_job` | `RayJob` | Yes | The metadata and spec of the RayJob to be created. |
| `create_options` | `k8s.io.apimachinery.pkg.apis.meta.v1.CreateOptions` | No | No description provided in source. |

**Response fields**

| Field | Type | Description |
|-------|------|-------------|
| `ray_job` | `RayJob` | The created RayJob. |

---

### GetRayJob

`GetRayJob(GetRayJobRequest) → GetRayJobResponse`

Get the specified RayJob.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | `string` | Yes | Name of the RayJob. |
| `namespace` | `string` | Yes | Object name and auth scope. |
| `get_options` | `k8s.io.apimachinery.pkg.apis.meta.v1.GetOptions` | No | No description provided in source. |

**Response fields**

| Field | Type | Description |
|-------|------|-------------|
| `ray_job` | `RayJob` | The requested RayJob. |

---

### UpdateRayJob

`UpdateRayJob(UpdateRayJobRequest) → UpdateRayJobResponse`

Update a RayJob with the given spec.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `ray_job` | `RayJob` | Yes | The metadata and spec of the RayJob to be updated. |
| `update_options` | `k8s.io.apimachinery.pkg.apis.meta.v1.UpdateOptions` | No | No description provided in source. |

**Response fields**

| Field | Type | Description |
|-------|------|-------------|
| `ray_job` | `RayJob` | The updated RayJob. |

---

### DeleteRayJob

`DeleteRayJob(DeleteRayJobRequest) → DeleteRayJobResponse`

Delete a RayJob.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | `string` | Yes | Name of the RayJob. |
| `namespace` | `string` | Yes | Object name and auth scope. |
| `delete_options` | `k8s.io.apimachinery.pkg.apis.meta.v1.DeleteOptions` | No | No description provided in source. |

**Response fields**

The response message carries no fields.

---

### DeleteRayJobCollection

`DeleteRayJobCollection(DeleteRayJobCollectionRequest) → DeleteRayJobCollectionResponse`

Delete collection of RayJob.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `namespace` | `string` | Yes | Object name and auth scope. |
| `delete_options` | `k8s.io.apimachinery.pkg.apis.meta.v1.DeleteOptions` | No | No description provided in source. |
| `list_options` | `k8s.io.apimachinery.pkg.apis.meta.v1.ListOptions` | No | No description provided in source. |

**Response fields**

The response message carries no fields.

---

### ListRayJob

`ListRayJob(ListRayJobRequest) → ListRayJobResponse`

List objects of type RayJob.

> **Note:** Watch and list across all namespaces are not supported.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `namespace` | `string` | Yes | No description provided in source. |
| `list_options` | `k8s.io.apimachinery.pkg.apis.meta.v1.ListOptions` | No | No description provided in source. |
| `list_options_ext` | `michelangelo.api.ListOptionsExt` | No | No description provided in source. |

**Response fields**

| Field | Type | Description |
|-------|------|-------------|
| `ray_job_list` | `RayJobList` | No description provided in source. |

---

## SparkJobService

SparkJob Service defines the SparkJob related methods, such as CRUD and list.

**Proto source:** `proto/api/v2/spark_job_svc.proto`

### CreateSparkJob

`CreateSparkJob(CreateSparkJobRequest) → CreateSparkJobResponse`

Create a new SparkJob with the given spec.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `spark_job` | `SparkJob` | Yes | The metadata and spec of the SparkJob to be created. |
| `create_options` | `k8s.io.apimachinery.pkg.apis.meta.v1.CreateOptions` | No | No description provided in source. |

**Response fields**

| Field | Type | Description |
|-------|------|-------------|
| `spark_job` | `SparkJob` | The created SparkJob. |

---

### GetSparkJob

`GetSparkJob(GetSparkJobRequest) → GetSparkJobResponse`

Get the specified SparkJob.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | `string` | Yes | Name of the SparkJob. |
| `namespace` | `string` | Yes | Object name and auth scope. |
| `get_options` | `k8s.io.apimachinery.pkg.apis.meta.v1.GetOptions` | No | No description provided in source. |

**Response fields**

| Field | Type | Description |
|-------|------|-------------|
| `spark_job` | `SparkJob` | The requested SparkJob. |

---

### UpdateSparkJob

`UpdateSparkJob(UpdateSparkJobRequest) → UpdateSparkJobResponse`

Update a SparkJob with the given spec.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `spark_job` | `SparkJob` | Yes | The metadata and spec of the SparkJob to be updated. |
| `update_options` | `k8s.io.apimachinery.pkg.apis.meta.v1.UpdateOptions` | No | No description provided in source. |

**Response fields**

| Field | Type | Description |
|-------|------|-------------|
| `spark_job` | `SparkJob` | The updated SparkJob. |

---

### DeleteSparkJob

`DeleteSparkJob(DeleteSparkJobRequest) → DeleteSparkJobResponse`

Delete a SparkJob.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | `string` | Yes | Name of the SparkJob. |
| `namespace` | `string` | Yes | Object name and auth scope. |
| `delete_options` | `k8s.io.apimachinery.pkg.apis.meta.v1.DeleteOptions` | No | No description provided in source. |

**Response fields**

The response message carries no fields.

---

### DeleteSparkJobCollection

`DeleteSparkJobCollection(DeleteSparkJobCollectionRequest) → DeleteSparkJobCollectionResponse`

Delete collection of SparkJob.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `namespace` | `string` | Yes | Object name and auth scope. |
| `delete_options` | `k8s.io.apimachinery.pkg.apis.meta.v1.DeleteOptions` | No | No description provided in source. |
| `list_options` | `k8s.io.apimachinery.pkg.apis.meta.v1.ListOptions` | No | No description provided in source. |

**Response fields**

The response message carries no fields.

---

### ListSparkJob

`ListSparkJob(ListSparkJobRequest) → ListSparkJobResponse`

List objects of type SparkJob.

> **Note:** Watch and list across all namespaces are not supported.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `namespace` | `string` | Yes | No description provided in source. |
| `list_options` | `k8s.io.apimachinery.pkg.apis.meta.v1.ListOptions` | No | No description provided in source. |
| `list_options_ext` | `michelangelo.api.ListOptionsExt` | No | No description provided in source. |

**Response fields**

| Field | Type | Description |
|-------|------|-------------|
| `spark_job_list` | `SparkJobList` | No description provided in source. |

---
