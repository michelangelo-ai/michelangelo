---
sidebar_position: 3
sidebar_label: Serving
---

# Serving gRPC API Reference

The Michelangelo AI serving control plane exposes five gRPC services that manage the full lifecycle of inference infrastructure and model deployments. All services are defined in `proto/api/v2/`.

> New to this reference? See [How to Read the gRPC API Reference](./conventions.md) for the shared CRUD-plus-list pattern, the Required column, and how undocumented fields are shown. For the architecture these services implement, see [Serving Overview](../../operator-guides/serving/index.md).

---

## InferenceServerService

InferenceServer Service defines the InferenceServer related methods, such as CRUD and list.

### CreateInferenceServer

`CreateInferenceServer(CreateInferenceServerRequest) → CreateInferenceServerResponse`

Create a new InferenceServer with the given spec.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| inference_server | InferenceServer | Yes | The metadata and spec of the InferenceServer to be created. |
| create_options | k8s.io.apimachinery.pkg.apis.meta.v1.CreateOptions | No | — |

**Response fields**

| Field | Type | Description |
|-------|------|-------------|
| inference_server | InferenceServer | The created InferenceServer. |

---

### GetInferenceServer

`GetInferenceServer(GetInferenceServerRequest) → GetInferenceServerResponse`

Get the specified InferenceServer.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| name | string | Yes | Name of the InferenceServer. |
| namespace | string | Yes | Object name and auth scope. |
| get_options | k8s.io.apimachinery.pkg.apis.meta.v1.GetOptions | No | — |

**Response fields**

| Field | Type | Description |
|-------|------|-------------|
| inference_server | InferenceServer | The requested InferenceServer. |

---

### UpdateInferenceServer

`UpdateInferenceServer(UpdateInferenceServerRequest) → UpdateInferenceServerResponse`

Update a InferenceServer with the given spec.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| inference_server | InferenceServer | Yes | The metadata and spec of the InferenceServer to be updated. |
| update_options | k8s.io.apimachinery.pkg.apis.meta.v1.UpdateOptions | No | — |

**Response fields**

| Field | Type | Description |
|-------|------|-------------|
| inference_server | InferenceServer | The updated InferenceServer. |

---

### DeleteInferenceServer

`DeleteInferenceServer(DeleteInferenceServerRequest) → DeleteInferenceServerResponse`

Delete a InferenceServer.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| name | string | Yes | Name of the InferenceServer. |
| namespace | string | Yes | Object name and auth scope. |
| delete_options | k8s.io.apimachinery.pkg.apis.meta.v1.DeleteOptions | No | — |

**Response fields**

The response message has no fields.

---

### DeleteInferenceServerCollection

`DeleteInferenceServerCollection(DeleteInferenceServerCollectionRequest) → DeleteInferenceServerCollectionResponse`

Delete collection of InferenceServer.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| namespace | string | Yes | Object name and auth scope. |
| delete_options | k8s.io.apimachinery.pkg.apis.meta.v1.DeleteOptions | No | — |
| list_options | k8s.io.apimachinery.pkg.apis.meta.v1.ListOptions | No | — |

**Response fields**

The response message has no fields.

---

### ListInferenceServer

`ListInferenceServer(ListInferenceServerRequest) → ListInferenceServerResponse`

List objects of type InferenceServer.

> **Note:** Watch and list across all namespaces are not supported.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| namespace | string | Yes | — |
| list_options | k8s.io.apimachinery.pkg.apis.meta.v1.ListOptions | No | — |
| list_options_ext | michelangelo.api.ListOptionsExt | No | — |

**Response fields**

| Field | Type | Description |
|-------|------|-------------|
| inference_server_list | InferenceServerList | — |

---

## DeploymentService

Deployment Service defines the Deployment related methods, such as CRUD and list.

### CreateDeployment

`CreateDeployment(CreateDeploymentRequest) → CreateDeploymentResponse`

Create a new Deployment with the given spec.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| deployment | Deployment | Yes | The metadata and spec of the Deployment to be created. |
| create_options | k8s.io.apimachinery.pkg.apis.meta.v1.CreateOptions | No | — |

**Response fields**

| Field | Type | Description |
|-------|------|-------------|
| deployment | Deployment | The created Deployment. |

---

### GetDeployment

`GetDeployment(GetDeploymentRequest) → GetDeploymentResponse`

Get the specified Deployment.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| name | string | Yes | Name of the Deployment. |
| namespace | string | Yes | Object name and auth scope. |
| get_options | k8s.io.apimachinery.pkg.apis.meta.v1.GetOptions | No | — |

**Response fields**

| Field | Type | Description |
|-------|------|-------------|
| deployment | Deployment | The requested Deployment. |

---

### UpdateDeployment

`UpdateDeployment(UpdateDeploymentRequest) → UpdateDeploymentResponse`

Update a Deployment with the given spec.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| deployment | Deployment | Yes | The metadata and spec of the Deployment to be updated. |
| update_options | k8s.io.apimachinery.pkg.apis.meta.v1.UpdateOptions | No | — |

**Response fields**

| Field | Type | Description |
|-------|------|-------------|
| deployment | Deployment | The updated Deployment. |

---

### DeleteDeployment

`DeleteDeployment(DeleteDeploymentRequest) → DeleteDeploymentResponse`

Delete a Deployment.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| name | string | Yes | Name of the Deployment. |
| namespace | string | Yes | Object name and auth scope. |
| delete_options | k8s.io.apimachinery.pkg.apis.meta.v1.DeleteOptions | No | — |

**Response fields**

The response message has no fields.

---

### DeleteDeploymentCollection

`DeleteDeploymentCollection(DeleteDeploymentCollectionRequest) → DeleteDeploymentCollectionResponse`

Delete collection of Deployment.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| namespace | string | Yes | Object name and auth scope. |
| delete_options | k8s.io.apimachinery.pkg.apis.meta.v1.DeleteOptions | No | — |
| list_options | k8s.io.apimachinery.pkg.apis.meta.v1.ListOptions | No | — |

**Response fields**

The response message has no fields.

---

### ListDeployment

`ListDeployment(ListDeploymentRequest) → ListDeploymentResponse`

List objects of type Deployment.

> **Note:** Watch and list across all namespaces are not supported.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| namespace | string | Yes | — |
| list_options | k8s.io.apimachinery.pkg.apis.meta.v1.ListOptions | No | — |
| list_options_ext | michelangelo.api.ListOptionsExt | No | — |

**Response fields**

| Field | Type | Description |
|-------|------|-------------|
| deployment_list | DeploymentList | — |

---

## RevisionService

Revision Service defines the Revision related methods, such as CRUD and list.

### CreateRevision

`CreateRevision(CreateRevisionRequest) → CreateRevisionResponse`

Create a new Revision with the given spec.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| revision | Revision | Yes | The metadata and spec of the Revision to be created. |
| create_options | k8s.io.apimachinery.pkg.apis.meta.v1.CreateOptions | No | — |

**Response fields**

| Field | Type | Description |
|-------|------|-------------|
| revision | Revision | The created Revision. |

---

### GetRevision

`GetRevision(GetRevisionRequest) → GetRevisionResponse`

Get the specified Revision.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| name | string | Yes | Name of the Revision. |
| namespace | string | Yes | Object name and auth scope. |
| get_options | k8s.io.apimachinery.pkg.apis.meta.v1.GetOptions | No | — |

**Response fields**

| Field | Type | Description |
|-------|------|-------------|
| revision | Revision | The requested Revision. |

---

### UpdateRevision

`UpdateRevision(UpdateRevisionRequest) → UpdateRevisionResponse`

Update a Revision with the given spec.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| revision | Revision | Yes | The metadata and spec of the Revision to be updated. |
| update_options | k8s.io.apimachinery.pkg.apis.meta.v1.UpdateOptions | No | — |

**Response fields**

| Field | Type | Description |
|-------|------|-------------|
| revision | Revision | The updated Revision. |

---

### DeleteRevision

`DeleteRevision(DeleteRevisionRequest) → DeleteRevisionResponse`

Delete a Revision.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| name | string | Yes | Name of the Revision. |
| namespace | string | Yes | Object name and auth scope. |
| delete_options | k8s.io.apimachinery.pkg.apis.meta.v1.DeleteOptions | No | — |

**Response fields**

The response message has no fields.

---

### DeleteRevisionCollection

`DeleteRevisionCollection(DeleteRevisionCollectionRequest) → DeleteRevisionCollectionResponse`

Delete collection of Revision.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| namespace | string | Yes | Object name and auth scope. |
| delete_options | k8s.io.apimachinery.pkg.apis.meta.v1.DeleteOptions | No | — |
| list_options | k8s.io.apimachinery.pkg.apis.meta.v1.ListOptions | No | — |

**Response fields**

The response message has no fields.

---

### ListRevision

`ListRevision(ListRevisionRequest) → ListRevisionResponse`

List objects of type Revision.

> **Note:** Watch and list across all namespaces are not supported.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| namespace | string | Yes | — |
| list_options | k8s.io.apimachinery.pkg.apis.meta.v1.ListOptions | No | — |
| list_options_ext | michelangelo.api.ListOptionsExt | No | — |

**Response fields**

| Field | Type | Description |
|-------|------|-------------|
| revision_list | RevisionList | — |

---

## ClusterService

Cluster Service defines the Cluster related methods, such as CRUD and list.

### CreateCluster

`CreateCluster(CreateClusterRequest) → CreateClusterResponse`

Create a new Cluster with the given spec.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| cluster | Cluster | Yes | The metadata and spec of the Cluster to be created. |
| create_options | k8s.io.apimachinery.pkg.apis.meta.v1.CreateOptions | No | — |

**Response fields**

| Field | Type | Description |
|-------|------|-------------|
| cluster | Cluster | The created Cluster. |

---

### GetCluster

`GetCluster(GetClusterRequest) → GetClusterResponse`

Get the specified Cluster.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| name | string | Yes | Name of the Cluster. |
| namespace | string | Yes | Object name and auth scope. |
| get_options | k8s.io.apimachinery.pkg.apis.meta.v1.GetOptions | No | — |

**Response fields**

| Field | Type | Description |
|-------|------|-------------|
| cluster | Cluster | The requested Cluster. |

---

### UpdateCluster

`UpdateCluster(UpdateClusterRequest) → UpdateClusterResponse`

Update a Cluster with the given spec.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| cluster | Cluster | Yes | The metadata and spec of the Cluster to be updated. |
| update_options | k8s.io.apimachinery.pkg.apis.meta.v1.UpdateOptions | No | — |

**Response fields**

| Field | Type | Description |
|-------|------|-------------|
| cluster | Cluster | The updated Cluster. |

---

### DeleteCluster

`DeleteCluster(DeleteClusterRequest) → DeleteClusterResponse`

Delete a Cluster.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| name | string | Yes | Name of the Cluster. |
| namespace | string | Yes | Object name and auth scope. |
| delete_options | k8s.io.apimachinery.pkg.apis.meta.v1.DeleteOptions | No | — |

**Response fields**

The response message has no fields.

---

### DeleteClusterCollection

`DeleteClusterCollection(DeleteClusterCollectionRequest) → DeleteClusterCollectionResponse`

Delete collection of Cluster.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| namespace | string | Yes | Object name and auth scope. |
| delete_options | k8s.io.apimachinery.pkg.apis.meta.v1.DeleteOptions | No | — |
| list_options | k8s.io.apimachinery.pkg.apis.meta.v1.ListOptions | No | — |

**Response fields**

The response message has no fields.

---

### ListCluster

`ListCluster(ListClusterRequest) → ListClusterResponse`

List objects of type Cluster.

> **Note:** Watch and list across all namespaces are not supported.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| namespace | string | Yes | — |
| list_options | k8s.io.apimachinery.pkg.apis.meta.v1.ListOptions | No | — |
| list_options_ext | michelangelo.api.ListOptionsExt | No | — |

**Response fields**

| Field | Type | Description |
|-------|------|-------------|
| cluster_list | ClusterList | — |

---

## RayClusterService

RayCluster Service defines the RayCluster related methods, such as CRUD and list.

### CreateRayCluster

`CreateRayCluster(CreateRayClusterRequest) → CreateRayClusterResponse`

Create a new RayCluster with the given spec.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| ray_cluster | RayCluster | Yes | The metadata and spec of the RayCluster to be created. |
| create_options | k8s.io.apimachinery.pkg.apis.meta.v1.CreateOptions | No | — |

**Response fields**

| Field | Type | Description |
|-------|------|-------------|
| ray_cluster | RayCluster | The created RayCluster. |

---

### GetRayCluster

`GetRayCluster(GetRayClusterRequest) → GetRayClusterResponse`

Get the specified RayCluster.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| name | string | Yes | Name of the RayCluster. |
| namespace | string | Yes | Object name and auth scope. |
| get_options | k8s.io.apimachinery.pkg.apis.meta.v1.GetOptions | No | — |

**Response fields**

| Field | Type | Description |
|-------|------|-------------|
| ray_cluster | RayCluster | The requested RayCluster. |

---

### UpdateRayCluster

`UpdateRayCluster(UpdateRayClusterRequest) → UpdateRayClusterResponse`

Update a RayCluster with the given spec.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| ray_cluster | RayCluster | Yes | The metadata and spec of the RayCluster to be updated. |
| update_options | k8s.io.apimachinery.pkg.apis.meta.v1.UpdateOptions | No | — |

**Response fields**

| Field | Type | Description |
|-------|------|-------------|
| ray_cluster | RayCluster | The updated RayCluster. |

---

### DeleteRayCluster

`DeleteRayCluster(DeleteRayClusterRequest) → DeleteRayClusterResponse`

Delete a RayCluster.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| name | string | Yes | Name of the RayCluster. |
| namespace | string | Yes | Object name and auth scope. |
| delete_options | k8s.io.apimachinery.pkg.apis.meta.v1.DeleteOptions | No | — |

**Response fields**

The response message has no fields.

---

### DeleteRayClusterCollection

`DeleteRayClusterCollection(DeleteRayClusterCollectionRequest) → DeleteRayClusterCollectionResponse`

Delete collection of RayCluster.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| namespace | string | Yes | Object name and auth scope. |
| delete_options | k8s.io.apimachinery.pkg.apis.meta.v1.DeleteOptions | No | — |
| list_options | k8s.io.apimachinery.pkg.apis.meta.v1.ListOptions | No | — |

**Response fields**

The response message has no fields.

---

### ListRayCluster

`ListRayCluster(ListRayClusterRequest) → ListRayClusterResponse`

List objects of type RayCluster.

> **Note:** Watch and list across all namespaces are not supported.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| namespace | string | Yes | — |
| list_options | k8s.io.apimachinery.pkg.apis.meta.v1.ListOptions | No | — |
| list_options_ext | michelangelo.api.ListOptionsExt | No | — |

**Response fields**

| Field | Type | Description |
|-------|------|-------------|
| ray_cluster_list | RayClusterList | — |

---

## Next Steps

* [Serving Overview](../../operator-guides/serving/index.md): Architecture, controller lifecycles, and core concepts for InferenceServer and Deployment
* [Cluster Setup for Serving](../../operator-guides/serving/cluster-setup.md): Configure a cluster for inference
