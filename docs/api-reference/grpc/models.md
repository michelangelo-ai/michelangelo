---
sidebar_position: 1
sidebar_label: Models
---

# Model Management

Michelangelo AI provides four gRPC services for managing ML models and their associated resources: `ModelService` for registered model artifacts, `ModelFamilyService` for grouping related model versions, `CachedOutputService` for intermediate pipeline outputs and training checkpoints, and `EvaluationReportService` for model evaluation reports.

Proto sources: `proto/api/v2/model_svc.proto`, `proto/api/v2/model_family_svc.proto`, `proto/api/v2/cached_output_svc.proto`, `proto/api/v2/evaluation_report_svc.proto`.

> New to this reference? See [How to Read the gRPC API Reference](./conventions.md) for the shared CRUD-plus-list pattern, the Required column, and how undocumented fields are shown.

---

## ModelService

Model Service defines the Model related methods, such as CRUD and list.

A `Model` records a trained ML artifact together with its metadata: ownership, ML problem kind, training framework, input/output schemas, quality scores, feature importance, and references to associated evaluation reports and pipeline runs.

### CreateModel

`CreateModel(CreateModelRequest) → CreateModelResponse`

Create a new Model with the given spec.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `model` | `Model` | Yes | The metadata and spec of the Model to be created. |
| `create_options` | `k8s.io.apimachinery.pkg.apis.meta.v1.CreateOptions` | No | — |

**Response fields**

| Field | Type | Description |
|-------|------|-------------|
| `model` | `Model` | The created Model. |

---

### GetModel

`GetModel(GetModelRequest) → GetModelResponse`

Get the specified Model.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | `string` | Yes | Name of the Model. |
| `namespace` | `string` | Yes | Object name and auth scope. |
| `get_options` | `k8s.io.apimachinery.pkg.apis.meta.v1.GetOptions` | No | — |

**Response fields**

| Field | Type | Description |
|-------|------|-------------|
| `model` | `Model` | The requested Model. |

---

### UpdateModel

`UpdateModel(UpdateModelRequest) → UpdateModelResponse`

Update a Model with the given spec.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `model` | `Model` | Yes | The metadata and spec of the Model to be updated. |
| `update_options` | `k8s.io.apimachinery.pkg.apis.meta.v1.UpdateOptions` | No | — |

**Response fields**

| Field | Type | Description |
|-------|------|-------------|
| `model` | `Model` | The updated Model. |

---

### DeleteModel

`DeleteModel(DeleteModelRequest) → DeleteModelResponse`

Delete a Model.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | `string` | Yes | Name of the Model. |
| `namespace` | `string` | Yes | Object name and auth scope. |
| `delete_options` | `k8s.io.apimachinery.pkg.apis.meta.v1.DeleteOptions` | No | — |

**Response fields**

`DeleteModelResponse` is empty.

---

### DeleteModelCollection

`DeleteModelCollection(DeleteModelCollectionRequest) → DeleteModelCollectionResponse`

Delete collection of Model.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `namespace` | `string` | Yes | Object name and auth scope. |
| `delete_options` | `k8s.io.apimachinery.pkg.apis.meta.v1.DeleteOptions` | No | — |
| `list_options` | `k8s.io.apimachinery.pkg.apis.meta.v1.ListOptions` | No | — |

**Response fields**

`DeleteModelCollectionResponse` is empty.

---

### ListModel

`ListModel(ListModelRequest) → ListModelResponse`

List objects of type Model.

> **Note:** Watch and list across all namespaces are not supported.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `namespace` | `string` | Yes | — |
| `list_options` | `k8s.io.apimachinery.pkg.apis.meta.v1.ListOptions` | No | — |
| `list_options_ext` | `michelangelo.api.ListOptionsExt` | No | — |

**Response fields**

| Field | Type | Description |
|-------|------|-------------|
| `model_list` | `ModelList` | — |

---

## ModelFamilyService

ModelFamily Service defines the ModelFamily related methods, such as CRUD and list.

A `ModelFamily` is a sub-problem in a project that logically groups related model versions under a stable name. The family name is limited to 40 alphanumeric/underscore/dash characters to avoid length issues in derived entities such as alerts and dashboards.

### CreateModelFamily

`CreateModelFamily(CreateModelFamilyRequest) → CreateModelFamilyResponse`

Create a new ModelFamily with the given spec.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `model_family` | `ModelFamily` | Yes | The metadata and spec of the ModelFamily to be created. |
| `create_options` | `k8s.io.apimachinery.pkg.apis.meta.v1.CreateOptions` | No | — |

**Response fields**

| Field | Type | Description |
|-------|------|-------------|
| `model_family` | `ModelFamily` | The created ModelFamily. |

---

### GetModelFamily

`GetModelFamily(GetModelFamilyRequest) → GetModelFamilyResponse`

Get the specified ModelFamily.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | `string` | Yes | Name of the ModelFamily. |
| `namespace` | `string` | Yes | Object name and auth scope. |
| `get_options` | `k8s.io.apimachinery.pkg.apis.meta.v1.GetOptions` | No | — |

**Response fields**

| Field | Type | Description |
|-------|------|-------------|
| `model_family` | `ModelFamily` | The requested ModelFamily. |

---

### UpdateModelFamily

`UpdateModelFamily(UpdateModelFamilyRequest) → UpdateModelFamilyResponse`

Update a ModelFamily with the given spec.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `model_family` | `ModelFamily` | Yes | The metadata and spec of the ModelFamily to be updated. |
| `update_options` | `k8s.io.apimachinery.pkg.apis.meta.v1.UpdateOptions` | No | — |

**Response fields**

| Field | Type | Description |
|-------|------|-------------|
| `model_family` | `ModelFamily` | The updated ModelFamily. |

---

### DeleteModelFamily

`DeleteModelFamily(DeleteModelFamilyRequest) → DeleteModelFamilyResponse`

Delete a ModelFamily.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | `string` | Yes | Name of the ModelFamily. |
| `namespace` | `string` | Yes | Object name and auth scope. |
| `delete_options` | `k8s.io.apimachinery.pkg.apis.meta.v1.DeleteOptions` | No | — |

**Response fields**

`DeleteModelFamilyResponse` is empty.

---

### DeleteModelFamilyCollection

`DeleteModelFamilyCollection(DeleteModelFamilyCollectionRequest) → DeleteModelFamilyCollectionResponse`

Delete collection of ModelFamily.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `namespace` | `string` | Yes | Object name and auth scope. |
| `delete_options` | `k8s.io.apimachinery.pkg.apis.meta.v1.DeleteOptions` | No | — |
| `list_options` | `k8s.io.apimachinery.pkg.apis.meta.v1.ListOptions` | No | — |

**Response fields**

`DeleteModelFamilyCollectionResponse` is empty.

---

### ListModelFamily

`ListModelFamily(ListModelFamilyRequest) → ListModelFamilyResponse`

List objects of type ModelFamily.

> **Note:** Watch and list across all namespaces are not supported.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `namespace` | `string` | Yes | — |
| `list_options` | `k8s.io.apimachinery.pkg.apis.meta.v1.ListOptions` | No | — |
| `list_options_ext` | `michelangelo.api.ListOptionsExt` | No | — |

**Response fields**

| Field | Type | Description |
|-------|------|-------------|
| `model_family_list` | `ModelFamilyList` | — |

---

## CachedOutputService

CachedOutput Service defines the CachedOutput related methods, such as CRUD and list.

A `CachedOutput` represents cached output generated by an ML application and stored in blob storage (e.g. S3). The resource covers four output types: generic pipeline variables (`CACHED_OUTPUT_TYPE_VARIABLE`), data-transform step checkpoints (`CACHED_OUTPUT_TYPE_TRANSFORM_CKPT`), trainer step checkpoints (`CACHED_OUTPUT_TYPE_TRAINING_CKPT`), and raw model files saved before packaging (`CACHED_OUTPUT_TYPE_RAW_MODEL`). Checkpoints are referenced by `IncrementalTrainingSpec` fields on a `Model` to enable incremental retraining.

### CreateCachedOutput

`CreateCachedOutput(CreateCachedOutputRequest) → CreateCachedOutputResponse`

Create a new CachedOutput with the given spec.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `cached_output` | `CachedOutput` | Yes | The metadata and spec of the CachedOutput to be created. |
| `create_options` | `k8s.io.apimachinery.pkg.apis.meta.v1.CreateOptions` | No | — |

**Response fields**

| Field | Type | Description |
|-------|------|-------------|
| `cached_output` | `CachedOutput` | The created CachedOutput. |

---

### GetCachedOutput

`GetCachedOutput(GetCachedOutputRequest) → GetCachedOutputResponse`

Get the specified CachedOutput.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | `string` | Yes | Name of the CachedOutput. |
| `namespace` | `string` | Yes | Object name and auth scope. |
| `get_options` | `k8s.io.apimachinery.pkg.apis.meta.v1.GetOptions` | No | — |

**Response fields**

| Field | Type | Description |
|-------|------|-------------|
| `cached_output` | `CachedOutput` | The requested CachedOutput. |

---

### UpdateCachedOutput

`UpdateCachedOutput(UpdateCachedOutputRequest) → UpdateCachedOutputResponse`

Update a CachedOutput with the given spec.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `cached_output` | `CachedOutput` | Yes | The metadata and spec of the CachedOutput to be updated. |
| `update_options` | `k8s.io.apimachinery.pkg.apis.meta.v1.UpdateOptions` | No | — |

**Response fields**

| Field | Type | Description |
|-------|------|-------------|
| `cached_output` | `CachedOutput` | The updated CachedOutput. |

---

### DeleteCachedOutput

`DeleteCachedOutput(DeleteCachedOutputRequest) → DeleteCachedOutputResponse`

Delete a CachedOutput.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | `string` | Yes | Name of the CachedOutput. |
| `namespace` | `string` | Yes | Object name and auth scope. |
| `delete_options` | `k8s.io.apimachinery.pkg.apis.meta.v1.DeleteOptions` | No | — |

**Response fields**

`DeleteCachedOutputResponse` is empty.

---

### DeleteCachedOutputCollection

`DeleteCachedOutputCollection(DeleteCachedOutputCollectionRequest) → DeleteCachedOutputCollectionResponse`

Delete collection of CachedOutput.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `namespace` | `string` | Yes | Object name and auth scope. |
| `delete_options` | `k8s.io.apimachinery.pkg.apis.meta.v1.DeleteOptions` | No | — |
| `list_options` | `k8s.io.apimachinery.pkg.apis.meta.v1.ListOptions` | No | — |

**Response fields**

`DeleteCachedOutputCollectionResponse` is empty.

---

### ListCachedOutput

`ListCachedOutput(ListCachedOutputRequest) → ListCachedOutputResponse`

List objects of type CachedOutput.

> **Note:** Watch and list across all namespaces are not supported.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `namespace` | `string` | Yes | — |
| `list_options` | `k8s.io.apimachinery.pkg.apis.meta.v1.ListOptions` | No | — |
| `list_options_ext` | `michelangelo.api.ListOptionsExt` | No | — |

**Response fields**

| Field | Type | Description |
|-------|------|-------------|
| `cached_output_list` | `CachedOutputList` | — |

---

## EvaluationReportService

EvaluationReport Service defines the EvaluationReport related methods, such as CRUD and list.

An `EvaluationReport` stores charts, filters, and data point sources that describe the evaluation results of a trained model. Reports are referenced by `ModelSpec` fields such as `performance_evaluation_report`, `feature_evaluation_report`, `feature_quality_report`, and `explainability_report`. A report can be sealed to prevent further modification.

### CreateEvaluationReport

`CreateEvaluationReport(CreateEvaluationReportRequest) → CreateEvaluationReportResponse`

Create a new EvaluationReport with the given spec.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `evaluation_report` | `EvaluationReport` | Yes | The metadata and spec of the EvaluationReport to be created. |
| `create_options` | `k8s.io.apimachinery.pkg.apis.meta.v1.CreateOptions` | No | — |

**Response fields**

| Field | Type | Description |
|-------|------|-------------|
| `evaluation_report` | `EvaluationReport` | The created EvaluationReport. |

---

### GetEvaluationReport

`GetEvaluationReport(GetEvaluationReportRequest) → GetEvaluationReportResponse`

Get the specified EvaluationReport.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | `string` | Yes | Name of the EvaluationReport. |
| `namespace` | `string` | Yes | Object name and auth scope. |
| `get_options` | `k8s.io.apimachinery.pkg.apis.meta.v1.GetOptions` | No | — |

**Response fields**

| Field | Type | Description |
|-------|------|-------------|
| `evaluation_report` | `EvaluationReport` | The requested EvaluationReport. |

---

### UpdateEvaluationReport

`UpdateEvaluationReport(UpdateEvaluationReportRequest) → UpdateEvaluationReportResponse`

Update an EvaluationReport with the given spec.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `evaluation_report` | `EvaluationReport` | Yes | The metadata and spec of the EvaluationReport to be updated. |
| `update_options` | `k8s.io.apimachinery.pkg.apis.meta.v1.UpdateOptions` | No | — |

**Response fields**

| Field | Type | Description |
|-------|------|-------------|
| `evaluation_report` | `EvaluationReport` | The updated EvaluationReport. |

---

### DeleteEvaluationReport

`DeleteEvaluationReport(DeleteEvaluationReportRequest) → DeleteEvaluationReportResponse`

Delete an EvaluationReport.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | `string` | Yes | Name of the EvaluationReport. |
| `namespace` | `string` | Yes | Object name and auth scope. |
| `delete_options` | `k8s.io.apimachinery.pkg.apis.meta.v1.DeleteOptions` | No | — |

**Response fields**

`DeleteEvaluationReportResponse` is empty.

---

### DeleteEvaluationReportCollection

`DeleteEvaluationReportCollection(DeleteEvaluationReportCollectionRequest) → DeleteEvaluationReportCollectionResponse`

Delete a collection of EvaluationReport objects.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `namespace` | `string` | Yes | Object name and auth scope. |
| `delete_options` | `k8s.io.apimachinery.pkg.apis.meta.v1.DeleteOptions` | No | — |
| `list_options` | `k8s.io.apimachinery.pkg.apis.meta.v1.ListOptions` | No | — |

**Response fields**

`DeleteEvaluationReportCollectionResponse` is empty.

---

### ListEvaluationReport

`ListEvaluationReport(ListEvaluationReportRequest) → ListEvaluationReportResponse`

List objects of type EvaluationReport.

> **Note:** Watch and list across all namespaces are not supported.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `namespace` | `string` | Yes | — |
| `list_options` | `k8s.io.apimachinery.pkg.apis.meta.v1.ListOptions` | No | — |
| `list_options_ext` | `michelangelo.api.ListOptionsExt` | No | — |

**Response fields**

| Field | Type | Description |
|-------|------|-------------|
| `evaluation_report_list` | `EvaluationReportList` | — |

---

## Next Steps

* [Model Registry](../../operator-guides/components/model-registry.md): Operate the built-in model registry — storage, RBAC, and CI/CD integration
* [API Reference Home](../index.md): Browse the Python SDK reference too
