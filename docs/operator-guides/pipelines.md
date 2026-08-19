---
sidebar_position: 5
sidebar_label: Pipelines
---

# Pipelines API Reference

The Michelangelo AI pipelines API provides four gRPC services that cover the full ML pipeline lifecycle. **PipelineService** manages pipeline definitions — the workflow templates that describe how to train, evaluate, and deploy models. **PipelineRunService** manages individual executions of those pipelines. **TriggerRunService** manages scheduled or batch trigger executions that spawn pipeline runs. **ProjectService** manages projects, the logical grouping that organizes related ML resources.

All four services expose the same six operations: Create, Get, Update, Delete, DeleteCollection, and List. Watch operations and cross-namespace listing are not supported on any service.

> **Note on required fields:** Proto3 does not annotate fields as required at the protocol level. The Required column in each table reflects logical necessity inferred from field semantics and proto comments, not a proto-level constraint.

---

## PipelineService

Pipeline Service defines the Pipeline related methods, such as CRUD and list.

A `Pipeline` represents a workflow that can be executed to train, evaluate, and deploy ML models. Manifests can be in YAML or UniFlow (Python SDK) format. A pipeline transitions through build states — CREATED, BUILDING, READY, or ERROR — before it can be run.

### CreatePipeline

`CreatePipeline(CreatePipelineRequest) → CreatePipelineResponse`

Create a new Pipeline with the given spec.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| pipeline | Pipeline | Yes | The metadata and spec of the Pipeline to be created. |
| create_options | CreateOptions | No | No description provided in source. |

**Response fields**

| Field | Type | Description |
|-------|------|-------------|
| pipeline | Pipeline | The created Pipeline. |

---

### GetPipeline

`GetPipeline(GetPipelineRequest) → GetPipelineResponse`

Get the specified Pipeline.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| name | string | Yes | Name of the Pipeline. |
| namespace | string | Yes | Object name and auth scope. |
| get_options | GetOptions | No | No description provided in source. |

**Response fields**

| Field | Type | Description |
|-------|------|-------------|
| pipeline | Pipeline | The requested Pipeline. |

---

### UpdatePipeline

`UpdatePipeline(UpdatePipelineRequest) → UpdatePipelineResponse`

Update a Pipeline with the given spec.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| pipeline | Pipeline | Yes | The metadata and spec of the Pipeline to be updated. |
| update_options | UpdateOptions | No | No description provided in source. |

**Response fields**

| Field | Type | Description |
|-------|------|-------------|
| pipeline | Pipeline | The updated Pipeline. |

---

### DeletePipeline

`DeletePipeline(DeletePipelineRequest) → DeletePipelineResponse`

Delete a Pipeline.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| name | string | Yes | Name of the Pipeline. |
| namespace | string | Yes | Object name and auth scope. |
| delete_options | DeleteOptions | No | No description provided in source. |

**Response fields**

This response has no fields.

---

### DeletePipelineCollection

`DeletePipelineCollection(DeletePipelineCollectionRequest) → DeletePipelineCollectionResponse`

Delete collection of Pipeline.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| namespace | string | Yes | Object name and auth scope. |
| delete_options | DeleteOptions | No | No description provided in source. |
| list_options | ListOptions | No | No description provided in source. |

**Response fields**

This response has no fields.

---

### ListPipeline

`ListPipeline(ListPipelineRequest) → ListPipelineResponse`

List objects of type Pipeline.

> **Note:** Watch and list across all namespaces are not supported.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| namespace | string | Yes | No description provided in source. |
| list_options | ListOptions | No | No description provided in source. |
| list_options_ext | ListOptionsExt | No | No description provided in source. |

**Response fields**

| Field | Type | Description |
|-------|------|-------------|
| pipeline_list | PipelineList | No description provided in source. |

---

## PipelineRunService

PipelineRun Service defines the PipelineRun related methods, such as CRUD and list.

A `PipelineRun` represents a single execution instance of a pipeline workflow. It tracks which pipeline version was used, who triggered the run, the input parameters, and execution progress through states — PENDING, RUNNING, SUCCEEDED, FAILED, KILLED, or SKIPPED.

### CreatePipelineRun

`CreatePipelineRun(CreatePipelineRunRequest) → CreatePipelineRunResponse`

Create a new PipelineRun with the given spec.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| pipeline_run | PipelineRun | Yes | The metadata and spec of the PipelineRun to be created. |
| create_options | CreateOptions | No | No description provided in source. |

**Response fields**

| Field | Type | Description |
|-------|------|-------------|
| pipeline_run | PipelineRun | The created PipelineRun. |

---

### GetPipelineRun

`GetPipelineRun(GetPipelineRunRequest) → GetPipelineRunResponse`

Get the specified PipelineRun.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| name | string | Yes | Name of the PipelineRun. |
| namespace | string | Yes | Object name and auth scope. |
| get_options | GetOptions | No | No description provided in source. |

**Response fields**

| Field | Type | Description |
|-------|------|-------------|
| pipeline_run | PipelineRun | The requested PipelineRun. |

---

### UpdatePipelineRun

`UpdatePipelineRun(UpdatePipelineRunRequest) → UpdatePipelineRunResponse`

Update a PipelineRun with the given spec.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| pipeline_run | PipelineRun | Yes | The metadata and spec of the PipelineRun to be updated. |
| update_options | UpdateOptions | No | No description provided in source. |

**Response fields**

| Field | Type | Description |
|-------|------|-------------|
| pipeline_run | PipelineRun | The updated PipelineRun. |

---

### DeletePipelineRun

`DeletePipelineRun(DeletePipelineRunRequest) → DeletePipelineRunResponse`

Delete a PipelineRun.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| name | string | Yes | Name of the PipelineRun. |
| namespace | string | Yes | Object name and auth scope. |
| delete_options | DeleteOptions | No | No description provided in source. |

**Response fields**

This response has no fields.

---

### DeletePipelineRunCollection

`DeletePipelineRunCollection(DeletePipelineRunCollectionRequest) → DeletePipelineRunCollectionResponse`

Delete collection of PipelineRun.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| namespace | string | Yes | Object name and auth scope. |
| delete_options | DeleteOptions | No | No description provided in source. |
| list_options | ListOptions | No | No description provided in source. |

**Response fields**

This response has no fields.

---

### ListPipelineRun

`ListPipelineRun(ListPipelineRunRequest) → ListPipelineRunResponse`

List objects of type PipelineRun.

> **Note:** Watch and list across all namespaces are not supported.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| namespace | string | Yes | No description provided in source. |
| list_options | ListOptions | No | No description provided in source. |
| list_options_ext | ListOptionsExt | No | No description provided in source. |

**Response fields**

| Field | Type | Description |
|-------|------|-------------|
| pipeline_run_list | PipelineRunList | No description provided in source. |

---

## TriggerRunService

TriggerRun Service defines the TriggerRun related methods, such as CRUD and list.

A `TriggerRun` represents a single execution of a pipeline trigger. A trigger can be a cron schedule, an interval schedule, or a batch rerun of existing pipeline runs. One trigger run may spawn multiple pipeline runs. Trigger runs can be paused and resumed (for cron and interval types) or killed.

### CreateTriggerRun

`CreateTriggerRun(CreateTriggerRunRequest) → CreateTriggerRunResponse`

Create a new TriggerRun with the given spec.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| trigger_run | TriggerRun | Yes | The metadata and spec of the TriggerRun to be created. |
| create_options | CreateOptions | No | No description provided in source. |

**Response fields**

| Field | Type | Description |
|-------|------|-------------|
| trigger_run | TriggerRun | The created TriggerRun. |

---

### GetTriggerRun

`GetTriggerRun(GetTriggerRunRequest) → GetTriggerRunResponse`

Get the specified TriggerRun.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| name | string | Yes | Name of the TriggerRun. |
| namespace | string | Yes | Object name and auth scope. |
| get_options | GetOptions | No | No description provided in source. |

**Response fields**

| Field | Type | Description |
|-------|------|-------------|
| trigger_run | TriggerRun | The requested TriggerRun. |

---

### UpdateTriggerRun

`UpdateTriggerRun(UpdateTriggerRunRequest) → UpdateTriggerRunResponse`

Update a TriggerRun with the given spec.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| trigger_run | TriggerRun | Yes | The metadata and spec of the TriggerRun to be updated. |
| update_options | UpdateOptions | No | No description provided in source. |

**Response fields**

| Field | Type | Description |
|-------|------|-------------|
| trigger_run | TriggerRun | The updated TriggerRun. |

---

### DeleteTriggerRun

`DeleteTriggerRun(DeleteTriggerRunRequest) → DeleteTriggerRunResponse`

Delete a TriggerRun.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| name | string | Yes | Name of the TriggerRun. |
| namespace | string | Yes | Object name and auth scope. |
| delete_options | DeleteOptions | No | No description provided in source. |

**Response fields**

This response has no fields.

---

### DeleteTriggerRunCollection

`DeleteTriggerRunCollection(DeleteTriggerRunCollectionRequest) → DeleteTriggerRunCollectionResponse`

Delete collection of TriggerRun.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| namespace | string | Yes | Object name and auth scope. |
| delete_options | DeleteOptions | No | No description provided in source. |
| list_options | ListOptions | No | No description provided in source. |

**Response fields**

This response has no fields.

---

### ListTriggerRun

`ListTriggerRun(ListTriggerRunRequest) → ListTriggerRunResponse`

List objects of type TriggerRun.

> **Note:** Watch and list across all namespaces are not supported.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| namespace | string | Yes | No description provided in source. |
| list_options | ListOptions | No | No description provided in source. |
| list_options_ext | ListOptionsExt | No | No description provided in source. |

**Response fields**

| Field | Type | Description |
|-------|------|-------------|
| trigger_run_list | TriggerRunList | No description provided in source. |

---

## ProjectService

Project Service defines the Project related methods, such as CRUD and list.

A `Project` is a logical grouping of ML resources for a specific ML use case. It carries ownership information, resource retention policies, a Git repository reference, and lifecycle phase tracking (DEVELOPMENT, STAGING, PRODUCTION, DECOMMISSION). New pipeline and pipeline run creation is blocked on decommissioned projects.

### CreateProject

`CreateProject(CreateProjectRequest) → CreateProjectResponse`

Create a new Project with the given spec.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| project | Project | Yes | The metadata and spec of the Project to be created. |
| create_options | CreateOptions | No | No description provided in source. |

**Response fields**

| Field | Type | Description |
|-------|------|-------------|
| project | Project | The created Project. |

---

### GetProject

`GetProject(GetProjectRequest) → GetProjectResponse`

Get the specified Project.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| name | string | Yes | Name of the Project. |
| namespace | string | Yes | Object name and auth scope. |
| get_options | GetOptions | No | No description provided in source. |

**Response fields**

| Field | Type | Description |
|-------|------|-------------|
| project | Project | The requested Project. |

---

### UpdateProject

`UpdateProject(UpdateProjectRequest) → UpdateProjectResponse`

Update a Project with the given spec.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| project | Project | Yes | The metadata and spec of the Project to be updated. |
| update_options | UpdateOptions | No | No description provided in source. |

**Response fields**

| Field | Type | Description |
|-------|------|-------------|
| project | Project | The updated Project. |

---

### DeleteProject

`DeleteProject(DeleteProjectRequest) → DeleteProjectResponse`

Delete a Project.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| name | string | Yes | Name of the Project. |
| namespace | string | Yes | Object name and auth scope. |
| delete_options | DeleteOptions | No | No description provided in source. |

**Response fields**

This response has no fields.

---

### DeleteProjectCollection

`DeleteProjectCollection(DeleteProjectCollectionRequest) → DeleteProjectCollectionResponse`

Delete collection of Project.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| namespace | string | Yes | Object name and auth scope. |
| delete_options | DeleteOptions | No | No description provided in source. |
| list_options | ListOptions | No | No description provided in source. |

**Response fields**

This response has no fields.

---

### ListProject

`ListProject(ListProjectRequest) → ListProjectResponse`

List objects of type Project.

> **Note:** Watch and list across all namespaces are not supported.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| namespace | string | Yes | No description provided in source. |
| list_options | ListOptions | No | No description provided in source. |
| list_options_ext | ListOptionsExt | No | No description provided in source. |

**Response fields**

| Field | Type | Description |
|-------|------|-------------|
| project_list | ProjectList | No description provided in source. |

---
